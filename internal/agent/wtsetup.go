package agent

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// StartWorktree creates or reuses a worktree, sets up its resources and starts the agent in it.
func StartWorktree(c *agentapi.Config, name, base string, args []string) error {
	w, err := PrepareWorktree(c, name, base)
	if err != nil {
		return err
	}
	port := Port(w.Slot)
	fmt.Printf("worktree %s  ->  %s\n", w.Branch, w.Dir)
	fmt.Printf("  dev URL  http://%s:%d   (ports %d-%d are this worktree's)\n", c.HostName, port, port, port+9)
	if len(w.DBs) > 0 {
		fmt.Printf("  db       %s\n", strings.Join(w.DBs, ", "))
	}
	if c.Redis != nil {
		fmt.Printf("  redis    db index %d (REDIS_URL)\n", w.Slot)
	}
	prompt := systemPrompt(c, w.Branch, w.Dir, port, w.DBs)
	return Exec(c, w.Dir, mergeEnv(os.Environ(), w.Env), append([]string{"--append-system-prompt", prompt}, args...))
}

// Prepared is a worktree ready to work in: its folder, slot, database copies and environment.
type Prepared struct {
	Branch string
	Dir    string
	Slot   int
	DBs    []string
	Env    map[string]string
	// SetupFailed: a setup command of the project failed (the worktree is there anyway).
	SetupFailed bool
}

// PrepareWorktree creates or reuses a worktree and sets up its resources: a slot, copies of the
// databases, the copied files, and (when new) the setup commands. Progress goes to stderr.
func PrepareWorktree(c *agentapi.Config, name, base string) (Prepared, error) {
	if err := CheckWorktreeName(name); err != nil {
		return Prepared{}, err
	}
	branch, slot, created, err := ensureWorktree(c, name, base)
	if err != nil {
		return Prepared{}, err
	}
	w := Prepared{Branch: branch, Dir: worktreeDir(c, branch), Slot: slot, Env: WorktreeEnv(c, branch, slot)}
	pairs := DBPairs(c, branch)
	if err := ensureDBs(c, pairs); err != nil {
		return Prepared{}, err
	}
	if err := copyFiles(c, w.Dir, pairs); err != nil {
		return Prepared{}, err
	}
	if created {
		w.SetupFailed = !runSetup(c, w.Dir, w.Env)
	}
	for _, p := range pairs {
		w.DBs = append(w.DBs, p[1])
	}
	return w, nil
}

// ExecIn runs a command in the main checkout or a worktree, with that worktree's environment.
func ExecIn(c *agentapi.Config, worktree string, cmd []string) error {
	dir, env := c.Dir, map[string]string{}
	if worktree != "" {
		branch, slot, err := Resolve(c, worktree)
		if err != nil {
			return err
		}
		dir, env = worktreeDir(c, branch), WorktreeEnv(c, branch, slot)
	}
	if len(cmd) == 0 {
		cmd = []string{"bash"}
	}
	bin, err := exec.LookPath(cmd[0])
	if err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	return execve(bin, cmd, mergeEnv(os.Environ(), env))
}

func mergeEnv(base []string, add map[string]string) []string {
	out := slices.DeleteFunc(slices.Clone(base), func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		_, replaced := add[k]
		return replaced
	})
	for _, k := range slices.Sorted(maps.Keys(add)) {
		out = append(out, k+"="+add[k])
	}
	return out
}

func systemPrompt(c *agentapi.Config, name, dir string, port int, dbs []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are in git worktree '%s' at %s, one of several parallel sessions on this repo. Stay inside this directory. ", name, dir)
	fmt.Fprintf(&b, "Isolated resources for this worktree: ports %d-%d ($PORT=%d) are reserved for this worktree; other sessions use the repo's default ports, so never start servers on defaults like 3000/5173 here. ", port, port+9, port)
	fmt.Fprintf(&b, "Bind to 0.0.0.0; the user opens http://%s:<port>. Give each server its own port from the range explicitly (env PORT=..., vite --port ..., astro dev --port ...) and point clients at the matching port.", c.HostName)
	if len(dbs) > 0 {
		fmt.Fprintf(&b, " Databases: this worktree has its own copies (%s); DATABASE_URL and any env files copied into this worktree already point at them.", strings.Join(dbs, ", "))
	}
	if c.Redis != nil {
		b.WriteString(" REDIS_URL is this worktree's Redis database; if the app reads Redis host/port instead of REDIS_URL, Redis is shared with the other sessions, so use distinct key or queue names where it matters.")
	}
	if slices.Contains(c.Services, "mongo") {
		b.WriteString(" MongoDB is shared with the other sessions of this project.")
	}
	b.WriteString(" Install dependencies in this worktree before running it.")
	return b.String()
}

// ensureWorktree creates the git worktree if needed and claims its slot.
func ensureWorktree(c *agentapi.Config, name, base string) (branch string, slot int, created bool, err error) {
	err = withRegistry(c, true, func(r *registry) error {
		if s, b := r.find(name); s > 0 {
			name = b // given by folder name: use the branch name
		}
		dir := worktreeDir(c, name)
		if err := excludeWorktrees(c.Dir); err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			if err := os.MkdirAll(worktreeRoot(c), 0o755); err != nil {
				return err
			}
			args := []string{"-C", c.Dir, "worktree", "add", "-b", name, dir, base}
			if git(c.Dir, "show-ref", "--verify", "--quiet", "refs/heads/"+name) == nil {
				args = []string{"-C", c.Dir, "worktree", "add", dir, name}
			}
			cmd := exec.Command("git", args...)
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("git worktree add: %w", err)
			}
			created = true
		}
		var err error
		slot, err = r.claim(name)
		branch = name
		return err
	})
	return branch, slot, created, err
}

func git(dir string, args ...string) error {
	return exec.Command("git", append([]string{"-C", dir}, args...)...).Run()
}

// excludeWorktrees keeps jailwick's folder (.jailwick/, the worktrees in it) out of git status;
// info/exclude is local only, never committed.
func excludeWorktrees(repo string) error {
	common, err := gitCommonDir(repo)
	if err != nil {
		return err
	}
	file := filepath.Join(common, "info", "exclude")
	data, _ := os.ReadFile(file)
	if slices.Contains(strings.Split(string(data), "\n"), excludeLine) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		f.WriteString("\n")
	}
	_, err = f.WriteString(excludeLine + "\n")
	return err
}

// excludeLine is the line excludeWorktrees adds: jailwick's whole folder.
const excludeLine = "/.jailwick/"

func pgConnect(ctx context.Context, c *agentapi.Config) (*pgx.Conn, error) {
	pg := c.Postgres
	cfg, err := pgx.ParseConfig("")
	if err != nil {
		return nil, err
	}
	cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database = "postgres", 5432, pg.User, pg.Password, pg.DB
	return pgx.ConnectConfig(ctx, cfg)
}

// ensureDBs creates each worktree database as a copy of its source (same owner, so per-database
// roles keep working). A source in use can't be copied: the copy starts empty.
func ensureDBs(c *agentapi.Config, pairs [][2]string) error {
	if len(pairs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, err := pgConnect(ctx, c)
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer conn.Close(ctx)
	for _, p := range pairs {
		from, to := p[0], p[1]
		var exists bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", to).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		owner := c.Postgres.User
		_ = conn.QueryRow(ctx, "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = $1", from).Scan(&owner)
		q := "CREATE DATABASE " + ident(to) + " OWNER " + ident(owner)
		if _, err := conn.Exec(ctx, q+" TEMPLATE "+ident(from)); err == nil {
			fmt.Fprintf(os.Stderr, "created database %s (copy of %s)\n", to, from)
			continue
		}
		if _, err := conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("creating database %s: %w", to, err)
		}
		fmt.Fprintf(os.Stderr, "created empty database %s (%s is in use, couldn't copy; run migrations)\n", to, from)
	}
	return nil
}

// DropDBs removes a worktree's databases.
func DropDBs(c *agentapi.Config, pairs [][2]string) error {
	if len(pairs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := pgConnect(ctx, c)
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer conn.Close(ctx)
	for _, p := range pairs {
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+ident(p[1])+" WITH (FORCE)"); err != nil {
			return fmt.Errorf("dropping %s: %w", p[1], err)
		}
	}
	return nil
}

func ident(s string) string { return pgx.Identifier{s}.Sanitize() }

// copyFiles brings configured files into a worktree with database names rewritten. Untracked
// files are copied from the main checkout once; tracked ones are rewritten in place and hidden
// from git status in that worktree (skip-worktree).
func copyFiles(c *agentapi.Config, dir string, pairs [][2]string) error {
	for _, f := range c.CopyFiles {
		src, dst := filepath.Join(c.Dir, f), filepath.Join(dir, f)
		if git(dir, "ls-files", "--error-unmatch", "--", f) == nil {
			out, _ := exec.Command("git", "-C", dir, "ls-files", "-v", "--", f).Output()
			if bytes.HasPrefix(out, []byte("S")) {
				continue // already rewritten
			}
			data, err := os.ReadFile(dst)
			if err != nil {
				continue
			}
			if rewritten := RewriteDBNames(string(data), pairs); rewritten != string(data) {
				if err := os.WriteFile(dst, []byte(rewritten), 0o644); err != nil {
					return err
				}
				_ = git(dir, "update-index", "--skip-worktree", "--", f)
				fmt.Fprintf(os.Stderr, "pointed tracked %s at this worktree's databases (hidden from git status here)\n", f)
			}
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, []byte(RewriteDBNames(string(data), pairs)), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "copied %s (database names -> this worktree's)\n", f)
	}
	return nil
}

func runSetup(c *agentapi.Config, dir string, env map[string]string) (ok bool) {
	ok = true
	for _, s := range c.Setup {
		fmt.Fprintf(os.Stderr, "setup: %s\n", s)
		cmd := exec.Command("sh", "-c", s)
		cmd.Dir, cmd.Env = dir, mergeEnv(os.Environ(), env)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "setup failed (%v); continuing\n", err)
			ok = false
		}
	}
	return ok
}

// RemoveOptions say how far removing a worktree goes.
type RemoveOptions struct {
	Force bool // even with uncommitted changes (they are lost)
	// Branch: "" keeps the branch; "merged" deletes it when it is merged (git branch -d, which
	// refuses otherwise); "any" deletes it whatever (git branch -D: unmerged commits are lost).
	Branch string
}

// RemoveWorktree stops the worktree's app run, deletes its folder, drops its databases and frees
// the slot; the branch is kept unless o.Branch says otherwise.
func RemoveWorktree(c *agentapi.Config, name string, o RemoveOptions) error {
	return withRegistry(c, true, func(r *registry) error {
		slot, branch := r.find(name)
		if slot == 0 {
			return fmt.Errorf("unknown worktree %q", name)
		}
		_ = StopRun(branch) // its dev server runs in the folder
		dir := worktreeDir(c, branch)
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			args := []string{"-C", c.Dir, "worktree", "remove", dir}
			if o.Force {
				args = append(args, "--force")
			}
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				return fmt.Errorf("worktree has uncommitted changes; commit them or use --force (%s)", strings.TrimSpace(string(out)))
			}
		}
		if err := DropDBs(c, DBPairs(c, branch)); err != nil {
			return err
		}
		_ = git(c.Dir, "worktree", "prune") // a folder already gone leaves git's record of it
		r.Slots[slot-1] = ""
		if o.Branch == "" || !branchExists(c.Dir, branch) {
			fmt.Fprintf(os.Stderr, "removed worktree %s (branch %q kept; delete it with: git branch -D %s)\n", branch, branch, branch)
			return nil
		}
		flag := "-d"
		if o.Branch == "any" {
			flag = "-D"
		}
		if out, err := exec.Command("git", "-C", c.Dir, "branch", flag, branch).CombinedOutput(); err != nil {
			return fmt.Errorf("removed worktree %s, but not its branch: %s", branch, strings.TrimSpace(string(out)))
		}
		fmt.Fprintf(os.Stderr, "removed worktree %s and its branch\n", branch)
		return nil
	})
}

func execve(bin string, argv, env []string) error { return syscall.Exec(bin, argv, env) }

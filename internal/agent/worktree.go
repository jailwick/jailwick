package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Each worktree gets a slot n (1, 2, …): ports 3000+10n … +9, Redis database n, and its own
// copies of the project's databases. The main checkout keeps the repo's defaults.
const (
	basePort = 3000
	portStep = 10
	MaxSlots = 99 // ports up to 3999; Redis runs with enough databases
)

var wtNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// CheckWorktreeName validates a worktree (branch) name.
func CheckWorktreeName(n string) error {
	if !wtNameRE.MatchString(n) || strings.Contains(n, "..") || len(n) > 100 {
		return fmt.Errorf("invalid worktree name %q", n)
	}
	return nil
}

// Folder is the worktree's folder name: the branch name with / replaced by -.
func Folder(name string) string { return strings.ReplaceAll(name, "/", "-") }

func worktreeRoot(c *agentapi.Config) string { return filepath.Join(c.Dir, agentapi.WorktreeFolder) }

// worktreeDir is a worktree's folder: where it already is (an older jailwick made it in
// .claude/worktrees), else in .jailwick/worktrees.
func worktreeDir(c *agentapi.Config, name string) string {
	for _, f := range agentapi.WorktreeFolders {
		if dir := filepath.Join(c.Dir, f, Folder(name)); exists(filepath.Join(dir, ".git")) {
			return dir
		}
	}
	return filepath.Join(worktreeRoot(c), Folder(name))
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Port is the first port of a slot.
func Port(slot int) int { return basePort + portStep*slot }

// registry stores slot assignments in the repo's git folder, so they survive sandbox resets and
// match the worktrees that exist.
type registry struct {
	file  string
	Slots []string `json:"slots"` // index i = slot i+1; "" = free
}

func gitCommonDir(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("%s is not a git repo", dir)
	}
	d := strings.TrimSpace(string(out))
	if !filepath.IsAbs(d) {
		d = filepath.Join(dir, d)
	}
	return d, nil
}

// withRegistry runs fn with the registry loaded and locked; changes are saved when fn succeeds.
func withRegistry(c *agentapi.Config, write bool, fn func(r *registry) error) error {
	common, err := gitCommonDir(c.Dir)
	if err != nil {
		return err
	}
	dir := filepath.Join(common, "jailwick")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "worktrees.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	mode := syscall.LOCK_SH
	if write {
		mode = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(lock.Fd()), mode); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	r := &registry{file: filepath.Join(dir, "worktrees.json")}
	if data, err := os.ReadFile(r.file); err == nil {
		if err := json.Unmarshal(data, r); err != nil {
			return fmt.Errorf("%s: %w", r.file, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := fn(r); err != nil {
		return err
	}
	if !write {
		return nil
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	tmp := r.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.file)
}

// find returns the slot of a worktree given its branch name or folder name (0 = unknown).
func (r *registry) find(name string) (slot int, branch string) {
	for i, n := range r.Slots {
		if n != "" && (n == name || Folder(n) == name) {
			return i + 1, n
		}
	}
	return 0, ""
}

func (r *registry) claim(name string) (int, error) {
	if n, _ := r.find(name); n > 0 {
		return n, nil
	}
	for i, n := range r.Slots {
		if n == "" {
			r.Slots[i] = name
			return i + 1, nil
		}
	}
	if len(r.Slots) >= MaxSlots {
		return 0, fmt.Errorf("all %d worktree slots are in use; remove some with `jw wt rm`", MaxSlots)
	}
	r.Slots = append(r.Slots, name)
	return len(r.Slots), nil
}

// Resolve returns a known worktree's branch name and slot, by branch or folder name.
func Resolve(c *agentapi.Config, name string) (branch string, slot int, err error) {
	err = withRegistry(c, false, func(r *registry) error {
		slot, branch = r.find(name)
		if slot == 0 {
			return fmt.Errorf("unknown worktree %q (see `jw wt ls`)", name)
		}
		return nil
	})
	return branch, slot, err
}

// WorktreeEnv is the environment of a worktree's processes.
func WorktreeEnv(c *agentapi.Config, branch string, slot int) map[string]string {
	env := map[string]string{"WORKTREE_NAME": branch, "PORT": strconv.Itoa(Port(slot))}
	if pg := c.Postgres; pg != nil {
		env["DATABASE_URL"] = "postgres://" + pg.User + ":" + pg.Password + "@postgres:5432/" + DBName(pg.DB, branch)
	}
	if c.Redis != nil {
		auth := ""
		if len(c.Redis.Passwords) > 0 {
			auth = ":" + c.Redis.Passwords[0] + "@"
		}
		env["REDIS_URL"] = "redis://" + auth + "redis:6379/" + strconv.Itoa(slot)
	}
	return env
}

// DBName is a worktree's copy of a database: lowercase, [a-z0-9_], at most 63 bytes.
func DBName(db, worktree string) string {
	s := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		}
		return '_'
	}, db+"_"+worktree)
	return truncate(s, 63)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// DBPairs maps each database to the worktree's copy: the main database, and extra databases
// whose name extends it (app_test → app_<worktree>_test).
func DBPairs(c *agentapi.Config, branch string) [][2]string {
	pg := c.Postgres
	if pg == nil {
		return nil
	}
	wt := DBName(pg.DB, branch)
	pairs := [][2]string{{pg.DB, wt}}
	for _, x := range pg.ExtraDBs {
		if rest, ok := strings.CutPrefix(x, pg.DB); ok {
			pairs = append(pairs, [2]string{x, truncate(wt+rest, 63)})
		}
	}
	return pairs
}

// RewriteDBNames points database names in an env file at the worktree's copies. Only the path
// after a host (scheme://[user:pass@]host[:port]/<db>) and whole values of *DB*/*DATABASE*
// settings change, never a user name that happens to equal a database name.
func RewriteDBNames(content string, pairs [][2]string) string {
	for _, p := range pairs {
		from, to := regexp.QuoteMeta(p[0]), p[1]
		url := regexp.MustCompile(`((?:@|//)[^/@\s"']+)/` + from + `([^A-Za-z0-9_]|$)`)
		content = url.ReplaceAllString(content, "${1}/"+to+"${2}")
		setting := regexp.MustCompile(`(?m)^([A-Za-z0-9_]*(?:DB|DATABASE)[A-Za-z0-9_]*\s*=\s*["']?)` + from + `(["']?\s*)$`)
		content = setting.ReplaceAllString(content, "${1}"+to+"${2}")
	}
	return content
}

// ListWorktrees returns the registered worktrees.
func ListWorktrees(c *agentapi.Config) []agentapi.Worktree {
	var out []agentapi.Worktree
	_ = withRegistry(c, false, func(r *registry) error {
		for i, name := range r.Slots {
			if name == "" {
				continue
			}
			slot := i + 1
			dir := worktreeDir(c, name)
			w := agentapi.Worktree{Name: name, Dir: dir, Slot: slot, Port: Port(slot), RedisDB: slot}
			for _, p := range DBPairs(c, name) {
				w.DBs = append(w.DBs, p[1])
			}
			if b, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output(); err == nil {
				w.Branch = strings.TrimSpace(string(b))
			} else {
				w.Missing = true
			}
			out = append(out, w)
		}
		return nil
	})
	return append(out, unmanagedWorktrees(c, out)...)
}

// unmanagedWorktrees are the git worktrees in the worktree folders that have no slot.
func unmanagedWorktrees(c *agentapi.Config, managed []agentapi.Worktree) []agentapi.Worktree {
	data, err := exec.Command("git", "-C", c.Dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	var out []agentapi.Worktree
	for _, line := range strings.Split(string(data), "\n") {
		dir, ok := strings.CutPrefix(line, "worktree ")
		if !ok || agentapi.WorktreeOf(c.Dir, dir+"/") == "" {
			continue
		}
		if slices.ContainsFunc(managed, func(w agentapi.Worktree) bool { return w.Dir == dir }) {
			continue
		}
		w := agentapi.Worktree{Name: filepath.Base(dir), Dir: dir, Unmanaged: true}
		if b, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output(); err == nil {
			w.Branch = strings.TrimSpace(string(b))
		}
		out = append(out, w)
	}
	return out
}

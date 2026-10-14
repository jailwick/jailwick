package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/session"
	"github.com/jailwick/jailwick/internal/spec"
)

const resumeHelp = `RESUME: last (most recent conversation here), pick (choose one), or a conversation id.`

func (a *app) claudeCmd() *cobra.Command {
	var f sessionFlags
	cmd := &cobra.Command{
		Use:   "claude [last|pick|ID]",
		Short: "Start a Claude session in the sandbox (in a worktree folder: in that worktree)",
		Long: `Start Claude in the project's sandbox, attached to this terminal. Run it in the repo for the
main checkout, or in .jailwick/worktrees/<name> (or with -w) for that worktree.

` + resumeHelp,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, p, wt, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			return a.startSession(cmd.Context(), m, p, f, session.Options{Worktree: wt, Base: "HEAD", Resume: first(args)})
		},
	}
	f.add(cmd)
	return cmd
}

const (
	openHelp    = "let this session reach the whole internet until it ends (others stay on the allowlist)"
	privateHelp = "keep this session out of messages between sessions: it can't see or message the project's other sessions, nor they it"
)

// sessionFlags are the flags of every command that starts a session.
type sessionFlags struct{ open, chrome, noChrome, private bool }

func (f *sessionFlags) add(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.open, "open", false, openHelp)
	cmd.Flags().BoolVar(&f.private, "private", false, privateHelp)
	cmd.Flags().BoolVar(&f.chrome, "chrome", false,
		"let this session use Claude in Chrome (your signed-in browser), whatever claude.chrome says")
	cmd.Flags().BoolVar(&f.noChrome, "no-chrome", false, "keep Claude in Chrome off for this session, whatever claude.chrome says")
	cmd.MarkFlagsMutuallyExclusive("chrome", "no-chrome")
}

// chromeFor is whether a session of p uses Claude in Chrome: the flags, else claude.chrome.
func (f sessionFlags) chromeFor(p config.Project) bool {
	switch {
	case f.chrome:
		return true
	case f.noChrome:
		return false
	}
	return p.Chrome()
}

// startSession brings the sandbox up and runs a session in it; f completes o.
func (a *app) startSession(ctx context.Context, m *sandbox.Manager, project string, f sessionFlags, o session.Options) error {
	prep, err := a.ensureUp(ctx, m, project)
	if err != nil {
		return err
	}
	if err := a.checkResume(ctx, m, project, &o); err != nil {
		return err
	}
	o.Open, o.Chrome, o.Private = f.open, f.chromeFor(prep.Project), f.private
	if o.Chrome {
		a.ui.Warn("Claude in Chrome is on: this session can use your signed-in Chrome, which reaches any site and account outside the allowlist")
	}
	a.warnLoginOverride(prep.Inputs.Secrets)
	return session.Start(ctx, m, prep, o)
}

func first(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func (a *app) wtCmd() *cobra.Command {
	var base string
	var f sessionFlags
	cmd := &cobra.Command{
		Use:   "wt [name] [last|pick|ID]",
		Short: "Start a Claude session in a git worktree with its own ports and databases",
		Long: `Create (or reuse) the worktree <name> at .jailwick/worktrees/<name> on branch <name>, give it
its own port range, database copies and Redis database, and start Claude in it.

` + resumeHelp,
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, p, wt, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			if len(args) > 0 {
				wt = args[0]
			}
			if wt == "" {
				return fmt.Errorf("which worktree? jw wt <name> (or jw wt ls)")
			}
			resume := ""
			if len(args) > 1 {
				resume = args[1]
			}
			return a.startSession(cmd.Context(), m, p, f, session.Options{Worktree: wt, Base: base, Resume: resume})
		},
	}
	cmd.Flags().StringVar(&base, "base", "HEAD", "branch or commit a new worktree starts from")
	f.add(cmd)
	cmd.AddCommand(a.wtLsCmd(), a.wtShellCmd(), a.wtRmCmd())
	return cmd
}

func (a *app) wtLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List worktrees with their port, databases and Redis database",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			st, err := m.AgentStatus(cmd.Context(), p)
			if err != nil {
				return fmt.Errorf("%s isn't running (jw up): %w", p, err)
			}
			rows := [][]string{{"(main checkout)", "3000", "-", "0", ""}}
			for _, w := range st.Worktrees {
				branch := w.Branch
				switch {
				case w.Missing:
					branch = "(folder missing)"
				case w.Unmanaged:
					rows = append(rows, []string{w.Name, "-", "-", "-", branch + "  (no slot: shares the main checkout's ports and databases)"})
					continue
				}
				rows = append(rows, []string{w.Name, fmt.Sprint(w.Port), join(w.DBs), fmt.Sprint(w.RedisDB), branch})
			}
			a.ui.Table([]string{"WORKTREE", "PORT", "DATABASES", "REDIS", "BRANCH"}, rows)
			return nil
		},
	}
}

func join(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	out := s[0]
	for _, v := range s[1:] {
		out += "," + v
	}
	return out
}

func (a *app) wtShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell [name]",
		Short: "Open a shell in a worktree, with its environment",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				a.worktree = args[0]
			}
			return a.runIn(cmd.Context(), nil)
		},
	}
}

func (a *app) wtRmCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "rm [name]",
		Short: "Remove a worktree and its databases (the branch is kept)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, p, wt, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			if len(args) == 1 {
				wt = args[0]
			}
			if wt == "" {
				return fmt.Errorf("which worktree? jw wt rm <name>")
			}
			c := []string{spec.AgentPath, "wt", "rm"}
			if force {
				c = append(c, "-force")
			}
			return a.execAgent(cmd.Context(), m, p, append(c, wt))
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "remove even with uncommitted changes")
	return cmd
}

// execAgent runs a jailwick-agent command in the sandbox with the terminal attached.
func (a *app) execAgent(ctx context.Context, m *sandbox.Manager, project string, cmd []string) error {
	proj, err := a.resolvedProject(project)
	if err != nil {
		return err
	}
	return m.Driver.Exec(ctx, runtime.ExecOptions{
		Container: sandbox.MainContainer(project), User: proj.Image.User, Cmd: cmd,
		Stdout: os.Stdout, Stderr: os.Stderr,
	})
}

func (a *app) shellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Open a shell in the sandbox (in a worktree folder: with that worktree's environment)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return a.runIn(cmd.Context(), nil) },
	}
}

func (a *app) runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run -- COMMAND [ARGS...]",
		Short: "Run a command in the sandbox (one argument runs through bash -c)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				args = []string{"bash", "-c", args[0]}
			}
			return a.runIn(cmd.Context(), args)
		},
	}
}

// runIn runs a command (nil: a shell) in the main checkout or the current worktree.
func (a *app) runIn(ctx context.Context, cmd []string) error {
	m, p, wt, err := a.needProject(ctx)
	if err != nil {
		return err
	}
	prep, err := a.ensureUp(ctx, m, p)
	if err != nil {
		return err
	}
	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	return session.Run(ctx, m.Driver, p, prep.Project.Image.User, prep.Project.Dir, wt, tty, cmd)
}

func (a *app) dbCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "db [psql args]",
		Short:              "Open psql on the project's database (the worktree's copy in a worktree)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.needService("postgres"); err != nil {
				return err
			}
			return a.runIn(cmd.Context(), append([]string{"sh", "-c", `exec psql "$DATABASE_URL" "$@"`, "psql"}, args...))
		},
	}
}

func (a *app) redisCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "redis [redis-cli args]",
		Short:              "Open redis-cli on the project's Redis (the worktree's database in a worktree)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.needService("redis"); err != nil {
				return err
			}
			return a.runIn(cmd.Context(), append([]string{"sh", "-c", `exec redis-cli -u "$REDIS_URL" "$@"`, "redis-cli"}, args...))
		},
	}
}

func (a *app) mongoCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "mongo [mongosh args]",
		Short:              "Open mongosh on the project's MongoDB (shared by all worktrees)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.needService("mongo"); err != nil {
				return err
			}
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			proj, err := a.resolvedProject(p)
			if err != nil {
				return err
			}
			if _, err := a.ensureUp(cmd.Context(), m, p); err != nil {
				return err
			}
			c := append([]string{"mongosh", "-u", proj.Mongo.User, "-p", proj.Mongo.Password, "--authenticationDatabase", "admin"}, args...)
			return m.Driver.Exec(cmd.Context(), runtime.ExecOptions{
				Container: runtime.ContainerName(spec.Prefix+p, "mongodb"), Cmd: c,
				TTY: term.IsTerminal(int(os.Stdin.Fd())), Stdout: os.Stdout, Stderr: os.Stderr,
			})
		},
	}
}

func (a *app) needService(s string) error {
	p, _, err := a.target()
	if err != nil {
		return err
	}
	proj, err := a.store.Project(p)
	if err != nil {
		return err
	}
	if !proj.Uses(s) {
		return fmt.Errorf("%s doesn't run %s (add it to services: jw config edit)", p, s)
	}
	return nil
}

func (a *app) resolvedProject(p string) (config.Project, error) {
	raw, err := a.store.Project(p)
	if err != nil {
		return raw, err
	}
	g, err := a.globalConfig()
	if err != nil {
		return raw, err
	}
	return raw.Resolved(g), nil
}

func (a *app) sessionsCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List Claude sessions (this project, or --all)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			m, err := a.manager(ctx, "")
			if err != nil {
				return err
			}
			var projects []string
			if all || a.project == "" && !a.detected() {
				if projects, err = m.Running(ctx); err != nil {
					return err
				}
			} else {
				p, _, err := a.target()
				if err != nil {
					return err
				}
				projects = []string{p}
			}
			if len(projects) == 0 {
				a.ui.Println("No running sandboxes.")
			}
			if all || a.project == "" && !a.detected() {
				defer a.noteLeftovers(ctx, m)
			}
			for _, p := range projects {
				st, err := m.AgentStatus(ctx, p)
				a.ui.Printf("%s %s\n", a.ui.Dot(err == nil), a.ui.Bold(p))
				if err != nil {
					a.ui.Println(a.ui.Dim("  not running"))
					continue
				}
				ss, err := m.Sessions(ctx, p)
				if err != nil {
					return err
				}
				a.printSessions(m.WithInternet(ctx, p, ss), st.Worktrees)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "every running sandbox")
	return cmd
}

func (a *app) detected() bool {
	_, _, ok := a.detect()
	return ok
}

func (a *app) stopSessionsCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "stop-sessions",
		Short: "End the Claude sessions of this project (or --all), e.g. before a restart",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			m, err := a.manager(ctx, "")
			if err != nil {
				return err
			}
			var projects []string
			if all {
				if projects, err = m.Running(ctx); err != nil {
					return err
				}
			} else {
				p, _, err := a.target()
				if err != nil {
					return err
				}
				projects = []string{p}
			}
			for _, p := range projects {
				if err := m.StopSessions(ctx, p); err != nil {
					return fmt.Errorf("%s: %w", p, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "every running sandbox")
	return cmd
}

// checkResume keeps a session off a conversation that another session holds: an id that runs is
// refused, and "last" continues the newest conversation at that place that doesn't run (Claude's
// own --continue would take the running one). When the sessions can't be listed, it lets the
// session start as asked.
func (a *app) checkResume(ctx context.Context, m *sandbox.Manager, project string, o *session.Options) error {
	if o.Resume == "" || o.Resume == "pick" {
		return nil
	}
	running, err := m.RunningConversations(ctx, project)
	if err != nil || len(running) == 0 {
		return nil
	}
	if o.Resume != "last" {
		return sandbox.CheckResume(running, o.Resume)
	}
	var convs []agentapi.Conversation
	if err := a.agentJSON(ctx, m, project, &convs, "history", "-n", "200"); err != nil {
		return nil
	}
	where := sandbox.WhereOf(o.Worktree)
	c, ok, skipped := sandbox.Latest(convs, where, running)
	switch {
	case !skipped:
		// The newest there doesn't run: --continue takes it.
	case ok:
		o.Resume = c.ID
		what := c.Name
		if what == "" {
			what = trunc(c.Prompt, 60)
		}
		a.ui.Step("the latest conversation in %s is running; continuing the one before it: %s", where, what)
	default:
		o.Resume = ""
		a.ui.Step("the only conversation in %s is running; starting a new one", where)
	}
	return nil
}

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/spec"
)

func (a *app) upCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up [service]",
		Short: "Start the project's sandbox, or one of its services again (never ends running sessions)",
		Long: `Start the project's sandbox: build missing images, then start the containers. A paused
sandbox resumes where it was.

A running sandbox is never recreated, because that would end its sessions. If the configuration,
image or jailwick changed since it started, up says so; jw restart applies the change.

With a service (egress-proxy, postgres, …), up starts that one again: one that stopped, crashed
or was paused. It is recreated alone (its data stays); the sessions carry on.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			if len(args) == 1 {
				if err := m.StartService(cmd.Context(), p, args[0]); err != nil {
					return err
				}
				a.ui.OK("%s: %s started again", p, args[0])
				return nil
			}
			_, st, err := m.Up(cmd.Context(), p)
			if err != nil {
				return err
			}
			a.ui.OK("%s is up", p)
			a.showPending(st)
			return nil
		},
	}
}

// showHealth says what a sandbox shows besides running or stopped, and what to do about it.
func (a *app) showHealth(p string, h sandbox.Health) {
	switch h.Main {
	case "paused":
		a.ui.Warn("%s is paused; resume it with: jw up -p %s", p, p)
	case "restarting":
		a.ui.Warn("%s crashed; the runtime is starting it again", p)
	}
	for _, pr := range h.Problems {
		a.ui.Warn("%s: %s", pr.Service, pr.What)
		a.ui.Hint("why: jw logs -p %s %s   start it again: jw up -p %s %s", p, pr.Service, p, pr.Service)
	}
}

func (a *app) showPending(st *sandbox.State) {
	if len(st.Pending) == 0 {
		return
	}
	a.ui.Warn("%s has pending changes; running sessions are untouched:", st.Name)
	for _, r := range st.Pending {
		a.ui.Hint("%s", r)
	}
	a.ui.Hint("apply them with: jw restart -p %s   (ends the project's sessions)", st.Name)
}

// ensureUp starts the sandbox when needed and reports pending changes.
func (a *app) ensureUp(ctx context.Context, m *sandbox.Manager, project string) (*sandbox.Prepared, error) {
	prep, st, err := m.Up(ctx, project)
	if err != nil {
		return nil, err
	}
	a.showPending(st)
	return prep, nil
}

func (a *app) restartCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Apply pending changes by recreating the sandbox (refuses while sessions run)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			st, err := m.Restart(cmd.Context(), p, force)
			if err != nil {
				return err
			}
			a.ui.OK("%s restarted", p)
			a.showPending(st)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "restart even with sessions running (ends them)")
	return cmd
}

func (a *app) downCmd() *cobra.Command {
	var all, sessions bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop the sandbox: remove its containers (data, images and Claude login are kept)",
		Long: `Stop the sandbox: its containers and networks are removed; its data (databases, caches,
Claude login and history) and images are kept, so jw up starts it again as it was. It refuses
while sessions run unless --include-sessions (sessions use the sandbox's networks). A sandbox
whose project file is gone is removed the same way: jw down -p <name>.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			var names []string
			if all {
				m, err := a.manager(ctx, "")
				if err != nil {
					return err
				}
				if names, err = m.Driver.Sandboxes(ctx); err != nil {
					return err
				}
			} else if a.project != "" && !a.isProject(a.project) {
				names = []string{a.project} // a sandbox left without its project
			} else {
				p, _, err := a.target()
				if err != nil {
					return err
				}
				names = []string{p}
			}
			var errs []error
			for _, name := range names {
				_, leftover, err := a.removeSandbox(ctx, name, sandbox.RemoveOptions{Sessions: sessions})
				switch {
				case err != nil:
					errs = append(errs, err)
				case leftover:
					a.ui.OK("%s had no project file: removed its leftover sandbox (its data is kept; jw destroy -p %s --volumes deletes it)", name, name)
				default:
					a.ui.OK("%s stopped", name)
				}
			}
			return errors.Join(errs...)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "stop every sandbox")
	cmd.Flags().BoolVar(&sessions, "include-sessions", false, "also end running sessions")
	return cmd
}

func (a *app) destroyCmd() *cobra.Command {
	var yes, sessions, volumes bool
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Remove the sandbox and its images (--volumes: also its data)",
		Long: `Remove the sandbox and the images built for its project that no other project uses (the
shared base and proxy stay while others need them). Its data (databases, caches, conversation
history and its own Claude login) is kept unless --volumes, which asks you to type the name first.
The app's Claude login (claude.login: app) is shared by projects and always stays. It refuses while
sessions run unless --include-sessions. A sandbox whose project file is gone is removed the same
way: jw destroy -p <name>.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name := a.project
			if name == "" || a.isProject(name) {
				p, _, err := a.target()
				if err != nil {
					return err
				}
				name = p
			}
			if volumes && !yes {
				a.ui.Warn("This deletes %s's databases, caches, conversation history and its own Claude login in the sandbox. The repo and the app's Claude login are not touched.", name)
				typed, err := a.ui.Input("Type the name to confirm", name, "", nil)
				if err != nil {
					return err
				}
				if typed != name {
					return fmt.Errorf("not confirmed")
				}
			}
			removed, leftover, err := a.removeSandbox(cmd.Context(), name,
				sandbox.RemoveOptions{Sessions: sessions, Images: true, Volumes: volumes})
			if err != nil {
				return err
			}
			what := "its sandbox"
			if leftover {
				what = "its leftover sandbox (no project file)"
			}
			switch n := len(removed.Images); {
			case n == 1:
				what += ", 1 image"
			case n > 1:
				what += fmt.Sprintf(", %d images", n)
			}
			if volumes {
				a.ui.OK("%s destroyed: %s and its data", name, what)
			} else {
				a.ui.OK("%s destroyed: %s (its data is kept; --volumes deletes it)", name, what)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&sessions, "include-sessions", false, "also end running sessions")
	cmd.Flags().BoolVar(&volumes, "volumes", false, "also delete its data (databases, caches, history, its own Claude login)")
	cmd.Flags().BoolVar(&yes, "yes", false, "don't ask for confirmation")
	return cmd
}

// removeSandbox removes a project's sandbox, or, when the name is no project, a sandbox left
// without it (leftover).
func (a *app) removeSandbox(ctx context.Context, name string, o sandbox.RemoveOptions) (removed sandbox.Removed, leftover bool, err error) {
	m, err := a.manager(ctx, name)
	if err != nil {
		return removed, false, err
	}
	if a.isProject(name) {
		removed, err = m.Remove(ctx, name, o)
		return removed, false, sessionsHint(err)
	}
	removed, err = m.RemoveLeftover(ctx, name, o)
	if errors.Is(err, sandbox.ErrNoSandbox) {
		err = fmt.Errorf("no project or sandbox named %q (jw project list shows the projects)", name)
	}
	return removed, true, sessionsHint(err)
}

// sessionsHint says how to go on when running sessions stopped a removal.
func sessionsHint(err error) error {
	if errors.Is(err, sandbox.ErrSessionsRunning) {
		return fmt.Errorf("%w (end them, or add --include-sessions)", err)
	}
	return err
}

func (a *app) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the sandbox: containers, sessions, worktrees, pending changes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.project == "" {
				if _, _, ok := a.detect(); !ok {
					return a.listProjects(cmd.Context(), true)
				}
			}
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			return a.showStatus(cmd.Context(), m, p)
		},
	}
}

func (a *app) showStatus(ctx context.Context, m *sandbox.Manager, p string) error {
	proj, err := a.store.Project(p)
	if err != nil {
		return err
	}
	st, err := m.Status(ctx, p)
	if err != nil {
		return err
	}
	a.ui.Printf("%s %s  %s\n", a.ui.Dot(st.Running), a.ui.Bold(p), a.ui.Dim(paths.Tilde(proj.Dir)))
	if proj.Workspace != "" {
		a.ui.Printf("  workspace  %s\n", proj.Workspace)
	}
	if len(st.Containers) == 0 {
		a.ui.Println(a.ui.Dim("  not started (jw up)"))
		return nil
	}
	a.ui.Println()
	var rows [][]string
	for _, c := range st.Containers {
		state := c.State
		if c.Health != "" {
			state += " (" + c.Health + ")"
		}
		rows = append(rows, []string{"  " + c.Service, state})
	}
	a.ui.Table([]string{"  SERVICE", "STATE"}, rows)
	a.showHealth(p, st.Health)
	if st.Running {
		ss, err := m.Sessions(ctx, p)
		if err == nil {
			ss = m.WithInternet(ctx, p, ss)
			var worktrees []agentapi.Worktree
			if as, err := m.AgentStatus(ctx, p); err == nil {
				worktrees = as.Worktrees
			}
			a.ui.Println()
			a.printSessions(ss, worktrees)
		}
	}
	a.showPending(st)
	return nil
}

func (a *app) printSessions(ss []sandbox.Session, worktrees []agentapi.Worktree) {
	if len(ss) == 0 {
		a.ui.Println(a.ui.Dim("  no sessions"))
	}
	for _, s := range ss {
		where := "main"
		if s.Worktree != "" {
			where = "wt " + s.Worktree
		}
		line := fmt.Sprintf("  └ %-14s %-6s %-5s", where, age(s.Started), s.State)
		if net := internetLabel(s.Internet, time.Now()); net != "" && net != "allowlist" {
			line += "  " + a.ui.Yellow("internet "+net)
		}
		if s.Chrome {
			line += "  " + a.ui.Yellow("chrome")
		}
		if s.Private {
			line += "  " + a.ui.Dim("private")
		}
		if s.Name != "" {
			line += "  " + a.ui.Dim(s.Name)
		}
		a.ui.Println(line)
	}
	for _, w := range worktrees {
		extra := fmt.Sprintf(":%d", w.Port)
		if w.Unmanaged {
			extra = "no slot: shares the main checkout's ports and databases"
		}
		if len(w.DBs) > 0 {
			extra += "  db " + strings.Join(w.DBs, ",")
		}
		if w.Missing {
			extra += "  (folder missing)"
		}
		a.ui.Println(a.ui.Dim(fmt.Sprintf("    worktree %-20s %s", w.Name, extra)))
	}
}

func age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	}
	return strconv.Itoa(int(d.Hours()/24)) + "d"
}

func (a *app) projectsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "projects",
		Aliases: []string{"ls"},
		Short:   "List projects with their state and sessions",
		Args:    cobra.NoArgs,
		RunE:    func(cmd *cobra.Command, _ []string) error { return a.listProjects(cmd.Context(), true) },
	}
}

func (a *app) listProjects(ctx context.Context, withSessions bool) error {
	ps, errs := a.store.Projects()
	for _, e := range errs {
		a.ui.Warn("%v", e)
	}
	if len(ps) == 0 {
		a.ui.Println("No projects yet. Add one with: jw project add")
		if m, err := a.manager(ctx, ""); err == nil {
			a.noteLeftovers(ctx, m)
		}
		return nil
	}
	m, err := a.manager(ctx, "")
	if err != nil {
		return err
	}
	running, _ := m.Running(ctx)
	current, _, _ := a.detect()
	var rows [][]string
	for _, p := range ps {
		on := contains(running, p.Name)
		sessions := "-"
		if on && withSessions {
			if ss, err := m.Sessions(ctx, p.Name); err == nil {
				sessions = strconv.Itoa(len(ss))
			}
		}
		name := p.Name
		if name == current {
			name += " *"
		}
		state := "stopped"
		if on {
			state = "running"
		}
		rows = append(rows, []string{a.ui.Dot(on) + " " + name, state, sessions, paths.Tilde(p.Dir)})
	}
	a.ui.Table([]string{"  PROJECT", "STATE", "SESSIONS", "FOLDER"}, rows)
	a.noteLeftovers(ctx, m)
	return nil
}

// noteLeftovers mentions sandboxes left without their project: no list shows their sessions.
func (a *app) noteLeftovers(ctx context.Context, m *sandbox.Manager) {
	ls, err := m.Leftovers(ctx)
	if err != nil {
		return
	}
	for _, l := range ls {
		a.ui.Warn("%s: a sandbox without a project (since %s, %d sessions). jw down -p %s --include-sessions removes it; "+
			"jw destroy -p %s --volumes also its data.", l.Name, l.Created.Local().Format("2 Jan 15:04"), len(l.Sessions), l.Name, l.Name)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (a *app) logsCmd() *cobra.Command {
	var follow bool
	var tail int
	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Show container logs (all services, or one: claude, egress-proxy, postgres, …)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			svc := ""
			if len(args) == 1 {
				svc = args[0]
			}
			return m.Driver.Logs(cmd.Context(), spec.Prefix+p, svc, runtime.LogOptions{Follow: follow, Tail: tail, Out: os.Stdout})
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep following")
	cmd.Flags().IntVarP(&tail, "tail", "n", 200, "lines from the end (0: all)")
	return cmd
}

func (a *app) statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Live CPU and memory of all running sandboxes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := a.manager(cmd.Context(), "")
			if err != nil {
				return err
			}
			running, err := m.Running(cmd.Context())
			if err != nil {
				return err
			}
			var names []string
			for _, p := range running {
				names = append(names, spec.Prefix+p)
			}
			return m.Driver.Stats(cmd.Context(), names, false, os.Stdout)
		},
	}
}

// devURL is where the host reaches a port of a worktree's (or the main checkout's) session.
func (a *app) devURL(ctx context.Context, m *sandbox.Manager, project, worktree string, port int) string {
	return fmt.Sprintf("http://%s:%d", m.DevHost(ctx, project, worktree), port)
}

// portFor returns the given port, else the worktree's first port, else 3000.
func (a *app) portFor(ctx context.Context, m *sandbox.Manager, project, worktree string, args []string) (int, error) {
	if len(args) == 1 {
		return strconv.Atoi(args[0])
	}
	if worktree != "" {
		if st, err := m.AgentStatus(ctx, project); err == nil {
			for _, w := range st.Worktrees {
				if w.Name == worktree || strings.ReplaceAll(w.Name, "/", "-") == worktree {
					return w.Port, nil
				}
			}
		}
	}
	return 3000, nil
}

func (a *app) urlCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url [port]",
		Short: "Print the URL of a port in the sandbox (in a worktree: its first port)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, p, wt, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			port, err := a.portFor(cmd.Context(), m, p, wt, args)
			if err != nil {
				return err
			}
			a.ui.Println(a.devURL(cmd.Context(), m, p, wt, port))
			return nil
		},
	}
}

func (a *app) openCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [port]",
		Short: "Open a port of the sandbox in the browser",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, p, wt, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			port, err := a.portFor(cmd.Context(), m, p, wt, args)
			if err != nil {
				return err
			}
			u := a.devURL(cmd.Context(), m, p, wt, port)
			a.ui.Step("opening %s", u)
			return hostutil.OpenURL(u)
		},
	}
}

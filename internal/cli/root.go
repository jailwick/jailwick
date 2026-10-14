// Package cli is jailwick's command line. Commands stay thin: they parse arguments, call the
// internal packages and print results.
package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/buildinfo"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/core"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/secrets"
	"github.com/jailwick/jailwick/internal/ui"
)

// app is shared by all commands; expensive parts are created on first use (core.Env).
type app struct {
	ui    *ui.UI
	env   *core.Env
	store *config.Store

	project, worktree string
	plain, verbose    bool
}

// Execute runs the command line and returns the process exit code.
func Execute() int {
	env := core.Open()
	a := &app{env: env, store: env.Store}
	root := a.rootCmd()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	if a.ui == nil {
		a.ui = ui.New(true)
	}
	var ee *runtime.ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	a.ui.Fail(err)
	return 1
}

func (a *app) rootCmd() *cobra.Command {
	name := filepath.Base(os.Args[0])
	if name != "jw" {
		name = "jailwick"
	}
	root := &cobra.Command{
		Use:   name,
		Short: "Run AI coding agents in a sandbox per repo",
		Long: `jailwick runs AI coding agents (Claude Code) in a locked-down container per repo: only the
repo is visible, the internet only through an allowlist, with its own databases and parallel
sessions in git worktrees.

Start with:  jw init  ·  jw project add  ·  jw claude`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       buildinfo.Get().String(),
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			a.ui = ui.New(a.plain)
			a.env.Log = a.ui.Step
			if a.verbose {
				a.env.Out = os.Stderr
			}
		},
	}
	root.PersistentFlags().StringVarP(&a.project, "project", "p", "", "project (default: detected from the current folder)")
	root.PersistentFlags().StringVarP(&a.worktree, "worktree", "w", "", "worktree (default: detected from the current folder)")
	root.PersistentFlags().BoolVar(&a.plain, "plain", false, "plain output: no colours or prompts")
	root.PersistentFlags().BoolVarP(&a.verbose, "verbose", "v", false, "show runtime output (builds, compose)")

	root.AddGroup(
		&cobra.Group{ID: "start", Title: "Getting started:"},
		&cobra.Group{ID: "session", Title: "Sessions:"},
		&cobra.Group{ID: "sandbox", Title: "Sandboxes:"},
		&cobra.Group{ID: "network", Title: "Network:"},
		&cobra.Group{ID: "config", Title: "Configuration:"},
	)
	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("start", a.initCmd(), a.doctorCmd(), a.traceCmd(), a.projectCmd(), a.importCmd(), a.loginCmd(), a.logoutCmd(), a.appCmd())
	add("session", a.claudeCmd(), a.wtCmd(), a.shellCmd(), a.runCmd(), a.dbCmd(), a.redisCmd(), a.mongoCmd(),
		a.sessionsCmd(), a.stopSessionsCmd(), a.historyCmd(), a.resumeCmd())
	add("sandbox", a.upCmd(), a.downCmd(), a.restartCmd(), a.destroyCmd(), a.statusCmd(), a.projectsCmd(),
		a.logsCmd(), a.statsCmd(), a.topCmd(), a.portsCmd(), a.urlCmd(), a.openCmd(), a.verifyCmd())
	add("network", a.allowCmd(), a.denyCmd(), a.blockedCmd(), a.internetCmd(), a.proxyCmd())
	add("config", a.configCmd(), a.settingsCmd(), a.secretCmd(), a.notesCmd(), a.workspaceCmd(), a.hiddenCmd(), a.handoffsCmd(),
		a.imageCmd(), a.aliasCmd(), a.whereCmd(), a.pathCmd())
	root.AddCommand(a.versionCmd(), a.updateCmd())
	return root
}

// globalConfig returns config.yaml.
func (a *app) globalConfig() (config.Global, error) { return a.env.Global() }

func (a *app) secretStore() (secrets.Store, error) { return a.env.Secrets() }

// manager returns the sandbox manager for a project's runtime.
func (a *app) manager(ctx context.Context, project string) (*sandbox.Manager, error) {
	return a.env.Manager(ctx, project)
}

// target resolves the project (and worktree) from flags or the current folder.
func (a *app) target() (project, worktree string, err error) {
	project, worktree = a.project, a.worktree
	if project != "" {
		if !a.isProject(project) { // a sandbox left without its project is no project (jw down removes it)
			return "", "", fmt.Errorf("project %q: %w (jw project list shows them)", project, config.ErrNotFound)
		}
		return project, worktree, nil
	}
	p, wt, ok := a.detect()
	if !ok {
		return "", "", a.noProject()
	}
	if worktree == "" {
		worktree = wt
	}
	return p, worktree, nil
}

// detect finds the project whose folder contains the current folder (the deepest one wins),
// and the worktree it is in (see agentapi.WorktreeOf).
func (a *app) detect() (project, worktree string, ok bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", false
	}
	cwd = paths.Real(cwd)
	ps, _ := a.store.Projects()
	best := ""
	for _, p := range ps {
		dir := paths.Real(p.Dir)
		if paths.Within(cwd, dir) && len(dir) > len(best) {
			best, project = dir, p.Name
		}
	}
	if project == "" {
		return "", "", false
	}
	return project, agentapi.WorktreeOf(best, cwd), true
}

func (a *app) noProject() error {
	ps, _ := a.store.Projects()
	if len(ps) == 0 {
		return errors.New("no projects yet; add one with: jw project add")
	}
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	return fmt.Errorf("no project for this folder; use -p <project> (have: %s) or cd into a project", strings.Join(names, ", "))
}

// needProject is the common start of project commands.
func (a *app) needProject(ctx context.Context) (*sandbox.Manager, string, string, error) {
	p, wt, err := a.target()
	if err != nil {
		return nil, "", "", err
	}
	m, err := a.manager(ctx, p)
	return m, p, wt, err
}

// isProject reports whether a project file exists for the name.
func (a *app) isProject(name string) bool { return hostutil.Exists(a.store.ProjectFile(name)) }

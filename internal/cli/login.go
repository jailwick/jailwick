package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/secrets"
	"github.com/jailwick/jailwick/internal/session"
)

const loginLong = `Claude's login is kept by the app (claude.login: app, the default), shared by every project
that uses it, or by the project itself (claude.login: project, e.g. another account). Signing in
runs claude auth login in the project's sandbox: open the link it prints, then paste the code.`

func (a *app) loginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign Claude in: once for every project on the app login, or a project's own",
		Long:  loginLong,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return a.claudeAuth(cmd.Context(), "login") },
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show which Claude login the project uses and who is signed in",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return a.claudeAuth(cmd.Context(), "status") },
	})
	return cmd
}

func (a *app) logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign Claude out of the login the project uses (the app login: every project on it)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return a.claudeAuth(cmd.Context(), "logout") },
	}
}

// claudeAuth runs claude auth <verb> in the project's sandbox, on the login it uses.
func (a *app) claudeAuth(ctx context.Context, verb string) error {
	m, p, _, err := a.needProject(ctx)
	if err != nil {
		return err
	}
	prep, err := a.ensureUp(ctx, m, p)
	if err != nil {
		return err
	}
	proj := prep.Project
	if proj.Agent.Command != config.DefaultAgentCommand {
		return fmt.Errorf("%s runs %s, not Claude Code (agent.command): there is no Claude login to manage", p, proj.Agent.Command)
	}
	if proj.AppLogin() {
		a.ui.Step("The app's Claude login: shared by every project with claude.login: app")
	} else {
		a.ui.Step("%s's own Claude login (claude.login: project)", p)
	}
	a.warnLoginOverride(prep.Inputs.Secrets)
	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	return session.Run(ctx, m.Driver, p, proj.Image.User, proj.Dir, "", tty, []string{"claude", "auth", verb})
}

// warnLoginOverride warns about secrets that make Claude in the sandbox skip its login.
func (a *app) warnLoginOverride(env map[string]string) {
	for _, k := range secrets.LoginOverrides {
		if env[k] != "" {
			a.ui.Warn("%s is set, so Claude uses it instead of its login; Remote Control and Claude in Chrome need the login. "+
				"To use the login, remove it where jw secret ls shows it: jw secret unset [--global] %s", k, k)
		}
	}
}

package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/claudesettings"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/paths"
	rt "github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/secrets"
	"github.com/jailwick/jailwick/internal/spec"
	"github.com/jailwick/jailwick/internal/ui"
	"github.com/jailwick/jailwick/internal/workspace"
)

func (a *app) initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Set up jailwick: runtime, git identity, Claude login, the jw command",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !a.ui.Interactive {
				return errors.New("init asks questions; run it in a terminal (or write config.yaml yourself: jw config path)")
			}
			g, err := a.store.Global()
			if err != nil {
				return err
			}

			var rts []ui.Option
			for _, r := range []struct{ name, bin, label string }{
				{"docker", "docker", "docker (OrbStack, Docker Desktop, Colima, Docker Engine)"},
				{"containerd", "nerdctl", "containerd (nerdctl)"},
				{"apple", "container", "Apple container (coming soon)"},
			} {
				if _, err := exec.LookPath(r.bin); err == nil {
					rts = append(rts, ui.Option{Label: r.label, Value: r.name})
				}
			}
			if len(rts) == 0 {
				return errors.New("no container runtime found; install OrbStack (recommended on macOS), Docker Desktop or Colima first")
			}
			def := g.Runtime
			if def == "" {
				def = rts[0].Value
			}
			if g.Runtime, err = a.ui.Select("Container runtime", rts, def); err != nil {
				return err
			}

			if g.Git.Name == "" {
				g.Git.Name = hostutil.GitConfig("user.name")
			}
			if g.Git.Email == "" {
				g.Git.Email = hostutil.GitConfig("user.email")
			}
			if g.Git.Name, err = a.ui.Input("Git author name for commits made in sandboxes", "", g.Git.Name, nil); err != nil {
				return err
			}
			if g.Git.Email, err = a.ui.Input("Git author email", "", g.Git.Email, nil); err != nil {
				return err
			}

			yolo := g.Claude.YOLO != nil && *g.Claude.YOLO
			if yolo, err = a.ui.Confirm("Skip Claude's permission prompts in sandboxes (--dangerously-skip-permissions)? The sandbox limits what it can reach, but Claude can still change anything in the repo.", yolo); err != nil {
				return err
			}
			g.Claude.YOLO = &yolo

			if err := a.store.SaveGlobal(g); err != nil {
				return err
			}
			a.env.Reload()
			a.ui.OK("saved %s", paths.Tilde(a.store.GlobalFile()))

			if err := a.askClaudeAuth(); err != nil {
				return err
			}

			if ok, err := a.ui.Confirm("Add the short command jw?", true); err == nil && ok {
				if link, err := addAlias("jw"); err != nil {
					a.ui.Warn("jw not added: %v", err)
				} else {
					a.ui.OK("jw added (%s)", paths.Tilde(link))
				}
			}
			a.ui.Println()
			a.ui.Println("Next: cd into a repo and run " + a.ui.Bold("jw project add") + ", then " + a.ui.Bold("jw claude") + ".")
			return nil
		},
	}
}

func (a *app) askClaudeAuth() error {
	s, err := a.secretStore()
	if err != nil {
		return err
	}
	if set := loginOverridesIn(s, secrets.Global); len(set) > 0 {
		a.ui.OK("Claude: %s is set for all projects", set[0])
		a.ui.Hint("it replaces Claude's login, which Remote Control and Claude in Chrome need; to sign in instead: jw secret unset --global %s, then jw login", set[0])
		return nil
	}
	choice, err := a.ui.Select("How should Claude sign in, in the sandboxes?", []ui.Option{
		{Label: "Sign in once, for all projects, with `jw login` (recommended)", Value: "login"},
		{Label: "Anthropic API key", Value: "key"},
		{Label: "Subscription token from `claude setup-token` (chat only: no Remote Control or Claude in Chrome)", Value: "token"},
	}, "login")
	if err != nil {
		return err
	}
	if choice == "login" {
		a.ui.Hint("once a project is added (jw project add), run: jw login")
		return nil
	}
	key := "CLAUDE_CODE_OAUTH_TOKEN"
	if choice == "key" {
		key = "ANTHROPIC_API_KEY"
	}
	val, err := a.ui.Secret("Paste the value (hidden)")
	if err != nil || val == "" {
		return err
	}
	if err := s.Set(secrets.Global, key, val); err != nil {
		return err
	}
	a.ui.OK("%s saved in the %s for all projects", key, s.Name())
	return nil
}

// loginOverridesIn lists the secrets set in a scope that replace Claude's login.
func loginOverridesIn(s secrets.Store, scope string) []string {
	keys, _ := s.Keys(scope)
	var set []string
	for _, k := range keys {
		if slices.Contains(secrets.LoginOverrides, k) {
			set = append(set, k)
		}
	}
	return set
}

func (a *app) doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the runtime, configuration, secrets and images",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			problems := a.doctor(cmd.Context())
			if problems > 0 {
				return fmt.Errorf("%d problem(s) found", problems)
			}
			a.ui.OK("all good")
			return nil
		},
	}
}

func (a *app) doctor(ctx context.Context) int {
	bad := 0
	ok := func(f string, v ...any) { a.ui.OK(f, v...) }
	fail := func(hint string, f string, v ...any) {
		bad++
		a.ui.Fail(fmt.Errorf(f, v...))
		if hint != "" {
			a.ui.Hint("%s", hint)
		}
	}
	g, err := a.store.Global()
	if err != nil {
		fail("fix it with: jw config edit --global", "config.yaml: %v", err)
	} else {
		ok("config %s", paths.Tilde(a.store.Dir))
	}
	a.updateNotice(ctx)

	d, err := rt.Open(g.Runtime, paths.StateDir())
	if err != nil {
		fail("", "runtime: %v", err)
	} else if err := d.Check(ctx); err != nil {
		fail("", "runtime %s: %v", d.Name(), err)
	} else {
		info := d.Info(ctx)
		ok("runtime %s %s on %s (%s)", info.Name, info.Version, info.Platform, info.Arch)
		if info.HostSuffix == "" {
			a.ui.Hint("dev URLs: this runtime has no per-container host names; servers are reachable from the sandbox only (OrbStack gives *.orb.local URLs)")
		}
		if _, err := assets.Agent(orHost(info.Arch)); err != nil {
			fail("", "agent: %v", err)
		} else {
			ok("agent for linux/%s included", orHost(info.Arch))
		}
		a.doctorNetworks(ctx, d)
	}

	s, err := secrets.Open(g.Secrets, a.store.Dir)
	if err != nil {
		fail("", "secrets: %v", err)
	} else if _, err := s.Keys(secrets.Global); err != nil {
		fail("", "secrets (%s): %v", s.Name(), err)
	} else {
		ok("secrets in the %s", s.Name())
		if set := loginOverridesIn(s, secrets.Global); len(set) > 0 {
			a.ui.Warn("%s set for all projects: Claude uses it instead of its login, so Remote Control and Claude in Chrome don't work", strings.Join(set, ", "))
		} else {
			a.ui.Hint("Claude: sign in once with jw login (shared by every project with claude.login: app)")
		}
	}

	ps, errs := a.store.Projects()
	for _, e := range errs {
		fail("fix it with: jw project edit", "%v", e)
	}
	for _, p := range ps {
		if err := p.Validate(); err != nil {
			fail("fix it with: jw project edit "+p.Name, "%v", err)
			continue
		}
		if _, err := claudesettings.Build(a.store, p.Resolved(g)); err != nil {
			fail("fix it with: jw settings edit (-p "+p.Name+")", "%s: Claude settings: %v", p.Name, err)
		}
		if s != nil {
			if set := loginOverridesIn(s, p.Name); len(set) > 0 {
				a.ui.Warn("%s: %s set: Claude uses it instead of its login", p.Name, strings.Join(set, ", "))
			}
		}
	}
	if len(ps) > 0 && len(errs) == 0 {
		ok("%d project(s) valid", len(ps))
	}
	for _, w := range a.store.Workspaces() {
		ws, err := a.store.Workspace(w)
		if err == nil {
			err = ws.Validate()
		}
		if err != nil {
			fail("", "%v", err)
			continue
		}
		// A project whose folder isn't a member: it sees the others, they don't see it. That can be
		// chosen (it keeps to itself), so it is said, not counted as a problem.
		for _, p := range ps {
			if p.Workspace != w {
				continue
			}
			if m, missing := workspace.MissingMember(ws, p); missing {
				a.ui.Warn("workspace %s: %s keeps to itself: the other projects don't see %s", w, p.Name, paths.Tilde(p.Dir))
				a.ui.Hint("to show it: Workspaces → %s → Settings → Members in the app, or jw workspace edit %s (a member \"- name: %s\"), then restart the other projects", w, w, m.Name)
			}
		}
	}
	return bad
}

func orHost(arch string) string {
	if arch == "" {
		return runtime.GOARCH
	}
	return arch
}

// doctorNetworks tells how many networks take an address range from the runtime's pools (with
// default settings about sandbox.DefaultNetworks fit, and a start fails beyond), and jailwick's
// networks that belong to no sandbox any more.
func (a *app) doctorNetworks(ctx context.Context, d rt.Driver) {
	nets, err := d.Networks(ctx)
	if err != nil {
		a.ui.Warn("networks: %v", err)
		return
	}
	total, ours := 0, 0
	for _, n := range nets {
		if n.Addressed {
			total++
			if strings.HasPrefix(n.Project, spec.Prefix) {
				ours++
			}
		}
	}
	if total >= sandbox.DefaultNetworks-5 {
		a.ui.Warn("%d networks use the runtime's address ranges (%d of them jailwick's): with default settings about %d fit, and starts fail beyond",
			total, ours, sandbox.DefaultNetworks)
		a.ui.Hint("each project uses 3, each session 1; more room: %s", sandbox.PoolsDoc)
	} else {
		a.ui.OK("%d networks (%d of them jailwick's)", total, ours)
	}
	if unused, err := (&sandbox.Manager{Driver: d}).UnusedNetworks(ctx); err == nil && len(unused) > 0 {
		a.ui.Hint("%d of jailwick's networks belong to no sandbox; a start that needs room removes them, or now: docker network rm %s",
			len(unused), strings.Join(unused, " "))
	}
}

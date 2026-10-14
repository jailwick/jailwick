package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/spec"
)

// agentJSON runs an agent command in a project's running sandbox and decodes its JSON output.
func (a *app) agentJSON(ctx context.Context, m *sandbox.Manager, project string, v any, args ...string) error {
	return agentErr(project, m.AgentJSON(ctx, sandbox.MainContainer(project), v, args...))
}

// agentErr says what to do when an agent command in a project's sandbox failed.
func agentErr(project string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sandbox.ErrAgentOlder):
		return fmt.Errorf("%s: %w (jw restart -p %s)", project, err, project)
	}
	return fmt.Errorf("%s: %w (is it running? jw up)", project, err)
}

func (a *app) verifyCmd() *cobra.Command {
	var all, asJSON bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check the sandbox's isolation: identity, files, network, tools",
		Long: `Run jailwick's isolation checks inside the sandbox: non-root without capabilities, only the
expected mounts (read-only where they should be), no runtime socket or SSH keys, allowed hosts
reachable through the proxy, everything else and the host blocked, no way around the proxy (no
outside name resolution, no public DNS servers, no IPv6, no other sandbox's services), services
and forwards up. It installs nothing and changes nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			var projects []string
			m, err := a.manager(ctx, "")
			if err != nil {
				return err
			}
			if all {
				if projects, err = m.Running(ctx); err != nil {
					return err
				}
			} else {
				p, _, err := a.target()
				if err != nil {
					return err
				}
				if _, err := a.ensureUp(ctx, m, p); err != nil {
					return err
				}
				projects = []string{p}
			}
			failed := 0
			for _, p := range projects {
				checks, err := m.Verify(ctx, p)
				if err != nil {
					return agentErr(p, err)
				}
				if asJSON {
					if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"project": p, "checks": checks}); err != nil {
						return err
					}
				} else {
					a.printChecks(p, checks)
				}
				for _, c := range checks {
					if !c.OK {
						failed++
					}
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d check(s) failed", failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "every running sandbox")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func (a *app) printChecks(project string, checks []agentapi.Check) {
	a.ui.Println(a.ui.Bold("verify " + project))
	section, pass, fail := "", 0, 0
	for _, c := range checks {
		if c.Section != section {
			section = c.Section
			a.ui.Println(a.ui.Dim(section))
		}
		mark := a.ui.Green("  ✔ ")
		if !a.ui.Rich {
			mark = "  PASS "
		}
		switch {
		case c.Info:
			mark = a.ui.Dim("  · ")
		case c.OK:
			pass++
		default:
			fail++
			mark = a.ui.Red("  ✘ ")
			if !a.ui.Rich {
				mark = "  FAIL "
			}
		}
		line := mark + c.Name
		if !c.OK && c.Detail != "" {
			line += a.ui.Dim("  (" + c.Detail + ")")
		}
		a.ui.Println(line)
	}
	summary := fmt.Sprintf("%d passed, %d failed", pass, fail)
	if fail == 0 {
		a.ui.Println(a.ui.Green(summary))
	} else {
		a.ui.Println(a.ui.Red(summary))
	}
}

func (a *app) portsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ports",
		Short: "List listening ports in the sandbox, with their command and session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			m, p, _, err := a.needProject(ctx)
			if err != nil {
				return err
			}
			ports, err := m.Ports(ctx, p)
			if err != nil {
				return err
			}
			if len(ports) == 0 {
				a.ui.Println("Nothing is listening.")
				return nil
			}
			var rows [][]string
			for _, pt := range ports {
				rows = append(rows, []string{strconv.Itoa(pt.Port), pt.Where, pt.Command, pt.URL})
			}
			a.ui.Table([]string{"PORT", "WHERE", "COMMAND", "URL"}, rows)
			return nil
		},
	}
}

func (a *app) topCmd() *cobra.Command {
	var once bool
	cmd := &cobra.Command{
		Use:   "top",
		Short: "CPU and memory of the sandbox and each session (one container each)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			return m.Driver.Stats(cmd.Context(), []string{spec.Prefix + p}, once, os.Stdout)
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "print one snapshot")
	return cmd
}

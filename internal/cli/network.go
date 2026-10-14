package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/presets"
	"github.com/jailwick/jailwick/internal/proxy"
)

func (a *app) allowCmd() *cobra.Command {
	var forProject bool
	var check string
	var withPresets []string
	cmd := &cobra.Command{
		Use:   "allow <domain>...",
		Short: "Let sandboxes reach domains (all projects, or --project for this one) or presets (--preset)",
		Long: `Add domains to the egress allowlist and reload the running proxies. A leading dot (.example.com)
or *.example.com also allows subdomains. By default domains go to your own allowlist
(` + "`" + `allowlist.txt` + "`" + `, all projects); --project adds them to this project only.

--preset adds whole presets (jw proxy presets lists them) to this project's allow_presets.
--check <host> tells whether a host is allowed, and by which entry.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if check != "" {
				return a.checkHost(check)
			}
			if len(withPresets) > 0 {
				p, err := a.changeProject(func(p *config.Project) error { return p.AddAllowPresets(withPresets...) })
				if err != nil {
					return err
				}
				a.ui.OK("allowed the %s preset(s) for %s", strings.Join(withPresets, ", "), p)
				if len(args) == 0 {
					return a.reloadProxies(cmd.Context(), []string{p})
				}
			}
			if len(args) == 0 {
				return errors.New("which domain? jw allow api.example.com (or a preset: jw allow --preset npm)")
			}
			var domains []string
			for _, d := range args {
				n, err := proxy.Normalize(d)
				if err != nil {
					return err
				}
				domains = append(domains, n)
			}
			var affected []string
			if forProject {
				p, _, err := a.target()
				if err != nil {
					return err
				}
				proj, err := a.store.Project(p)
				if err != nil {
					return err
				}
				proj.Allow = proxy.Minimize(append(proj.Allow, domains...))
				proj.Blocked = slices.DeleteFunc(proj.Blocked, func(h string) bool { // allowed: no longer kept blocked
					return slices.ContainsFunc(domains, func(d string) bool { return proxy.Covers(d, h) })
				})
				if err := a.store.SaveProject(proj); err != nil {
					return err
				}
				affected = []string{p}
			} else {
				if err := a.editUserAllowlist(func(list []string) []string { return proxy.Minimize(append(list, domains...)) }); err != nil {
					return err
				}
			}
			a.ui.OK("allowed %s", strings.Join(domains, ", "))
			return a.reloadProxies(cmd.Context(), affected)
		},
	}
	cmd.Flags().BoolVar(&forProject, "project", false, "only for this project")
	cmd.Flags().StringVar(&check, "check", "", "tell whether a host is allowed")
	cmd.Flags().StringSliceVar(&withPresets, "preset", nil, "add presets to this project (repeat, or comma-separate)")
	return cmd
}

// changeProject changes the current project's file and saves it; it returns the project's name.
func (a *app) changeProject(change func(*config.Project) error) (string, error) {
	name, _, err := a.target()
	if err != nil {
		return "", err
	}
	p, err := a.store.Project(name)
	if err != nil {
		return "", err
	}
	if err := change(&p); err != nil {
		return "", err
	}
	return name, a.store.SaveProject(p)
}

func (a *app) denyCmd() *cobra.Command {
	var forProject bool
	var withPresets []string
	cmd := &cobra.Command{
		Use:   "deny <domain>...",
		Short: "Remove domains you allowed, or presets from this project (--preset)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(withPresets) > 0 {
				p, err := a.changeProject(func(p *config.Project) error { return p.RemoveAllowPresets(withPresets...) })
				if err != nil {
					return err
				}
				a.ui.OK("removed the %s preset(s) from %s", strings.Join(withPresets, ", "), p)
				if len(args) == 0 {
					return a.reloadProxies(cmd.Context(), []string{p})
				}
			}
			if len(args) == 0 {
				return errors.New("which domain? jw deny api.example.com (or a preset: jw deny --preset npm)")
			}
			remove := func(list []string) []string {
				return slices.DeleteFunc(list, func(e string) bool {
					for _, d := range args {
						if n, err := proxy.Normalize(d); err == nil && n == e {
							return true
						}
					}
					return false
				})
			}
			var affected []string
			if forProject {
				p, _, err := a.target()
				if err != nil {
					return err
				}
				proj, err := a.store.Project(p)
				if err != nil {
					return err
				}
				proj.Allow = remove(proj.Allow)
				if err := a.store.SaveProject(proj); err != nil {
					return err
				}
				affected = []string{p}
			} else if err := a.editUserAllowlist(remove); err != nil {
				return err
			}
			a.ui.OK("removed %s", strings.Join(args, ", "))
			return a.reloadProxies(cmd.Context(), affected)
		},
	}
	cmd.Flags().BoolVar(&forProject, "project", false, "from this project's list")
	cmd.Flags().StringSliceVar(&withPresets, "preset", nil, "remove presets from this project (repeat, or comma-separate)")
	return cmd
}

func (a *app) editUserAllowlist(change func([]string) []string) error {
	var list []string
	if data, err := os.ReadFile(a.store.AllowFile()); err == nil {
		list = assets.ParseDomains(data)
	}
	list = change(list)
	var b strings.Builder
	b.WriteString("# Your domains, allowed in every jailwick sandbox (one per line, .domain = with subdomains).\n")
	for _, d := range list {
		b.WriteString(d + "\n")
	}
	return config.WriteFileAtomic(a.store.AllowFile(), []byte(b.String()), 0o644)
}

func (a *app) checkHost(host string) error {
	h, err := proxy.Normalize(host)
	if err != nil {
		return err
	}
	h = strings.TrimPrefix(h, ".")
	p, _, err := a.target()
	if err != nil {
		return err
	}
	m, err := a.manager(context.Background(), p)
	if err != nil {
		return err
	}
	list, err := m.Allowlist(p)
	if err != nil {
		return err
	}
	for _, e := range list {
		if proxy.Covers(e, h) {
			a.ui.OK("%s is allowed for %s (entry %s)", h, p, e)
			return nil
		}
	}
	a.ui.Warn("%s is not allowed for %s; allow it with: jw allow %s", h, p, h)
	return nil
}

// reloadProxies refreshes the allowlist of running sandboxes (all when projects is nil).
func (a *app) reloadProxies(ctx context.Context, projects []string) error {
	m, err := a.manager(ctx, "")
	if err != nil {
		return err
	}
	running, err := m.Running(ctx)
	if err != nil {
		return err
	}
	if projects != nil {
		running = slices.DeleteFunc(running, func(p string) bool { return !slices.Contains(projects, p) })
	}
	for _, p := range running {
		if err := m.ReloadProxy(ctx, p); err != nil {
			a.ui.Warn("%s: proxy not reloaded: %v", p, err)
			continue
		}
		a.ui.Step("reloaded %s's proxy", p)
	}
	return nil
}

func (a *app) proxyCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "proxy", Short: "Manage the egress proxies"}
	reload := &cobra.Command{
		Use:   "reload",
		Short: "Make running proxies re-read their allowlists (after editing allowlist.txt by hand)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return a.reloadProxies(cmd.Context(), nil) },
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "Print the project's full allowlist",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			l, err := m.Allowlist(p)
			if err != nil {
				return err
			}
			a.ui.Println(strings.Join(l, "\n"))
			return nil
		},
	}
	presetsCmd := &cobra.Command{
		Use:   "presets",
		Short: "List the allowlist presets by category (names for allow_presets); add or remove your own",
		Args:  cobra.NoArgs,
		Run:   func(*cobra.Command, []string) { a.printPresets() },
	}
	presetsCmd.AddCommand(a.presetAddCmd(), a.presetRmCmd())
	cmd.AddCommand(reload, list, presetsCmd)
	return cmd
}

func (a *app) blockedCmd() *cobra.Command {
	var n int
	var all bool
	cmd := &cobra.Command{
		Use:   "blocked",
		Short: "Show requests the proxy refused, most frequent first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			m, err := a.manager(ctx, "")
			if err != nil {
				return err
			}
			var projects []string
			if p, _, err := a.target(); err == nil && !all {
				projects = []string{p}
			} else if projects, err = m.Running(ctx); err != nil {
				return err
			}
			acc := map[string]*proxy.Denied{}
			for _, p := range projects {
				var buf bytes.Buffer
				if err := m.ProxyLogs(ctx, p, &buf); err != nil {
					continue
				}
				one := map[string]*proxy.Denied{}
				proxy.ParseDenied(&buf, p, one)
				if proj, err := a.store.Project(p); err == nil {
					for _, h := range proj.Blocked { // kept blocked: not listed (see the app's Network tab)
						delete(one, h)
					}
				}
				proxy.MergeDenied(acc, one)
			}
			list := proxy.SortDenied(acc)
			if len(list) == 0 {
				a.ui.Println("Nothing blocked.")
				return nil
			}
			var rows [][]string
			for i, d := range list {
				if i == n {
					break
				}
				rows = append(rows, []string{strconv.Itoa(d.Count), d.Host, d.Last.Format(time.DateTime), strings.Join(d.Projects, ",")})
			}
			a.ui.Table([]string{"COUNT", "HOST", "LAST", "PROJECTS"}, rows)
			a.ui.Hint("allow one with: jw allow <host>")
			return nil
		},
	}
	cmd.Flags().IntVarP(&n, "lines", "n", 30, "how many hosts")
	cmd.Flags().BoolVar(&all, "all", false, "all running sandboxes")
	return cmd
}

func (a *app) presetAddCmd() *cobra.Command {
	var description string
	cmd := &cobra.Command{
		Use:   "add <name> <domain>...",
		Short: "Create (or replace) a preset of your own; any project can then list it in allow_presets",
		Long: `Create a preset of your own: a named group of domains (.example.com includes subdomains), kept
in <config>/presets/<name>.yaml. Projects use it like any preset: jw allow --preset <name>.
Replacing one reloads the running proxies of the projects using it.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			hosts, err := proxy.ParseList(strings.Join(args[1:], "\n"))
			if err != nil {
				return err
			}
			if err := presets.SaveCustom(args[0], description, hosts); err != nil {
				return err
			}
			a.ui.OK("saved the %s preset: %s", args[0], strings.Join(hosts, ", "))
			if users := a.store.ProjectsUsingPreset(args[0]); len(users) > 0 {
				return a.reloadProxies(cmd.Context(), users)
			}
			a.ui.Hint("add it to a project: jw allow --preset %s", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&description, "description", "d", "", "what the preset is for")
	return cmd
}

func (a *app) presetRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a preset of your own (once no project uses it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if users := a.store.ProjectsUsingPreset(args[0]); len(users) > 0 {
				return fmt.Errorf("%s uses the %s preset: remove it there first (jw deny --preset %s -p <project>)",
					strings.Join(users, ", "), args[0], args[0])
			}
			if err := presets.DeleteCustom(args[0]); err != nil {
				return err
			}
			a.ui.OK("removed the %s preset", args[0])
			return nil
		},
	}
}

// printPresets lists the catalogue by category: name, description, hosts, and why a broad
// preset can receive data. Preset files of the user's that can't be read are reported.
func (a *app) printPresets() {
	if _, err := presets.Custom(); err != nil {
		a.ui.Warn("presets of yours left out: %v", err)
	}
	category := ""
	for _, p := range presets.Catalogue() {
		if p.Category != category {
			category = p.Category
			a.ui.Println()
			a.ui.Println(a.ui.Bold(category))
		}
		name := p.Name
		if p.Always {
			name += " (always)"
		}
		a.ui.Printf("  %-18s %s\n", name, p.Description)
		a.ui.Printf("  %-18s %s\n", "", a.ui.Dim(strings.Join(p.Hosts, " ")))
		if p.Broad != "" {
			a.ui.Printf("  %-18s %s\n", "", a.ui.Yellow("broad: "+p.Broad))
		}
	}
}

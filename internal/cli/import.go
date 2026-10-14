package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/importer"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/proxy"
)

func (a *app) importCmd() *cobra.Command {
	var dry, force bool
	var only []string
	cmd := &cobra.Command{
		Use:   "import [csb-folder]",
		Short: "Import projects, workspaces, notes and allowlist from a csb setup",
		Long: `Convert a csb setup (claude-container-setup) into jailwick configuration:
  projects/*.env      → projects/<name>.yaml (tokens go to the secret store, never into YAML)
  workspaces/*.env    → workspaces/<name>.yaml
  config/*/notes.md   → notes (with /etc/sandbox paths updated)
  proxy allowlist     → your allowlist.txt (domains no built-in preset covers)
The csb folder is only read. Existing jailwick projects are kept unless --force.
Imported projects keep csb's --dangerously-skip-permissions (claude.yolo: true).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setup := first(args)
			if setup == "" {
				setup = findCSB()
			}
			if setup == "" {
				return errors.New("where is the csb setup? jw import <folder>")
			}
			plan, err := importer.Read(paths.Real(setup))
			if err != nil {
				return err
			}
			keep := func(name string) bool { return len(only) == 0 || slices.Contains(only, name) }
			a.ui.Printf("Importing from %s%s\n\n", paths.Tilde(setup), map[bool]string{true: " (dry run: nothing is written)"}[dry])

			for _, w := range plan.Workspaces {
				if !keep(w.Config.Name) && !usedBy(plan, w.Config.Name, keep) {
					continue
				}
				_, exists := a.store.Workspace(w.Config.Name)
				if exists == nil && !force {
					a.ui.Step("workspace %s exists: kept", w.Config.Name)
					continue
				}
				a.ui.OK("workspace %s  %s  members: %s", w.Config.Name, paths.Tilde(w.Config.Root), memberNames(w.Config))
				if !dry {
					if err := a.store.SaveWorkspace(w.Config); err != nil {
						return err
					}
					a.writeNotes(a.store.WorkspaceNotesDir(w.Config.Name), w.Notes)
				}
			}

			s, err := a.secretStore()
			if err != nil {
				return err
			}
			for _, p := range plan.Projects {
				if !keep(p.Config.Name) {
					continue
				}
				if _, err := a.store.Project(p.Config.Name); err == nil && !force {
					a.ui.Step("project %s exists: kept (--force replaces it)", p.Config.Name)
					continue
				}
				var secretNames []string
				for k := range p.Secrets {
					secretNames = append(secretNames, k)
				}
				slices.Sort(secretNames)
				a.ui.OK("project %-16s %s  services: %s%s", p.Config.Name, paths.Tilde(p.Config.Dir),
					orDash(strings.Join(p.Config.Services, " ")), secretList(secretNames))
				if dry {
					continue
				}
				if err := a.store.SaveProject(p.Config); err != nil {
					return err
				}
				for _, k := range secretNames {
					if err := s.Set(p.Config.Name, k, p.Secrets[k]); err != nil {
						return fmt.Errorf("%s: saving %s: %w", p.Config.Name, k, err)
					}
				}
				a.writeNotes(a.store.ProjectNotesDir(p.Config.Name), p.Notes)
			}

			if len(plan.Allow) > 0 {
				a.ui.OK("allowlist: %s", strings.Join(plan.Allow, " "))
				if !dry {
					if err := a.editUserAllowlist(func(l []string) []string { return proxy.Minimize(append(l, plan.Allow...)) }); err != nil {
						return err
					}
				}
			}
			for _, sk := range plan.Skipped {
				a.ui.Warn("skipped %s", sk)
			}
			if !dry {
				a.ui.Println()
				a.ui.Println("Done. Your csb sandboxes keep running; start a project under jailwick with: jw up -p <name>")
				a.ui.Hint("jailwick sandboxes have their own volumes: log in again (or the token was imported), databases start empty")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dry, "dry-run", false, "show what would be imported (no secret values)")
	cmd.Flags().BoolVar(&force, "force", false, "replace existing jailwick projects and workspaces")
	cmd.Flags().StringSliceVar(&only, "only", nil, "import only these projects (and their workspaces)")
	return cmd
}

func usedBy(plan *importer.Plan, ws string, keep func(string) bool) bool {
	for _, p := range plan.Projects {
		if p.Config.Workspace == ws && keep(p.Config.Name) {
			return true
		}
	}
	return false
}

func (a *app) writeNotes(dir string, data []byte) {
	if len(data) == 0 {
		return
	}
	file := filepath.Join(dir, "notes.md")
	if fileExists(file) {
		a.ui.Step("notes %s exist: kept", paths.Tilde(file))
		return
	}
	if err := config.WriteFileAtomic(file, data, 0o644); err != nil {
		a.ui.Warn("notes: %v", err)
	}
}

func memberNames(w config.Workspace) string {
	var n []string
	for _, m := range w.Members {
		n = append(n, m.Name)
	}
	return strings.Join(n, " ")
}

func secretList(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return "  secrets: " + strings.Join(names, " ")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// findCSB locates a csb setup through the csb command on PATH (bin/csb → its parent folder).
func findCSB() string {
	bin, err := exec.LookPath("csb")
	if err != nil {
		return ""
	}
	real, err := filepath.EvalSymlinks(bin)
	if err != nil {
		return ""
	}
	setup := filepath.Dir(filepath.Dir(real))
	if _, err := os.Stat(filepath.Join(setup, "projects")); err != nil {
		return ""
	}
	return setup
}

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/claudesettings"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/paths"
)

const settingsLong = `Claude settings are layers, each overriding the ones before it:

  app       one file for every project
  profiles  named files; a project uses the ones it lists in claude.profiles, in order
  project   one project's own file

Objects merge key by key, lists keep every distinct entry, other values: the later layer wins.
The result is passed to Claude with --settings, so it takes precedence over what /model and
/config save. Changes apply to the next session; no restart needed.`

func (a *app) settingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Edit or show Claude's settings: app-wide, profiles and per project",
		Long:  settingsLong,
	}
	cmd.AddCommand(a.settingsEditCmd(), a.settingsShowCmd(), a.settingsLsCmd())
	return cmd
}

func (a *app) settingsEditCmd() *cobra.Command {
	var app bool
	var profile string
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit the project's Claude settings in $EDITOR (--app: every project's, --profile: a profile's)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := a.settingsFile(app, profile)
			if err != nil {
				return err
			}
			if _, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) {
				if err := config.WriteFileAtomic(file, []byte("{}\n"), 0o644); err != nil {
					return err
				}
			}
			if err := a.editUntilValid(file, func() error { return claudesettings.Check(file) }); err != nil {
				return err
			}
			a.ui.OK("saved %s; applies to the next session", paths.Tilde(file))
			if profile != "" {
				a.ui.Hint("projects use it when they list it: claude.profiles: [%s] (jw config edit)", profile)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&app, "app", false, "the settings every project uses")
	cmd.Flags().StringVar(&profile, "profile", "", "a named profile (created if new)")
	cmd.MarkFlagsMutuallyExclusive("app", "profile")
	return cmd
}

// settingsFile is the layer file to edit: the app's, a profile's, or the project's.
func (a *app) settingsFile(app bool, profile string) (string, error) {
	switch {
	case app:
		return a.store.ClaudeSettingsFile(), nil
	case profile != "":
		if err := config.ValidName(profile); err != nil {
			return "", fmt.Errorf("profile: %w", err)
		}
		return a.store.ProfileFile(profile), nil
	}
	p, _, err := a.target()
	if err != nil {
		return "", err
	}
	return a.store.ProjectSettingsFile(p), nil
}

func (a *app) settingsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the Claude settings the project's sessions get, and the layers they come from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _, err := a.target()
			if err != nil {
				return err
			}
			r, err := a.resolvedProject(p)
			if err != nil {
				return err
			}
			layers, err := claudesettings.Layers(a.store, r)
			if err != nil {
				return err
			}
			merged, err := claudesettings.Merge(layers)
			if err != nil {
				return err
			}
			names := make([]string, 0, len(layers))
			for _, l := range layers {
				names = append(names, l.Name)
			}
			a.ui.Step("layers: %s", strings.Join(names, " < "))
			a.ui.Printf("%s", merged)
			return nil
		},
	}
}

func (a *app) settingsLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List the Claude settings files: app-wide, profiles (with the projects using them) and projects'",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profiles, err := a.store.Profiles()
			if err != nil {
				return err
			}
			projects, _ := a.store.Projects() // unreadable projects show up in jw doctor
			usedBy := map[string][]string{}
			var rows [][]string
			for _, p := range projects {
				for _, n := range p.Claude.Profiles {
					usedBy[n] = append(usedBy[n], p.Name)
				}
			}
			present := func(file string) string {
				if hostutil.Exists(file) {
					return paths.Tilde(file)
				}
				return "(none)"
			}
			rows = append(rows, []string{"app", "every project", present(a.store.ClaudeSettingsFile())})
			for _, n := range profiles {
				rows = append(rows, []string{"profile " + n, namesOrDash(usedBy[n]), paths.Tilde(a.store.ProfileFile(n))})
			}
			for _, p := range projects {
				if f := a.store.ProjectSettingsFile(p.Name); hostutil.Exists(f) {
					rows = append(rows, []string{"project", p.Name, paths.Tilde(f)})
				}
			}
			// Profiles a project lists but that don't exist break its sessions: show them.
			for _, n := range slices.Sorted(maps.Keys(usedBy)) {
				if !slices.Contains(profiles, n) {
					rows = append(rows, []string{"profile " + n, namesOrDash(usedBy[n]), "missing: jw settings edit --profile " + n})
				}
			}
			a.ui.Table([]string{"LAYER", "USED BY", "FILE"}, rows)
			return nil
		},
	}
}

func namesOrDash(names []string) string {
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ", ")
}

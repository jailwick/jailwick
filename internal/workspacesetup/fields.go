package workspacesetup

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/setup"
	"github.com/jailwick/jailwick/internal/workspace"
)

// The setup's types, from the shared engine.
type (
	Field    = setup.Field[*Draft]
	StepView = setup.StepView
)

const what = "workspace"

// Form is the guided flow; a review of the file and of what happens to the projects follows.
var Form = setup.Form[*Draft]{What: what, Steps: steps(), Clone: clone}

func steps() []setup.Step[*Draft] {
	return []setup.Step[*Draft]{
		{ID: "workspace", Title: "Workspace", Fields: []Field{name()}},
		{ID: "members", Title: "Members", Fields: []Field{
			members(),
			elsewhere(),
		}},
		{ID: "projects", Title: "Projects", Fields: []Field{
			setup.MultiField("projects", "projects", "Projects in the workspace",
				"Each sees the members read-only, after a restart. Projects are the repos under the root that "+
					"were added with New project.",
				projectOptions, func(d *Draft) *[]string { return &d.Projects }),
		}},
		{ID: "sharing", Title: "Sharing", Fields: []Field{
			setup.TextField("handoff", "handoff", "Handoff folder",
				"Sessions leave requests for each other's repos here. Empty: off.",
				func(d *Draft) *string { return &d.Workspace.Handoff }).WithPlaceholder(".handoff"),
			setup.TextField("root_notes", "root-notes", "Workspace instructions",
				"A file shown read-only as <root>/CLAUDE.md to every session, e.g. CLAUDE.md. Empty: none.",
				func(d *Draft) *string { return &d.Workspace.RootNotes }).WithPlaceholder("CLAUDE.md"),
			setup.ListField("exclude", "exclude", "More hiding rules",
				"On top of the defaults ("+strings.Join(workspace.DefaultRules, " ")+"): x hides, -x shows "+
					"again, -* drops the defaults.",
				func(d *Draft) *[]string { return &d.Workspace.Exclude }).WithPlaceholder("secrets/").NotedBy(hiddenNote),
		}},
	}
}

func clone(d *Draft) *Draft {
	c := *d
	c.Workspace.Members = slices.Clone(d.Workspace.Members) // Set rebuilds it; the rest is replaced whole
	return &c
}

// name is the workspace's name. Renaming an existing workspace moves its projects and notes along.
func name() Field {
	return setup.TextField("name", "", "Workspace name",
		"Lowercase letters, digits and dashes.",
		func(d *Draft) *string { return &d.Workspace.Name }).CheckedBy(config.ValidName)
}

// The members field holds the folders under the root; elsewhere holds the others (by path).

func memberOptions(d *Draft) []setup.Option {
	if d == nil {
		return nil
	}
	names := slices.Clone(d.Found.Folders)
	for _, m := range d.Workspace.Members {
		if n := strings.TrimSuffix(m.Name, "/"); m.Path == "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	out := make([]setup.Option, len(names))
	for i, n := range names {
		out[i] = setup.Option{Value: n}
		switch p, ok := d.projectAt(n); {
		case ok:
			out[i].Help = "project " + p
		case slices.Contains(d.Found.Repos, n):
			out[i].Help = "git repo"
		}
	}
	return out
}

// members are the member folders under the root (those without a path).
func members() Field {
	return Field{
		Key: "members", Flag: "members", Label: "Members", Kind: setup.Multi, Placeholder: "api, web",
		Help:      "Folders under the root that each project sees read-only. Secrets in them (.env, keys) are hidden.",
		OptionsOf: memberOptions,
		Get: func(d *Draft) any {
			var out []string
			for _, m := range d.Workspace.Members {
				if m.Path == "" {
					out = append(out, strings.TrimSuffix(m.Name, "/"))
				}
			}
			return out
		},
		Set: func(d *Draft, v any) error {
			ms := slices.DeleteFunc(slices.Clone(d.Workspace.Members), func(m config.Member) bool { return m.Path == "" })
			for _, n := range v.([]string) {
				ms = append(ms, config.Member{Name: n})
			}
			d.Workspace.Members = ms
			return nil
		},
	}
}

func elsewhere() Field {
	return Field{
		Key: "members_elsewhere", Flag: "member-path", Label: "Folders from elsewhere", Kind: setup.List,
		Help:        "Other folders shown read-only, by path. Each appears under the root with its folder's name.",
		Placeholder: "~/code/shared-docs",
		Get: func(d *Draft) any {
			var out []string
			for _, m := range d.Workspace.Members {
				if m.Path != "" {
					out = append(out, m.Path)
				}
			}
			return out
		},
		Set: func(d *Draft, v any) error {
			ms := slices.DeleteFunc(slices.Clone(d.Workspace.Members), func(m config.Member) bool { return m.Path != "" })
			for _, p := range v.([]string) {
				ms = append(ms, config.Member{Name: filepath.Base(paths.Expand(p)), Path: p})
			}
			d.Workspace.Members = ms
			return nil
		},
	}
}

// projectOptions are the projects under the root; one in another workspace can't be chosen.
func projectOptions(d *Draft) []setup.Option {
	if d == nil {
		return nil
	}
	out := make([]setup.Option, 0, len(d.Found.Inside))
	for _, p := range d.Found.Inside {
		o := setup.Option{Value: p.Name, Help: paths.Tilde(p.Dir)}
		if p.Workspace != "" && p.Workspace != d.Before {
			o.Tag, o.Warn, o.Disabled = "in "+p.Workspace, "It is in the workspace "+p.Workspace+": leave it there first.", true
		}
		out = append(out, o)
	}
	return out
}

// projectAt is the project whose folder is the member folder n, if any.
func (d *Draft) projectAt(n string) (string, bool) {
	dir := filepath.Join(paths.Real(d.Workspace.Root), n)
	for _, p := range d.Found.Inside {
		if paths.Real(p.Dir) == dir {
			return p.Name, true
		}
	}
	return "", false
}

// hiddenNote counts what the rules hide in the members, so a rule's reach shows as it is typed.
func hiddenNote(d *Draft) string {
	n := len(workspace.Hidden(d.Workspace))
	note := fmt.Sprintf("%d paths are hidden in the members.", n)
	if n > workspace.MaskWarn {
		note += " That many slow a start down: hiding a whole folder (e.g. build/) is lighter."
	}
	return note
}

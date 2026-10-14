// Package workspacesetup is the guided creation and editing of a workspace, shared by
// `jw workspace add` and the app: a draft prefilled from the root folder, the fields of Form,
// then the file and the projects that join or leave (workspace.Apply).
package workspacesetup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/workspace"
)

// Draft is a workspace being created or edited.
type Draft struct {
	Workspace config.Workspace
	Projects  []string // the projects in it after saving
	Found     Found
	Before    string // the name of the workspace being edited; "" for a new one
}

// Found is what the root folder holds.
type Found struct {
	Folders []string         // folders directly under the root (hidden ones aside)
	Repos   []string         // those that are git repos
	Inside  []config.Project // projects whose folder is inside the root
}

// New starts a workspace for a root folder: its repos become members, the projects inside it
// that are in no workspace join, a handoff folder is on, and a CLAUDE.md at the root is shown.
func New(store *config.Store, root string) (*Draft, error) {
	root = paths.Real(paths.Expand(strings.TrimSpace(root)))
	if err := config.CheckMountable(root); err != nil {
		return nil, problem("%v", err)
	}
	if strings.ContainsAny(root, " \t") {
		return nil, problem("%s: paths with spaces aren't supported", paths.Tilde(root))
	}
	for _, n := range store.Workspaces() {
		if w, err := store.Workspace(n); err == nil && paths.Real(w.Root) == root {
			return nil, problem("%s is already the workspace %q", paths.Tilde(root), n)
		}
	}
	d := &Draft{Workspace: config.Workspace{Name: config.NameFrom(root, "workspace"), Root: root, Handoff: ".handoff"}}
	d.scan(store)
	for _, r := range d.Found.Repos {
		d.Workspace.Members = append(d.Workspace.Members, config.Member{Name: r})
	}
	for _, p := range d.Found.Inside {
		if p.Workspace == "" {
			d.Projects = append(d.Projects, p.Name)
		}
	}
	d.Projects = nonNil(d.Projects)
	if st, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err == nil && !st.IsDir() {
		d.Workspace.RootNotes = "CLAUDE.md"
	}
	return d, nil
}

// Load starts editing an existing workspace.
func Load(store *config.Store, name string) (*Draft, error) {
	w, err := store.Workspace(name)
	if err != nil {
		return nil, err
	}
	d := &Draft{Workspace: w, Before: name}
	d.scan(store)
	ps, _ := store.Projects()
	for _, p := range ps {
		if p.Workspace == name {
			d.Projects = append(d.Projects, p.Name)
		}
	}
	return d, nil
}

func (d *Draft) scan(store *config.Store) {
	root := paths.Real(d.Workspace.Root)
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			d.Found.Folders = append(d.Found.Folders, e.Name())
			if _, err := os.Stat(filepath.Join(root, e.Name(), ".git")); err == nil {
				d.Found.Repos = append(d.Found.Repos, e.Name())
			}
		}
	}
	ps, _ := store.Projects()
	for _, p := range ps {
		if dir := paths.Real(p.Dir); dir != root && paths.Within(dir, root) {
			d.Found.Inside = append(d.Found.Inside, p)
		}
	}
}

// Hints say what saving does to the projects.
func (d *Draft) Hints() []string {
	var hints []string
	if len(d.Found.Repos) == 0 && d.Before == "" {
		hints = append(hints, "No git repos directly under this folder: add members by hand, or choose the folder that holds them.")
	}
	if len(d.Projects) > 0 {
		hints = append(hints, join(d.Projects)+" will see the members read-only once restarted.")
	}
	return hints
}

// Finish returns the workspace to save, checked as a start would check it for each of its
// projects, so problems show now rather than at the next start.
func (d *Draft) Finish(store *config.Store) (config.Workspace, error) {
	w := d.Workspace
	pr := &config.Problems{What: what}
	if err := w.Validate(); err != nil {
		return w, err
	}
	if w.Name != d.Before {
		if _, err := store.Workspace(w.Name); err == nil {
			pr.List = append(pr.List, fmt.Sprintf("a workspace named %q exists; pick another name", w.Name))
		}
	}
	for _, name := range d.Projects {
		p, err := store.Project(name)
		if err != nil {
			pr.List = append(pr.List, fmt.Sprintf("project %s: %v", name, err))
			continue
		}
		p.Workspace = w.Name
		if err := workspace.Check(w, p); err != nil {
			pr.List = append(pr.List, fmt.Sprintf("%s: %s", name, strings.TrimPrefix(err.Error(), "workspace: ")))
		}
	}
	if len(pr.List) > 0 {
		return w, pr
	}
	return w, nil
}

// Save checks and saves the workspace and its projects. It returns the projects that changed.
func (d *Draft) Save(store *config.Store) ([]string, error) {
	w, err := d.Finish(store)
	if err != nil {
		return nil, err
	}
	return workspace.Apply(store, workspace.Change{Before: d.Before, After: &w, Projects: d.Projects})
}

// Remove removes a workspace: its projects leave it; repos, the handoff folder and notes stay.
// It returns the projects that left.
func Remove(store *config.Store, name string) ([]string, error) {
	if _, err := store.Workspace(name); err != nil {
		return nil, err
	}
	return workspace.Apply(store, workspace.Change{Before: name})
}

func join(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func problem(format string, a ...any) error {
	return &config.Problems{What: what, List: []string{fmt.Sprintf(format, a...)}}
}

// SetAll sets values by field key, in the order of the steps.
func (d *Draft) SetAll(values map[string]any) error { return Form.SetAll(d, values) }

// View describes every step with the draft's current values.
func (d *Draft) View() []StepView { return Form.View(d) }

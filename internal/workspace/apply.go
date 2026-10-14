package workspace

import (
	"errors"
	"io/fs"
	"os"
	"slices"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
)

// Change is a workspace created, edited, renamed or removed, with the projects in it afterwards.
type Change struct {
	Before   string            // its name before; "" for a new workspace
	After    *config.Workspace // nil removes it
	Projects []string          // the projects in it afterwards
}

// Apply saves a change and returns the projects whose file changed (a restart applies it). The
// steps keep every state in between valid, so no project ever names a workspace without a file:
// the new file first, then the projects that join, move or leave, then the old file and notes of
// a renamed or removed workspace. Leaving clears a project's workspace settings, as before it joined.
func Apply(store *config.Store, c Change) ([]string, error) {
	name := ""
	if c.After != nil {
		name = c.After.Name
		if err := store.SaveWorkspace(*c.After); err != nil {
			return nil, err
		}
	}
	ps, _ := store.Projects()
	var changed []string
	for _, p := range ps {
		in := c.Before != "" && p.Workspace == c.Before
		want := name != "" && slices.Contains(c.Projects, p.Name)
		switch {
		case want && p.Workspace != name:
			if !in { // joining afresh: no settings from another workspace
				p.WorkspaceRelated, p.WorkspaceExclude = nil, nil
			}
			p.Workspace = name
		case in && !want:
			p.Workspace, p.WorkspaceRelated, p.WorkspaceExclude = "", nil, nil
		default:
			continue
		}
		if err := store.SaveProject(p); err != nil {
			return changed, err
		}
		changed = append(changed, p.Name)
	}
	if c.Before != "" && c.Before != name {
		if err := os.Remove(store.WorkspaceFile(c.Before)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return changed, err
		}
		if err := moveNotes(store, c.Before, name); err != nil {
			return changed, err
		}
	}
	return changed, nil
}

// moveNotes keeps a renamed workspace's notes; a removed one's stay where they are, for a
// workspace of the same name later.
func moveNotes(store *config.Store, from, to string) error {
	if to == "" {
		return nil
	}
	err := os.Rename(store.WorkspaceNotesDir(from), store.WorkspaceNotesDir(to))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Workspaces is what Containing needs of the config store.
type Workspaces interface {
	Workspaces() []string
	Workspace(name string) (config.Workspace, error)
}

// Containing names the workspaces whose root holds dir: those a project there can join.
func Containing(store Workspaces, dir string) []string {
	out := []string{}
	dir = paths.Real(dir)
	for _, n := range store.Workspaces() {
		if w, err := store.Workspace(n); err == nil {
			if root := paths.Real(w.Root); dir != root && paths.Within(dir, root) {
				out = append(out, n)
			}
		}
	}
	return out
}

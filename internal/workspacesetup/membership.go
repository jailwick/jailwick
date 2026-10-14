package workspacesetup

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/workspace"
)

// Membership is a project's side of a workspace: which one it is in, the members it sees (all
// when Related is empty) and its own hiding rules. Members and Candidates describe the choices.
type Membership struct {
	Workspace  string   `json:"workspace"`
	Related    []string `json:"related"`
	Exclude    []string `json:"exclude"`
	Members    []string `json:"members"`    // of its workspace, its own folder aside
	Candidates []string `json:"candidates"` // workspaces whose root holds the project
	// ShowFolder, when joining: make the project's folder a member too, so the other projects see
	// it (read-only). Left false, it keeps to itself: it sees them, they don't see it.
	ShowFolder bool `json:"show_folder,omitempty"`
	// RemoveFolder, when leaving: take the project's folder out of the workspace's members too,
	// so the other projects no longer see it. Left false, the folder stays shown to them.
	RemoveFolder bool `json:"remove_folder,omitempty"`
}

// MembershipOf describes a project's membership and its choices.
func MembershipOf(store *config.Store, project string) (Membership, error) {
	p, err := store.Project(project)
	if err != nil {
		return Membership{}, err
	}
	m := Membership{Workspace: p.Workspace, Related: nonNil(p.WorkspaceRelated), Exclude: nonNil(p.WorkspaceExclude),
		Members: []string{}, Candidates: workspace.Containing(store, p.Dir)}
	if w, err := store.Workspace(p.Workspace); err == nil {
		own := paths.Real(p.Dir)
		for _, mb := range w.Members {
			if workspace.MemberDir(w, mb) != own {
				m.Members = append(m.Members, strings.TrimSuffix(mb.Name, "/"))
			}
		}
	}
	return m, nil
}

// SetMembership saves a project's workspace, related members and hiding rules, checked as a start
// would check them. Joining another workspace starts with all its members and no rules of its own,
// and makes the project's folder a member when asked (ShowFolder; workspace.AddMember); leaving
// takes it out only when asked (RemoveFolder). restart: the workspace's other projects, when the folder was added or
// taken out (each sees the change after a restart).
func SetMembership(store *config.Store, project string, m Membership) (restart []string, err error) {
	p, err := store.Project(project)
	if err != nil {
		return nil, err
	}
	pr := &config.Problems{What: "project " + project}
	was := p.Workspace
	switch {
	case m.Workspace == "":
		p.Workspace, p.WorkspaceRelated, p.WorkspaceExclude = "", nil, nil
		if err := store.SaveProject(p); err != nil || was == "" || !m.RemoveFolder {
			return nil, err
		}
		if removed, err := workspace.RemoveMember(store, was, p.Dir); err != nil || !removed {
			return nil, err
		}
		return workspace.Others(store, was, project), nil
	case m.Workspace != p.Workspace && !slices.Contains(workspace.Containing(store, p.Dir), m.Workspace):
		pr.List = append(pr.List, fmt.Sprintf("workspace %s: its root doesn't hold %s", m.Workspace, paths.Tilde(p.Dir)))
		return nil, pr
	case m.Workspace != p.Workspace:
		p.Workspace, p.WorkspaceRelated, p.WorkspaceExclude = m.Workspace, nil, nil
	default:
		m.ShowFolder = false // not joining: the members stay as they are
		p.WorkspaceRelated, p.WorkspaceExclude = clean(m.Related), clean(m.Exclude)
	}
	w, err := store.Workspace(p.Workspace)
	if err != nil {
		return nil, err
	}
	if err := workspace.Check(w, p); err != nil {
		pr.List = append(pr.List, strings.TrimPrefix(err.Error(), "workspace: "))
		return nil, pr
	}
	if err := store.SaveProject(p); err != nil {
		return nil, err
	}
	if !m.ShowFolder {
		return nil, nil
	}
	if added, err := workspace.AddMember(store, p); err != nil || !added {
		return nil, err
	}
	return workspace.Others(store, p.Workspace, project), nil
}

func clean(l []string) []string {
	var out []string
	for _, s := range l {
		if s = strings.TrimSpace(s); s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

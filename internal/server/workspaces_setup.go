package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/workspace"
	"github.com/jailwick/jailwick/internal/workspacesetup"
)

// workspaceRequest is a new workspace's root folder, or an existing workspace's name, and the
// answers given so far, by field key.
type workspaceRequest struct {
	Root   string         `json:"root"`
	Name   string         `json:"name"`
	Values map[string]any `json:"values"`
}

// changedView is what a save changed: the projects that joined or left (a restart applies it).
type changedView struct {
	Name    string   `json:"name,omitempty"`
	Changed []string `json:"changed"`
}

// draftWorkspace answers with the setup steps of a new workspace (root) or an existing one (name),
// prefilled and with the values set; problems come back next to the steps.
func (s *Server) draftWorkspace(w http.ResponseWriter, r *http.Request) {
	var req workspaceRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	d, setErr := s.workspaceDraft(w, req)
	if d == nil {
		return
	}
	ws, err := d.Finish(s.env.Store)
	writeDraft(w, DraftView{Dir: paths.Tilde(d.Workspace.Root), Steps: d.View(), Hints: d.Hints()}, ws, setErr, err)
}

// createWorkspace saves a new workspace and its projects.
func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var req workspaceRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	req.Name = ""
	s.saveDraft(w, req)
}

// saveWorkspace saves an existing workspace's settings and projects.
func (s *Server) saveWorkspace(w http.ResponseWriter, r *http.Request) {
	var req workspaceRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	req.Root, req.Name = "", r.PathValue("name")
	s.saveDraft(w, req)
}

// removeWorkspace removes a workspace; its projects leave it.
func (s *Server) removeWorkspace(w http.ResponseWriter, r *http.Request) {
	left, err := workspacesetup.Remove(s.env.Store, r.PathValue("name"))
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, changedView{Changed: nonNil(left)})
}

func (s *Server) saveDraft(w http.ResponseWriter, req workspaceRequest) {
	d, err := s.workspaceDraft(w, req)
	if d == nil {
		return
	}
	var changed []string
	if err == nil {
		changed, err = d.Save(s.env.Store)
	}
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, changedView{Name: d.Workspace.Name, Changed: nonNil(changed)})
}

// workspaceDraft starts the draft the request names; when it can't, it answers itself and returns
// a nil draft. The error is from setting the values, for the caller to report.
func (s *Server) workspaceDraft(w http.ResponseWriter, req workspaceRequest) (*workspacesetup.Draft, error) {
	var d *workspacesetup.Draft
	var err error
	switch {
	case req.Name != "":
		d, err = workspacesetup.Load(s.env.Store, req.Name)
	case req.Root != "":
		d, err = workspacesetup.New(s.env.Store, req.Root)
	default:
		err = badInput("root: required (the folder that holds the repos)")
	}
	if err != nil {
		apiError(w, err)
		return nil, nil
	}
	return d, d.SetAll(req.Values)
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// workspaceNotes and saveWorkspaceNotes serve the notes every session of a workspace reads.
func (s *Server) workspaceNotes(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.env.Store.Workspace(name); err != nil {
		apiError(w, err)
		return
	}
	readNotes(w, s.env.Store.WorkspaceNotesDir(name))
}

func (s *Server) saveWorkspaceNotes(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.env.Store.Workspace(name); err != nil {
		apiError(w, err)
		return
	}
	writeNotes(w, r, s.env.Store.WorkspaceNotesDir(name))
}

// handoff answers with one request of the workspace's handoff folder (?file=): {meta, body}.
func (s *Server) handoff(w http.ResponseWriter, r *http.Request) {
	wc, err := s.env.Store.Workspace(r.PathValue("name"))
	if err != nil {
		apiError(w, err)
		return
	}
	dir := workspace.HandoffDir(wc)
	if dir == "" {
		apiError(w, fmt.Errorf("workspace %s has no handoff folder: %w", wc.Name, config.ErrNotFound))
		return
	}
	h, body, err := workspace.ReadHandoff(dir, r.URL.Query().Get("file"))
	if errors.Is(err, fs.ErrNotExist) {
		err = fmt.Errorf("%w: %v", config.ErrNotFound, err)
	}
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]any{"meta": h, "body": body, "path": paths.Tilde(h.File)}) // path: for showing
}

// projectWorkspace and setProjectWorkspace serve a project's side of a workspace (Membership).
func (s *Server) projectWorkspace(w http.ResponseWriter, r *http.Request) {
	m, err := workspacesetup.MembershipOf(s.env.Store, r.PathValue("name"))
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, m)
}

func (s *Server) setProjectWorkspace(w http.ResponseWriter, r *http.Request) {
	var m workspacesetup.Membership
	if err := readJSON(r, &m); err != nil {
		apiError(w, err)
		return
	}
	name := r.PathValue("name")
	restart, err := workspacesetup.SetMembership(s.env.Store, name, m)
	if err != nil {
		apiError(w, err)
		return
	}
	v, err := workspacesetup.MembershipOf(s.env.Store, name)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, MembershipChange{Membership: v, Restart: nonNil(restart)})
}

// MembershipChange is a project's membership after a change, with the workspace's other projects
// that see the change of its folder (added to the members, or taken out) after a restart.
type MembershipChange struct {
	workspacesetup.Membership
	Restart []string `json:"restart"`
}

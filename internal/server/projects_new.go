package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/projectsetup"
	"github.com/jailwick/jailwick/internal/setup"
	"github.com/jailwick/jailwick/internal/workspace"
)

// newProjectRequest is a repo folder and the answers given so far, by field key.
type newProjectRequest struct {
	Dir    string         `json:"dir"`
	Values map[string]any `json:"values"`
}

// DraftView is a project being set up: the steps with their current values, and the file that
// would be written. Problems lists what still has to change before it can be created.
type DraftView struct {
	Dir      string           `json:"dir"`
	Steps    []setup.StepView `json:"steps"`
	Hints    []string         `json:"hints"`
	YAML     string           `json:"yaml"`
	Problems []string         `json:"problems"`
}

// draftProject answers with the setup steps for a folder, prefilled from the repo and the values.
// Values that don't fit come back as problems next to the steps, so the form keeps working.
func (s *Server) draftProject(w http.ResponseWriter, r *http.Request) {
	d, setErr := s.newDraft(w, r)
	if d == nil {
		return
	}
	p, err := d.Finish(s.env.Store)
	writeDraft(w, DraftView{Dir: paths.Tilde(d.Project.Dir), Steps: d.View(), Hints: d.Hints}, p, setErr, err)
}

// writeDraft answers with a draft: the file it would write, and the problems in errs (other
// errors fail the request). Problems are part of the answer, so the form keeps working.
func writeDraft(w http.ResponseWriter, v DraftView, file any, errs ...error) {
	v.Problems = []string{}
	if v.Hints == nil {
		v.Hints = []string{}
	}
	for _, e := range errs {
		var pr *config.Problems
		switch {
		case errors.As(e, &pr):
			v.Problems = append(v.Problems, pr.List...)
		case e != nil:
			apiError(w, e)
			return
		}
	}
	data, _ := config.Marshal(file)
	v.YAML = string(data)
	writeJSON(w, v)
}

// createProject saves a new project from a folder and the answers.
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	d, err := s.newDraft(w, r)
	if d == nil {
		return
	}
	var p config.Project
	if err == nil {
		p, err = d.Finish(s.env.Store)
	}
	if err == nil {
		err = s.env.Store.SaveProject(p)
	}
	if err != nil {
		apiError(w, err)
		return
	}
	// Its folder becomes a member of its workspace, unless it keeps to itself: the other projects
	// see it after a restart.
	restart := []string{}
	if !d.AddsMember() {
		writeJSON(w, map[string]any{"name": p.Name, "workspace": p.Workspace, "restart": restart})
		return
	}
	if added, err := workspace.AddMember(s.env.Store, p); err != nil {
		apiError(w, fmt.Errorf("%s was added, but not to the members of %s: %w", p.Name, p.Workspace, err))
		return
	} else if added {
		restart = nonNil(workspace.Others(s.env.Store, p.Workspace, p.Name))
	}
	writeJSON(w, map[string]any{"name": p.Name, "workspace": p.Workspace, "restart": restart})
}

// newDraft reads the request and starts the draft; when it can't, it answers itself and returns a
// nil draft. The error is from setting the values, for the caller to report.
func (s *Server) newDraft(w http.ResponseWriter, r *http.Request) (*projectsetup.Draft, error) {
	var req newProjectRequest
	err := readJSON(r, &req)
	if err == nil && req.Dir == "" {
		err = badInput("dir: required (the repo folder)")
	}
	var d *projectsetup.Draft
	if err == nil {
		d, err = projectsetup.New(s.env.Store, req.Dir)
	}
	if err != nil {
		apiError(w, err)
		return nil, nil
	}
	return d, d.SetAll(req.Values)
}

// pickFolder shows the folder dialog on this machine (the app runs here) and answers with the
// chosen folder, "" when cancelled. The dialog opens in start when it exists, else the home folder.
func (s *Server) pickFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Start string `json:"start"`
		Title string `json:"title"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	start := paths.Expand(req.Start)
	if req.Start == "" || !hostutil.Exists(start) {
		start = paths.Home()
	}
	if req.Title == "" {
		req.Title = "Choose the repo to add to jailwick"
	}
	path, err := s.PickFolder(r.Context(), req.Title, start)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]string{"path": path})
}

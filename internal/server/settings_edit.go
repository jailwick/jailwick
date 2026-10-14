package server

import (
	"net/http"

	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/projectsetup"
)

type settingsRequest struct {
	Values map[string]any `json:"values"`
}

// draftSettings answers with a project's settings as setup fields (projectsetup.EditForm:
// services and logins, toolset, network, worktrees, resources) with the values given, the file
// it would save and the problems.
func (s *Server) draftSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	d, err := projectsetup.Edit(s.env.Store, r.PathValue("name"), req.Values)
	if d == nil {
		apiError(w, err)
		return
	}
	writeDraft(w, DraftView{Dir: paths.Tilde(d.Project.Dir), Steps: projectsetup.EditForm.View(d)}, d.Project, err)
}

// saveSettings saves a project's settings. A restart applies them (the page lists it as pending).
func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	d, err := projectsetup.Edit(s.env.Store, r.PathValue("name"), req.Values)
	if err == nil {
		err = s.env.Store.SaveProject(d.Project)
	}
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

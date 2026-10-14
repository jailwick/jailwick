package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/workspace"
)

// Keeping the books of a workspace's handoffs from the app: a request's status (done moves it to
// done/), deleting one, and opening its file. Each change names the version of the file the page
// read: one a session changed since is left alone (409).

// handoffDir is the workspace's handoff folder, or why there is none.
func (s *Server) handoffDir(name string) (string, error) {
	wc, err := s.env.Store.Workspace(name)
	if err != nil {
		return "", err
	}
	dir := workspace.HandoffDir(wc)
	if dir == "" {
		return "", fmt.Errorf("workspace %s has no handoff folder: %w", wc.Name, config.ErrNotFound)
	}
	return dir, nil
}

func handoffError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		err = fmt.Errorf("%w: %v", config.ErrNotFound, err)
	case errors.Is(err, workspace.ErrHandoffChanged):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	apiError(w, err)
}

// setHandoffStatus: {file, version, status} → {file} (where it is now).
func (s *Server) setHandoffStatus(w http.ResponseWriter, r *http.Request) {
	dir, err := s.handoffDir(r.PathValue("name"))
	if err != nil {
		apiError(w, err)
		return
	}
	var req struct{ File, Version, Status string }
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	file, err := workspace.SetHandoffStatus(dir, req.File, req.Version, req.Status)
	if err != nil {
		handoffError(w, err)
		return
	}
	writeJSON(w, map[string]string{"file": file})
}

// deleteHandoff: ?file=&version=.
func (s *Server) deleteHandoff(w http.ResponseWriter, r *http.Request) {
	dir, err := s.handoffDir(r.PathValue("name"))
	if err != nil {
		apiError(w, err)
		return
	}
	q := r.URL.Query()
	if err := workspace.DeleteHandoff(dir, q.Get("file"), q.Get("version")); err != nil {
		handoffError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// openHandoff opens a request's file in its default app, or ({reveal}) shows it in the Finder.
func (s *Server) openHandoff(w http.ResponseWriter, r *http.Request) {
	dir, err := s.handoffDir(r.PathValue("name"))
	if err != nil {
		apiError(w, err)
		return
	}
	var req struct {
		File   string
		Reveal bool
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	path, err := workspace.HandoffPath(dir, req.File)
	if err != nil {
		handoffError(w, err)
		return
	}
	open := s.OpenFile
	if open == nil {
		open = hostutil.OpenFile
	}
	if err := open(path, req.Reveal); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

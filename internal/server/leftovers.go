package server

import (
	"errors"
	"net/http"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/sandbox"
)

// leftovers lists the sandboxes whose project file is gone: no project, so their sessions are in
// no other list. They are shown so they can be stopped.
func (s *Server) leftovers(w http.ResponseWriter, r *http.Request) {
	m, err := s.env.Manager(r.Context(), "")
	if err != nil {
		apiError(w, err)
		return
	}
	ls, err := m.Leftovers(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	if ls == nil {
		ls = []sandbox.Leftover{}
	}
	writeJSON(w, ls)
}

// removeLeftover stops a sandbox without a project, ending its sessions (the page's confirmation
// says so); ?data=1 also deletes its volumes.
func (s *Server) removeLeftover(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := config.ValidName(name); err != nil {
		apiError(w, badInput(err.Error()))
		return
	}
	m, err := s.env.Manager(r.Context(), "")
	if err != nil {
		apiError(w, err)
		return
	}
	_, err = m.RemoveLeftover(r.Context(), name,
		sandbox.RemoveOptions{Sessions: true, Volumes: r.URL.Query().Get("data") == "1"})
	if errors.Is(err, sandbox.ErrNoSandbox) {
		err = errors.Join(config.ErrNotFound, err)
	}
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]string{"removed": name})
}

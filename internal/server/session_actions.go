package server

import (
	"fmt"
	"net/http"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/hostterm"
)

// What the app can do with a running session, having no terminal of its own: bring the host
// terminal it runs in forward, or end it.

// showTerminal brings the terminal a session runs in forward: {found, app, note} (hostterm.Result).
func (s *Server) showTerminal(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	sid := r.PathValue("sid")
	b, err := m.FindSession(r.Context(), name, sid)
	if err == nil && b.ID != sid {
		err = fmt.Errorf("%w: no running session %q in %s", config.ErrNotFound, sid, name)
	}
	if err != nil {
		apiError(w, err)
		return
	}
	focus := s.Focus
	if focus == nil {
		focus = hostterm.Focus
	}
	writeJSON(w, focus(r.Context(), b.Terminal))
}

// endSession ends a session (its Claude); its terminal stays open.
func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	if err := m.StopSession(r.Context(), name, r.PathValue("sid")); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

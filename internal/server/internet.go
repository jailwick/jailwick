package server

import (
	"net/http"
	"time"

	"github.com/jailwick/jailwick/internal/proxy"
)

// internetRequest opens a session's internet for Minutes; 0 keeps it open until closed.
type internetRequest struct {
	Minutes int `json:"minutes"`
}

// openInternet lets one session reach the whole internet (PUT; again: a new end).
func (s *Server) openInternet(w http.ResponseWriter, r *http.Request) {
	var req internetRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	d := time.Duration(req.Minutes) * time.Minute
	if err := proxy.CheckDuration(d); err != nil {
		apiError(w, badInput(err.Error()))
		return
	}
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	b, err := m.FindSession(r.Context(), name, r.PathValue("sid"))
	if err != nil {
		apiError(w, err)
		return
	}
	in, err := m.OpenInternet(r.Context(), b, d)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, in)
}

// closeInternet puts a session back on the allowlist (DELETE).
func (s *Server) closeInternet(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	b, err := m.FindSession(r.Context(), name, r.PathValue("sid"))
	if err == nil {
		err = m.CloseInternet(r.Context(), b)
	}
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

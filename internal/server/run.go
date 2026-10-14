package server

import (
	"net/http"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// runAction starts or stops the project's run command in its main checkout or a worktree:
// {action: start|stop, worktree}. It answers with the runs there are now.
func (s *Server) runAction(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	var req struct {
		Action   string `json:"action"`
		Worktree string `json:"worktree"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	run := m.StartRun
	switch req.Action {
	case "start":
	case "stop":
		run = m.StopRun
	default:
		apiError(w, badInput("action: start or stop"))
		return
	}
	runs, err := run(r.Context(), name, req.Worktree)
	if err != nil {
		apiError(w, agentError(r.Context(), name, err))
		return
	}
	writeJSON(w, runs)
}

// runLog answers with the end of a run's output: {text}.
func (s *Server) runLog(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	text, err := m.RunLog(r.Context(), name, r.URL.Query().Get("worktree"))
	if err != nil {
		apiError(w, agentError(r.Context(), name, err))
		return
	}
	writeJSON(w, map[string]string{"text": text})
}

// changes answers with what the main checkout or a worktree changed (?worktree=), with one
// file's diff (?file=).
func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	ch, err := m.Changes(r.Context(), name, q.Get("worktree"), q.Get("file"))
	if err != nil {
		apiError(w, agentError(r.Context(), name, err))
		return
	}
	if ch.Files == nil {
		ch.Files = []agentapi.ChangedFile{}
	}
	writeJSON(w, ch)
}

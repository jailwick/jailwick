// Package server is jailwick's local API and web UI: JSON endpoints over the same packages the
// CLI uses, plus the embedded front end. The desktop app serves it in-process; `jw app --browser`
// serves it on 127.0.0.1 behind a random token.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/core"
	"github.com/jailwick/jailwick/internal/hostterm"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/sandbox"
)

// Server handles API requests and serves the UI.
type Server struct {
	env     *core.Env
	ui      fs.FS  // built front end (index.html + assets)
	token   string // required unless empty (in-process use)
	version string
	mux     *http.ServeMux
	builds  builds
	ops     ops
	wt      wtJobs // worktrees being made
	// listing is the last listing of every running session, shared for a moment.
	listing sessionsCache
	upd     updater
	// PickFolder shows a folder dialog on this machine ("" when cancelled). The desktop app sets
	// its own, attached to its window.
	PickFolder func(ctx context.Context, title, start string) (string, error)
	// Notify shows a notification; nil: notify.Send. The desktop app sends its own, so they carry
	// its name and icon and a click opens it.
	Notify func(Notification) error
	// Focus brings a session's host terminal forward; nil: hostterm.Focus (tests replace it).
	Focus func(context.Context, hostterm.Terminal) hostterm.Result
	// OpenFile opens a file in its default app (reveal: in the Finder); nil: hostutil.OpenFile.
	OpenFile func(path string, reveal bool) error
	// OnChange is told when what a project shows may have changed: one of its containers changed,
	// or a start, restart or stop began or ended (the menu bar shows it then, not at its next look).
	// Set it before Watch.
	OnChange func()
}

// New returns a server. An empty token disables authentication (only for in-process use by the
// desktop app, which never listens on a socket).
func New(env *core.Env, ui fs.FS, token, version string) *Server {
	s := &Server{env: env, ui: ui, token: token, version: version, mux: http.NewServeMux(),
		builds: builds{by: map[string]BuildView{}}, ops: newOps(), PickFolder: hostutil.PickFolder}
	s.ops.changed = s.changed
	s.routes()
	return s
}

// changed tells OnChange, when set, that what a project shows may have changed.
func (s *Server) changed() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// NewToken returns a random token for browser mode.
func NewToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const cookieName = "jailwick_token"

// ServeHTTP checks the host and token, then routes.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.token != "" {
		// Only loopback host names: a page elsewhere can't reach us through DNS rebinding.
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if t := r.URL.Query().Get("token"); t != "" && r.URL.Path == "/" && s.validToken(t) {
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil || !s.validToken(c.Value) {
			http.Error(w, "open the link printed by `jw app --browser`", http.StatusUnauthorized)
			return
		}
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) validToken(t string) bool {
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/overview", s.overview)
	m.HandleFunc("GET /api/live", s.live)
	m.HandleFunc("POST /api/projects/draft", s.draftProject)
	m.HandleFunc("POST /api/projects", s.createProject)
	m.HandleFunc("POST /api/pick-folder", s.pickFolder)
	m.HandleFunc("GET /api/projects/{name}", s.project)
	m.HandleFunc("POST /api/projects/{name}/up", s.up)
	m.HandleFunc("POST /api/projects/{name}/restart", s.restart)
	m.HandleFunc("POST /api/projects/{name}/image/build", s.rebuild)
	m.HandleFunc("POST /api/projects/{name}/down", s.down)
	m.HandleFunc("POST /api/projects/{name}/services/{service}/start", s.startService)
	m.HandleFunc("POST /api/projects/{name}/stop-sessions", s.stopSessions)
	m.HandleFunc("GET /api/projects/{name}/logs", s.logs)
	m.HandleFunc("GET /api/projects/{name}/verify", s.verify)
	m.HandleFunc("GET /api/projects/{name}/ports", s.ports)
	m.HandleFunc("GET /api/projects/{name}/blocked", s.blocked)
	m.HandleFunc("POST /api/projects/{name}/block", s.block)
	m.HandleFunc("POST /api/projects/{name}/discard", s.discard)
	m.HandleFunc("GET /api/projects/{name}/history", s.history)
	m.HandleFunc("GET /api/projects/{name}/notes", s.notes)
	m.HandleFunc("PUT /api/projects/{name}/notes", s.saveNotes)
	m.HandleFunc("POST /api/projects/{name}/terminal", s.terminal)
	m.HandleFunc("POST /api/projects/{name}/run", s.runAction)
	m.HandleFunc("GET /api/projects/{name}/run/log", s.runLog)
	m.HandleFunc("GET /api/projects/{name}/build/log", s.buildLog)
	m.HandleFunc("GET /api/projects/{name}/changes", s.changes)
	m.HandleFunc("PUT /api/projects/{name}/node-modules", s.saveNodeModules)
	m.HandleFunc("POST /api/projects/{name}/settings/draft", s.draftSettings)
	m.HandleFunc("PUT /api/projects/{name}/settings", s.saveSettings)
	m.HandleFunc("PUT /api/projects/{name}/sessions/{sid}/internet", s.openInternet)
	m.HandleFunc("DELETE /api/projects/{name}/sessions/{sid}/internet", s.closeInternet)
	m.HandleFunc("POST /api/projects/{name}/worktrees", s.addWorktree)
	m.HandleFunc("GET /api/projects/{name}/worktree", s.checkWorktree)
	m.HandleFunc("DELETE /api/projects/{name}/worktree", s.removeWorktree)
	m.HandleFunc("GET /api/projects/{name}/worktree/log", s.worktreeLog)
	m.HandleFunc("DELETE /api/projects/{name}/worktree/job", s.dismissWorktreeJob)
	m.HandleFunc("GET /api/projects/{name}/branches", s.branches)
	m.HandleFunc("POST /api/projects/{name}/sessions/{sid}/show", s.showTerminal)
	m.HandleFunc("POST /api/projects/{name}/sessions/{sid}/end", s.endSession)
	m.HandleFunc("GET /api/sessions", s.sessions)
	m.HandleFunc("GET /api/network", s.network)
	m.HandleFunc("POST /api/network/allow", s.allow)
	m.HandleFunc("POST /api/network/deny", s.deny)
	m.HandleFunc("GET /api/workspaces/{name}", s.workspace)
	m.HandleFunc("POST /api/workspaces/draft", s.draftWorkspace)
	m.HandleFunc("POST /api/workspaces", s.createWorkspace)
	m.HandleFunc("PUT /api/workspaces/{name}", s.saveWorkspace)
	m.HandleFunc("DELETE /api/workspaces/{name}", s.removeWorkspace)
	m.HandleFunc("GET /api/workspaces/{name}/notes", s.workspaceNotes)
	m.HandleFunc("PUT /api/workspaces/{name}/notes", s.saveWorkspaceNotes)
	m.HandleFunc("GET /api/workspaces/{name}/handoff", s.handoff)
	m.HandleFunc("PATCH /api/workspaces/{name}/handoff", s.setHandoffStatus)
	m.HandleFunc("DELETE /api/workspaces/{name}/handoff", s.deleteHandoff)
	m.HandleFunc("POST /api/workspaces/{name}/handoff/open", s.openHandoff)
	m.HandleFunc("GET /api/projects/{name}/workspace", s.projectWorkspace)
	m.HandleFunc("PUT /api/projects/{name}/workspace", s.setProjectWorkspace)
	m.HandleFunc("GET /api/leftovers", s.leftovers)
	m.HandleFunc("DELETE /api/leftovers/{name}", s.removeLeftover)
	m.HandleFunc("GET /api/images", s.images)
	m.HandleFunc("POST /api/images/prune", s.pruneImages)
	m.HandleFunc("POST /api/images/check", s.checkImage)
	m.HandleFunc("GET /api/settings", s.settings)
	m.HandleFunc("GET /api/about", s.about)
	m.HandleFunc("PATCH /api/projects/{name}/claude", s.saveClaude)
	m.HandleFunc("PATCH /api/settings/claude", s.saveDefaultLogin)
	m.HandleFunc("PATCH /api/settings/terminal", s.saveTerminal)
	m.HandleFunc("PATCH /api/settings/notifications", s.saveNotifications)
	m.HandleFunc("PUT /api/settings/image", s.saveDefaultImage)
	m.HandleFunc("PATCH /api/settings/updates", s.saveUpdates)
	m.HandleFunc("GET /api/update", s.updateStatus)
	m.HandleFunc("POST /api/update", s.runUpdate)
	m.HandleFunc("GET /api/secrets", s.secretsList)
	m.HandleFunc("PUT /api/secrets", s.setSecret)
	m.HandleFunc("DELETE /api/secrets/{name}", s.unsetSecret)
	m.HandleFunc("GET /api/presets", s.presetsList)
	m.HandleFunc("PUT /api/presets/{name}", s.savePreset)
	m.HandleFunc("DELETE /api/presets/{name}", s.deletePreset)
	m.Handle("GET /", s.static())
}

// static serves the front end; unknown paths fall back to index.html (client-side routes).
func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(s.ui, strings.TrimPrefix(r.URL.Path, "/")); err != nil || r.URL.Path == "/" {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// badInput is a request the API can't act on as sent (400).
type badInput string

func (b badInput) Error() string { return string(b) }

// apiError answers with {"error": "..."} and a status that fits the error.
func apiError(w http.ResponseWriter, err error) {
	code, msg := http.StatusInternalServerError, err.Error()
	var pr *config.Problems
	var bad badInput
	var busy errBusy
	var held *sandbox.ErrBusy
	var build *image.BuildError
	switch {
	case errors.Is(err, config.ErrNotFound):
		code = http.StatusNotFound
	case errors.As(err, &pr):
		code = http.StatusUnprocessableEntity
	case errors.As(err, &bad):
		code = http.StatusBadRequest
	case errors.Is(err, hostutil.ErrNoFolderPicker):
		code = http.StatusNotImplemented
	case errors.Is(err, sandbox.ErrSessionsRunning), errors.Is(err, sandbox.ErrConversationRunning):
		code = http.StatusConflict
	case errors.As(err, &busy), errors.As(err, &held):
		code = statusBusy
	case errors.As(err, &build):
		msg = "Setting up the sandbox failed. Details are on the project page." // its output is (BuildView)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badInput("the request body isn't valid JSON: " + err.Error())
	}
	return nil
}

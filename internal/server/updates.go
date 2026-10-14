package server

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/jailwick/jailwick/internal/update"
)

// UpdateView is what the app shows about new versions: the check's answer, and how this
// jailwick updates.
type UpdateView struct {
	update.Status
	Install update.Install `json:"install"`
	Off     bool           `json:"off"` // updates: off in config.yaml (the app checks only when asked)
}

// UpdateRun is the outcome of an update.
type UpdateRun struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// updater checks for and runs updates; one update at a time.
type updater struct {
	mu      sync.Mutex
	running bool
}

// Timeouts: a check asks GitHub; an update waits for Homebrew (which may update itself first).
const (
	checkTimeout  = 15 * time.Second
	updateTimeout = 15 * time.Minute
	// updateEvery is how often the running app looks again (the check itself asks at most daily).
	updateEvery = 6 * time.Hour
)

func (s *Server) checker() *update.Checker { return update.New(s.version) }

// updateStatus answers whether a newer version is out: from the daily check, or checking now with
// ?refresh=1. With updates off it only reports the last answer unless asked.
func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	off := false
	if g, err := s.env.Global(); err == nil {
		off = !g.UpdatesOn()
	}
	c := s.checker()
	var st update.Status
	switch {
	case r.URL.Query().Get("refresh") == "1":
		ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
		defer cancel()
		st = c.Check(ctx, 0)
	case off:
		st = c.Cached()
	default:
		ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
		defer cancel()
		st = c.Check(ctx, update.Daily)
	}
	writeJSON(w, UpdateView{Status: st, Install: update.Detect(), Off: off})
}

// runUpdate updates jailwick with Homebrew, the CLI and the app (those Homebrew has). Nothing
// restarts: the app says to reopen it, and sandboxes take the new helper at their next restart.
func (s *Server) runUpdate(w http.ResponseWriter, r *http.Request) {
	in := update.Detect()
	if !in.Homebrew() {
		apiError(w, badInput("jailwick wasn't installed with Homebrew: download the new version from its release page"))
		return
	}
	s.upd.mu.Lock()
	if s.upd.running {
		s.upd.mu.Unlock()
		apiError(w, errBusy{project: "jailwick", op: "updating"})
		return
	}
	s.upd.running = true
	s.upd.mu.Unlock()
	defer func() { s.upd.mu.Lock(); s.upd.running = false; s.upd.mu.Unlock() }()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), updateTimeout)
	defer cancel()
	var out bytes.Buffer
	run := UpdateRun{OK: true}
	if err := in.Upgrade(ctx, &out); err != nil {
		run.OK, run.Error = false, err.Error()
	}
	run.Output = out.String()
	writeJSON(w, run)
}

// saveUpdates turns the daily check on or off (config.yaml's updates).
func (s *Server) saveUpdates(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	g, err := s.env.Store.Global()
	if err != nil {
		apiError(w, err)
		return
	}
	g.Updates = &req.On
	if err := s.env.Store.SaveGlobal(g); err != nil {
		apiError(w, err)
		return
	}
	s.env.Reload()
	writeJSON(w, map[string]bool{"ok": true})
}

// watchUpdates checks for a new version while the app runs (the check asks GitHub at most once a
// day) and tells once per version, when updates and notifications are on.
func (s *Server) watchUpdates(ctx context.Context) {
	tick := time.NewTicker(updateEvery)
	defer tick.Stop()
	for {
		if g, err := s.env.Global(); err == nil && g.UpdatesOn() {
			c := s.checker()
			cctx, cancel := context.WithTimeout(ctx, checkTimeout)
			st := c.Check(cctx, update.Daily)
			cancel()
			if g.NotificationsOn() && c.ShouldNotify(st) {
				_ = s.send(Notification{Title: "jailwick " + st.Latest.Version + " is available",
					Body: "You have " + st.Current + ". About has what's new and how to update.", Route: "about"})
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

package server

import (
	"context"
	"net/http"
	"time"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/image"
)

// imageCheckTimeout bounds a check: pulling a large image the first time takes a while.
const imageCheckTimeout = 10 * time.Minute

// checkImage tries an image for jailwick ({ref, user?, home?, as_is?, platform?}): whether it is
// Debian or Ubuntu with glibc, its architecture, the sandbox user and home, and for one used as is
// Claude Code and git. {ok, findings[{name, ok, detail}]}; a 422 says why it couldn't be checked
// (no such image, a private registry, no shell).
func (s *Server) checkImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref      string `json:"ref"`
		User     string `json:"user"`
		Home     string `json:"home"`
		AsIs     bool   `json:"as_is"`
		Platform string `json:"platform"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	for _, err := range []error{config.CheckImageRef(req.Ref), config.CheckUserName(req.User), config.CheckHome(req.Home)} {
		if err != nil {
			apiError(w, badInput(err.Error()))
			return
		}
	}
	if req.Ref == "" {
		apiError(w, badInput("ref: name the image"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), imageCheckTimeout)
	defer cancel()
	m, err := s.env.Manager(ctx, "")
	if err != nil {
		apiError(w, err)
		return
	}
	found, err := m.CheckImage(ctx, image.CheckOptions{Ref: req.Ref, User: req.User, Home: req.Home, AsIs: req.AsIs, Platform: req.Platform})
	if err != nil {
		apiError(w, &config.Problems{What: "image check", List: []string{err.Error()}})
		return
	}
	ok := true
	for _, f := range found {
		ok = ok && f.OK
	}
	writeJSON(w, map[string]any{"ok": ok, "findings": found})
}

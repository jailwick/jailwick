package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/paths"
)

// BuildView is the image a project's sandbox is building: the step, how far it is, its log file
// and its latest output line. A failed build stays, with its last lines, until the next start.
type BuildView struct {
	Step   string   `json:"step"`
	Log    string   `json:"log"`
	Line   string   `json:"line"`
	Done   int      `json:"done,omitempty"`
	Total  int      `json:"total,omitempty"`
	Failed bool     `json:"failed,omitempty"`
	Tail   []string `json:"tail,omitempty"`
}

// builds tracks image builds by project, for the project page while a start or restart runs. A
// failed build is also written down (failedFile), so it is still shown after the app restarts or
// is reinstalled, until the next start.
type builds struct {
	mu sync.Mutex
	by map[string]BuildView
}

// watch returns a context for a start or restart of project (ctx: the op's own, see ops) that
// records the builds it runs. done takes the start's error: a failed build is kept for the page,
// anything else is forgotten.
func (b *builds) watch(ctx context.Context, project string) (watched context.Context, done func(error)) {
	b.mu.Lock()
	delete(b.by, project) // an earlier failure no longer applies
	b.mu.Unlock()
	_ = os.Remove(failedFile(project))
	ctx = image.Watch(ctx, func(e image.Event) {
		b.mu.Lock()
		defer b.mu.Unlock()
		v := b.by[project]
		if e.Step != "" {
			v = BuildView{Step: e.Step, Log: e.Log}
		}
		if e.Line != "" {
			v.Line, v.Done, v.Total = e.Line, e.Done, e.Total
		}
		b.by[project] = v
	})
	return ctx, func(err error) {
		b.mu.Lock()
		defer b.mu.Unlock()
		var be *image.BuildError
		if v, ok := b.by[project]; ok && errors.As(err, &be) {
			v.Failed, v.Tail = true, be.Tail
			b.by[project] = v
			if data, err := json.Marshal(v); err == nil {
				_ = os.MkdirAll(filepath.Dir(failedFile(project)), 0o755)
				_ = config.WriteFileAtomic(failedFile(project), data, 0o644)
			}
			return
		}
		delete(b.by, project)
	}
}

// failedFile keeps a project's last failed build (a BuildView) until its next start.
func failedFile(project string) string {
	return filepath.Join(paths.ProjectState(project), "build-failed.json")
}

// of returns the build running for project, if any.
func (b *builds) of(project string) *BuildView {
	b.mu.Lock()
	defer b.mu.Unlock()
	if v, ok := b.by[project]; ok {
		return &v
	}
	// One that failed before the app (re)started.
	var v BuildView
	if data, err := os.ReadFile(failedFile(project)); err == nil && json.Unmarshal(data, &v) == nil && v.Failed {
		b.by[project] = v
		return &v
	}
	return nil
}

// buildLogMax is the most of a build's output buildLog answers with at once: the end of it.
const buildLogMax = 512 << 10

// buildLog answers with the output of the build running for a project (or the failed one) from
// byte ?from= on, so the page can append what is new: {text, from, size}. When from is past the
// end (a new build's log) or too far back, the answer starts elsewhere and says where.
func (s *Server) buildLog(w http.ResponseWriter, r *http.Request) {
	v := s.builds.of(r.PathValue("name"))
	if v == nil || v.Log == "" {
		apiError(w, fmt.Errorf("no build: %w", config.ErrNotFound))
		return
	}
	f, err := os.Open(paths.Expand(v.Log))
	if err != nil {
		apiError(w, err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		apiError(w, err)
		return
	}
	size := st.Size()
	from, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	if err != nil || from < 0 || from > size {
		from = 0
	}
	from = max(from, size-buildLogMax)
	text := make([]byte, size-from)
	if _, err := f.ReadAt(text, from); err != nil && !errors.Is(err, io.EOF) {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]any{"text": string(text), "from": from, "size": size})
}

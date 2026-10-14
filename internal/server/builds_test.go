package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jailwick/jailwick/internal/image"
)

func TestBuildLog(t *testing.T) {
	s := testServer(t, "")
	if w := do(s, "GET", "/api/projects/api/build/log", nil); w.Code != 404 {
		t.Fatalf("no build: %d", w.Code)
	}
	log := filepath.Join(t.TempDir(), "build.log")
	os.WriteFile(log, []byte("#1 one\n"), 0o644)
	s.builds.by["api"] = BuildView{Step: "Building", Log: log}

	read := func(from string) (r struct {
		Text string
		From int64
		Size int64
	}) {
		w := do(s, "GET", "/api/projects/api/build/log?from="+from, nil)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		json.Unmarshal(w.Body.Bytes(), &r)
		return r
	}
	if r := read("0"); r.Text != "#1 one\n" || r.From != 0 || r.Size != 7 {
		t.Fatalf("whole: %+v", r)
	}
	os.WriteFile(log, []byte("#1 one\n#2 two\n"), 0o644)
	if r := read("7"); r.Text != "#2 two\n" || r.From != 7 || r.Size != 14 {
		t.Fatalf("what is new: %+v", r)
	}
	if r := read("99"); r.Text != "#1 one\n#2 two\n" || r.From != 0 {
		t.Fatalf("past the end (another log): %+v", r)
	}
}

// A failed build is still shown after the app restarts, until the next start.
func TestBuildFailureOutlivesTheApp(t *testing.T) {
	before := builds{by: map[string]BuildView{}}
	_, done := before.watch(context.Background(), "api")
	before.mu.Lock()
	before.by["api"] = BuildView{Step: "Building the sandbox image", Log: "~/x.log"}
	before.mu.Unlock()
	done(&image.BuildError{Image: "x", Err: errors.New("exit 100"), Tail: []string{"E: Unable to locate package nope"}})

	after := builds{by: map[string]BuildView{}} // the app started again
	v := after.of("api")
	if v == nil || !v.Failed || v.Tail[0] != "E: Unable to locate package nope" {
		t.Fatalf("%+v", v)
	}
	after.watch(context.Background(), "api") // the next start
	if v := (&builds{by: map[string]BuildView{}}).of("api"); v != nil {
		t.Fatalf("still failed after a new start: %+v", v)
	}
}

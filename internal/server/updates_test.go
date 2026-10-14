package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/update"
)

// The app's update status: from the daily check, or now with refresh; with updates off it asks
// only when told to.
func TestUpdateStatus(t *testing.T) {
	var calls atomic.Int32
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.0","html_url":"https://example.com/v0.3.0","published_at":"2026-10-20T10:00:00Z"}`))
	}))
	defer gh.Close()
	old := update.LatestURL
	update.LatestURL = gh.URL
	defer func() { update.LatestURL = old }()
	_ = os.Remove(filepath.Join(paths.StateDir(), "update.json"))

	s := testServer(t, "")
	s.version = "0.2.8"
	var v UpdateView
	if code := call(t, s, "PATCH", "/api/settings/updates", `{"on":false}`, nil); code != 200 {
		t.Fatalf("turning off: %d", code)
	}
	if code := call(t, s, "GET", "/api/update", ``, &v); code != 200 || !v.Off || v.Latest != nil || calls.Load() != 0 {
		t.Fatalf("off: no check, got %d %+v (%d calls)", code, v, calls.Load())
	}
	if code := call(t, s, "GET", "/api/update?refresh=1", ``, &v); code != 200 || !v.Available || v.Latest.Version != "0.3.0" || calls.Load() != 1 {
		t.Fatalf("refresh: %d %+v (%d calls)", code, v, calls.Load())
	}
	if code := call(t, s, "PATCH", "/api/settings/updates", `{"on":true}`, nil); code != 200 {
		t.Fatalf("turning on: %d", code)
	}
	if code := call(t, s, "GET", "/api/update", ``, &v); code != 200 || v.Off || !v.Available || calls.Load() != 1 {
		t.Fatalf("on: the day's answer, got %d %+v (%d calls)", code, v, calls.Load())
	}
}

// Updating needs Homebrew; a test binary isn't Homebrew's.
func TestRunUpdateWithoutHomebrew(t *testing.T) {
	s := testServer(t, "")
	if code := call(t, s, "POST", "/api/update", ``, nil); code != http.StatusBadRequest {
		t.Fatalf("got %d", code)
	}
}

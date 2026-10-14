package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.3.0", "0.2.8", true},
		{"v0.2.10", "0.2.9", true},
		{"1.0.0", "0.99.99", true},
		{"0.2.8", "0.2.8", false},
		{"0.2.7", "0.2.8", false},
		{"0.3.0", "0.3.0-rc1", true},
		{"0.3.0-rc1", "0.2.9", true},
		{"0.3.0-rc1", "0.3.0", false},
		{"0.3", "0.2.8", false},
		{"latest", "0.2.8", false},
		{"0.3.0", "dev", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// A check asks GitHub at most once per maxAge; a failure keeps the last known release.
func TestCheck(t *testing.T) {
	var calls atomic.Int32
	answer := `{"tag_name":"v0.3.0","html_url":"https://github.com/jailwick/jailwick/releases/tag/v0.3.0","body":"New things","published_at":"2026-10-20T10:00:00Z"}`
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("User-Agent") != "jailwick/0.2.8" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	old := LatestURL
	LatestURL = srv.URL
	defer func() { LatestURL = old }()

	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	c := &Checker{Current: "0.2.8", File: filepath.Join(t.TempDir(), "update.json"), Now: func() time.Time { return now }}
	ctx := context.Background()
	if st := c.Cached(); st.Latest != nil || st.Available {
		t.Fatalf("nothing checked yet: %+v", st)
	}
	st := c.Check(ctx, Daily)
	if !st.Available || st.Latest.Version != "0.3.0" || st.Latest.Notes != "New things" || st.Error != "" {
		t.Fatalf("status %+v", st)
	}
	c.Check(ctx, Daily)
	if calls.Load() != 1 {
		t.Fatalf("asked %d times within a day", calls.Load())
	}
	now = now.Add(Daily)
	status = http.StatusBadGateway
	st = c.Check(ctx, Daily)
	if calls.Load() != 2 || st.Error == "" || st.Latest == nil || !st.Available {
		t.Fatalf("a failed check keeps the last release: %+v (%d calls)", st, calls.Load())
	}
	c.Check(ctx, 0)
	if calls.Load() != 3 {
		t.Fatal("maxAge 0 always asks")
	}

	// Told once per version.
	if !c.ShouldNotify(st) || c.ShouldNotify(st) {
		t.Error("notify once")
	}
	if c.ShouldNotify(Status{Current: "0.3.0", Latest: st.Latest}) {
		t.Error("nothing to tell when up to date")
	}
}

func TestCheckRefusesNonVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"nightly"}`))
	}))
	defer srv.Close()
	old := LatestURL
	LatestURL = srv.URL
	defer func() { LatestURL = old }()
	c := &Checker{Current: "0.2.8", File: filepath.Join(t.TempDir(), "update.json")}
	if st := c.Check(context.Background(), 0); st.Latest != nil || st.Error == "" {
		t.Fatalf("status %+v", st)
	}
}

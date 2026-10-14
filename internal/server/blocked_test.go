package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/paths"
)

func TestBlockAndAllow(t *testing.T) {
	s := testServer(t, "")
	kept := func() []string {
		p, err := s.env.Store.Project("api")
		if err != nil {
			t.Fatal(err)
		}
		return p.Blocked
	}
	if w := post(s, "/api/projects/api/block", `{"hosts":["b.example","a.example","b.example"]}`); w.Code != 200 {
		t.Fatalf("block: %d %s", w.Code, w.Body)
	}
	if got := kept(); !slices.Equal(got, []string{"a.example", "b.example"}) {
		t.Fatalf("kept: %v", got)
	}
	if w := post(s, "/api/projects/api/block", `{"hosts":["not a host"]}`); w.Code != 422 {
		t.Fatalf("a bad host: %d", w.Code)
	}
	if w := post(s, "/api/projects/api/block", `{"hosts":["a.example"],"unblock":true}`); w.Code != 200 {
		t.Fatalf("unblock: %d %s", w.Code, w.Body)
	}
	if got := kept(); !slices.Equal(got, []string{"b.example"}) {
		t.Fatalf("after unblock: %v", got)
	}

	// Allowing several at once; an allowed host is no longer kept blocked.
	if w := post(s, "/api/network/allow", `{"domains":["b.example","c.example"],"project":"api"}`); w.Code != 200 {
		t.Fatalf("allow: %d %s", w.Code, w.Body)
	}
	p, _ := s.env.Store.Project("api")
	if !slices.Contains(p.Allow, "b.example") || !slices.Contains(p.Allow, "c.example") || len(p.Blocked) != 0 {
		t.Fatalf("allow %v, blocked %v", p.Allow, p.Blocked)
	}
}

// The list comes from the records: a sandbox restarted, stopped or reinstalled still lists what
// it was refused; discarding hides a host until it is refused again.
func TestBlockedFromRecords(t *testing.T) {
	s := testServer(t, "")
	dir := filepath.Join(paths.ProjectData("api"), "blocked")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "sess01.json"), []byte(`{"session":"sess01","worktree":"fix","name":"webhooks",
		"hosts":{"a.example":{"count":3,"first":"2026-10-14T10:00:00Z","last":"2026-10-14T11:00:00Z"},
		         "b.example":{"count":1,"first":"2026-10-14T10:00:00Z","last":"2026-10-14T10:00:00Z"}},
		"updated":"2026-10-14T11:00:00Z"}`), 0o644)
	get := func() BlockedView {
		w := do(s, "GET", "/api/projects/api/blocked", nil)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var v BlockedView
		json.Unmarshal(w.Body.Bytes(), &v)
		return v
	}
	v := get() // no runtime here: as after a stop
	if len(v.Hosts) != 2 || v.Hosts[0].Host != "a.example" || len(v.By["a.example"]) != 1 || v.By["a.example"][0].Name != "webhooks" {
		t.Fatalf("%+v", v)
	}
	if w := post(s, "/api/projects/api/discard", `{"hosts":[{"host":"a.example","last":"2026-10-14T11:00:00Z"}]}`); w.Code != 200 {
		t.Fatalf("discard: %d %s", w.Code, w.Body)
	}
	if v := get(); len(v.Hosts) != 1 || len(v.Discarded) != 1 || v.Discarded[0].Host != "a.example" {
		t.Fatalf("after a discard: %+v", v)
	}
	post(s, "/api/projects/api/discard", `{"hosts":[{"host":"a.example"}],"restore":true}`)
	if v := get(); len(v.Hosts) != 2 || len(v.Discarded) != 0 {
		t.Fatalf("restored: %+v", v)
	}
	post(s, "/api/projects/api/block", `{"hosts":["b.example"]}`)
	if v := get(); len(v.Hosts) != 1 || v.Kept[0] != "b.example" {
		t.Fatalf("kept blocked: %+v", v)
	}
}

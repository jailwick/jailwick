package server

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestChanges(t *testing.T) {
	prev := snapshot{
		sessions: map[string]sessionNow{
			"api//1": {project: "api", key: ":1", where: "api · main checkout", busy: true, appTells: true},
			"api//2": {where: "api · auth", busy: true},
			"web//3": {where: "web · main checkout", busy: false},
		},
		handoffs: map[string]handoffNow{"shop/to-web/1.md": {"shop", "to-web/1.md", "for web from api: old"}},
		blocked:  map[string][]string{"api": {"a.example"}},
	}
	cur := snapshot{
		sessions: map[string]sessionNow{
			"api//1": {project: "api", key: ":1", where: "api · main checkout", busy: false, appTells: true}, // done: waits for you
			// api//2 ended: nothing to say
			"web//3": {where: "web · main checkout", busy: false}, // was idle already
		},
		handoffs: map[string]handoffNow{
			"shop/to-web/1.md": {"shop", "to-web/1.md", "for web from api: old"},
			"shop/to-api/2.md": {"shop", "to-api/2.md", "for api from web: new"},
		},
		blocked: map[string][]string{"api": {"a.example", "b.example", "c.example", "d.example", "e.example"}},
	}
	got := changes(prev, cur, true)
	want := []Notification{
		// Each opens the page to act on it, with what it was about pointed out.
		{"Claude is waiting for you", "api · main checkout", "projects/api/sessions?session=%3A1"},
		{"New handoff request", "for api from web: new", "workspaces/shop/overview?handoff=to-api%2F2.md"},
		{"api: blocked by the allowlist", "b.example, c.example, d.example and 1 more. Allow them on the Network tab.",
			"projects/api/network?hosts=b.example%2Cc.example%2Cd.example%2Ce.example"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
	if len(changes(cur, cur, true)) != 0 {
		t.Fatal("nothing changed: no notification")
	}
}

func TestNotifyOutcome(t *testing.T) {
	s := testServer(t, "")
	var sent []string
	s.Notify = func(n Notification) error { sent = append(sent, n.Title+": "+n.Body+" → "+n.Route); return nil }
	s.notifyOutcome("api", "start", 2*time.Second, nil) // quick: the page showed it
	s.notifyOutcome("api", "start", time.Minute, nil)
	s.notifyOutcome("api", "start", time.Minute, errors.New("building failed (exit 1)\nlast lines…"))
	if !slices.Equal(sent, []string{
		"api is ready: The sandbox is up: open a session. → projects/api/sessions",
		"api: start failed: building failed (exit 1) → projects/api/overview?show=build",
	}) {
		t.Fatalf("%q", sent)
	}
}

func TestChangesFollowTheSettings(t *testing.T) {
	prev := snapshot{
		sessions: map[string]sessionNow{"api//1": {project: "api", where: "api · main checkout", busy: true}},
		blocked:  map[string][]string{},
	}
	cur := snapshot{
		sessions: map[string]sessionNow{"api//1": {project: "api", where: "api · main checkout", busy: false}}, // its terminal tells
		blocked:  map[string][]string{"api": {"a.example"}},
	}
	if got := changes(prev, cur, false); len(got) != 0 {
		t.Fatalf("its terminal tells, blocked hosts are off: %q", got)
	}
	if got := changes(prev, cur, true); len(got) != 1 || got[0].Route != "projects/api/network?hosts=a.example" {
		t.Fatalf("blocked hosts on: %q", got)
	}
}

// A session waiting is looked at every tick; handoffs and refused hosts less often, keeping the
// last answer in between (so nothing looks new, or gone, meanwhile).
func TestLookCadence(t *testing.T) {
	var hand, blocked int
	for n := range 12 {
		w := lookAt(n)
		if w.handoffs {
			hand++
		}
		if w.blocked {
			blocked++
		}
	}
	if hand != 6 || blocked != 2 {
		t.Fatalf("in a minute: %d handoff looks, %d blocked looks", hand, blocked)
	}
	s := testServer(t, "")
	prev := &snapshot{handoffs: map[string]handoffNow{"f": {text: "x"}}, blocked: map[string][]string{"api": {"a.example"}}}
	cur := s.look(context.Background(), prev, lookWhat{})
	if cur.handoffs["f"].text != "x" || len(cur.blocked["api"]) != 1 {
		t.Fatalf("kept from the last look: %+v", cur)
	}
	if first := s.look(context.Background(), nil, lookWhat{}); first.handoffs == nil || first.blocked == nil {
		t.Fatal("a first look reads everything")
	}
}

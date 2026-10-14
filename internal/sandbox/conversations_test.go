package sandbox

import (
	"errors"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

func TestRunningAndCheckResume(t *testing.T) {
	ss := []Session{
		{Session: agentapi.Session{PID: 7, ID: "a1", State: "busy"}},
		{Session: agentapi.Session{PID: 8, ID: "b2", Worktree: "feat"}},
		{Session: agentapi.Session{PID: 9}}, // no id reported
	}
	run := Running(ss)
	if len(run) != 2 || run["a1"].PID != 7 {
		t.Fatalf("running: %v", run)
	}
	err := CheckResume(run, "a1")
	if !errors.Is(err, ErrConversationRunning) || err.Error() != "the conversation is running in main checkout, busy: switch to its terminal, or end it before resuming it" {
		t.Errorf("a1: %v", err)
	}
	if err := CheckResume(run, "b2"); err == nil || !errors.Is(err, ErrConversationRunning) {
		t.Errorf("b2: %v", err)
	}
	if err := CheckResume(run, "c3"); err != nil {
		t.Errorf("c3 isn't running: %v", err)
	}
}

// "last" continues the newest conversation at the place that no session holds.
func TestLatest(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 10, 14, h, 0, 0, 0, time.UTC) }
	convs := []agentapi.Conversation{
		{ID: "old", Where: "main checkout", When: at(9)},
		{ID: "wt", Where: "wt feat", When: at(12)},
		{ID: "run", Where: "main checkout", When: at(11)},
		{ID: "gone", Where: "main checkout", When: at(10), Gone: true},
	}
	running := map[string]Session{"run": {}}
	c, ok, skipped := Latest(convs, "main checkout", running)
	if !ok || c.ID != "old" || !skipped {
		t.Errorf("main: %q %v %v", c.ID, ok, skipped)
	}
	if c, ok, skipped := Latest(convs, "wt feat", running); !ok || c.ID != "wt" || skipped {
		t.Errorf("wt: %q %v %v", c.ID, ok, skipped)
	}
	if _, ok, skipped := Latest(convs[2:3], "main checkout", running); ok || !skipped {
		t.Errorf("only a running one: %v %v", ok, skipped)
	}
	if WhereOf("") != "main checkout" || WhereOf("feat/x") != "wt feat-x" {
		t.Error("WhereOf")
	}
}

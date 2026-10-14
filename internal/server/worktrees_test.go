package server

import (
	"errors"
	"testing"
)

// A worktree made fine leaves no job; one that failed (or whose setup did) keeps its output
// until dismissed; one being made isn't made twice.
func TestWorktreeJobs(t *testing.T) {
	var j wtJobs
	a, ok := j.start("api", "feat-a", "main")
	if !ok {
		t.Fatal("start")
	}
	if _, again := j.start("api", "feat-a", "main"); again {
		t.Fatal("a worktree being made isn't made twice")
	}
	lockedWriter{&j.mu, a.log}.Write([]byte("setup: pnpm install\n"))
	if text, _ := j.log("api", "feat-a"); text != "setup: pnpm install\n" {
		t.Fatalf("output while it runs: %q", text)
	}
	b, _ := j.start("api", "feat-b", "main")
	c, _ := j.start("api", "feat-c", "main")
	j.end("api", a, false, nil)
	j.end("api", b, true, nil)
	j.end("api", c, false, errors.New("git worktree add: invalid reference: nope"))
	got := j.of("api")
	if len(got) != 2 || got[0].Name != "feat-b" || !got[0].SetupFailed || got[1].Error == "" || got[1].Running {
		t.Fatalf("jobs: %+v", got)
	}
	j.dismiss("api", "feat-c")
	if len(j.of("api")) != 1 || len(j.of("web")) != 0 {
		t.Fatalf("dismissed: %+v", j.of("api"))
	}
}

func TestWorktreeName(t *testing.T) {
	for _, ok := range []string{"feat-x", "fix/login", "v2.1"} {
		if checkWorktreeName(ok) != nil {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "-x", "a..b", "a b", "x/", "x.lock", "/x"} {
		if checkWorktreeName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

func TestRun(t *testing.T) {
	runDir = t.TempDir()
	c := &agentapi.Config{Dir: gitRepo(t), Run: "echo started in $PWD; sleep 30"}
	if err := StartRun(&agentapi.Config{Dir: c.Dir}, ""); err == nil {
		t.Fatal("no run command: a clear error")
	}
	if err := StartRun(c, ""); err != nil {
		t.Fatal(err)
	}
	if err := StartRun(c, ""); err != nil { // already running: left alone
		t.Fatal(err)
	}
	waitFor(t, func() bool { return strings.Contains(RunLog("").Text, "started in "+c.Dir) })
	runs := Runs(c)
	if len(runs) != 1 || !runs[0].Running || runs[0].Worktree != "" {
		t.Fatalf("%+v", runs)
	}
	if err := StopRun(""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !Runs(c)[0].Running })
	if !strings.HasPrefix(RunLog("").Text, "$ echo started") {
		t.Fatalf("the output stays: %q", RunLog("").Text)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for range 50 {
		if ok() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out")
}

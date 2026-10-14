package sandbox

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

func (f *fakeDriver) Stop(_ context.Context, container string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, container)
	return nil
}

func (f *fakeDriver) Unpause(_ context.Context, containers ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unpaused = append(f.unpaused, containers...)
	return nil
}

// withStates is a project's containers: the main one in state main, the proxy in state proxy.
func withStates(main, proxy string) []runtime.Container {
	cs := projectContainers()
	cs[0].State, cs[1].State = main, proxy
	for i := range cs {
		cs[i].Policy = "unless-stopped"
	}
	return cs
}

func TestProblems(t *testing.T) {
	if p := Problems(withStates("running", "running")); len(p) != 0 {
		t.Fatalf("all running: %+v", p)
	}
	if p := Problems(withStates("exited", "exited")); len(p) != 0 {
		t.Fatalf("a stopped sandbox has a state, not problems: %+v", p)
	}
	cs := withStates("running", "exited")
	cs[1].ExitCode = 1
	if p := Problems(cs); len(p) != 1 || p[0].Service != spec.ProxyService || p[0].What != "stopped (exit code 1)" {
		t.Fatalf("proxy down: %+v", p)
	}
	cs = withStates("running", "running")
	cs[1].Health = "unhealthy"
	if p := Problems(cs); len(p) != 1 || !strings.HasPrefix(p[0].What, "unhealthy") {
		t.Fatalf("unhealthy: %+v", p)
	}
	cs = withStates("running", "exited")
	cs[1].Policy = "no" // runs once: done, not down
	if p := Problems(cs); len(p) != 0 {
		t.Fatalf("one-shot: %+v", p)
	}
	cs = withStates("exited", "running")
	cs[0].Restarts, cs[0].ExitCode = crashLimit, 2
	if p := Problems(cs); len(p) != 1 || !p[0].Crashed || p[0].What != "stopped after 5 crashes (exit code 2)" {
		t.Fatalf("main crashed: %+v", p)
	}
	if h := HealthOf(withStates("paused", "paused")); h.Main != "paused" || len(h.Problems) != 0 {
		t.Fatalf("paused: %+v", h)
	}
	if h := HealthOf(withStates("running", "paused")); h.Main != "" || len(h.Problems) != 1 || h.Problems[0].What != "paused" {
		t.Fatalf("paused: %+v", h)
	}
	if h := HealthOf(withStates("restarting", "running")); h.Main != "restarting" {
		t.Fatalf("recovering: %+v", h)
	}
}

// A container the runtime keeps restarting is stopped at crashLimit, not before.
func TestStopCrashing(t *testing.T) {
	cs := withStates("running", "restarting")
	cs[1].Restarts = crashLimit - 1
	d := &fakeDriver{}
	m := testManager(t, d)
	if got := m.stopCrashing(context.Background(), cs); len(got) != 0 {
		t.Fatalf("below the limit: stopped %v", got)
	}
	cs[1].Restarts = crashLimit
	if got := m.stopCrashing(context.Background(), cs); !slices.Equal(got, []string{spec.ProxyService}) || len(d.stops) != 1 {
		t.Fatalf("at the limit: stopped %v (%v)", got, d.stops)
	}
}

// One start, restart or stop at a time on a project, whichever process; OpOf sees it.
func TestOpLock(t *testing.T) {
	if _, ok := OpOf("api"); ok {
		t.Fatal("an op before any")
	}
	release, err := lockOp("api", OpRestarting)
	if err != nil {
		t.Fatal(err)
	}
	if op, ok := OpOf("api"); !ok || op.Name != OpRestarting || op.PID == 0 {
		t.Fatalf("op %+v %v", op, ok)
	}
	if ops := OpsUnderWay(); ops["api"].Name != OpRestarting || len(ops) != 1 {
		t.Fatalf("under way: %+v", ops)
	}
	var busy *ErrBusy
	if _, err := lockOp("api", OpStopping); !errors.As(err, &busy) || busy.Op.Name != OpRestarting {
		t.Fatalf("a second op: %v", err)
	}
	other, err := lockOp("web", OpStarting) // another project is free
	if err != nil {
		t.Fatal(err)
	}
	other()
	release()
	if _, ok := OpOf("api"); ok {
		t.Fatal("an op after it ended")
	}
	if release, err = lockOp("api", OpStopping); err != nil {
		t.Fatalf("free again: %v", err)
	}
	release()
}

// Resume unpauses a paused sandbox: its containers carry on, nothing is recreated.
func TestResume(t *testing.T) {
	d := &fakeDriver{containers: withStates("paused", "paused")}
	if err := testManager(t, d).Resume(context.Background(), "api"); err != nil {
		t.Fatal(err)
	}
	if len(d.unpaused) != 2 || len(d.ups) != 0 {
		t.Fatalf("unpaused %v, ups %v", d.unpaused, d.ups)
	}
}

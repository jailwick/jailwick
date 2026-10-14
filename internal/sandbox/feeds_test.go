package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/runtime"
)

// Once streaming, a container's status comes from its watch: one exec, kept open, instead of one
// per ask. A line that isn't a status is ignored.
func TestFeedServesStatus(t *testing.T) {
	main := MainContainer("api")
	d := &fakeDriver{containers: projectContainers(), watches: map[string][]string{
		main: {`not json`, `{"sessions":[{"pid":7,"dir":"/x"}],"worktrees":[{"name":"feat"}]}`},
	}}
	m := &Manager{Driver: d}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Stream(ctx, nil)
	for range 3 {
		st, err := m.AgentStatus(ctx, "api")
		if err != nil || len(st.Sessions) != 1 || st.Sessions[0].PID != 7 || len(st.Worktrees) != 1 {
			t.Fatalf("status: %+v %v", st, err)
		}
		st.Sessions[0].PID = 99 // a caller's change stays the caller's
	}
	if d.count(main+" watch") != 1 || d.count(main+" status --json") != 0 {
		t.Fatalf("execs: %v", d.execs)
	}
}

// An agent without watch (a sandbox not restarted since): asked directly, and its feed not tried
// again for a while.
func TestFeedFallsBackToStatus(t *testing.T) {
	main := MainContainer("api")
	d := &fakeDriver{containers: projectContainers(), status: map[string]string{
		main: `{"sessions":[{"pid":8,"dir":"/x"}]}`,
	}}
	m := &Manager{Driver: d}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Stream(ctx, nil)
	for range 2 {
		st, err := m.AgentStatus(ctx, "api")
		if err != nil || len(st.Sessions) != 1 || st.Sessions[0].PID != 8 {
			t.Fatalf("status: %+v %v", st, err)
		}
	}
	if d.count(main+" watch") != 1 || d.count(main+" status --json") != 2 {
		t.Fatalf("execs: %v", d.execs)
	}
}

// A feed whose heartbeat stopped is started again.
func TestStaleFeedRestarts(t *testing.T) {
	main := MainContainer("api")
	d := &fakeDriver{containers: projectContainers(),
		watches: map[string][]string{main: {`{"sessions":[]}`}},
		status:  map[string]string{main: `{"sessions":[]}`}}
	m := &Manager{Driver: d}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Stream(ctx, nil)
	if _, err := m.AgentStatus(ctx, "api"); err != nil {
		t.Fatal(err)
	}
	f := m.feeds.get(m, main)
	f.mu.Lock()
	f.at = time.Now().Add(-2 * staleFeed)
	f.mu.Unlock()
	if _, err := m.AgentStatus(ctx, "api"); err != nil { // asked directly this once
		t.Fatal(err)
	}
	if _, err := m.AgentStatus(ctx, "api"); err != nil {
		t.Fatal(err)
	}
	if d.count(main+" watch") != 2 {
		t.Fatalf("restarted: %v", d.execs)
	}
}

// Without Stream (a one-shot command), status is asked directly.
func TestNoStreamAsksDirectly(t *testing.T) {
	main := MainContainer("api")
	d := &fakeDriver{containers: projectContainers(), status: map[string]string{main: `{"sessions":[]}`}}
	if _, err := (&Manager{Driver: d}).AgentStatus(context.Background(), "api"); err != nil {
		t.Fatal(err)
	}
	if d.count(main+" watch") != 0 || d.count(main+" status --json") != 1 {
		t.Fatalf("execs: %v", d.execs)
	}
}

// A feed nobody asks about stops; the next ask starts it again (it isn't taken for an agent
// without watch).
func TestIdleFeedStops(t *testing.T) {
	main := MainContainer("api")
	d := &fakeDriver{containers: projectContainers(), watches: map[string][]string{main: {`{"sessions":[]}`}}}
	m := &Manager{Driver: d}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Stream(ctx, nil)
	if _, err := m.AgentStatus(ctx, "api"); err != nil {
		t.Fatal(err)
	}
	f := m.feeds.get(m, main)
	f.mu.Lock()
	f.asked = time.Now().Add(-2 * idleFeed)
	f.mu.Unlock()
	stopWhenIdle(ctx, f.cancel, f.lastAsk, idleFeed)
	deadline := time.Now().Add(2 * time.Second)
	for {
		m.feeds.mu.Lock()
		_, still := m.feeds.by[main]
		_, noWatch := m.feeds.noWatch[main]
		m.feeds.mu.Unlock()
		if noWatch {
			t.Fatal("a stopped feed was taken for an agent without watch")
		}
		if !still {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the idle feed didn't stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := m.AgentStatus(ctx, "api"); err != nil || d.count(main+" watch") != 2 {
		t.Fatalf("started again: %v %v", err, d.execs)
	}
}

// While streaming, a session's internet is asked once and remembered, until the proxy's events
// say it may have changed or an open period ends.
func TestInternetRemembered(t *testing.T) {
	cs := append(projectContainers(), box("main01", "", "running", time.Now())...)
	px := proxyName("main01")
	d := &fakeDriver{containers: cs, status: map[string]string{px: "closed 0 100\n"}}
	m := &Manager{Driver: d}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Stream(ctx, nil)
	ss := []Session{{SID: "main01"}}
	ask := func() {
		t.Helper()
		if got := m.WithInternet(ctx, "api", ss); got[0].Internet == nil {
			t.Fatal("no internet state")
		}
	}
	asks := func() int { return d.count(px + " status") }

	ask()
	ask()
	if asks() != 1 {
		t.Fatalf("asked %d times", asks())
	}
	m.feeds.onContainerEvent(runtime.ContainerEvent{Name: px, Action: "health_status: healthy"})
	m.feeds.onContainerEvent(runtime.ContainerEvent{Name: px, Action: "exec_start: jailwick-proxy status"})
	ask()
	if asks() != 1 {
		t.Fatal("a health check or a status read forgot it")
	}
	m.feeds.onContainerEvent(runtime.ContainerEvent{Name: px, Action: "exec_start: jailwick-proxy open 0"}) // jw internet open, elsewhere
	ask()
	if asks() != 2 {
		t.Fatal("an open elsewhere wasn't seen")
	}
	m.feeds.keepInternet(px, proxy.Internet{Open: true, Until: time.Now().Add(-time.Second)}) // its period ended
	ask()
	if asks() != 3 {
		t.Fatal("an ended open period was kept")
	}
}

package sandbox

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
	"github.com/jailwick/jailwick/internal/untrusted"
)

// A long-running jailwick (the app) follows its sandboxes instead of asking them again and
// again: Stream turns on, for this Manager, the runtime's events (container listings, see
// runtime.Watcher) and one feed per container, a `jailwick-agent watch` that prints the
// container's status whenever it changes. A one-shot command never streams: it asks directly.

// Feed timings: how long a first ask waits for a feed's first status; a feed silent for longer
// than staleFeed (it sends at least every agent.WatchHeartbeat) is started again; a feed nobody
// asked about for idleFeed stops (the next ask starts it again), so a container nobody looks at
// costs nothing; an agent without watch is asked directly, and its feed tried again after
// noWatchFor.
const (
	firstStatus = 3 * time.Second
	staleFeed   = 90 * time.Second
	idleFeed    = 2 * time.Minute
	noWatchFor  = 5 * time.Minute
)

// Stream follows the runtime and the sandboxes until ctx ends; calling it again does nothing.
// on (when not nil) is told of every event of a jailwick container, after the Manager's own
// work: a container that keeps crashing is stopped (guardCrashes).
func (m *Manager) Stream(ctx context.Context, on func(runtime.ContainerEvent)) {
	m.streamOnce.Do(func() {
		m.feeds = &feeds{ctx: ctx, by: map[string]*feed{}, noWatch: map[string]time.Time{},
			internet: map[string]proxy.Internet{}}
		events := func(e runtime.ContainerEvent) {
			m.feeds.onContainerEvent(e)
			go m.guardCrashes(ctx, e)
			if on != nil {
				on(e)
			}
		}
		if w, ok := m.Driver.(runtime.Watcher); ok {
			go func() {
				if err := w.Watch(ctx, events); err != nil && m.Log != nil {
					m.Log("Following the runtime: %v; asking it directly instead", err)
				}
			}()
		}
	})
}

// feeds are the containers followed, by name.
type feeds struct {
	ctx      context.Context
	mu       sync.Mutex
	by       map[string]*feed
	noWatch  map[string]time.Time      // containers whose agent has no watch, until when
	internet map[string]proxy.Internet // session proxies' internet, by container (see internetFor)
}

// A session's internet (open or not, until when) changes only when jailwick opens or closes it:
// this process (OpenInternet, CloseInternet) or another one, which shows as an event of the
// proxy (an exec of jailwick-proxy, a restart). So it is asked once, then remembered until one of
// those, or until an open period ends.

// internetFor is a proxy's remembered internet; ok is false when there's none, or it may have
// changed (an open period passed its end).
func (fs *feeds) internetFor(proxyContainer string) (v proxy.Internet, ok bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	v, ok = fs.internet[proxyContainer]
	if ok && v.Open && !v.Until.IsZero() && !time.Now().Before(v.Until) {
		return proxy.Internet{}, false
	}
	return v, ok
}

func (fs *feeds) keepInternet(proxyContainer string, v proxy.Internet) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.internet[proxyContainer] = v
}

func (fs *feeds) forgetInternet(proxyContainer string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	delete(fs.internet, proxyContainer)
}

// onContainerEvent forgets a proxy's internet when something may have changed it: anything but
// a health check or a status read (jailwick's own).
func (fs *feeds) onContainerEvent(e runtime.ContainerEvent) {
	if strings.HasPrefix(e.Action, "health_status") || strings.Contains(e.Action, strings.Join(proxy.StatusCommand, " ")) {
		return
	}
	fs.forgetInternet(e.Name)
}

// feed is one container's `jailwick-agent watch`.
type feed struct {
	cancel context.CancelFunc
	ready  chan struct{} // closed at the first status, or when the feed ends without one
	once   sync.Once
	mu     sync.Mutex
	st     *agentapi.Status
	at     time.Time // the last status
	asked  time.Time // the last ask
}

func (f *feed) markReady() { f.once.Do(func() { close(f.ready) }) }

// status is a container's latest status from its feed, started if it isn't running; ok is false
// when the feed has none to give (no watch in its agent, not started yet, or gone): the caller
// asks directly.
func (fs *feeds) status(ctx context.Context, m *Manager, container string) (st *agentapi.Status, ok bool) {
	f := fs.get(m, container)
	if f == nil {
		return nil, false
	}
	select {
	case <-f.ready:
	case <-time.After(firstStatus):
		return nil, false
	case <-ctx.Done():
		return nil, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = time.Now()
	if f.st == nil {
		return nil, false
	}
	if time.Since(f.at) > staleFeed { // its heartbeat stopped: start it again next time
		fs.drop(container, f)
		return nil, false
	}
	return cloneStatus(f.st), true
}

// get returns the container's feed, starting one; nil while its agent is known to have no watch.
func (fs *feeds) get(m *Manager, container string) *feed {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if until, ok := fs.noWatch[container]; ok {
		if time.Now().Before(until) {
			return nil
		}
		delete(fs.noWatch, container)
	}
	if f := fs.by[container]; f != nil {
		return f
	}
	ctx, cancel := context.WithCancel(fs.ctx)
	f := &feed{cancel: cancel, ready: make(chan struct{}), asked: time.Now()}
	fs.by[container] = f
	go fs.run(ctx, m, container, f)
	go stopWhenIdle(ctx, cancel, f.lastAsk, idleFeed)
	return f
}

// run follows a container until its watch ends: the container stopped, the feed was dropped, or
// the agent has no watch (it ends at once, having sent nothing).
func (fs *feeds) run(ctx context.Context, m *Manager, container string, f *feed) {
	lines := &untrusted.LineWriter{Max: maxAgentOutput, Line: func(line []byte) {
		var st agentapi.Status
		if decodeAgent(line, &st) != nil {
			return
		}
		f.mu.Lock()
		f.st, f.at = &st, time.Now()
		f.mu.Unlock()
		f.markReady()
	}}
	_ = m.Driver.Exec(ctx, runtime.ExecOptions{
		Container: container, Cmd: []string{spec.AgentPath, "watch"}, Stdout: lines, Stderr: io.Discard,
	})
	f.mu.Lock()
	sent := f.st != nil
	f.mu.Unlock()
	f.markReady()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.by[container] == f {
		delete(fs.by, container)
	}
	if !sent && ctx.Err() == nil { // it ended by itself, having sent nothing: no watch there
		fs.noWatch[container] = time.Now().Add(noWatchFor)
	}
}

// lastAsk is when the feed was last asked about.
func (f *feed) lastAsk() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}

// stopWhenIdle calls stop once nobody asked (lastAsk) for idle, or returns when ctx ends: what
// follows a container (a feed, a log) costs nothing while nobody looks.
func stopWhenIdle(ctx context.Context, stop func(), lastAsk func() time.Time, idle time.Duration) {
	for {
		left := idle - time.Since(lastAsk())
		if left <= 0 {
			stop()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(left):
		}
	}
}

// drop ends a feed (f.mu held).
func (fs *feeds) drop(container string, f *feed) {
	f.cancel()
	fs.mu.Lock()
	if fs.by[container] == f {
		delete(fs.by, container)
	}
	fs.mu.Unlock()
}

// cloneStatus copies a status: callers may change what they get.
func cloneStatus(st *agentapi.Status) *agentapi.Status {
	c := *st
	c.Sessions = slices.Clone(st.Sessions)
	c.Worktrees = slices.Clone(st.Worktrees)
	c.Runs = slices.Clone(st.Runs)
	c.EmptyNodeModules = slices.Clone(st.EmptyNodeModules)
	return &c
}

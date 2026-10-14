package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jailwick/jailwick/internal/spec"
	"github.com/jailwick/jailwick/internal/untrusted"
)

// A long-running jailwick (the app) follows the runtime's events instead of asking it again and
// again: one `docker events` stream, and a listing of jailwick's containers only when one of them
// changed. While the stream runs, container listings and image lookups are answered from memory;
// when it stops, they ask the runtime again, as a one-shot command always does.

// ContainerEvent is a change of a jailwick container as the runtime reports it: its name and
// the action ("start", "die", "exec_start: jailwick-proxy open 0", "health_status: healthy").
type ContainerEvent struct {
	Name, Action string
	Project      string // the jailwick project it belongs to
	Service      string
}

// ErrNoEvents means the runtime gives no event stream: the driver keeps asking directly.
var ErrNoEvents = errors.New("the runtime gives no event stream")

// Event stream timings: a stream that ran this long worked (one that ends sooner failed); the
// pause that gathers a burst of events into one listing; the waits before starting again.
const (
	streamWorked = 5 * time.Second
	relistPause  = 200 * time.Millisecond
	retryFirst   = time.Second
	retryMost    = 30 * time.Second
	maxFailures  = 3
	maxEventLine = 1 << 20
)

// live is what the events stream keeps current.
type live struct {
	mu        sync.Mutex
	streaming bool              // an events stream runs
	gen       int               // counts streams: a listing from an earlier one is dropped
	valid     bool              // all is current
	all       []Container       // every jailwick container
	images    map[string]string // image ref -> ID ("" absent), while valid
	relist    chan struct{}     // asks for a listing
}

// Watch follows the runtime's events until ctx ends (see Watcher), telling on (when not nil) of
// every jailwick container's. It returns ErrNoEvents when the stream fails again and again
// without ever running.
func (d *cli) Watch(ctx context.Context, on func(ContainerEvent)) error {
	d.live.mu.Lock()
	d.live.relist = make(chan struct{}, 1)
	d.live.mu.Unlock()
	go d.relister(ctx)
	failures, wait := 0, retryFirst
	for {
		start := time.Now()
		d.follow(ctx, on)
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(start) >= streamWorked {
			failures, wait = 0, retryFirst
		} else if failures++; failures >= maxFailures {
			return ErrNoEvents
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		wait = min(wait*2, retryMost)
	}
}

// follow runs one events stream until it ends, keeping the listing current meanwhile.
func (d *cli) follow(ctx context.Context, on func(ContainerEvent)) {
	d.live.mu.Lock()
	d.live.streaming, d.live.gen = true, d.live.gen+1
	d.live.mu.Unlock()
	d.askRelist() // the listing the events go on from
	cmd := d.command(ctx, nil, "events", "--format", "{{json .}}", "--filter", "type=container", "--filter", "type=image")
	cmd.Stdout = &untrusted.LineWriter{Max: maxEventLine, Line: func(line []byte) { d.onEvent(line, on) }}
	_ = d.do(cmd)
	d.live.mu.Lock()
	d.live.streaming, d.live.valid, d.live.all, d.live.images = false, false, nil, nil
	d.live.mu.Unlock()
}

// onEvent acts on one event: a new listing for a jailwick container's change, forgotten image
// lookups for an image's; and tells on of a jailwick container's.
func (d *cli) onEvent(line []byte, on func(ContainerEvent)) {
	var e event
	if json.Unmarshal(line, &e) != nil {
		return
	}
	relist, images := classify(e)
	if images {
		d.live.mu.Lock()
		d.live.images = nil
		d.live.mu.Unlock()
	}
	if relist {
		d.askRelist()
	}
	if on != nil && e.Type == "container" && e.Actor.Attributes[spec.LabelProject] != "" {
		a := e.Actor.Attributes
		on(ContainerEvent{Name: a["name"], Action: e.action(), Project: a[spec.LabelProject], Service: a["com.docker.compose.service"]})
	}
}

func (d *cli) askRelist() {
	d.live.mu.Lock()
	ch := d.live.relist
	d.live.mu.Unlock()
	select {
	case ch <- struct{}{}:
	default: // one is already asked for: it will see this change too
	}
}

// relister lists jailwick's containers when asked, a burst of asks making one listing. A listing
// counts only if the stream it was asked for still runs.
func (d *cli) relister(ctx context.Context) {
	d.live.mu.Lock()
	ch := d.live.relist
	d.live.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(relistPause):
		}
		select { // the asks of the pause: this listing sees them
		case <-ch:
		default:
		}
		d.live.mu.Lock()
		gen := d.live.gen
		d.live.mu.Unlock()
		cs, err := d.listAll(ctx)
		d.live.mu.Lock()
		if d.live.streaming && d.live.gen == gen {
			d.live.all, d.live.valid = cs, err == nil
		}
		d.live.mu.Unlock()
	}
}

// cached is the current listing, a copy; ok is false when there's none (no stream, or it
// couldn't list).
func (d *cli) cached() (cs []Container, ok bool) {
	d.live.mu.Lock()
	defer d.live.mu.Unlock()
	if !d.live.valid {
		return nil, false
	}
	return slices.Clone(d.live.all), true
}

// cachedImage is an image lookup kept from earlier while the stream runs.
func (d *cli) cachedImage(ref string) (id string, ok bool) {
	d.live.mu.Lock()
	defer d.live.mu.Unlock()
	if !d.live.valid {
		return "", false
	}
	id, ok = d.live.images[ref]
	return id, ok
}

// keepImage remembers an image lookup while the stream runs.
func (d *cli) keepImage(ref, id string) {
	d.live.mu.Lock()
	defer d.live.mu.Unlock()
	if !d.live.valid {
		return
	}
	if d.live.images == nil {
		d.live.images = map[string]string{}
	}
	d.live.images[ref] = id
}

// event is the part of a runtime event jailwick reads.
type event struct {
	Type   string
	Action string
	Status string // older runtimes name the action here
	Actor  struct{ Attributes map[string]string }
}

// lifecycle are the container events that change a listing (state, health, name); execs and
// attaches don't, jailwick's own included.
var lifecycle = []string{"create", "start", "restart", "die", "stop", "kill", "destroy", "pause",
	"unpause", "rename", "update", "oom", "health_status"}

// action is the event's action (older runtimes name it Status).
func (e event) action() string {
	if e.Action != "" {
		return e.Action
	}
	return e.Status
}

// classify reads one event: relist for a change of a jailwick container, images for any change
// of an image.
func classify(e event) (relist, images bool) {
	switch e.Type {
	case "image":
		return false, true
	case "container":
		if e.Actor.Attributes[spec.LabelProject] == "" {
			return false, false
		}
		name, _, _ := strings.Cut(e.action(), ":") // "health_status: healthy"
		return slices.Contains(lifecycle, strings.TrimSpace(name)), false
	}
	return false, false
}

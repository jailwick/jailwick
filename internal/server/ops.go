package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/jailwick/jailwick/internal/sandbox"
	"net/http"
	"slices"
	"sync"
	"time"
)

// Operations a project's sandbox can be going through, as the page shows them.
const (
	opStarting   = sandbox.OpStarting
	opRestarting = sandbox.OpRestarting
	opStopping   = sandbox.OpStopping
	opBuilding   = "building" // its image, without starting it
)

// opOf is the op under way on project: the app's own, else one another process (jw) runs.
func (s *Server) opOf(project string) string {
	if op := s.ops.of(project); op != "" {
		return op
	}
	op, _ := sandbox.OpOf(project)
	return op.Name
}

// opsUnderWay is every op under way, by project: the app's own and other processes'.
func (s *Server) opsUnderWay() map[string]string {
	out := s.ops.running()
	for p, op := range sandbox.OpsUnderWay() {
		if _, ok := out[p]; !ok {
			out[p] = op.Name
		}
	}
	return out
}

// ops tracks the start, restart or stop running for each project. Each runs on a context of its
// own, not the request's: a reloaded page doesn't stop a long build, and the page still shows it.
// A second op on a project is refused meanwhile. Quitting the app cancels them all (cancelAll).
type ops struct {
	mu      sync.Mutex
	by      map[string]*op
	closing bool   // the app is quitting: no new op starts
	changed func() // told when an op begins or ends (nil: no one)
}

type op struct {
	name   string
	cancel context.CancelFunc
}

func newOps() ops { return ops{by: map[string]*op{}} }

// errBusy means the project is already going through an operation.
type errBusy struct{ project, op string }

func (e errBusy) Error() string { return fmt.Sprintf("%s is already %s", e.project, e.op) }

// errClosing refuses an op while the app quits.
var errClosing = errors.New("jailwick is quitting")

// begin records op for project and returns the context it runs on; end forgets it. It fails
// while another op runs on the project, or while the app quits.
func (o *ops) begin(project, name string) (ctx context.Context, end func(), err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closing {
		return nil, nil, errClosing
	}
	if cur, ok := o.by[project]; ok {
		return nil, nil, errBusy{project, cur.name}
	}
	ctx, cancel := context.WithCancel(context.Background())
	o.by[project] = &op{name: name, cancel: cancel}
	o.tell()
	return ctx, func() {
		cancel()
		o.mu.Lock()
		delete(o.by, project)
		o.mu.Unlock()
		o.tell()
	}, nil
}

// tell tells changed, outside the caller's lock (it may ask about the ops).
func (o *ops) tell() {
	if o.changed != nil {
		go o.changed()
	}
}

// of returns the op running for project ("" for none).
func (o *ops) of(project string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if cur, ok := o.by[project]; ok {
		return cur.name
	}
	return ""
}

// running returns the ops under way, by project.
func (o *ops) running() map[string]string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make(map[string]string, len(o.by))
	for p, cur := range o.by {
		out[p] = cur.name
	}
	return out
}

// cancelAll stops every op under way and refuses new ones; it returns the projects interrupted.
func (o *ops) cancelAll() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closing = true
	var out []string
	for p, cur := range o.by {
		cur.cancel()
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// wait returns once no op runs, or after timeout; it reports whether they all ended.
func (o *ops) wait(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		o.mu.Lock()
		n := len(o.by)
		o.mu.Unlock()
		if n == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// statusBusy is errBusy's HTTP status: 423, not 409 (which asks to end running sessions).
const statusBusy = http.StatusLocked

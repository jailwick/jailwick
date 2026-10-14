package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/sandbox"
)

// Actions on a project's sandbox, as the page's buttons and the menu bar run them.
const (
	ActStart   = "up"
	ActRestart = "restart"
	ActStop    = "down"
)

var actOps = map[string]string{ActStart: opStarting, ActRestart: opRestarting, ActStop: opStopping}

// Phase is the start, restart or stop under way on a project, in words ("Restarting…"), or "".
// Its containers come and go meanwhile: show this, not running or stopped.
func (s *Server) Phase(project string) string {
	return phaseWords[s.opOf(project)]
}

var phaseWords = map[string]string{opStarting: "Starting…", opRestarting: "Restarting…", opStopping: "Stopping…"}

// Act starts, restarts or stops a project's sandbox. It runs on an op of its own (see ops), so a
// reloaded page doesn't stop it and the app can cancel it when it quits; a second one on the same
// project is refused meanwhile. force restarts or stops even with sessions running (ends them).
// The state is nil for a stop.
func (s *Server) Act(project, action string, force bool) (*sandbox.State, error) {
	name, ok := actOps[action]
	if !ok {
		return nil, fmt.Errorf("unknown action %q", action)
	}
	ctx, end, err := s.ops.begin(project, name)
	if err != nil {
		return nil, err
	}
	defer end()
	m, err := s.env.Manager(ctx, project)
	if err != nil {
		return nil, err
	}
	began := time.Now()
	var st *sandbox.State
	switch action {
	case ActStart, ActRestart:
		watched, done := s.builds.watch(ctx, project)
		if action == ActStart {
			_, st, err = m.Up(watched, project)
		} else {
			st, err = m.Restart(watched, project, force)
		}
		done(err)
		s.notifyOutcome(project, map[string]string{ActStart: "start", ActRestart: "restart"}[action], time.Since(began), err)
	case ActStop:
		_, err = m.Remove(ctx, project, sandbox.RemoveOptions{Sessions: force})
	}
	return st, err
}

// Rebuild builds a project's images again (like jw image build): on an op of its own, with the
// page's build banner. fresh: without the build cache, pulling the bases again. The sandbox
// running keeps its image until a restart.
func (s *Server) Rebuild(project string, fresh bool) error {
	ctx, end, err := s.ops.begin(project, opBuilding)
	if err != nil {
		return err
	}
	defer end()
	m, err := s.env.Manager(ctx, project)
	if err != nil {
		return err
	}
	watched, done := s.builds.watch(ctx, project)
	_, err = m.Prepare(watched, project, image.Options{Rebuild: true, Fresh: fresh})
	done(err)
	return err
}

// Busy describes the starts, restarts and stops under way ("api is starting"), sorted.
func (s *Server) Busy() []string {
	var out []string
	for p, op := range s.ops.running() {
		out = append(out, p+" is "+op)
	}
	slices.Sort(out)
	return out
}

// Shutdown makes the app ready to quit: it cancels every start, restart and stop under way (so
// image builds and the runtime's commands stop rather than carry on), waits up to timeout for
// them to end, and stops each sandbox one of them left half done, keeping its data; the next
// Start brings it up cleanly. A sandbox with sessions running is left as it is.
func (s *Server) Shutdown(timeout time.Duration) {
	interrupted := s.ops.cancelAll()
	s.ops.wait(timeout)
	for _, p := range interrupted {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		if m, err := s.env.Manager(ctx, p); err == nil {
			if ss, err := m.Sessions(ctx, p); err == nil && len(ss) == 0 {
				_ = m.Down(ctx, p, false)
			}
		}
		cancel()
	}
}

// interrupted reports an error that only says the op was cancelled (the app quit).
func interrupted(err error) bool { return errors.Is(err, context.Canceled) }

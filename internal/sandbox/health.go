package sandbox

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

// A sandbox's containers restart by themselves when they end (their restart policy, which also
// brings them back after the runtime or the machine restarts). One that keeps crashing is
// stopped once the runtime restarted it crashLimit times, and shows as a problem with its logs a
// click away; starting it again (StartService) recreates it, which starts its count again.

// crashLimit is how many restarts make a container one that keeps crashing.
const crashLimit = 5

// Problem is a container of a sandbox that isn't doing its work: stopped, crashing, unhealthy
// or paused.
type Problem struct {
	Service  string `json:"service"`
	State    string `json:"state"`            // the runtime's: exited, restarting, paused, running, …
	Health   string `json:"health,omitempty"` // unhealthy
	Restarts int    `json:"restarts,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Crashed  bool   `json:"crashed,omitempty"` // stopped by jailwick after crashLimit restarts
	What     string `json:"what"`              // the problem in words
}

// describe is the problem in words.
func (p Problem) describe() string {
	switch {
	case p.Crashed:
		return fmt.Sprintf("stopped after %d crashes (exit code %d)", p.Restarts, p.ExitCode)
	case p.State == "restarting":
		return fmt.Sprintf("crashing: restarted %d times", p.Restarts)
	case p.State == "paused":
		return "paused"
	case p.Health == "unhealthy":
		return "unhealthy: its health check fails"
	case p.State == "exited" && p.ExitCode != 0:
		return fmt.Sprintf("stopped (exit code %d)", p.ExitCode)
	default:
		return p.State
	}
}

// crashed reports a container stopped after it kept crashing.
func crashed(c runtime.Container) bool {
	return c.Policy != "no" && c.Restarts >= crashLimit && !c.Running() && c.State != "restarting"
}

// isMain reports the sandbox's main container (under its old name too).
func isMain(c runtime.Container) bool {
	return c.Service == spec.SandboxService || c.Service == spec.LegacySandboxService
}

// Problems lists a project's containers that aren't doing their work (cs: its own, without its
// sessions'). The services count only while the main container is up (a stopped sandbox has no
// problems, only a state); the main container counts when it crashed. Containers that run once
// (restart policy no) never count.
func Problems(cs []runtime.Container) []Problem {
	up := slices.ContainsFunc(cs, func(c runtime.Container) bool {
		return isMain(c) && (c.Running() || c.State == "paused" || c.State == "restarting")
	})
	paused := mainState(cs) == "paused" // its services paused with it: that's the sandbox's state
	var out []Problem
	for _, c := range cs {
		if c.Policy == "no" || c.Labels[spec.LabelSession] != "" {
			continue
		}
		bad := crashed(c) || (up && !isMain(c) && (!c.Running() || c.Health == "unhealthy") && !(paused && c.State == "paused"))
		if !bad {
			continue
		}
		p := Problem{Service: c.Service, State: c.State, Health: c.Health, Restarts: c.Restarts,
			ExitCode: c.ExitCode, Crashed: crashed(c)}
		p.What = p.describe()
		out = append(out, p)
	}
	return out
}

// Health is what a project's sandbox shows besides running or stopped (see State).
type Health struct {
	Main     string    `json:"main,omitempty"` // paused or restarting
	Problems []Problem `json:"problems,omitempty"`
}

// HealthOf is the health of a project's containers (its sessions' are left out).
func HealthOf(cs []runtime.Container) Health {
	cs = projectOnly(cs)
	return Health{Main: mainState(cs), Problems: Problems(cs)}
}

// mainState is the main container's state when it isn't simply running or stopped: paused or
// restarting; else "".
func mainState(cs []runtime.Container) string {
	for _, c := range cs {
		if isMain(c) && c.Labels[spec.LabelSession] == "" && (c.State == "paused" || c.State == "restarting") {
			return c.State
		}
	}
	return ""
}

// stopCrashing stops the containers among cs that keep crashing (the runtime restarted them
// crashLimit times and they are down again): their restart policy then leaves them stopped. It
// returns the services stopped.
func (m *Manager) stopCrashing(ctx context.Context, cs []runtime.Container) []string {
	var stopped []string
	for _, c := range cs {
		if c.Policy == "no" || c.Restarts < crashLimit || c.State != "restarting" {
			continue
		}
		if err := m.Driver.Stop(ctx, c.Name); err == nil {
			stopped = append(stopped, c.Service)
			if m.Log != nil {
				m.Log("%s kept crashing (%d restarts): stopped it; its logs say why", c.Name, c.Restarts)
			}
		}
	}
	return stopped
}

// guardCrashes is the app's crash guard: a container of a project that ended (an event) is
// looked at a moment later, once the runtime has restarted it or not, and stopped when it keeps
// crashing.
func (m *Manager) guardCrashes(ctx context.Context, e runtime.ContainerEvent) {
	if e.Action != "die" || e.Project == "" {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Second):
	}
	cs, err := m.Driver.Containers(ctx, spec.Prefix+e.Project)
	if err != nil {
		return
	}
	m.stopCrashing(ctx, slices.DeleteFunc(cs, func(c runtime.Container) bool { return c.Name != e.Name }))
}

// Resume unpauses a paused sandbox: its containers and its sessions' carry on where they were.
func (m *Manager) Resume(ctx context.Context, name string) error {
	cs, err := m.Driver.Containers(ctx, spec.Prefix+name)
	if err != nil {
		return err
	}
	var paused []string
	for _, c := range cs {
		if c.State == "paused" {
			paused = append(paused, c.Name)
		}
	}
	return m.Driver.Unpause(ctx, paused...)
}

// StartService starts one service of a running sandbox again: a paused one resumes; any other is
// recreated alone (its data volumes stay), which also starts its crash count again.
func (m *Manager) StartService(ctx context.Context, name, service string) error {
	release, err := lockOp(name, OpStarting)
	if err != nil {
		return err
	}
	defer release()
	cs, err := m.Driver.Containers(ctx, spec.Prefix+name)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(projectOnly(cs), func(c runtime.Container) bool { return c.Service == service })
	if i < 0 {
		return fmt.Errorf("%s has no service %q", name, service)
	}
	if c := projectOnly(cs)[i]; c.State == "paused" {
		return m.Driver.Unpause(ctx, c.Name)
	}
	prep, err := m.Prepare(ctx, name, image.Options{})
	if err != nil {
		return err
	}
	if service == spec.ProxyService {
		m.recordProjectDenied(ctx, name) // the proxy's log goes with its container
	}
	return m.up(ctx, prep.Spec, runtime.UpOptions{Only: []string{service}, Out: m.Out})
}

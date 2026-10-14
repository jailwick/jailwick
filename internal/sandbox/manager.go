package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/secrets"
	"github.com/jailwick/jailwick/internal/spec"
	"github.com/jailwick/jailwick/internal/untrusted"
)

// Manager runs sandboxes on one runtime.
type Manager struct {
	Store    *config.Store
	Global   config.Global
	Secrets  secrets.Store
	Driver   runtime.Driver
	StateDir string
	Out      io.Writer                        // runtime output (builds, compose)
	Log      func(format string, args ...any) // progress lines

	streamOnce sync.Once
	feeds      *feeds // the containers followed, once Stream ran (feeds.go)
}

// ErrSessionsRunning means a restart would end running sessions.
var ErrSessionsRunning = errors.New("sessions are running")

// State is a sandbox as it is now.
type State struct {
	Name       string
	Containers []runtime.Container
	Running    bool     // the main container is up
	Health              // paused or restarting, containers not doing their work
	Pending    []string // why a restart is needed, one line per container; empty when up to date
	Changes    []Change // the same, item by item, with values before and after
}

// MainContainer returns the runtime name of a project's main container (service sandbox).
func MainContainer(project string) string {
	return runtime.ContainerName(spec.Prefix+project, spec.SandboxService)
}

// Up starts the sandbox. A running main container is never recreated; changes are reported as
// pending instead. The one exception is a main container from before 0.2.5 (service claude): it
// is replaced once, so commands find the container by its new name.
func (m *Manager) Up(ctx context.Context, name string) (*Prepared, *State, error) {
	release, err := lockOp(name, OpStarting)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	prep, err := m.Prepare(ctx, name, image.Options{})
	if err != nil {
		return nil, nil, err
	}
	before, err := m.Driver.Containers(ctx, prep.Spec.Name)
	if err != nil {
		return nil, nil, err
	}
	if mainState(projectOnly(before)) == "paused" { // carry on where it was, sessions and all
		if err := m.Resume(ctx, name); err != nil {
			return nil, nil, err
		}
		st, err := m.state(ctx, prep)
		return prep, st, err
	}
	opts, legacy := upOptions(before)
	if opts.Recreate && len(before) > 0 {
		m.recordProjectDenied(ctx, name) // the proxy's log goes with its container
	}
	if legacy && m.Log != nil {
		m.Log("The sandbox's main container is now called %q: replacing the old one (once)", spec.SandboxService)
	}
	opts.Out = m.Out
	running := !opts.Recreate
	if err := m.up(ctx, prep.Spec, opts); err != nil {
		return nil, nil, err
	}
	if !running {
		m.saveApplied(prep.Spec)
	}
	st, err := m.state(ctx, prep)
	return prep, st, err
}

// Restart applies pending changes by recreating containers. It refuses while sessions run
// unless force is set, which ends them.
func (m *Manager) Restart(ctx context.Context, name string, force bool) (*State, error) {
	release, err := lockOp(name, OpRestarting)
	if err != nil {
		return nil, err
	}
	defer release()
	if !force {
		if ss, err := m.Sessions(ctx, name); err == nil && len(ss) > 0 {
			return nil, fmt.Errorf("%w: %d in %s (end them, or use --force)", ErrSessionsRunning, len(ss), name)
		}
	}
	prep, err := m.Prepare(ctx, name, image.Options{})
	if err != nil {
		return nil, err
	}
	// Session sandboxes use the project's networks; they go first.
	if err := m.endAll(ctx, name); err != nil {
		return nil, err
	}
	m.recordProjectDenied(ctx, name) // the proxy's log goes with its container
	if err := m.up(ctx, prep.Spec, runtime.UpOptions{Recreate: true, RemoveOrphans: true, Out: m.Out}); err != nil {
		return nil, err
	}
	m.saveApplied(prep.Spec)
	return m.state(ctx, prep)
}

// Down stops the sandbox and its sessions; volumes also deletes its data, including its own
// Claude login (the app login is shared and stays).
func (m *Manager) Down(ctx context.Context, name string, volumes bool) error {
	if err := m.endAll(ctx, name); err != nil {
		return err
	}
	m.recordProjectDenied(ctx, name) // the proxy's log goes with its container
	if err := m.Driver.Down(ctx, spec.Prefix+name, volumes); err != nil {
		return err
	}
	if volumes {
		_ = os.Remove(m.appliedFile(name))
	}
	return nil
}

// Status returns the current state without building images or starting anything.
func (m *Manager) Status(ctx context.Context, name string) (*State, error) {
	prep, err := m.Prepare(ctx, name, image.Options{NoBuild: true})
	if err != nil {
		// Still show the containers when the config is broken.
		cs, cerr := m.Driver.Containers(ctx, spec.Prefix+name)
		if cerr != nil {
			return nil, err
		}
		cs = projectOnly(cs)
		return &State{Name: name, Containers: cs, Running: MainRunning(cs), Health: HealthOf(cs),
			Pending: []string{"configuration error: " + err.Error()}}, nil
	}
	return m.state(ctx, prep)
}

func (m *Manager) state(ctx context.Context, prep *Prepared) (*State, error) {
	cs, err := m.Driver.Containers(ctx, prep.Spec.Name)
	if err != nil {
		return nil, err
	}
	cs = projectOnly(cs)                // sessions have their own sandboxes and life cycle
	if m.stopCrashing(ctx, cs) != nil { // what a look finds crashing stops, as the app's guard does
		if cs, err = m.Driver.Containers(ctx, prep.Spec.Name); err != nil {
			return nil, err
		}
		cs = projectOnly(cs)
	}
	st := &State{Name: prep.Project.Name, Containers: cs, Running: MainRunning(cs), Health: HealthOf(cs)}
	if st.Running {
		st.Changes = m.pending(ctx, prep, cs)
		st.Pending = summarize(st.Changes)
	}
	return st, nil
}

// upOptions decides how Up starts a sandbox from the containers it has: a running main container
// is left alone, except one from before 0.2.5 (legacy), which is replaced along with its name.
func upOptions(before []runtime.Container) (opts runtime.UpOptions, legacy bool) {
	legacy = serviceRunning(before, spec.LegacySandboxService)
	running := MainRunning(before) && !legacy
	return runtime.UpOptions{Recreate: !running, RemoveOrphans: legacy}, legacy
}

// MainRunning reports whether the main container is up (under its old name too: a sandbox from
// before 0.2.5 runs until its next start, and shows the rename as pending).
func MainRunning(cs []runtime.Container) bool {
	return serviceRunning(cs, spec.SandboxService) || serviceRunning(cs, spec.LegacySandboxService)
}

func serviceRunning(cs []runtime.Container, service string) bool {
	for _, c := range cs {
		if c.Service == service && c.Running() {
			return true
		}
	}
	return false
}

// pending compares running containers with the current spec and images.
func (m *Manager) pending(ctx context.Context, prep *Prepared, cs []runtime.Container) []Change {
	var changes []Change
	applied := m.loadApplied(prep.Project.Name)
	byService := map[string]runtime.Container{}
	for _, c := range cs {
		byService[c.Service] = c
	}
	for _, want := range prep.Spec.Containers {
		if want.OneShot {
			continue
		}
		have, ok := byService[want.Name]
		if !ok {
			changes = append(changes, Change{Container: want.Name, What: whatContainer, After: "not created yet"})
			continue
		}
		if have.Labels[spec.LabelSpecHash] != want.Labels[spec.LabelSpecHash] {
			old, digests := applied.container(want.Name)
			changes = append(changes, containerChanges(old, digests, want, agentInfoAt)...)
		}
		if id, err := m.Driver.ImageID(ctx, want.Image); err == nil && id != "" && have.ImageID != "" && id != have.ImageID {
			changes = append(changes, Change{Container: want.Name, What: whatImageBuild, Item: want.Image,
				Before: shortID(have.ImageID), After: shortID(id)})
		}
	}
	for _, c := range cs {
		if prep.Spec.Container(c.Service) == nil && c.Service != "" {
			changes = append(changes, Change{Container: c.Service, What: whatContainer, Before: "running", After: "no longer used"})
		}
	}
	return changes
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// applied is the definition the containers were created from, as saved at the last start:
// secrets only as digests (by container), to tell which changed.
type applied struct {
	*spec.Sandbox
	SecretDigests map[string]map[string]string `json:",omitempty"`
}

// container returns the applied container and its secret digests (nil when not recorded).
func (a *applied) container(name string) (*spec.Container, map[string]string) {
	if a == nil || a.Sandbox == nil {
		return nil, nil
	}
	return a.Container(name), a.SecretDigests[name]
}

func (m *Manager) appliedFile(project string) string {
	return filepath.Join(paths.ProjectState(project), "applied.json")
}

// saveApplied records the definition the containers were created from; secrets only as digests.
func (m *Manager) saveApplied(sb *spec.Sandbox) {
	a := applied{Sandbox: sb, SecretDigests: map[string]map[string]string{}}
	for _, c := range sb.Containers {
		a.SecretDigests[c.Name] = secretDigests(c.Secrets)
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err == nil {
		_ = config.WriteFileAtomic(m.appliedFile(sb.Project), data, 0o644)
	}
}

func (m *Manager) loadApplied(project string) *applied {
	data, err := os.ReadFile(m.appliedFile(project))
	if err != nil {
		return nil
	}
	var a applied
	if json.Unmarshal(data, &a) != nil {
		return nil
	}
	return &a
}

// AgentStatus asks the agent in a project's own container for its worktrees (and sessions
// running there; see Sessions for all of them).
func (m *Manager) AgentStatus(ctx context.Context, name string) (*agentapi.Status, error) {
	return m.agentStatusIn(ctx, MainContainer(name))
}

func (m *Manager) agentStatusIn(ctx context.Context, container string) (*agentapi.Status, error) {
	if m.feeds != nil {
		if st, ok := m.feeds.status(ctx, m, container); ok {
			return st, nil
		}
	}
	var st agentapi.Status
	if err := m.AgentJSON(ctx, container, &st, "status", "--json"); err != nil {
		return nil, err
	}
	return &st, nil
}

// maxAgentOutput bounds what the host reads of an agent answer: processes in the sandbox can
// inflate it (sessions, names), and the app asks every few seconds.
const maxAgentOutput = 16 << 20

// ErrAgentAnswer is an agent that ran but answered with something unusable (too large, not JSON).
var ErrAgentAnswer = errors.New("unusable agent answer")

// ErrAgentOlder is a sandbox whose agent is older than this jailwick: a restart brings the new one.
var ErrAgentOlder = errors.New("the sandbox runs an older jailwick agent; restart it to use this")

// AgentJSON runs a jailwick-agent command in a container and decodes its JSON answer. The answer
// describes what runs in the sandbox, so it is untrusted data: its size is bounded and its text
// cleaned of control characters before anyone prints it.
func (m *Manager) AgentJSON(ctx context.Context, container string, v any, args ...string) error {
	out := &untrusted.LimitedBuffer{Max: maxAgentOutput}
	errOut := &untrusted.LimitedBuffer{Max: 64 << 10}
	err := m.Driver.Exec(ctx, runtime.ExecOptions{
		Container: container,
		Cmd:       append([]string{spec.AgentPath}, args...),
		Stdout:    out,
		Stderr:    errOut,
	})
	if errors.Is(err, untrusted.ErrLimit) {
		return fmt.Errorf("agent %s: %w: %w", args[0], ErrAgentAnswer, err)
	}
	if err != nil {
		if m.agentOlder(ctx, container) {
			return ErrAgentOlder
		}
		if msg := lastLine(untrusted.Text(string(errOut.Bytes()))); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	if err := decodeAgent(out.Bytes(), v); err != nil {
		if m.agentOlder(ctx, container) {
			return ErrAgentOlder
		}
		return fmt.Errorf("agent %s: %w: %w", args[0], ErrAgentAnswer, err)
	}
	return nil
}

// decodeAgent decodes an agent's JSON answer and cleans its text: it describes what runs in the
// sandbox, so it is untrusted data.
func decodeAgent(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	untrusted.Clean(v)
	return nil
}

// agentOlder reports whether a container's agent speaks an older contract than this jailwick.
// It is asked only after a command failed, so a working sandbox costs nothing. An agent from
// before contracts existed has no `version -json` and counts as older.
func (m *Manager) agentOlder(ctx context.Context, container string) bool {
	out := &untrusted.LimitedBuffer{Max: 4 << 10}
	err := m.Driver.Exec(ctx, runtime.ExecOptions{
		Container: container, Cmd: []string{spec.AgentPath, "version", "-json"}, Stdout: out, Stderr: io.Discard,
	})
	if err != nil {
		return false // not running, or gone: not a version question
	}
	var v agentapi.Version
	return json.Unmarshal(out.Bytes(), &v) != nil || v.Contract < agentapi.Contract
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	return s[strings.LastIndexByte(s, '\n')+1:]
}

// Running lists projects whose main container is up.
func (m *Manager) Running(ctx context.Context) ([]string, error) {
	by, err := m.RunningContainers(ctx)
	if err != nil {
		return nil, err
	}
	return slices.Sorted(maps.Keys(by)), nil
}

// RunningContainers returns the containers of each project whose main container is up (its
// sessions' included), from one listing of every sandbox. Only projects count: a sandbox whose
// project file is gone (removed, or never written) is no project, and its sessions are no one's.
func (m *Manager) RunningContainers(ctx context.Context) (map[string][]runtime.Container, error) {
	by, err := m.ProjectContainers(ctx)
	if err != nil {
		return nil, err
	}
	maps.DeleteFunc(by, func(_ string, cs []runtime.Container) bool { return !MainRunning(cs) })
	return by, nil
}

// ProjectContainers returns every project's containers, in any state (its sessions' included),
// from one listing of every sandbox.
func (m *Manager) ProjectContainers(ctx context.Context) (map[string][]runtime.Container, error) {
	all, err := m.Driver.AllContainers(ctx)
	if err != nil {
		return nil, err
	}
	by := byProject(all)
	maps.DeleteFunc(by, func(p string, _ []runtime.Container) bool { return !m.isProject(p) })
	return by, nil
}

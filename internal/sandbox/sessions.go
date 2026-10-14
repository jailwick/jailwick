package sandbox

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/hostterm"
	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

// Box is one session's sandbox: the container the session runs in and its own egress proxy.
type Box struct {
	Project  string
	ID       string
	Worktree string            // "" for the main checkout
	Session  runtime.Container // State is "" when the container is gone
	Proxy    runtime.Container
	Chrome   bool              // the session may use Claude in Chrome
	Private  bool              // kept out of messages between sessions
	Notify   string            // where the session tells it waits (notify.Channel); "" before 0.3.2
	Terminal hostterm.Terminal // the host terminal it runs in (zero: unknown)
}

// Running reports whether the session's container is up.
func (b Box) Running() bool { return b.Session.Running() }

// Session is one agent session, and the sandbox it runs in.
type Session struct {
	agentapi.Session
	// SID is the session's sandbox; "" for a session in the project's own container (started
	// before each session got one).
	SID string `json:"sid,omitempty"`
	// URL reaches the servers the session starts (http://host; append :port). "" without host
	// names.
	URL string `json:"url,omitempty"`
	// Internet is whether the session's proxy is open to everything (nil: no proxy of its own).
	Internet *proxy.Internet `json:"internet,omitempty"`
	// Chrome is whether the session may use Claude in Chrome (the user's own browser).
	Chrome bool `json:"chrome,omitempty"`
	// Private is whether the session is kept out of messages between sessions.
	Private bool `json:"private,omitempty"`
	// Terminal names the host terminal app it runs in, for Show terminal: "" when unknown (or
	// a name jailwick doesn't know: "terminal" then).
	Terminal string `json:"terminal,omitempty"`
}

// sessionGrace is how long a session sandbox may take to start before it counts as failed.
const sessionGrace = time.Minute

// groupBoxes collects the session containers among a project's containers.
func groupBoxes(project string, cs []runtime.Container) []Box {
	byID := map[string]*Box{}
	var order []string
	for _, c := range cs {
		id := c.Labels[spec.LabelSession]
		if id == "" {
			continue
		}
		b, ok := byID[id]
		if !ok {
			b = &Box{Project: project, ID: id}
			byID[id] = b
			order = append(order, id)
		}
		switch c.Service {
		case spec.SessionService:
			b.Session, b.Worktree = c, c.Labels[spec.LabelWorktree]
			b.Chrome = c.Labels[spec.LabelChrome] == "true"
			b.Private = c.Labels[spec.LabelPrivate] == "true"
			b.Terminal = hostterm.Decode(c.Labels[spec.LabelTerminal])
			b.Notify = c.Labels[spec.LabelNotify]
		case spec.ProxyService:
			b.Proxy = c
		}
	}
	out := make([]Box, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

// reapable reports whether a session sandbox is finished: its session ended (the container
// exits with it), or it never got going.
func reapable(b Box, now time.Time) bool {
	switch b.Session.State {
	case "running", "restarting", "paused":
		return false
	case "exited", "dead":
		return true
	}
	// Missing or still being created: finished only once it clearly isn't starting.
	created := b.Session.Created
	if created.IsZero() {
		created = b.Proxy.Created
	}
	return !created.IsZero() && now.Sub(created) > sessionGrace
}

// projectOnly drops the session containers from a project's containers.
func projectOnly(cs []runtime.Container) []runtime.Container {
	return slices.DeleteFunc(slices.Clone(cs), func(c runtime.Container) bool { return c.Labels[spec.LabelSession] != "" })
}

// Boxes lists a project's session sandboxes. Finished ones (a session that ended while the host
// wasn't there to clean up) are removed on the way.
func (m *Manager) Boxes(ctx context.Context, project string) ([]Box, error) {
	cs, err := m.Driver.Containers(ctx, spec.Prefix+project)
	if err != nil {
		return nil, err
	}
	return m.boxesOf(ctx, project, cs), nil
}

// boxesOf is Boxes for the project's containers, already listed.
func (m *Manager) boxesOf(ctx context.Context, project string, cs []runtime.Container) []Box {
	now := time.Now()
	var live []Box
	for _, b := range groupBoxes(project, cs) {
		if reapable(b, now) {
			_ = m.EndSession(ctx, project, b.ID)
			continue
		}
		live = append(live, b)
	}
	return live
}

// StartSession creates a session's sandbox next to the project's (which must be up). Its proxy
// has an allowlist of its own; o.Chrome adds Claude in Chrome's relay to it. A project container
// from before sessions could message each other (until its restart) has nothing to share: its
// sessions start private.
func (m *Manager) StartSession(ctx context.Context, prep *Prepared, worktree string, o spec.SessionOptions) (Box, error) {
	project := prep.Project.Name
	if _, err := m.Boxes(ctx, project); err != nil { // clears finished ones, freeing their networks
		return Box{}, err
	}
	if !o.Private && !m.sharesPeers(project) {
		o.Private = true
		if m.Log != nil {
			m.Log("This session is private: sessions of %s can message each other after its restart (jw restart)", project)
		}
	}
	id := newSessionID()
	allow, err := m.allowlist(prep.Project, o.Chrome)
	if err != nil {
		return Box{}, err
	}
	// Written before the proxy starts: a missing bind source is an error, never created.
	if err := (fileSet{allowFile(project, id): proxy.File(allow)}).write(); err != nil {
		return Box{}, err
	}
	// A missing bind source is an error, never created: the folder exists before the container.
	if err := os.MkdirAll(ClipboardDir(project, id), 0o755); err != nil {
		return Box{}, err
	}
	in := prep.Inputs
	in.HostName = sessionHost(project, id, prep.HostSuffix)
	in.AllowDir = allowDir(project, id)
	in.ClipboardDir = ClipboardDir(project, id)
	sb := spec.BuildSession(in, id, worktree, o)
	if err := m.up(ctx, sb, runtime.UpOptions{Recreate: true, Out: m.Out}); err != nil {
		_ = m.EndSession(context.WithoutCancel(ctx), project, id)
		return Box{}, err
	}
	return Box{
		Project: project, ID: id, Worktree: worktree, Chrome: o.Chrome, Private: o.Private,
		Session: runtime.Container{Name: runtime.ContainerName(sb.Name, spec.SessionService), State: "running"},
		Proxy:   runtime.Container{Name: runtime.ContainerName(sb.Name, spec.ProxyService), State: "running"},
	}, nil
}

// sharesPeers reports whether the project's running container was created with the PeersVolume
// (as last applied), so its sessions can join it.
func (m *Manager) sharesPeers(project string) bool {
	c, _ := m.loadApplied(project).container(spec.SandboxService)
	if c == nil {
		return false
	}
	return slices.ContainsFunc(c.Mounts, func(mt spec.Mount) bool { return mt.Source == spec.PeersVolume })
}

// EndSession removes a session's sandbox (its container and proxy; the shared data stays) and
// its own files.
func (m *Manager) EndSession(ctx context.Context, project, id string) error {
	m.recordSessionDenied(ctx, project, id) // its proxy's log goes with it
	if err := m.Driver.Down(ctx, spec.SessionName(project, id), false); err != nil {
		return err
	}
	return os.RemoveAll(sessionState(project, id))
}

// endAll removes every session sandbox of a project.
func (m *Manager) endAll(ctx context.Context, project string) error {
	cs, err := m.Driver.Containers(ctx, spec.Prefix+project)
	if err != nil {
		return err
	}
	var errs []error
	for _, b := range groupBoxes(project, cs) {
		if err := m.EndSession(ctx, project, b.ID); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("ending sessions: %v", errs)
	}
	return nil
}

// Sessions lists a project's agent sessions: one per session sandbox, plus any still running in
// the project's own container.
func (m *Manager) Sessions(ctx context.Context, project string) ([]Session, error) {
	cs, err := m.Driver.Containers(ctx, spec.Prefix+project)
	if err != nil {
		return nil, err
	}
	return m.SessionsOf(ctx, project, cs), nil
}

// SessionsOf is Sessions for the project's containers, already listed (RunningContainers).
func (m *Manager) SessionsOf(ctx context.Context, project string, cs []runtime.Container) []Session {
	return m.SessionsWhere(ctx, project, cs, func(Box) bool { return true }, true)
}

// SessionsWhere is SessionsOf for the session sandboxes keep accepts, and the project's own
// container when withMain: only those are asked.
func (m *Manager) SessionsWhere(ctx context.Context, project string, cs []runtime.Container, keep func(Box) bool, withMain bool) []Session {
	boxes := slices.DeleteFunc(m.boxesOf(ctx, project, cs), func(b Box) bool { return !keep(b) })
	suffix := m.hostSuffix(ctx)
	found := make([][]Session, len(boxes)+1)
	var wg sync.WaitGroup
	ask := func(i int, container string, b *Box, host string) {
		defer wg.Done()
		st, err := m.agentStatusIn(ctx, container)
		if err != nil {
			return
		}
		for _, s := range st.Sessions {
			ss := Session{Session: s}
			if b != nil {
				ss.SID, ss.Chrome, ss.Private = b.ID, b.Chrome, b.Private
				if b.Terminal.Known() {
					ss.Terminal = cmp.Or(b.Terminal.Name(), "terminal")
				}
			}
			if suffix != "" {
				ss.URL = "http://" + host
			}
			found[i] = append(found[i], ss)
		}
	}
	if withMain {
		wg.Add(1)
		go ask(len(boxes), MainContainer(project), nil, projectHost(project, suffix))
	}
	for i := range boxes {
		if b := &boxes[i]; b.Running() {
			wg.Add(1)
			go ask(i, b.Session.Name, b, sessionHost(project, b.ID, suffix))
		}
	}
	wg.Wait()
	var out []Session
	for _, f := range found {
		out = append(out, f...)
	}
	slices.SortFunc(out, func(a, b Session) int { return a.Started.Compare(b.Started) })
	return out
}

// WithInternet adds each session's internet (open or not, and until when) to sessions of a
// project, asking their proxies in parallel. Listing sessions doesn't, as most who list them
// (the notifier, counts) don't need it: only what shows it asks.
func (m *Manager) WithInternet(ctx context.Context, project string, ss []Session) []Session {
	out := slices.Clone(ss)
	var wg sync.WaitGroup
	for i := range out {
		if out[i].SID == "" {
			continue // in the project's own container: no proxy of its own
		}
		wg.Go(func() {
			proxyC := runtime.ContainerName(spec.SessionName(project, out[i].SID), spec.ProxyService)
			if v, err := m.internetOf(ctx, proxyC); err == nil {
				out[i].Internet = &v
			}
		})
	}
	wg.Wait()
	return out
}

// StopSessions asks every session of a project to end; each session's sandbox goes with it.
func (m *Manager) StopSessions(ctx context.Context, project string) error {
	boxes, err := m.Boxes(ctx, project)
	if err != nil {
		return err
	}
	containers := []string{MainContainer(project)}
	for _, b := range boxes {
		if b.Running() {
			containers = append(containers, b.Session.Name)
		}
	}
	var errs []error
	for _, c := range containers {
		err := m.Driver.Exec(ctx, runtime.ExecOptions{
			Container: c, Cmd: []string{spec.AgentPath, "stop"}, Stdout: io.Discard, Stderr: io.Discard,
		})
		if err != nil && c != MainContainer(project) { // the project's container may be down
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("stopping sessions: %v", errs)
	}
	return nil
}

// FindSession picks a running session sandbox of a project by its id, its worktree name, or
// "main" for the main checkout; with no reference, the only one there is.
func (m *Manager) FindSession(ctx context.Context, project, ref string) (Box, error) {
	boxes, err := m.Boxes(ctx, project)
	if err != nil {
		return Box{}, err
	}
	var match []Box
	for _, b := range boxes {
		if !b.Running() {
			continue
		}
		where := b.Worktree
		if where == "" {
			where = "main"
		}
		if ref == "" || ref == b.ID || ref == where {
			match = append(match, b)
		}
	}
	switch {
	case len(match) == 1:
		return match[0], nil
	case len(match) == 0 && ref == "":
		return Box{}, fmt.Errorf("%w: %s has no running session of its own", config.ErrNotFound, project)
	case len(match) == 0:
		return Box{}, fmt.Errorf("%w: no running session %q in %s", config.ErrNotFound, ref, project)
	}
	var ids []string
	for _, b := range match {
		ids = append(ids, b.ID)
	}
	return Box{}, fmt.Errorf("%w: %d sessions match; name one: %s", ErrAmbiguous, len(match), strings.Join(ids, ", "))
}

// ErrAmbiguous means a reference matches more than one session.
var ErrAmbiguous = errors.New("more than one session")

// Internet reads whether a session's proxy is open.
func (m *Manager) Internet(ctx context.Context, b Box) (proxy.Internet, error) {
	return m.internetOf(ctx, b.Proxy.Name)
}

// internetOf reads whether a proxy container lets its session reach everything: remembered while
// streaming (see feeds.internetFor), else asked.
func (m *Manager) internetOf(ctx context.Context, proxyContainer string) (proxy.Internet, error) {
	if m.feeds != nil {
		if v, ok := m.feeds.internetFor(proxyContainer); ok {
			return v, nil
		}
	}
	v, err := m.askInternet(ctx, proxyContainer)
	if err == nil && m.feeds != nil {
		m.feeds.keepInternet(proxyContainer, v)
	}
	return v, err
}

// askInternet asks a proxy container whether it lets its session reach everything.
func (m *Manager) askInternet(ctx context.Context, proxyContainer string) (proxy.Internet, error) {
	var out strings.Builder
	err := m.Driver.Exec(ctx, runtime.ExecOptions{
		Container: proxyContainer, Cmd: proxy.StatusCommand, Stdout: &out, Stderr: io.Discard,
	})
	if err != nil {
		return proxy.Internet{}, err
	}
	return proxy.ParseStatus(out.String(), time.Now())
}

// OpenInternet lets a session reach every destination (the host and private networks stay
// refused) for d, or until closed when d is 0. Opening again sets a new end.
func (m *Manager) OpenInternet(ctx context.Context, b Box, d time.Duration) (proxy.Internet, error) {
	if err := proxy.CheckDuration(d); err != nil {
		return proxy.Internet{}, err
	}
	var out strings.Builder
	err := m.Driver.Exec(ctx, runtime.ExecOptions{
		Container: b.Proxy.Name, Cmd: proxy.OpenCommand(d), Stdout: &out, Stderr: io.Discard,
	})
	if err != nil {
		return proxy.Internet{}, fmt.Errorf("opening the internet for session %s: %w", b.ID, err)
	}
	v, err := proxy.ParseStatus(out.String(), time.Now())
	if err == nil && m.feeds != nil {
		m.feeds.keepInternet(b.Proxy.Name, v)
	}
	return v, err
}

// CloseInternet puts a session back on the allowlist: its proxy restarts (the open state lives
// only in its memory), which also ends connections opened meanwhile.
func (m *Manager) CloseInternet(ctx context.Context, b Box) error {
	if err := m.Driver.Restart(ctx, b.Proxy.Name); err != nil {
		return fmt.Errorf("closing the internet for session %s: %w", b.ID, err)
	}
	if m.feeds != nil {
		m.feeds.forgetInternet(b.Proxy.Name)
	}
	return nil
}

// Port is a listening port in one of a project's containers.
type Port struct {
	Port    int    `json:"port"`
	Address string `json:"address"`
	PID     int    `json:"pid,omitempty"`
	Command string `json:"command,omitempty"`
	Where   string `json:"where"`         // main, a worktree, or forward
	SID     string `json:"sid,omitempty"` // the session whose container it's in; "" for the project's
	URL     string `json:"url,omitempty"` // http://host:port, when the host reaches it by name
}

// Ports lists the listening ports of a project's own container and of its sessions'.
func (m *Manager) Ports(ctx context.Context, project string) ([]Port, error) {
	boxes, err := m.Boxes(ctx, project)
	if err != nil {
		return nil, err
	}
	suffix := m.hostSuffix(ctx)
	type target struct{ container, sid, host string }
	targets := []target{{MainContainer(project), "", projectHost(project, suffix)}}
	for _, b := range boxes {
		if b.Running() {
			targets = append(targets, target{b.Session.Name, b.ID, sessionHost(project, b.ID, suffix)})
		}
	}
	found := make([][]Port, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var ps []agentapi.Port
			if m.AgentJSON(ctx, t.container, &ps, "ports") != nil {
				return
			}
			for _, p := range ps {
				pt := Port{Port: p.Port, Address: p.Address, PID: p.PID, Command: p.Command, Where: p.Where, SID: t.sid}
				if suffix != "" && p.Address != "127.0.0.1" && p.Where != "forward" {
					pt.URL = fmt.Sprintf("http://%s:%d", t.host, p.Port)
				}
				found[i] = append(found[i], pt)
			}
		}()
	}
	wg.Wait()
	out := []Port{}
	for _, f := range found {
		out = append(out, f...)
	}
	return out, nil
}

// ProxyLogs writes the logs of a project's proxies (its own and its sessions') to w. It fails
// only when none could be read.
func (m *Manager) ProxyLogs(ctx context.Context, project string, w io.Writer) error {
	sandboxes := []string{spec.Prefix + project}
	if boxes, err := m.Boxes(ctx, project); err == nil {
		for _, b := range boxes {
			sandboxes = append(sandboxes, spec.SessionName(project, b.ID))
		}
	}
	var errs []error
	for _, sb := range sandboxes {
		if err := m.Driver.Logs(ctx, sb, spec.ProxyService, runtime.LogOptions{Out: w}); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == len(sandboxes) {
		return errors.Join(errs...)
	}
	return nil
}

// Proxies are the running egress proxies of a project: its own and its sessions'.
func (m *Manager) Proxies(ctx context.Context, project string) ([]string, error) {
	cs, err := m.Driver.Containers(ctx, spec.Prefix+project)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range cs {
		if c.Service == spec.ProxyService && c.Running() {
			out = append(out, c.Name)
		}
	}
	return out, nil
}

// DevHost is where the host reaches the servers started for a worktree ("" for the main
// checkout): the newest running session there, else the project's own container.
func (m *Manager) DevHost(ctx context.Context, project, worktree string) string {
	suffix := m.hostSuffix(ctx)
	if suffix == "" {
		return "localhost"
	}
	if boxes, err := m.Boxes(ctx, project); err == nil {
		var newest *Box
		for i, b := range boxes {
			if b.Running() && b.Worktree == worktree && (newest == nil || b.Session.Created.After(newest.Session.Created)) {
				newest = &boxes[i]
			}
		}
		if newest != nil {
			return sessionHost(project, newest.ID, suffix)
		}
	}
	return projectHost(project, suffix)
}

// hostSuffix is the runtime's host name suffix, asked once.
func (m *Manager) hostSuffix(ctx context.Context) string {
	return m.Driver.Info(ctx).HostSuffix // the driver keeps it
}

// projectHost is the name the host reaches a project's own container by.
func projectHost(project, suffix string) string {
	if suffix == "" {
		return "localhost"
	}
	return spec.SandboxService + "." + spec.Prefix + project + suffix
}

// sessionHost is the name the host reaches a session's container by ("localhost" without host
// names): session.jailwick-<project>--<id><suffix>.
func sessionHost(project, id, suffix string) string {
	if suffix == "" {
		return "localhost"
	}
	return spec.SessionService + "." + spec.SessionName(project, id) + suffix
}

// newSessionID is a short random id: lowercase letters and digits, starting with a letter.
func newSessionID() string {
	const letters, alnum = "abcdefghijkmnpqrstuvwxyz", "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	out := []byte{letters[int(b[0])%len(letters)]}
	for _, x := range b[1:] {
		out = append(out, alnum[int(x)%len(alnum)])
	}
	return string(out)
}

// StopSession asks the agent session in sandbox id of a project to end, as its terminal would by
// closing; the jailwick process attached to it then removes the sandbox.
func (m *Manager) StopSession(ctx context.Context, project, id string) error {
	b, err := m.FindSession(ctx, project, id)
	if err != nil {
		return err
	}
	if b.ID != id {
		return fmt.Errorf("%w: no running session %q in %s", config.ErrNotFound, id, project)
	}
	return m.Driver.Exec(ctx, runtime.ExecOptions{
		Container: b.Session.Name, Cmd: []string{spec.AgentPath, "stop"}, Stdout: io.Discard, Stderr: io.Discard,
	})
}

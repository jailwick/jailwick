package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

// fakeDriver records calls; methods it doesn't implement panic (the embedded nil interface).
type fakeDriver struct {
	runtime.Driver
	mu          sync.Mutex
	containers  []runtime.Container
	status      map[string]string // container -> agent status JSON
	upErr       error
	ups, downs  []string
	volumes     []bool // Down's volumes, call by call
	execs       []string
	restarts    []string
	logs        map[string]string      // sandbox -> its proxy's log
	watches     map[string][]string    // container -> the lines its agent's watch sends
	listings    int                    // Containers calls
	logFeeds    map[string]chan string // sandbox -> lines its proxy logs while followed
	logReads    []runtime.LogOptions
	networks    []runtime.Network
	removedNets []string
	upFails     int      // the first upFails Ups fail with upErr (0: all of them)
	stops       []string // containers stopped
	unpaused    []string // containers unpaused
}

func (f *fakeDriver) Logs(ctx context.Context, sandbox, _ string, o runtime.LogOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	text, ok := f.logs[sandbox]
	if !ok {
		return fmt.Errorf("no logs for %s", sandbox)
	}
	f.logReads = append(f.logReads, o)
	if o.Follow {
		return f.followLog(ctx, sandbox, text, o)
	}
	if !o.Timestamps {
		_, err := io.WriteString(o.Out, text)
		return err
	}
	// As the runtime does: each line with its time (line i at fakeLogStart + i seconds), and with
	// Since only the lines from then on.
	for i, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		at := fakeLogStart.Add(time.Duration(i) * time.Second)
		if !o.Since.IsZero() && at.Before(o.Since) {
			continue
		}
		if _, err := io.WriteString(o.Out, at.Format(time.RFC3339Nano)+" "+line); err != nil {
			return err
		}
	}
	return nil
}

// followLog answers `logs -f` as the runtime does: the lines from Since on, then each line sent
// on the sandbox's logFeed, until ctx ends. Called with f.mu held; it lets go while it follows.
func (f *fakeDriver) followLog(ctx context.Context, sandbox, text string, o runtime.LogOptions) error {
	feed := f.logFeeds[sandbox]
	f.mu.Unlock()
	defer f.mu.Lock()
	n := 0
	for i, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		n = i + 1
		at := fakeLogStart.Add(time.Duration(i) * time.Second)
		if !o.Since.IsZero() && at.Before(o.Since) {
			continue
		}
		io.WriteString(o.Out, at.Format(time.RFC3339Nano)+" "+line)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line := <-feed:
			at := fakeLogStart.Add(time.Duration(n) * time.Second)
			n++
			io.WriteString(o.Out, at.Format(time.RFC3339Nano)+" "+line)
		}
	}
}

// fakeLogStart is the time of a fake log's first line.
var fakeLogStart = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

func (f *fakeDriver) Networks(context.Context) ([]runtime.Network, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.networks), nil
}

func (f *fakeDriver) RemoveNetwork(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removedNets = append(f.removedNets, name)
	f.networks = slices.DeleteFunc(f.networks, func(n runtime.Network) bool { return n.Name == name })
	return nil
}

func (f *fakeDriver) Restart(_ context.Context, container string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restarts = append(f.restarts, container)
	return nil
}

func (f *fakeDriver) Info(context.Context) runtime.Info {
	return runtime.Info{HostSuffix: ".orb.local"}
}

func (f *fakeDriver) Containers(context.Context, string) ([]runtime.Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listings++
	return f.containers, nil
}

func (f *fakeDriver) AllContainers(context.Context) ([]runtime.Container, error) {
	return f.containers, nil
}

func (f *fakeDriver) Up(_ context.Context, sb *spec.Sandbox, _ runtime.UpOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ups = append(f.ups, sb.Name)
	if f.upFails > 0 && len(f.ups) > f.upFails {
		return nil
	}
	return f.upErr
}

func (f *fakeDriver) Down(_ context.Context, sandbox string, volumes bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downs = append(f.downs, sandbox)
	f.volumes = append(f.volumes, volumes)
	return nil
}

func (f *fakeDriver) Exec(ctx context.Context, o runtime.ExecOptions) error {
	if len(o.Cmd) > 1 && o.Cmd[1] == "watch" {
		return f.watch(ctx, o)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execs = append(f.execs, o.Container+" "+strings.Join(o.Cmd[1:], " "))
	js, ok := f.status[o.Container]
	if !ok {
		return errors.New("not running")
	}
	if o.Stdout != nil {
		io.WriteString(o.Stdout, js)
	}
	return nil
}

// watch answers `jailwick-agent watch` as an agent does: its lines, then nothing until the feed
// ends. A container without lines has an agent without watch: it fails at once.
func (f *fakeDriver) watch(ctx context.Context, o runtime.ExecOptions) error {
	f.mu.Lock()
	f.execs = append(f.execs, o.Container+" watch")
	lines, ok := f.watches[o.Container]
	f.mu.Unlock()
	if !ok {
		return errors.New("unknown command: watch")
	}
	for _, l := range lines {
		io.WriteString(o.Stdout, l+"\n")
	}
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeDriver) count(exec string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range f.execs {
		if e == exec {
			n++
		}
	}
	return n
}

func box(id, wt, state string, created time.Time) []runtime.Container {
	name := spec.SessionName("api", id)
	return []runtime.Container{
		{Name: runtime.ContainerName(name, spec.SessionService), Service: spec.SessionService, State: state, Created: created,
			Labels: map[string]string{spec.LabelProject: "api", spec.LabelSession: id, spec.LabelWorktree: wt}},
		{Name: runtime.ContainerName(name, spec.ProxyService), Service: spec.ProxyService, State: "running", Created: created,
			Labels: map[string]string{spec.LabelProject: "api", spec.LabelSession: id}},
	}
}

func projectContainers() []runtime.Container {
	return []runtime.Container{
		{Name: MainContainer("api"), Service: spec.SandboxService, State: "running", Labels: map[string]string{spec.LabelProject: "api"}},
		{Name: "jailwick-api-egress-proxy-1", Service: spec.ProxyService, State: "running", Labels: map[string]string{spec.LabelProject: "api"}},
	}
}

func TestReapable(t *testing.T) {
	now := time.Now()
	young, old := now.Add(-10*time.Second), now.Add(-2*time.Minute)
	cases := []struct {
		b    Box
		want bool
	}{
		{Box{Session: runtime.Container{State: "running", Created: old}}, false},
		{Box{Session: runtime.Container{State: "exited", Created: young}}, true}, // the session ended
		{Box{Session: runtime.Container{State: "created", Created: young}}, false},
		{Box{Session: runtime.Container{State: "created", Created: old}}, true}, // never started
		{Box{Proxy: runtime.Container{State: "running", Created: young}}, false},
		{Box{Proxy: runtime.Container{State: "running", Created: old}}, true}, // the session container is gone
	}
	for i, c := range cases {
		if got := reapable(c.b, now); got != c.want {
			t.Errorf("case %d: reapable = %v, want %v", i, got, c.want)
		}
	}
}

func TestGroupBoxes(t *testing.T) {
	cs := append(projectContainers(), box("aaa111", "feat", "running", time.Now())...)
	bs := groupBoxes("api", cs)
	if len(bs) != 1 || bs[0].ID != "aaa111" || bs[0].Worktree != "feat" || bs[0].Proxy.Name == "" || !bs[0].Running() {
		t.Fatalf("boxes: %+v", bs)
	}
	if got := projectOnly(cs); len(got) != 2 || got[0].Service != spec.SandboxService {
		t.Fatalf("projectOnly kept session containers: %+v", got)
	}
}

func TestBoxesRemovesFinishedSessions(t *testing.T) {
	cs := projectContainers()
	cs = append(cs, box("live01", "", "running", time.Now())...)
	cs = append(cs, box("gone01", "", "exited", time.Now())...)
	d := &fakeDriver{containers: cs}
	m := &Manager{Driver: d}
	bs, err := m.Boxes(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].ID != "live01" {
		t.Fatalf("boxes: %+v", bs)
	}
	if !slices.Equal(d.downs, []string{spec.SessionName("api", "gone01")}) {
		t.Fatalf("downs: %v", d.downs)
	}
}

// One listing says which projects run, with their sessions' containers. A running sandbox
// without a project file is no project.
func TestRunningContainers(t *testing.T) {
	cs := append(projectContainers(), box("live01", "", "running", time.Now())...)
	cs = append(cs, runtime.Container{Name: MainContainer("web"), Service: spec.SandboxService, State: "exited",
		Labels: map[string]string{spec.LabelProject: "web"}})
	cs = append(cs, runtime.Container{Name: MainContainer("ghost"), Service: spec.SandboxService, State: "running",
		Labels: map[string]string{spec.LabelProject: "ghost"}})
	m := &Manager{Store: storeWith(t, "api", "web"), Driver: &fakeDriver{containers: cs}}
	by, err := m.RunningContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(by) != 1 || len(by["api"]) != 4 {
		t.Fatalf("running: %v", by)
	}
	if running, _ := m.Running(context.Background()); !slices.Equal(running, []string{"api"}) {
		t.Fatalf("running names: %v", running)
	}
}

// storeWith is a config store with a project file for each name.
func storeWith(t *testing.T, names ...string) *config.Store {
	t.Helper()
	store := &config.Store{Dir: t.TempDir()}
	for _, name := range names {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveProject(config.Project{Name: name, Dir: dir}); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestSessionsAggregates(t *testing.T) {
	cs := append(projectContainers(), box("live01", "feat", "running", time.Now())...)
	stat := func(pid int, started string) string {
		return fmt.Sprintf(`{"sessions":[{"pid":%d,"dir":"/x","started":"%s"}],"worktrees":[]}`, pid, started)
	}
	d := &fakeDriver{containers: cs, status: map[string]string{
		MainContainer("api"): stat(10, "2026-10-13T10:00:00Z"), // started before sessions had their own sandbox
		runtime.ContainerName(spec.SessionName("api", "live01"), spec.SessionService): stat(7, "2026-10-13T11:00:00Z"),
	}}
	ss, err := (&Manager{Driver: d}).Sessions(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 2 || ss[0].PID != 10 || ss[0].SID != "" || ss[1].PID != 7 || ss[1].SID != "live01" {
		t.Fatalf("sessions: %+v", ss)
	}
	if ss[0].URL != "http://sandbox.jailwick-api.orb.local" || ss[1].URL != "http://session.jailwick-api--live01.orb.local" {
		t.Fatalf("urls: %q %q", ss[0].URL, ss[1].URL)
	}
}

func prepared() *Prepared {
	p := config.Project{Name: "api", Dir: "/code/api"}.Resolved(config.Global{})
	return &Prepared{Project: p, HostSuffix: ".orb.local", Inputs: spec.Inputs{Project: p, Image: "img", ProxyImage: "proxy"}}
}

func TestStartSession(t *testing.T) {
	d := &fakeDriver{containers: projectContainers()}
	b, err := testManager(t, d).StartSession(context.Background(), prepared(), "feat", spec.SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[a-z][a-z2-9]{5}$`).MatchString(b.ID) || b.Worktree != "feat" {
		t.Fatalf("box: %+v", b)
	}
	if !slices.Equal(d.ups, []string{spec.SessionName("api", b.ID)}) {
		t.Fatalf("ups: %v", d.ups)
	}
	if want := runtime.ContainerName(spec.SessionName("api", b.ID), spec.SessionService); b.Session.Name != want {
		t.Fatalf("session container %q, want %q", b.Session.Name, want)
	}
}

// A session shares the project's peers only once its container was created with them (applied);
// before that restart, and when asked, it is private.
func TestStartSessionPrivate(t *testing.T) {
	ctx := context.Background()
	m := testManager(t, &fakeDriver{containers: projectContainers()})
	_ = os.Remove(m.appliedFile("api"))
	if b, err := m.StartSession(ctx, prepared(), "", spec.SessionOptions{}); err != nil || !b.Private {
		t.Fatalf("before the restart: private %v, %v", b.Private, err)
	}
	m.saveApplied(spec.Build(prepared().Inputs))
	if b, err := m.StartSession(ctx, prepared(), "", spec.SessionOptions{}); err != nil || b.Private {
		t.Fatalf("after: shared, got private %v, %v", b.Private, err)
	}
	if b, err := m.StartSession(ctx, prepared(), "", spec.SessionOptions{Private: true}); err != nil || !b.Private {
		t.Fatalf("asked: private %v, %v", b.Private, err)
	}
}

func TestStartSessionCleansUpAndExplains(t *testing.T) {
	d := &fakeDriver{containers: projectContainers(),
		upErr: errors.New("could not find an available, non-overlapping IPv4 address pool among the defaults")}
	_, err := testManager(t, d).StartSession(context.Background(), prepared(), "", spec.SessionOptions{})
	if !errors.Is(err, ErrNoNetworks) || !strings.Contains(err.Error(), PoolsDoc) || !strings.Contains(err.Error(), "docker network prune") {
		t.Fatalf("want an actionable error, got %v", err)
	}
	if len(d.downs) != 1 || !strings.HasPrefix(d.downs[0], "jailwick-api--") {
		t.Fatalf("a failed start must be removed: %v", d.downs)
	}
}

func TestDevHost(t *testing.T) {
	old, young := time.Now().Add(-time.Hour), time.Now().Add(-time.Minute)
	cs := append(projectContainers(), box("old001", "feat", "running", old)...)
	cs = append(cs, box("new001", "feat", "running", young)...)
	m := &Manager{Driver: &fakeDriver{containers: cs}}
	ctx := context.Background()
	if h := m.DevHost(ctx, "api", "feat"); h != "session.jailwick-api--new001.orb.local" {
		t.Fatalf("worktree with sessions: %q", h)
	}
	if h := m.DevHost(ctx, "api", ""); h != "sandbox.jailwick-api.orb.local" {
		t.Fatalf("no session there: %q", h)
	}
}

func TestSessionHost(t *testing.T) {
	if h := sessionHost("api", "k3x9q2", ".orb.local"); h != "session.jailwick-api--k3x9q2.orb.local" {
		t.Fatal(h)
	}
	if h := sessionHost("api", "k3x9q2", ""); h != "localhost" {
		t.Fatal(h)
	}
}

func proxyName(id string) string {
	return runtime.ContainerName(spec.SessionName("api", id), spec.ProxyService)
}

func TestFindSession(t *testing.T) {
	cs := projectContainers()
	cs = append(cs, box("main01", "", "running", time.Now())...)
	cs = append(cs, box("feat01", "feat", "running", time.Now())...)
	cs = append(cs, box("feat02", "feat", "running", time.Now())...)
	m := &Manager{Driver: &fakeDriver{containers: cs}}
	ctx := context.Background()
	for ref, want := range map[string]string{"main": "main01", "feat02": "feat02", "main01": "main01"} {
		if b, err := m.FindSession(ctx, "api", ref); err != nil || b.ID != want {
			t.Errorf("FindSession(%q) = %s, %v; want %s", ref, b.ID, err, want)
		}
	}
	if _, err := m.FindSession(ctx, "api", "feat"); !errors.Is(err, ErrAmbiguous) {
		t.Errorf("two sessions in feat: %v", err)
	}
	if _, err := m.FindSession(ctx, "api", ""); !errors.Is(err, ErrAmbiguous) {
		t.Errorf("no reference with three sessions: %v", err)
	}
	if _, err := m.FindSession(ctx, "api", "nope"); !errors.Is(err, config.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

func TestOpenAndCloseInternet(t *testing.T) {
	cs := append(projectContainers(), box("main01", "", "running", time.Now())...)
	d := &fakeDriver{containers: cs, status: map[string]string{proxyName("main01"): "open 0 100\n"}}
	m := &Manager{Driver: d}
	ctx := context.Background()
	b, err := m.FindSession(ctx, "api", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenInternet(ctx, b, 10*time.Second); err == nil || len(d.execs) != 0 {
		t.Fatalf("a bad duration must fail before reaching the proxy: %v %v", err, d.execs)
	}
	in, err := m.OpenInternet(ctx, b, 0)
	if err != nil || !in.Open || !in.Until.IsZero() {
		t.Fatalf("open until closed: %+v %v", in, err)
	}
	if want := proxyName("main01") + " open 0"; len(d.execs) != 1 || d.execs[0] != want {
		t.Fatalf("execs: %v, want %q", d.execs, want)
	}
	if err := m.CloseInternet(ctx, b); err != nil || !slices.Equal(d.restarts, []string{proxyName("main01")}) {
		t.Fatalf("close restarts the session's proxy, nothing else: %v %v", err, d.restarts)
	}
}

// Listing sessions doesn't ask their proxies; WithInternet does, for those with a proxy.
func TestSessionsAndInternet(t *testing.T) {
	cs := append(projectContainers(), box("main01", "", "running", time.Now())...)
	d := &fakeDriver{containers: cs, status: map[string]string{
		runtime.ContainerName(spec.SessionName("api", "main01"), spec.SessionService): `{"sessions":[{"pid":7,"dir":"/x","started":"2026-10-13T11:00:00Z"}]}`,
		proxyName("main01"): "closed 0 100\n",
	}}
	m := &Manager{Driver: d}
	ctx := context.Background()
	ss, err := m.Sessions(ctx, "api")
	if err != nil || len(ss) != 1 || ss[0].Internet != nil {
		t.Fatalf("sessions: %+v %v", ss, err)
	}
	if slices.Contains(d.execs, proxyName("main01")+" status") {
		t.Fatalf("a proxy asked: %v", d.execs)
	}
	ss = append(ss, Session{}) // one in the project's own container: no proxy
	with := m.WithInternet(ctx, "api", ss)
	if with[0].Internet == nil || with[0].Internet.Open || with[1].Internet != nil || ss[0].Internet != nil {
		t.Fatalf("with internet: %+v (the given ones unchanged: %+v)", with, ss)
	}
}

// SessionsWhere asks only the session sandboxes kept (by their notification label), and the
// project's own container only when asked to.
func TestSessionsWhere(t *testing.T) {
	cs := append(projectContainers(), box("app001", "", "running", time.Now())...)
	cs = append(cs, box("kit001", "", "running", time.Now())...)
	for i := range cs {
		switch cs[i].Labels[spec.LabelSession] {
		case "kit001":
			cs[i].Labels[spec.LabelNotify] = "kitty"
		case "app001":
			cs[i].Labels[spec.LabelNotify] = "app"
		}
	}
	session := func(id string) string { return runtime.ContainerName(spec.SessionName("api", id), spec.SessionService) }
	d := &fakeDriver{containers: cs, status: map[string]string{
		session("app001"): `{"sessions":[{"pid":7}]}`, session("kit001"): `{"sessions":[{"pid":8}]}`,
		MainContainer("api"): `{"sessions":[{"pid":9}]}`,
	}}
	m := &Manager{Driver: d}
	ss := m.SessionsWhere(context.Background(), "api", cs, func(b Box) bool { return b.Notify == "app" }, false)
	if len(ss) != 1 || ss[0].PID != 7 {
		t.Fatalf("sessions: %+v", ss)
	}
	if d.count(session("kit001")+" status --json") != 0 || d.count(MainContainer("api")+" status --json") != 0 {
		t.Fatalf("asked: %v", d.execs)
	}
}

// The terminal a session was started in comes back from its container's label, as a record of
// the host's; one it can't read is unknown, not an error.
func TestBoxTerminal(t *testing.T) {
	cs := box("aaa111", "", "running", time.Now())
	cs[0].Labels[spec.LabelTerminal] = "app=iTerm.app&tty=%2Fdev%2Fttys003"
	cs = append(cs, box("bbb222", "", "running", time.Now())...)
	cs[2].Labels[spec.LabelTerminal] = "tty=nonsense"
	bs := groupBoxes("api", cs)
	if bs[0].Terminal.Name() != "iTerm2" || bs[0].Terminal.TTY != "/dev/ttys003" || bs[1].Terminal.Known() {
		t.Fatalf("terminals: %+v, %+v", bs[0].Terminal, bs[1].Terminal)
	}
}

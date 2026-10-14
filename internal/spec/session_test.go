package spec

import (
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
)

func TestSessionName(t *testing.T) {
	n := SessionName("my-api", "k3x9q2")
	if n != "jailwick-my-api--k3x9q2" {
		t.Fatal(n)
	}
	if p, id, ok := ParseSessionName(n); !ok || p != "my-api" || id != "k3x9q2" {
		t.Fatalf("parse: %q %q %v", p, id, ok)
	}
	for _, name := range []string{"jailwick-my-api", "other--x"} {
		if _, _, ok := ParseSessionName(name); ok {
			t.Errorf("%q isn't a session sandbox", name)
		}
	}
}

// What makes a session isolated: its proxy is reachable only from its own container, and
// nothing of the project's services is duplicated.
func TestBuildSessionIsolation(t *testing.T) {
	p := config.Project{Name: "api", Dir: "/code/api", Services: []string{"postgres"}}.Resolved(config.Global{})
	sb := BuildSession(Inputs{Project: p, Image: "img", ProxyImage: "proxy"}, "k3x9q2", "", SessionOptions{})
	if len(sb.Containers) != 3 {
		t.Fatalf("a session has its container, its volume preparation and its proxy only: %d containers", len(sb.Containers))
	}
	s, px, vi := sb.Container(SessionService), sb.Container(ProxyService), sb.Container(VolumeInit)
	if vi == nil || !vi.NoNetwork || !vi.OneShot || vi.Labels[LabelSession] != "k3x9q2" {
		t.Errorf("volume init: %+v", vi)
	}
	if !slices.Equal(s.Networks, []string{SandboxNetwork, ProxyNetwork}) {
		t.Errorf("session networks: %v", s.Networks)
	}
	if slices.Contains(px.Networks, SandboxNetwork) || !slices.Contains(px.Networks, ProxyNetwork) {
		t.Errorf("the proxy must not be on the shared network: %v", px.Networks)
	}
	for _, n := range sb.Networks {
		if n.Name == ProxyNetwork && (n.External || !n.Internal) {
			t.Errorf("the session's proxy network is its own and internal: %+v", n)
		}
	}
	if !s.Ephemeral || px.Ephemeral {
		t.Errorf("the session ends with its work; its proxy restarts (that is how it closes)")
	}
	if s.Labels[LabelSession] != "k3x9q2" || px.Labels[LabelSession] != "k3x9q2" || s.Labels[LabelProject] != "api" {
		t.Errorf("labels: %v / %v", s.Labels, px.Labels)
	}
}

func TestBuildSessionChromeAndVolumes(t *testing.T) {
	p := config.Project{Name: "api", Dir: "/code/api"}.Resolved(config.Global{})
	in := Inputs{Project: p, Image: "img", ProxyImage: "proxy"}
	if sb := BuildSession(in, "k3x9q2", "", SessionOptions{}); sb.Container(SessionService).Labels[LabelChrome] != "" {
		t.Error("chrome off: no label")
	}
	sb := BuildSession(in, "k3x9q2", "", SessionOptions{Chrome: true})
	if sb.Container(SessionService).Labels[LabelChrome] != "true" {
		t.Error("chrome on: labelled")
	}
	// claude-config is mounted once per subpath, but it is one volume of the parent; the login
	// volume is the app's, by its own name.
	for i, v := range sb.Volumes {
		if slices.Contains(sb.Volumes[:i], v) || v == LoginVolume {
			t.Errorf("volumes: %v", sb.Volumes)
		}
	}
	if !slices.Equal(sb.AppVolumes, []string{LoginVolume}) {
		t.Errorf("app volumes: %v", sb.AppVolumes)
	}
}

// A session joins the project container's PID namespace and shares its PeersVolume; a private
// one keeps both to itself.
func TestBuildSessionPrivate(t *testing.T) {
	p := config.Project{Name: "api", Dir: "/code/api"}.Resolved(config.Global{})
	in := Inputs{Project: p, Image: "img", ProxyImage: "proxy"}
	peers := func(c *Container) int {
		n := 0
		for _, m := range c.Mounts {
			if m.Source == PeersVolume {
				n++
			}
		}
		return n
	}

	c := BuildSession(in, "k3x9q2", "", SessionOptions{}).Container(SessionService)
	if c.SharePIDs != SandboxService || peers(c) != 2 || len(c.Tmpfs) != 0 || c.Env["TINI_SUBREAPER"] == "" {
		t.Errorf("shared: pid %q, peer mounts %d, tmpfs %v, env %v", c.SharePIDs, peers(c), c.Tmpfs, c.Env["TINI_SUBREAPER"])
	}
	if c.Labels[LabelPrivate] != "" || c.Env[agentapi.PrivateEnv] != "" {
		t.Error("shared: not marked private")
	}

	c = BuildSession(in, "k3x9q2", "", SessionOptions{Private: true}).Container(SessionService)
	if c.SharePIDs != "" || peers(c) != 0 || !slices.Equal(c.Tmpfs, []string{"/home/node/.claude/sessions:mode=1777"}) {
		t.Errorf("private: pid %q, peer mounts %d, tmpfs %v", c.SharePIDs, peers(c), c.Tmpfs)
	}
	if c.Labels[LabelPrivate] != "true" || c.Env[agentapi.PrivateEnv] == "" {
		t.Error("private: marked")
	}
}

// A session gets its clipboard folder read-only, and the agent as the clipboard commands; the
// project's own container gets neither.
func TestBuildSessionClipboard(t *testing.T) {
	p := config.Project{Name: "api", Dir: "/code/api"}.Resolved(config.Global{})
	in := Inputs{Project: p, Image: "img", ProxyImage: "proxy", AgentBinary: "/state/bin/agent", ClipboardDir: "/state/s/clipboard"}
	targets := func(c *Container) map[string]Mount {
		out := map[string]Mount{}
		for _, m := range c.Mounts {
			out[m.Target] = m
		}
		return out
	}
	ms := targets(BuildSession(in, "k3x9q2", "", SessionOptions{}).Container(SessionService))
	if m := ms[agentapi.ClipboardDir]; m.Source != in.ClipboardDir || !m.ReadOnly {
		t.Errorf("clipboard folder: %+v", m)
	}
	for _, tool := range agentapi.ClipboardTools {
		if m := ms[tool]; m.Source != in.AgentBinary || !m.ReadOnly {
			t.Errorf("%s: %+v", tool, m)
		}
	}
	main := targets(Build(in).Container(SandboxService))
	if _, ok := main[agentapi.ClipboardDir]; ok {
		t.Error("the project's container has no clipboard")
	}
}

// The host terminal a session runs in is a label of its container, only when known.
func TestBuildSessionTerminal(t *testing.T) {
	p := config.Project{Name: "api", Dir: "/code/api"}.Resolved(config.Global{})
	in := Inputs{Project: p, Image: "img", ProxyImage: "proxy"}
	if s := BuildSession(in, "k3x9q2", "", SessionOptions{Terminal: "app=WezTerm&pane=3"}).Container(SessionService); s.Labels[LabelTerminal] != "app=WezTerm&pane=3" {
		t.Errorf("labels: %v", s.Labels)
	}
	if _, ok := BuildSession(in, "k3x9q2", "", SessionOptions{}).Container(SessionService).Labels[LabelTerminal]; ok {
		t.Error("an unknown terminal has no label")
	}
}

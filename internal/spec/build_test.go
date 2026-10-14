package spec

import (
	"maps"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
)

// claudeConfigMounts are the mounts under ~/.claude, but for the shared session list (see
// TestPeers).
func claudeConfigMounts(c *Container, home string) map[string]Mount {
	out := map[string]Mount{}
	dir := path.Join(home, ".claude")
	for _, m := range c.Mounts {
		if m.Source != PeersVolume && (m.Target == dir || strings.HasPrefix(m.Target, dir+"/")) {
			out[m.Target] = m
		}
	}
	return out
}

// With the app login, ~/.claude is the shared login volume and the project's own data stays in
// claude-config, mounted over it where Claude keeps it.
func TestAppLoginMounts(t *testing.T) {
	p := config.Project{Name: "api", Dir: "/code/api"}.Resolved(config.Global{})
	sb := Build(Inputs{Project: p, Image: "img", ProxyImage: "proxy"})
	home := p.Image.Home
	ms := claudeConfigMounts(sb.Container(SandboxService), home)
	if m := ms[path.Join(home, ".claude")]; m.Source != LoginVolume || m.Subpath != "" {
		t.Errorf("~/.claude: %+v", m)
	}
	for _, sub := range projectClaudePaths {
		m := ms[path.Join(home, ".claude", sub)]
		if m.Source != "claude-config" || m.Subpath != sub {
			t.Errorf("%s: %+v", sub, m)
		}
	}
	if len(ms) != 1+len(projectClaudePaths) || !slices.Contains(projectClaudePaths, "history.jsonl") {
		t.Errorf("mounts under ~/.claude: %v", ms)
	}
	if !slices.Equal(sb.AppVolumes, []string{LoginVolume}) || slices.Contains(sb.Volumes, LoginVolume) {
		t.Errorf("app volumes %v, volumes %v", sb.AppVolumes, sb.Volumes)
	}
	vi := sb.Container(VolumeInit)
	script := vi.Entrypoint[len(vi.Entrypoint)-1]
	for _, want := range []string{"mkdir -p", "touch /jailwick/volumes/claude-config/history.jsonl", "/jailwick/volumes/" + LoginVolume} {
		if !strings.Contains(script, want) {
			t.Errorf("volume init lacks %q:\n%s", want, script)
		}
	}
}

// With the project login, ~/.claude is the project's claude-config, as before.
func TestProjectLoginMounts(t *testing.T) {
	p := config.Project{Name: "api", Dir: "/code/api", Claude: config.Claude{Login: config.LoginProject}}.Resolved(config.Global{})
	sb := Build(Inputs{Project: p, Image: "img", ProxyImage: "proxy"})
	ms := claudeConfigMounts(sb.Container(SandboxService), p.Image.Home)
	if m := ms[path.Join(p.Image.Home, ".claude")]; len(ms) != 1 || m.Source != "claude-config" || m.Subpath != "" {
		t.Errorf("mounts under ~/.claude: %v", ms)
	}
	if len(sb.AppVolumes) != 0 {
		t.Errorf("no app volume: %v", sb.AppVolumes)
	}
	vi := sb.Container(VolumeInit)
	if script := vi.Entrypoint[len(vi.Entrypoint)-1]; strings.Contains(script, "claude-config") || strings.Contains(script, LoginVolume) {
		t.Errorf("no login folders to prepare:\n%s", script)
	}
}

// Claude Code's list of running sessions and its sockets folder are the project's PeersVolume,
// prepared for the user alone: the login volume holding the list may be shared by projects.
func TestPeers(t *testing.T) {
	p := config.Project{Name: "api", Dir: "/code/api"}.Resolved(config.Global{})
	sb := Build(Inputs{Project: p, Image: "img", ProxyImage: "proxy"})
	c := sb.Container(SandboxService)
	got := map[string]string{}
	for _, m := range c.Mounts {
		if m.Source == PeersVolume {
			got[m.Target] = m.Subpath
		}
	}
	want := map[string]string{path.Join(p.Image.Home, ".claude", "sessions"): "sessions", agentapi.SocketsDir: "sockets"}
	if !maps.Equal(got, want) || len(c.Tmpfs) != 0 || c.SharePIDs != "" {
		t.Errorf("peer mounts %v, tmpfs %v, share %q", got, c.Tmpfs, c.SharePIDs)
	}
	if !slices.Contains(sb.Volumes, PeersVolume) {
		t.Errorf("volumes: %v", sb.Volumes)
	}
	vi := sb.Container(VolumeInit)
	script := vi.Entrypoint[len(vi.Entrypoint)-1]
	for _, want := range []string{"mkdir -p -m 700 /jailwick/volumes/peers/sessions /jailwick/volumes/peers/sockets",
		"/jailwick/volumes/peers/sessions /jailwick/volumes/peers/sockets"} {
		if !strings.Contains(script, want) {
			t.Errorf("volume init lacks %q:\n%s", want, script)
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Fatal(got)
	}
}

// Each package folder gets a node_modules volume of its own, owned by the sandbox's user.
func TestNodeModulesVolumes(t *testing.T) {
	in := Inputs{Project: config.Project{Name: "api", Dir: "/r", Image: config.Image{ImageSource: config.ImageSource{User: "node"}}},
		NodeModules: []string{"web"}}
	sb := Build(in)
	v := NodeModulesVolume("web")
	if !slices.Contains(sb.Volumes, v) {
		t.Fatalf("volumes: %v", sb.Volumes)
	}
	main := sb.Container(SandboxService)
	if !slices.ContainsFunc(main.Mounts, func(m Mount) bool { return m.Source == v && m.Target == "/r/web/node_modules" }) {
		t.Fatalf("mounts: %+v", main.Mounts)
	}
	init := sb.Container(VolumeInit)
	if !strings.Contains(strings.Join(init.Entrypoint, " "), "/jailwick/volumes/"+v) {
		t.Fatalf("not owned by the user: %v", init.Entrypoint)
	}
	if NodeModulesVolume("web") == NodeModulesVolume("apps/web") {
		t.Fatal("folders must not share a volume")
	}
}

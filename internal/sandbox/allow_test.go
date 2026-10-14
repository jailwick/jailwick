package sandbox

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/spec"
)

// testManager is a Manager with a configuration folder of its own.
func testManager(t *testing.T, d *fakeDriver) *Manager {
	t.Helper()
	forgetProxyLogs() // the totals this process kept are another test's
	return &Manager{Driver: d, Store: &config.Store{Dir: t.TempDir()}}
}

func readAllow(t *testing.T, project, session string) string {
	t.Helper()
	data, err := os.ReadFile(allowFile(project, session))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Claude in Chrome's relay is on a session's allowlist only when that session opted in.
func TestSessionAllowlist(t *testing.T) {
	for _, chrome := range []bool{false, true} {
		m := testManager(t, &fakeDriver{containers: projectContainers()})
		b, err := m.StartSession(context.Background(), prepared(), "", spec.SessionOptions{Chrome: chrome})
		if err != nil {
			t.Fatal(err)
		}
		if b.Chrome != chrome {
			t.Errorf("box chrome %v, want %v", b.Chrome, chrome)
		}
		allow := readAllow(t, "api", b.ID)
		if strings.Contains(allow, proxy.ChromeHost) != chrome {
			t.Errorf("chrome %v: allowlist:\n%s", chrome, allow)
		}
		if !strings.Contains(allow, "api.anthropic.com") {
			t.Errorf("the session's allowlist is the project's too:\n%s", allow)
		}
	}
}

func TestEndSessionRemovesItsFiles(t *testing.T) {
	m := testManager(t, &fakeDriver{containers: projectContainers()})
	b, err := m.StartSession(context.Background(), prepared(), "", spec.SessionOptions{Chrome: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.EndSession(context.Background(), "api", b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sessionState("api", b.ID)); !os.IsNotExist(err) {
		t.Fatalf("the session's folder is left: %v", err)
	}
}

// A reload rewrites the project's allowlist and each session's, keeping each session's Chrome.
func TestReloadProxyRewritesSessionAllowlists(t *testing.T) {
	now := time.Now()
	cs := append(projectContainers(), box("plain1", "", "running", now)...)
	withChrome := box("chrom1", "", "running", now)
	withChrome[0].Labels[spec.LabelChrome] = "true"
	cs = append(cs, withChrome...)
	d := &fakeDriver{containers: cs, status: map[string]string{}}
	for _, c := range cs {
		if c.Service == spec.ProxyService {
			d.status[c.Name] = "" // reconfigure succeeds
		}
	}
	m := testManager(t, d)
	if err := m.Store.SaveProject(config.Project{Name: "api", Dir: t.TempDir(), Allow: []string{"example.org"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.ReloadProxy(context.Background(), "api"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		session string
		chrome  bool
	}{{"", false}, {"plain1", false}, {"chrom1", true}} {
		allow := readAllow(t, "api", c.session)
		if !strings.Contains(allow, "example.org") || strings.Contains(allow, proxy.ChromeHost) != c.chrome {
			t.Errorf("session %q (chrome %v):\n%s", c.session, c.chrome, allow)
		}
	}
	if len(d.execs) != 3 {
		t.Errorf("every proxy reloads: %v", d.execs)
	}
}

func TestGroupBoxesReadsChrome(t *testing.T) {
	cs := box("chrom1", "", "running", time.Now())
	cs[0].Labels[spec.LabelChrome] = "true"
	cs = append(cs, box("plain1", "", "running", time.Now())...)
	boxes := groupBoxes("api", cs)
	if len(boxes) != 2 || !boxes[0].Chrome || boxes[1].Chrome {
		t.Fatalf("boxes: %+v", boxes)
	}
}

// verify knows the tmpfs mounts (no "unexpected mount") and the login the sandbox uses.
func TestAgentConfigExpectsTmpfsAndLogin(t *testing.T) {
	p := config.Project{Name: "api", Dir: "/code/api"}.Resolved(config.Global{})
	sb := spec.Build(spec.Inputs{Project: p, Image: "img", ProxyImage: "proxy"})
	c := agentConfig(p, "localhost", sb, nil, nil, toolset{})
	if !slices.Contains(c.Expected.Mounts, p.Image.Home+"/.claude/sessions") || c.ClaudeLogin != config.LoginApp {
		t.Fatalf("expected mounts %v, login %q", c.Expected.Mounts, c.ClaudeLogin)
	}
}

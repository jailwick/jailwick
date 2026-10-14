package compose

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/spec"
)

var update = flag.Bool("update", false, "rewrite golden files")

func sample() spec.Inputs {
	p := config.Project{
		Name: "api", Dir: "/Users/me/code/api",
		Services: []string{"postgres", "redis"}, TestServices: []string{"postgres"},
		Postgres: config.Postgres{Password: "pa$$word", ExtraDBs: []config.ExtraDB{{Name: "app_test"}}},
		Redis:    config.Redis{Passwords: []string{"r1", "r2"}},
	}.Resolved(config.Global{})
	return spec.Inputs{
		Project: p, Image: "jailwick-sandbox:abc", ProxyImage: "jailwick-proxy:def",
		AgentBinary: "/state/bin/jailwick-agent", PointerFile: "/state/shared/pointer.md",
		SharedDir: "/state/shared", GeneratedDir: "/state/projects/api/generated",
		NotesDir: "/cfg/notes/projects/api", AllowDir: "/state/projects/api/allow",
		PostgresInit: "/state/postgres", Timezone: "Europe/Berlin",
		HostName: "sandbox.jailwick-api.orb.local",
		Secrets:  map[string]string{"GH_TOKEN": "ghp_secret"},
	}
}

func TestGolden(t *testing.T) {
	out, env, err := Render(spec.Build(sample()))
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "api.compose.yml")
	if *update {
		os.WriteFile(golden, out, 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(want) {
		t.Errorf("compose output changed; run go test ./internal/compose -update and review the diff")
	}
	if strings.Contains(string(out), "ghp_secret") {
		t.Error("secret value written into the compose file")
	}
	if env[SecretVar("GH_TOKEN")] != "ghp_secret" {
		t.Error("secret missing from the process environment")
	}
	if !strings.Contains(string(out), "pa$$$$word") {
		t.Error("dollar signs not escaped")
	}
}

func TestDeterministic(t *testing.T) {
	a, _, _ := Render(spec.Build(sample()))
	b, _, _ := Render(spec.Build(sample()))
	if string(a) != string(b) {
		t.Fatal("render is not deterministic")
	}
}

func TestSecretChangeChangesHash(t *testing.T) {
	in := sample()
	h1 := spec.Build(in).Container(spec.SandboxService).Labels[spec.LabelSpecHash]
	in.Secrets = map[string]string{"GH_TOKEN": "rotated"}
	h2 := spec.Build(in).Container(spec.SandboxService).Labels[spec.LabelSpecHash]
	if h1 == h2 {
		t.Fatal("rotating a secret must change the spec hash")
	}
}

func TestSessionGolden(t *testing.T) {
	in := sample()
	in.HostName = "session.jailwick-api--k3x9q2.orb.local"
	out, env, err := Render(spec.BuildSession(in, "k3x9q2", "feat-login", spec.SessionOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "api-session.compose.yml")
	if *update {
		os.WriteFile(golden, out, 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(want) {
		t.Errorf("session compose output changed; run go test ./internal/compose -update and review the diff")
	}
	s := string(out)
	if strings.Contains(s, "ghp_secret") || env[SecretVar("GH_TOKEN")] != "ghp_secret" {
		t.Error("the secret must reach the process environment, never the file")
	}
	for _, want := range []string{
		"name: jailwick-api--k3x9q2",
		"external: true\n    name: jailwick-api_sandbox", // the project's networks and volumes
		"external: true\n    name: jailwick-api_claude-config",
		"restart: \"no\"", // a session is never restarted
		"dev.jailwick.session: k3x9q2",
		"dev.jailwick.worktree: feat-login",
		"SANDBOX_HOST: session.jailwick-api--k3x9q2.orb.local",
		"external: true\n    name: jailwick-claude-login", // the app's, by its own name
		"subpath: projects",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("session file lacks %q", want)
		}
	}
	// Only the session's own containers: the services stay in the project sandbox.
	if strings.Contains(s, "jailwick-api_jailwick-claude-login") {
		t.Error("the login volume is the app's, never the project's")
	}
	for _, svc := range []string{"postgres:", "redis:"} {
		if strings.Contains(s, "\n  "+svc) {
			t.Errorf("session file has the project's %s service", svc)
		}
	}
}

// The app login volume is never one Compose creates or removes with a project.
func TestAppVolumeExternal(t *testing.T) {
	out, _, err := Render(spec.Build(sample()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "  jailwick-claude-login:\n    external: true\n    name: jailwick-claude-login\n") {
		t.Errorf("login volume not external:\n%s", out)
	}
}

// With the project login, nothing of the app's is used, and claude-config isn't mounted by
// subpath (only the peers volume is).
func TestProjectLogin(t *testing.T) {
	in := sample()
	in.Project.Claude.Login = config.LoginProject
	out, _, err := Render(spec.Build(in))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "jailwick-claude-login") || strings.Count(s, "subpath:") != 2 {
		t.Errorf("project login uses app volumes or subpaths:\n%s", s)
	}
}

package claudesettings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/spec"
)

func TestMerge(t *testing.T) {
	out, err := Merge([]Layer{
		{Name: "a", Data: []byte(`{"model":"sonnet","env":{"A":"1","B":"1"},"permissions":{"allow":["Bash(ls)",{"x":1}]},"n":12345678901234567890}`)},
		{Name: "b", Data: []byte(`{"model":"opus","env":{"B":"2"},"permissions":{"allow":["Bash(ls)","Read",{"x":1}]},"theme":["x"]}`)},
		{Name: "c", Data: []byte(`{"theme":"dark"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "env": {
    "A": "1",
    "B": "2"
  },
  "model": "opus",
  "n": 12345678901234567890,
  "permissions": {
    "allow": [
      "Bash(ls)",
      {
        "x": 1
      },
      "Read"
    ]
  },
  "theme": "dark"
}
`
	if string(out) != want {
		t.Fatalf("merged:\n%s", out)
	}
}

func TestMergeErrorsNameTheLayer(t *testing.T) {
	for _, data := range []string{`[1]`, `{`, `null`, `{} {}`} {
		_, err := Merge([]Layer{{Name: "~/x.json", Data: []byte(data)}})
		if err == nil || !strings.HasPrefix(err.Error(), "~/x.json: ") {
			t.Errorf("%s: %v", data, err)
		}
	}
}

func TestLayers(t *testing.T) {
	s := &config.Store{Dir: t.TempDir()}
	write := func(file, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := config.Project{Name: "api", Claude: config.Claude{Profiles: []string{"strict", "web"}}}

	if _, err := Layers(s, p); err == nil || !strings.Contains(err.Error(), `no profile "strict"`) {
		t.Fatalf("a listed profile must exist: %v", err)
	}
	write(s.ProfileFile("strict"), `{"model":"opus"}`)
	write(s.ProfileFile("web"), `{"model":"haiku"}`)
	layers, err := Layers(s, p)
	if err != nil || len(layers) != 3 || layers[0].Name != "jailwick" {
		t.Fatalf("missing app and project files are skipped: %v %v", layers, err)
	}

	write(s.ClaudeSettingsFile(), `{"model":"sonnet","env":{"FROM":"app"}}`)
	write(s.ProjectSettingsFile("api"), `{"env":{"FROM":"project"}}`)
	out, err := Build(s, p)
	if err != nil {
		t.Fatal(err)
	}
	// Order: jailwick < app < profiles in order < project.
	if !strings.Contains(string(out), `"model": "haiku"`) || !strings.Contains(string(out), `"FROM": "project"`) ||
		!strings.Contains(string(out), `"theme": "auto"`) {
		t.Fatalf("built:\n%s", out)
	}
}

func TestCheck(t *testing.T) {
	file := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(file, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Check(file); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"a":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Check(file); err == nil {
		t.Fatal("invalid JSON passes")
	}
}

// jailwick's own settings route Claude Code's worktrees to the agent at its mounted path.
func TestWorktreeHooks(t *testing.T) {
	data := string(assets.MustRead("claude/settings.json"))
	for _, event := range []string{"create", "remove"} {
		if !strings.Contains(data, `"`+spec.AgentPath+" worktree-hook "+event+`"`) {
			t.Errorf("no %s hook calling %s", event, spec.AgentPath)
		}
	}
}

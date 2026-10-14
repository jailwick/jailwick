package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/agentapi"
)

func TestClaudeLevels(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude")
	for _, n := range []string{".credentials.json", ".claude.json.backup.123", "projects", "history.jsonl.lock", "brand-new"} {
		must(t, os.MkdirAll(filepath.Join(dir, n), 0o755))
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	v := &verifier{c: &agentapi.Config{Home: home, ClaudeLogin: "app"}}
	v.claudeLevels()
	if len(v.checks) != 1 || !v.checks[0].Info || !strings.HasSuffix(v.checks[0].Name, ": brand-new") {
		t.Fatalf("checks: %+v", v.checks)
	}

	v = &verifier{c: &agentapi.Config{Home: home, ClaudeLogin: "project"}}
	v.claudeLevels()
	if len(v.checks) != 0 {
		t.Fatalf("a project's own login is the project's: %+v", v.checks)
	}
}

package importer

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func file(t *testing.T, p, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRead(t *testing.T) {
	setup := t.TempDir()
	repo := filepath.Join(t.TempDir(), "api")
	os.MkdirAll(repo, 0o755)
	file(t, filepath.Join(setup, "projects", "_template.env"), "PROJECT_DIR=/x\n")
	file(t, filepath.Join(setup, "projects", "api.env"), `# api
PROJECT_DIR="`+repo+`"
CLAUDE_CODE_OAUTH_TOKEN=sk-secret
GIT_AUTHOR_NAME=Claude Sandbox
POSTGRES_USER=app
POSTGRES_DB=app
POSTGRES_EXTRA_DBS=app_test:tester:pw other
REDIS_PASSWORDS=a b
REDIS_PASSWORD_FIRST=b
LOCAL_FORWARDS=5432:postgres:5432
WT_COPY_FILES=.env apps/web/.env.local
CLAUDE_CODE_VERSION=latest
PLAYWRIGHT_BROWSERS=chromium
WORKSPACE=acme
`)
	file(t, filepath.Join(setup, "config", "projects", "api", "notes.md"), "see /etc/sandbox/project; run csb restart\n")
	file(t, filepath.Join(setup, "workspaces", "acme.env"), "WORKSPACE_ROOT=/r\nWORKSPACE_MEMBERS=web docs=/elsewhere/docs\nWORKSPACE_HANDOFF=.handoff\n")
	file(t, filepath.Join(setup, "proxy", "allow", "allowlist.txt"), "api.anthropic.com\n.github.com\napi.github.com\nbackend.composio.dev\n")

	plan, err := Read(setup)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Projects) != 1 {
		t.Fatalf("projects: %+v skipped: %v", plan.Projects, plan.Skipped)
	}
	p := plan.Projects[0]
	c := p.Config
	if c.Dir != repo || c.Git.Name != "" || !slices.Equal(c.Services, []string{"postgres", "redis"}) {
		t.Errorf("basic fields: %+v", c)
	}
	if len(c.Postgres.ExtraDBs) != 2 || c.Postgres.ExtraDBs[0].User != "tester" {
		t.Errorf("extra dbs: %+v", c.Postgres.ExtraDBs)
	}
	if !slices.Equal(c.Redis.Passwords, []string{"b", "a"}) {
		t.Errorf("redis passwords: %v", c.Redis.Passwords)
	}
	if p.Secrets["CLAUDE_CODE_OAUTH_TOKEN"] != "sk-secret" || c.Image.ClaudeCodeVersion != "" || !c.YOLO() {
		t.Errorf("secrets/version/yolo: %+v", p)
	}
	if !slices.Equal(c.Image.Playwright.Browsers, []string{"chromium"}) {
		t.Errorf("csb's browsers are kept (no browsers means no Playwright): %v", c.Image.Playwright.Browsers)
	}
	if string(p.Notes) != "see /etc/jailwick/project; run jw restart\n" {
		t.Errorf("notes: %q", p.Notes)
	}
	w := plan.Workspaces[0].Config
	if len(w.Members) != 2 || w.Members[1].Path != "/elsewhere/docs" || w.Handoff != ".handoff" {
		t.Errorf("workspace: %+v", w)
	}
	if !slices.Equal(plan.Allow, []string{"backend.composio.dev"}) {
		t.Errorf("allow: %v", plan.Allow)
	}
	// csb allowed .github.com to every project: imported projects keep it, as an explicit preset.
	if got := plan.Projects[0].Config.AllowPresets; !slices.Contains(got, "anthropic") || !slices.Contains(got, "github") {
		t.Errorf("allow_presets: %v", got)
	}
}

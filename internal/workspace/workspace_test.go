package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/spec"
)

func touch(t *testing.T, p string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRules(t *testing.T) {
	got := ApplyRules([]string{"a", "b"}, []string{"+c", "-a", "d"})
	if !slices.Equal(got, []string{"b", "c", "d"}) {
		t.Fatalf("got %v", got)
	}
	if got := ApplyRules([]string{"a"}, []string{"-*", "x"}); !slices.Equal(got, []string{"x"}) {
		t.Fatalf("got %v", got)
	}
}

func TestFindHidden(t *testing.T) {
	d := t.TempDir()
	for _, f := range []string{".env", ".env.local", ".env.example", "keys/a.pem", "src/app.ts", "src/.env",
		"node_modules/x/index.js", "build/out.js", "nested/build/x", ".git/config", "secrets/token"} {
		touch(t, filepath.Join(d, f))
	}
	rules := ApplyRules(DefaultRules, []string{"/build/", "secrets"})
	var got []string
	for _, p := range FindHidden(d, rules) {
		got = append(got, strings.TrimPrefix(p, d+"/"))
	}
	want := []string{".env", ".env.local", "build", "keys/a.pem", "node_modules", "secrets", "src/.env"}
	if !slices.Equal(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func setup(t *testing.T) (*config.Store, config.Project) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "acme")
	touch(t, filepath.Join(root, "web", "package.json"))
	touch(t, filepath.Join(root, "api", "src", "main.go"))
	touch(t, filepath.Join(root, "api", ".env"))
	touch(t, filepath.Join(root, "CLAUDE.md"))
	touch(t, filepath.Join(root, "infra", "secret.tf"))
	s := &config.Store{Dir: t.TempDir()}
	w := config.Workspace{Name: "acme", Root: root, Members: []config.Member{{Name: "web"}, {Name: "api"}},
		RootNotes: "CLAUDE.md", Handoff: ".handoff"}
	if err := s.SaveWorkspace(w); err != nil {
		t.Fatal(err)
	}
	return s, config.Project{Name: "web", Dir: filepath.Join(root, "web"), Workspace: "acme"}
}

func TestGenerate(t *testing.T) {
	s, p := setup(t)
	mask := t.TempDir()
	r, err := Generate(s, p, mask)
	if err != nil {
		t.Fatal(err)
	}
	root := r.Info.Root
	if !slices.Equal(r.AddDirs, []string{filepath.Join(root, "api")}) {
		t.Fatalf("members: %v", r.AddDirs)
	}
	if !slices.Contains(r.Info.Hidden, filepath.Join(root, "api", ".env")) {
		t.Fatalf("api/.env not hidden: %v", r.Info.Hidden)
	}
	if !slices.Equal(r.Info.RootList, []string{".handoff", "CLAUDE.md", "api", "web"}) {
		t.Fatalf("root list: %v", r.Info.RootList)
	}
	var rw []string
	for _, m := range r.Extra.Mounts {
		if !m.ReadOnly {
			rw = append(rw, m.Target)
		}
		if m.Type != spec.Bind {
			t.Fatalf("unexpected mount type %v", m)
		}
	}
	if len(rw) != 1 || filepath.Base(rw[0]) != ".handoff" {
		t.Fatalf("only the handoff folder may be writable: %v", rw)
	}
	md := string(r.Markdown)
	for _, want := range []string{"`api/` | read-only", "to-web/", "jw restart"} {
		if !strings.Contains(md, want) {
			t.Errorf("workspace.md misses %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".handoff", "README.md")); err != nil {
		t.Error("handoff README not created")
	}
}

func TestGenerateRefusesProjectOutsideRoot(t *testing.T) {
	s, p := setup(t)
	p.Dir = t.TempDir()
	if _, err := Generate(s, p, t.TempDir()); err == nil || !strings.Contains(err.Error(), "inside the workspace root") {
		t.Fatalf("want outside-root error, got %v", err)
	}
}

func TestHandoffs(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "to-api"), 0o755)
	os.MkdirAll(filepath.Join(d, "done"), 0o755)
	os.WriteFile(filepath.Join(d, "to-api", "a.md"), []byte("---\nfrom: web\nto: api\nstatus: open  # open -> answered\ncreated: 2026-10-12 14:30\nsubject: Avatar endpoint\n---\n## Request\n"), 0o644)
	os.WriteFile(filepath.Join(d, "done", "b.md"), []byte("---\nfrom: api\nto: web\nstatus: done\nsubject: Old\n---\n"), 0o644)
	hs, err := ListHandoffs(d)
	if err != nil || len(hs) != 2 {
		t.Fatalf("got %v %v", hs, err)
	}
	if hs[0].Status != "open" || hs[0].Inbox != "api" || hs[0].Subject != "Avatar endpoint" || hs[1].Done != true {
		t.Fatalf("unexpected: %+v", hs)
	}
}

func TestReadHandoff(t *testing.T) {
	dir := t.TempDir()
	req := filepath.Join(dir, "to-web", "2026-01-01-fix.md")
	os.MkdirAll(filepath.Dir(req), 0o755)
	if err := os.WriteFile(req, []byte("---\nfrom: api\nstatus: open\nsubject: Fix the login\n---\n\nPlease fix it.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, body, err := ReadHandoff(dir, req)
	if err != nil || h.From != "api" || h.Inbox != "web" || h.Subject != "Fix the login" || body != "Please fix it.\n" {
		t.Fatalf("%+v %q %v", h, body, err)
	}
	outside := filepath.Join(t.TempDir(), "to-web", "x.md")
	touch(t, outside)
	for _, f := range []string{outside, filepath.Join(dir, "README.md"), filepath.Join(dir, "to-web", "..", "..", "secret.md")} {
		if _, _, err := ReadHandoff(dir, f); err == nil {
			t.Errorf("%s must be refused", f)
		}
	}
}

// The sandbox writes the handoff folder: links are skipped and text is cleaned for a terminal.
func TestListHandoffsUntrusted(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "creds.md")
	os.WriteFile(secret, []byte("---\nsubject: stolen\n---\n"), 0o600)
	os.MkdirAll(filepath.Join(dir, "to-web"), 0o755)
	os.WriteFile(filepath.Join(dir, "to-web", "1.md"), []byte("---\nfrom: api\nsubject: hi\x1b]52;c;eA==\x07\n---\n"), 0o644)
	os.Symlink(secret, filepath.Join(dir, "to-web", "2.md"))
	os.Symlink(filepath.Dir(secret), filepath.Join(dir, "to-api"))

	hs, err := ListHandoffs(dir)
	if err != nil || len(hs) != 1 || hs[0].Subject != "hi]52;c;eA==" {
		t.Fatalf("%+v %v", hs, err)
	}
}

// A link where the README goes is never written through.
func TestHandoffReadmeNotThroughALink(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "api"), 0o755)
	target := filepath.Join(t.TempDir(), ".zshenv")
	os.MkdirAll(filepath.Join(root, ".handoff"), 0o755)
	os.Symlink(target, filepath.Join(root, ".handoff", "README.md"))
	store := &config.Store{Dir: t.TempDir()}
	w := config.Workspace{Name: "shop", Root: root, Handoff: ".handoff"}
	store.SaveWorkspace(w)
	if _, err := Generate(store, config.Project{Name: "api", Dir: filepath.Join(root, "api"), Workspace: "shop"}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the README was written through the link")
	}
}

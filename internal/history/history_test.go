package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, root, id, cwd string, lines ...string) {
	t.Helper()
	dir := filepath.Join(root, "p")
	os.MkdirAll(dir, 0o755)
	all := append([]string{`{"type":"summary"}`, `{"type":"user","cwd":"` + cwd + `","isMeta":true,"message":{"content":"<command-name>/init"}}`}, lines...)
	os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(all, "\n")), 0o644)
}

func TestList(t *testing.T) {
	root := t.TempDir()
	live := t.TempDir()
	write(t, root, "a", live, `{"type":"user","message":{"content":[{"type":"text","text":"<ide>x</ide>\nFix the login bug"}]}}`)
	time.Sleep(10 * time.Millisecond)
	write(t, root, "b", "/gone/folder", `{"type":"user","message":{"content":"Old work"}}`)
	write(t, root, "c", live) // no prompt: skipped

	got := List(root, Options{})
	if len(got) != 1 || got[0].ID != "a" || got[0].Prompt != "Fix the login bug" {
		t.Fatalf("got %+v", got)
	}
	got = List(root, Options{Missing: true})
	if len(got) != 2 || got[0].ID != "b" || !got[0].Gone {
		t.Fatalf("with missing: %+v", got)
	}
	got = List(root, Options{Missing: true, Keep: func(d string) bool { return d == live }})
	if len(got) != 1 {
		t.Fatalf("filter: %+v", got)
	}
}

// No conversations is an empty list: the app reads null as "not loaded yet".
func TestListEmptyIsNotNull(t *testing.T) {
	out := List(t.TempDir(), Options{})
	if data, _ := json.Marshal(out); string(data) != "[]" {
		t.Fatalf("got %s", data)
	}
}

// A conversation renamed with /rename shows its name: from Claude Code's title file, else from
// the last custom-title line of the conversation.
func TestName(t *testing.T) {
	root := t.TempDir()
	live := t.TempDir()
	write(t, root, "a", live, `{"type":"user","message":{"content":"Fix the login bug"}}`,
		`{"type":"custom-title","customTitle":"first-name","sessionId":"a"}`,
		`{"type":"custom-title","customTitle":"login-fix","sessionId":"a"}`)
	write(t, root, "b", live, `{"type":"user","message":{"content":"Other work"}}`)
	os.MkdirAll(filepath.Join(root, "p", "b"), 0o755)
	os.WriteFile(filepath.Join(root, "p", "b", "custom-title.json"), []byte(`{"customTitle":"from-the-file"}`), 0o644)
	write(t, root, "c", live, `{"type":"user","message":{"content":"Never renamed"}}`)

	names := map[string]string{}
	for _, c := range List(root, Options{}) {
		names[c.ID] = c.Name
	}
	if names["a"] != "login-fix" || names["b"] != "from-the-file" || names["c"] != "" {
		t.Fatalf("%v", names)
	}
}

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeRequest(t *testing.T, dir, rel, text string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const request = "---\nfrom: web\nto: api\nstatus: answered   # open -> answered -> done\nsubject: Order status\n---\n\n## Request\nShow it.\n"

// Done moves a request to done/ with its status; reopening takes it back to its inbox.
func TestSetHandoffStatus(t *testing.T) {
	dir := t.TempDir()
	file := writeRequest(t, dir, "to-api/2026-10-14-web-status.md", request)
	h, _, _ := ReadHandoff(dir, file)

	done, err := SetHandoffStatus(dir, file, h.Version, "done")
	if err != nil {
		t.Fatal(err)
	}
	if done != filepath.Join(dir, "done", "2026-10-14-web-status.md") {
		t.Fatalf("moved to %s", done)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("the inbox no longer has it")
	}
	h, body, _ := ReadHandoff(dir, done)
	if h.Status != "done" || !h.Done || h.Inbox != "api" || !strings.Contains(body, "Show it.") {
		t.Fatalf("done: %+v %q", h, body)
	}

	back, err := SetHandoffStatus(dir, done, h.Version, "open")
	if err != nil || back != file {
		t.Fatalf("reopened to %s: %v", back, err)
	}
	h, _, _ = ReadHandoff(dir, back)
	if h.Status != "open" {
		t.Fatalf("reopened: %+v", h)
	}

	ans, err := SetHandoffStatus(dir, back, h.Version, "answered")
	if err != nil || ans != back {
		t.Fatalf("answered in place: %s %v", ans, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "to-api")); len(entries) != 1 {
		t.Fatalf("no file left behind: %v", entries)
	}
}

// A request a session changed since the page read it is left as it is.
func TestHandoffChangedSince(t *testing.T) {
	dir := t.TempDir()
	file := writeRequest(t, dir, "to-api/a.md", request)
	h, _, _ := ReadHandoff(dir, file)
	time.Sleep(10 * time.Millisecond)
	writeRequest(t, dir, "to-api/a.md", request+"\n## Reply (api)\nDone.\n")
	if _, err := SetHandoffStatus(dir, file, h.Version, "done"); !errors.Is(err, ErrHandoffChanged) {
		t.Fatalf("changed since: %v", err)
	}
	if err := DeleteHandoff(dir, file, h.Version); !errors.Is(err, ErrHandoffChanged) {
		t.Fatalf("deleting what changed since: %v", err)
	}
	h, _, _ = ReadHandoff(dir, file)
	if err := DeleteHandoff(dir, file, h.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("deleted")
	}
}

// Only requests of the folder are changed, never through a link.
func TestHandoffEditRefuses(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	victim := writeRequest(t, outside, "x.md", request)
	os.MkdirAll(filepath.Join(dir, "to-api"), 0o755)
	os.Symlink(victim, filepath.Join(dir, "to-api", "link.md"))
	for _, f := range []string{victim, filepath.Join(dir, "to-api", "link.md"), filepath.Join(dir, "notes.md")} {
		if _, err := SetHandoffStatus(dir, f, version(f), "done"); err == nil {
			t.Errorf("%s changed", f)
		}
	}
	if _, err := SetHandoffStatus(dir, filepath.Join(dir, "to-api", "link.md"), "x", "bogus"); err == nil {
		t.Error("an unknown status")
	}
	if data, _ := os.ReadFile(victim); string(data) != request {
		t.Fatal("the file outside is untouched")
	}
	// A done request with no "to" can't be reopened: nothing says which inbox.
	noTo := writeRequest(t, dir, "done/n.md", "---\nstatus: done\n---\nx\n")
	if _, err := SetHandoffStatus(dir, noTo, version(noTo), "open"); err == nil {
		t.Error("reopened without an inbox")
	}
}

func TestWithStatus(t *testing.T) {
	if got := withStatus("no front matter\n", "done"); !strings.HasPrefix(got, "---\nstatus: done\n---\n\nno front matter") {
		t.Fatalf("%q", got)
	}
	if got := withStatus("---\nfrom: a\n---\nbody\n", "open"); got != "---\nfrom: a\nstatus: open\n---\nbody\n" {
		t.Fatalf("%q", got)
	}
}

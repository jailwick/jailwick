package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/agentapi"
)

func TestChanges(t *testing.T) {
	c := &agentapi.Config{Dir: gitRepo(t)}
	w, err := PrepareWorktree(c, "auth", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(w.Dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn := func(args ...string) {
		if out, err := exec.Command("git", append([]string{"-C", w.Dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	write("login.go", "package auth\n")
	gitIn("add", "login.go")
	gitIn("commit", "-q", "-m", "Add login")
	write("login.go", "package auth\n\nfunc Login() {}\n") // uncommitted, on top
	write("notes.txt", "todo\n")                           // untracked

	ch, err := Changes(c, "auth", "login.go")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Base != "main" || ch.Head != "auth" || len(ch.Commits) != 1 || ch.Commits[0].Subject != "Add login" {
		t.Fatalf("%+v", ch)
	}
	if len(ch.Files) != 2 || ch.Files[0].Path != "login.go" || ch.Files[0].Status != "added" || ch.Files[0].Added != 3 ||
		ch.Files[1].Path != "notes.txt" || ch.Files[1].Status != "untracked" {
		t.Fatalf("files: %+v", ch.Files)
	}
	if !strings.Contains(ch.Diff, "+func Login() {}") {
		t.Fatalf("diff: %q", ch.Diff)
	}

	// The main checkout: its uncommitted edits.
	if main, err := Changes(c, "", ""); err != nil || main.Head != "main" || len(main.Files) != 0 {
		t.Fatalf("main: %+v %v", main, err)
	}
}

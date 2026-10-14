package agent

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// gitRepo is a repo with one commit.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

// Claude Code's own worktrees go through jailwick: a slot, the folder on stdout and nothing else.
func TestWorktreeHook(t *testing.T) {
	c := &agentapi.Config{Dir: gitRepo(t)}
	var out bytes.Buffer
	if err := WorktreeHook(c, "create", strings.NewReader(`{"name":"bright-fox","session_id":"s"}`), &out); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(c.Dir, ".jailwick", "worktrees", "bright-fox")
	if out.String() != dir+"\n" {
		t.Fatalf("stdout is the folder only: %q", out.String())
	}
	if env := WorktreeEnvFor(c, filepath.Join(dir, "src")); env["PORT"] != "3010" {
		t.Fatalf("a command in it gets its own ports: %v", env)
	}
	if WorktreeEnvFor(c, c.Dir) != nil {
		t.Fatal("the main checkout keeps its environment")
	}

	if err := WorktreeHook(c, "remove", strings.NewReader(`{"worktree_path":"`+dir+`"}`), &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("the worktree is removed")
	}
	if err := WorktreeHook(c, "remove", strings.NewReader(`{"worktree_path":"/elsewhere/x"}`), &out); err == nil {
		t.Fatal("only jailwick's worktrees are removed")
	}
}

// A worktree made around jailwick has no slot, and says so.
func TestUnmanagedWorktree(t *testing.T) {
	c := &agentapi.Config{Dir: gitRepo(t)}
	dir := filepath.Join(c.Dir, ".claude", "worktrees", "own")
	if out, err := exec.Command("git", "-C", c.Dir, "worktree", "add", "-q", "-b", "own", dir).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	wts := ListWorktrees(c)
	if len(wts) != 1 || !wts[0].Unmanaged || wts[0].Name != "own" || wts[0].Branch != "own" {
		t.Fatalf("%+v", wts)
	}
}

func TestPrintExports(t *testing.T) {
	var b bytes.Buffer
	PrintExports(&b, map[string]string{"B": "it's", "A": "1"})
	if b.String() != "export A='1'\nexport B='it'\\''s'\n" {
		t.Fatalf("%q", b.String())
	}
}

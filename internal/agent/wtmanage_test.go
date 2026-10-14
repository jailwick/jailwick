package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// A worktree made from the app, checked and removed as the app does: uncommitted changes and
// unmerged commits are said before, and the branch goes only when asked.
func TestAddCheckRemoveWorktree(t *testing.T) {
	c := &agentapi.Config{Dir: gitRepo(t), Setup: []string{"false"}}
	w, err := AddWorktree(c, "feat-x", "main")
	if err != nil {
		t.Fatal(err)
	}
	if w.Branch != "feat-x" || w.Port != 3010 || !w.SetupFailed {
		t.Fatalf("added: %+v", w)
	}
	if _, err := AddWorktree(c, "feat-x", "main"); err == nil {
		t.Fatal("a worktree that exists isn't added again")
	}
	dir := filepath.Join(c.Dir, ".jailwick", "worktrees", "feat-x")
	gitIn := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}

	ch, err := CheckWorktree(c, "feat-x")
	if err != nil || ch.Changed != 0 || !ch.Merged || ch.Ahead != 0 || ch.Into != "main" {
		t.Fatalf("a new worktree has nothing to lose: %+v %v", ch, err)
	}
	gitIn("commit", "-q", "--allow-empty", "-m", "work")
	os.WriteFile(filepath.Join(dir, "draft.txt"), []byte("x"), 0o644)
	if ch, _ = CheckWorktree(c, "feat-x"); ch.Changed != 1 || ch.Merged || ch.Ahead != 1 || ch.Pushed {
		t.Fatalf("a commit and a file of its own: %+v", ch)
	}

	if err := RemoveWorktree(c, "feat-x", RemoveOptions{}); err == nil {
		t.Fatal("uncommitted changes refuse a removal without force")
	}
	if err := RemoveWorktree(c, "feat-x", RemoveOptions{Force: true, Branch: "merged"}); err == nil {
		t.Fatal("an unmerged branch isn't deleted as merged")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) || !branchExists(c.Dir, "feat-x") {
		t.Fatal("the worktree is gone, its unmerged branch kept")
	}

	if _, err := AddWorktree(c, "feat-y", "main"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(c, "feat-y", RemoveOptions{Branch: "merged"}); err != nil || branchExists(c.Dir, "feat-y") {
		t.Fatalf("a merged branch goes with its worktree: %v", err)
	}
	if bs := Branches(c); len(bs) != 2 || bs[0] != "main" || bs[1] != "feat-x" {
		t.Fatalf("branches, the main checkout's first: %v", bs)
	}
}

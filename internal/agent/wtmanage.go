package agent

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Worktrees made and removed from the app: created without a session in them, and checked
// before they are removed (uncommitted changes, whether the branch is merged).

// AddWorktree creates a worktree with its slot, database copies, copied files and setup
// commands, without starting a session in it. Progress goes to stderr.
func AddWorktree(c *agentapi.Config, name, base string) (agentapi.WorktreeAdded, error) {
	if _, slot := findSlot(c, name); slot > 0 {
		return agentapi.WorktreeAdded{}, fmt.Errorf("worktree %q already exists", name)
	}
	w, err := PrepareWorktree(c, name, base)
	if err != nil {
		return agentapi.WorktreeAdded{}, err
	}
	fmt.Fprintf(os.Stderr, "worktree %s ready at %s (ports %d-%d)\n", w.Branch, w.Dir, Port(w.Slot), Port(w.Slot)+9)
	return agentapi.WorktreeAdded{Name: w.Branch, Branch: w.Branch, Port: Port(w.Slot), DBs: w.DBs, SetupFailed: w.SetupFailed}, nil
}

func findSlot(c *agentapi.Config, name string) (branch string, slot int) {
	_ = withRegistry(c, false, func(r *registry) error {
		slot, branch = r.find(name)
		return nil
	})
	return branch, slot
}

// CheckWorktree says what removing a worktree would take away.
func CheckWorktree(c *agentapi.Config, name string) (agentapi.WorktreeCheck, error) {
	branch, slot := findSlot(c, name)
	if slot == 0 {
		return agentapi.WorktreeCheck{}, fmt.Errorf("unknown worktree %q", name)
	}
	out := agentapi.WorktreeCheck{Name: name, Branch: branch}
	dir := worktreeDir(c, branch)
	if st, err := gitOut(dir, "status", "--porcelain"); err != nil {
		out.Missing = true
	} else if st != "" {
		out.Changed = len(strings.Split(st, "\n"))
	}
	if !branchExists(c.Dir, branch) {
		return out, nil // a branch already deleted: nothing to merge or keep
	}
	out.Into, _ = gitOut(c.Dir, "branch", "--show-current")
	if out.Into != "" && out.Into != branch {
		out.Merged = git(c.Dir, "merge-base", "--is-ancestor", branch, out.Into) == nil
		if n, err := gitOut(c.Dir, "rev-list", "--count", out.Into+".."+branch); err == nil {
			out.Ahead, _ = strconv.Atoi(n)
		}
	}
	if up, err := gitOut(c.Dir, "rev-parse", "--abbrev-ref", branch+"@{upstream}"); err == nil && up != "" {
		n, err := gitOut(c.Dir, "rev-list", "--count", up+".."+branch)
		out.Pushed = err == nil && n == "0"
	}
	return out, nil
}

func branchExists(repo, branch string) bool {
	return git(repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch) == nil
}

// gitOut runs git in dir and returns its output, trimmed.
func gitOut(dir string, args ...string) (string, error) {
	b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(b)), err
}

// Branches are the repo's local branches, for a new worktree to start from: the main
// checkout's first.
func Branches(c *agentapi.Config) []string {
	cur, _ := gitOut(c.Dir, "branch", "--show-current")
	all, err := gitOut(c.Dir, "for-each-ref", "--sort=-committerdate", "--format=%(refname:short)", "refs/heads/")
	out := []string{}
	if cur != "" {
		out = append(out, cur)
	}
	if err != nil || all == "" {
		return out
	}
	for _, b := range strings.Split(all, "\n") {
		if b != cur {
			out = append(out, b)
		}
	}
	return out
}

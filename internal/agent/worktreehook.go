package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Claude Code makes worktrees of its own (claude --worktree, subagents with worktree
// isolation). jailwick's settings route them here through its WorktreeCreate and WorktreeRemove
// hooks, so each gets a slot like jw wt's: its own ports, database copies and Redis database.
// Without that it would share the main checkout's, and a subagent's migration would hit the
// main database.

// hookInput is what Claude Code passes a worktree hook on stdin (other fields aside).
type hookInput struct {
	Name         string `json:"name"`          // WorktreeCreate: the worktree's name
	WorktreePath string `json:"worktree_path"` // WorktreeRemove: the path WorktreeCreate gave
}

// WorktreeHook serves one hook event (create or remove). For create, the worktree's folder is
// the only thing written to out: Claude Code reads it from stdout. Everything else goes to stderr.
func WorktreeHook(c *agentapi.Config, event string, in io.Reader, out io.Writer) error {
	var h hookInput
	if err := json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&h); err != nil {
		return fmt.Errorf("worktree hook: reading its input: %w", err)
	}
	switch event {
	case "create":
		w, err := prepareQuietly(c, h.Name)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, w.Dir)
		return err
	case "remove":
		name := agentapi.WorktreeOf(c.Dir, filepath.Clean(h.WorktreePath)+"/")
		if name == "" {
			return fmt.Errorf("worktree hook: %s isn't a jailwick worktree", h.WorktreePath)
		}
		return RemoveWorktree(c, name, RemoveOptions{}) // refuses uncommitted changes
	}
	return fmt.Errorf("worktree hook: unknown event %q (create or remove)", event)
}

// prepareQuietly prepares a worktree from HEAD with anything it prints (git, setup commands)
// sent to stderr, so stdout carries only the folder.
func prepareQuietly(c *agentapi.Config, name string) (Prepared, error) {
	stdout := os.Stdout
	os.Stdout = os.Stderr
	defer func() { os.Stdout = stdout }()
	return PrepareWorktree(c, name, "HEAD")
}

// WorktreeEnvFor returns the environment of the worktree dir is in: its ports, database and
// Redis. It is empty in the main checkout, and in a worktree jailwick has no slot for.
func WorktreeEnvFor(c *agentapi.Config, dir string) map[string]string {
	name := agentapi.WorktreeOf(c.Dir, filepath.Clean(dir)+"/")
	if name == "" {
		return nil
	}
	branch, slot, err := Resolve(c, name)
	if err != nil {
		return nil
	}
	return WorktreeEnv(c, branch, slot)
}

// PrintExports writes env as shell exports, for a shell to eval.
func PrintExports(w io.Writer, env map[string]string) {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "export %s='%s'\n", k, strings.ReplaceAll(env[k], "'", `'\''`))
	}
}

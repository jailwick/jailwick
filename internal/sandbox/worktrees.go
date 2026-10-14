package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
	"github.com/jailwick/jailwick/internal/untrusted"
)

// Worktrees made and removed from the app (see agent.AddWorktree). git runs in the sandbox, as
// its user (user; "" for the image's), on the repo the sandbox mounts.

// AddWorktree creates a worktree with its resources, without a session in it. Its progress (the
// setup commands' output too) goes to log as it comes.
func (m *Manager) AddWorktree(ctx context.Context, project, user, name, base string, log io.Writer) (agentapi.WorktreeAdded, error) {
	var w agentapi.WorktreeAdded
	out := &untrusted.LimitedBuffer{Max: 64 << 10}
	errOut := &tail{w: log}
	err := m.Driver.Exec(ctx, runtime.ExecOptions{
		Container: MainContainer(project), User: user,
		Cmd:    []string{spec.AgentPath, "wt", "add", "-base", base, name},
		Stdout: out, Stderr: errOut,
	})
	if err != nil {
		if m.agentOlder(ctx, MainContainer(project)) {
			return w, ErrAgentOlder
		}
		if msg := errOut.last(); msg != "" {
			return w, fmt.Errorf("%s", msg)
		}
		return w, err
	}
	if err := json.Unmarshal(out.Bytes(), &w); err != nil {
		return w, fmt.Errorf("agent wt add: %w: %w", ErrAgentAnswer, err)
	}
	untrusted.Clean(&w)
	return w, nil
}

// CheckWorktree says what removing a worktree would take away.
func (m *Manager) CheckWorktree(ctx context.Context, project, name string) (agentapi.WorktreeCheck, error) {
	var ch agentapi.WorktreeCheck
	err := m.AgentJSON(ctx, MainContainer(project), &ch, "wt", "check", name)
	return ch, err
}

// Branches are the repo's local branches, the main checkout's first.
func (m *Manager) Branches(ctx context.Context, project string) ([]string, error) {
	out := []string{}
	err := m.AgentJSON(ctx, MainContainer(project), &out, "wt", "branches")
	return out, err
}

// RemoveWorktree removes a worktree (force: with its uncommitted changes) and, with branch
// "merged" or "any", its branch (see agent.RemoveOptions).
func (m *Manager) RemoveWorktree(ctx context.Context, project, user, name string, force bool, branch string) error {
	cmd := []string{spec.AgentPath, "wt", "rm"}
	if force {
		cmd = append(cmd, "-force")
	}
	if branch != "" {
		cmd = append(cmd, "-branch", branch)
	}
	errOut := &tail{w: io.Discard}
	err := m.Driver.Exec(ctx, runtime.ExecOptions{
		Container: MainContainer(project), User: user, Cmd: append(cmd, name), Stdout: io.Discard, Stderr: errOut,
	})
	if err != nil {
		if msg := errOut.last(); msg != "" {
			return fmt.Errorf("%s", msg)
		}
	}
	return err
}

// tail passes what is written on to w, keeping the last line for an error to say.
type tail struct {
	w    io.Writer
	line []byte
	prev []byte
}

func (t *tail) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' {
			if len(t.line) > 0 {
				t.prev = append(t.prev[:0], t.line...)
			}
			t.line = t.line[:0]
		} else if len(t.line) < 4096 {
			t.line = append(t.line, b)
		}
	}
	return t.w.Write(p)
}

func (t *tail) last() string {
	if len(t.line) > 0 {
		return untrusted.Text(string(t.line))
	}
	return untrusted.Text(string(t.prev))
}

package sandbox

import (
	"context"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// StartRun starts the project's run command in its main checkout ("") or a worktree, in the
// background; StopRun ends it. Both return the runs there are now.
func (m *Manager) StartRun(ctx context.Context, project, worktree string) ([]agentapi.Run, error) {
	return m.run(ctx, project, "start", worktree)
}

func (m *Manager) StopRun(ctx context.Context, project, worktree string) ([]agentapi.Run, error) {
	return m.run(ctx, project, "stop", worktree)
}

func (m *Manager) run(ctx context.Context, project, action, worktree string) ([]agentapi.Run, error) {
	runs := []agentapi.Run{}
	err := m.AgentJSON(ctx, MainContainer(project), &runs, "run", action, "-worktree", worktree)
	return runs, err
}

// RunLog is the end of a run's output (cleaned: it comes from the sandbox).
func (m *Manager) RunLog(ctx context.Context, project, worktree string) (string, error) {
	var l agentapi.RunLog
	err := m.AgentJSON(ctx, MainContainer(project), &l, "run", "log", "-worktree", worktree)
	return l.Text, err
}

// Changes is what the main checkout ("") or a worktree changed, with file's diff when file is
// set. git runs in the sandbox (see agent.Changes); the answer is cleaned like any agent answer.
func (m *Manager) Changes(ctx context.Context, project, worktree, file string) (agentapi.Changes, error) {
	var ch agentapi.Changes
	args := []string{"changes", "-worktree", worktree}
	if file != "" {
		args = append(args, "-file", file)
	}
	err := m.AgentJSON(ctx, MainContainer(project), &ch, args...)
	return ch, err
}

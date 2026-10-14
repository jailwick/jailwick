package sandbox

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// A conversation is resumed by its id, and a running session reports the id of the conversation it
// holds (Claude Code's list of running sessions; resuming keeps the id). Resuming one that runs
// would start a second Claude on the same conversation, both writing its history: the app and the
// CLI check here first.

// ErrConversationRunning refuses to resume a conversation that a session holds.
var ErrConversationRunning = errors.New("the conversation is running")

// Running maps the conversations sessions hold to those sessions. A session whose conversation
// Claude didn't report isn't in it.
func Running(ss []Session) map[string]Session {
	out := map[string]Session{}
	for _, s := range ss {
		if s.ID != "" {
			out[s.ID] = s
		}
	}
	return out
}

// RunningConversations is Running for a project's sessions now.
func (m *Manager) RunningConversations(ctx context.Context, project string) (map[string]Session, error) {
	ss, err := m.Sessions(ctx, project)
	if err != nil {
		return nil, err
	}
	return Running(ss), nil
}

// Where says where a session runs, as conversations say it: "main checkout" or "wt <name>".
func Where(s Session) string { return WhereOf(s.Worktree) }

// CheckResume refuses resuming conversation id while a session holds it.
func CheckResume(running map[string]Session, id string) error {
	s, ok := running[id]
	if !ok {
		return nil
	}
	state := ""
	if s.State != "" {
		state = ", " + s.State
	}
	return fmt.Errorf("%w in %s%s: switch to its terminal, or end it before resuming it", ErrConversationRunning, Where(s), state)
}

// Latest is the newest conversation at where (as Conversation.Where has it) that no session holds
// and whose folder is still there; skipped says a newer one there is running.
func Latest(convs []agentapi.Conversation, where string, running map[string]Session) (c agentapi.Conversation, ok, skipped bool) {
	sorted := slices.Clone(convs)
	slices.SortStableFunc(sorted, func(a, b agentapi.Conversation) int { return b.When.Compare(a.When) })
	for _, c := range sorted {
		if c.Where != where || c.Gone {
			continue
		}
		if _, run := running[c.ID]; run {
			skipped = true
			continue
		}
		return c, true, skipped
	}
	return agentapi.Conversation{}, false, skipped
}

// WhereOf is where a session in worktree runs, as conversations say it ("" is the main checkout).
func WhereOf(worktree string) string {
	if worktree == "" {
		return "main checkout"
	}
	return "wt " + strings.ReplaceAll(worktree, "/", "-")
}

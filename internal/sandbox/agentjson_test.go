package sandbox

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/runtime"
)

// answerDriver answers agent commands (their arguments, joined) with fixed output.
type answerDriver struct {
	runtime.Driver
	answers map[string]string
}

func (d answerDriver) Exec(_ context.Context, o runtime.ExecOptions) error {
	out, ok := d.answers[strings.Join(o.Cmd[1:], " ")]
	if !ok {
		return errors.New("exit status 2")
	}
	io.WriteString(o.Stdout, out)
	return nil
}

func TestAgentJSON(t *testing.T) {
	ctx := context.Background()
	var st agentapi.Status

	// An answer is cleaned of control characters.
	m := &Manager{Driver: answerDriver{answers: map[string]string{
		"status --json": `{"sessions":[{"pid":1,"name":"fix\u001b]52;c;eA==\u0007"}]}`,
	}}}
	if err := m.AgentJSON(ctx, "c", &st, "status", "--json"); err != nil || st.Sessions[0].Name != "fix]52;c;eA==" {
		t.Fatalf("%+v %v", st, err)
	}

	// An agent from before contracts: no `version -json`, so it is older; a restart is the answer.
	m = &Manager{Driver: answerDriver{answers: map[string]string{
		"version -json": "5113f8f3a533\n", "status --json": `{"unknown": [`,
	}}}
	if err := m.AgentJSON(ctx, "c", &st, "status", "--json"); !errors.Is(err, ErrAgentOlder) {
		t.Fatalf("older agent: %v", err)
	}

	// An agent of this contract that answers badly is a bad answer, not an old agent.
	m = &Manager{Driver: answerDriver{answers: map[string]string{
		"version -json": `{"contract":1,"build":"x"}`, "status --json": `nope`,
	}}}
	if err := m.AgentJSON(ctx, "c", &st, "status", "--json"); !errors.Is(err, ErrAgentAnswer) {
		t.Fatalf("bad answer: %v", err)
	}
}

func TestRunCommands(t *testing.T) {
	m := &Manager{Driver: answerDriver{answers: map[string]string{
		"run start -worktree auth": `[{"worktree":"auth","pid":42,"running":true,"port":3010}]`,
		"run log -worktree auth":   `{"text":"ready on :3010\u001b[0m"}`,
	}}}
	runs, err := m.StartRun(context.Background(), "api", "auth")
	if err != nil || len(runs) != 1 || !runs[0].Running || runs[0].Port != 3010 {
		t.Fatalf("%+v %v", runs, err)
	}
	if text, err := m.RunLog(context.Background(), "api", "auth"); err != nil || text != "ready on :3010[0m" {
		t.Fatalf("the output is cleaned: %q %v", text, err)
	}
}

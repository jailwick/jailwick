// Package session starts agent sessions and commands inside a sandbox.
package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/hostterm"
	"github.com/jailwick/jailwick/internal/notify"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/spec"
)

// Options describe one session.
type Options struct {
	Worktree string // "" for the main checkout
	Base     string // what a new worktree branches from
	Resume   string // "", "last", "pick" or a conversation ID
	Open     bool   // reach the whole internet from the start
	Chrome   bool   // may use Claude in Chrome (the user's own browser)
	Private  bool   // kept out of messages between sessions
}

// AgentArgs are the agent CLI's flags for a session: workspace folders first (--add-dir takes
// several values and would swallow what follows), then settings, Chrome, permissions and resume.
// Chrome is always explicit: Claude's own default for it lives in its config, which projects on
// the app login share.
func AgentArgs(prep *sandbox.Prepared, resume string, chrome bool) []string {
	var args []string
	for _, d := range prep.AddDirs {
		args = append(args, "--add-dir", d)
	}
	args = append(args, "--settings", prep.Settings, "--mcp-config", prep.MCPConfig)
	if chrome {
		args = append(args, "--chrome")
	} else {
		args = append(args, "--no-chrome")
	}
	if prep.Project.YOLO() {
		args = append(args, "--dangerously-skip-permissions")
	}
	switch resume {
	case "":
	case "last":
		args = append(args, "--continue")
	case "pick":
		args = append(args, "--resume")
	default:
		args = append(args, "--resume", resume)
	}
	return args
}

// terminalEnv passes the host terminal's capabilities into the sandbox.
func terminalEnv() map[string]string {
	env := map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}
	for _, k := range []string{"TERM", "COLORTERM", "TERM_PROGRAM"} {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	return env
}

// sessionEnv is an agent session's terminal environment, with its notification channel (see
// notify.Channel): the agent sets it in the session's Claude settings.
func sessionEnv(channel string) map[string]string {
	env := terminalEnv()
	env[agentapi.NotifyEnv] = channel
	return env
}

// Start runs an interactive agent session in the main checkout or a worktree, in a sandbox of
// its own (see sandbox.StartSession) that is removed when the session ends. o.Open lets it reach
// the whole internet from the start, until it ends or the internet is closed again.
func Start(ctx context.Context, m *sandbox.Manager, prep *sandbox.Prepared, o Options) error {
	// Where the session tells it waits: this terminal, for session_notifications. Its sandbox is
	// labelled with it, so the app knows which sessions it announces without asking them.
	channel := notify.Channel(m.Global.SessionNotify(), notify.Terminal(os.Getenv))
	box, err := m.StartSession(ctx, prep, o.Worktree, spec.SessionOptions{Chrome: o.Chrome, Private: o.Private, Notify: channel,
		Terminal: hostterm.Current().Encode()}) // for the app's Show terminal
	if err != nil {
		return err
	}
	// A closed terminal or a stop signal ends the attached session (the runtime's client gets
	// it too, with the default action: a caught signal isn't inherited); this process stays to
	// remove the session's sandbox.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGTERM)
	defer signal.Stop(sigs)
	defer func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		_ = m.EndSession(ctx, prep.Project.Name, box.ID)
	}()
	if o.Open {
		if _, err := m.OpenInternet(ctx, box, 0); err != nil {
			return err
		}
	}
	return attach(ctx, m.Driver, prep, box, channel, o)
}

// attach runs the agent in a session's container, with this terminal. Where the host's clipboard
// can be read, the terminal is relayed so an image can be pasted (see paste.go);
// JAILWICK_PASTE=off attaches directly.
func attach(ctx context.Context, d runtime.Driver, prep *sandbox.Prepared, box sandbox.Box, channel string, o Options) error {
	cmd := []string{spec.AgentPath, "session"}
	if o.Worktree != "" {
		cmd = append(cmd, "-worktree", o.Worktree, "-base", o.Base)
	}
	cmd = append(cmd, "--")
	cmd = append(cmd, AgentArgs(prep, o.Resume, o.Chrome)...)
	exec := runtime.ExecOptions{
		Container: box.Session.Name,
		User:      prep.Project.Image.User,
		WorkDir:   prep.Project.Dir,
		Env:       sessionEnv(channel),
		Cmd:       cmd,
		TTY:       true,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	}
	var err error
	if read := hostClipboard(); read != nil && relayed() && os.Getenv("JAILWICK_PASTE") != "off" {
		p := &paster{dir: sandbox.ClipboardDir(prep.Project.Name, box.ID), read: read}
		err = relay(ctx, func() { p.paste(ctx) }, func(tty *os.File) error {
			exec.Terminal = tty
			return d.Exec(ctx, exec)
		})
	} else {
		err = d.Exec(ctx, exec)
	}
	ResetTerminal()
	var ee *runtime.ExitError
	if errors.As(err, &ee) && (ee.Code == 255 || ee.Code == 137) {
		return fmt.Errorf("lost the connection to the session (exit %d); resume it with: jw claude last", ee.Code)
	}
	if errors.As(err, &ee) {
		return nil // the agent's own exit status; it already reported why
	}
	return err
}

// Run runs a command (or a shell) in the main checkout or a worktree.
func Run(ctx context.Context, d runtime.Driver, project, user, dir, worktree string, tty bool, cmd []string) error {
	full := []string{spec.AgentPath, "exec"}
	if worktree != "" {
		full = append(full, "-worktree", worktree)
	}
	full = append(full, "--")
	full = append(full, cmd...)
	opts := runtime.ExecOptions{
		Container: sandbox.MainContainer(project), User: user, WorkDir: dir,
		Env: terminalEnv(), Cmd: full, TTY: tty,
		Stdout: os.Stdout, Stderr: os.Stderr,
	}
	if !tty {
		opts.Stdin = os.Stdin
	}
	err := d.Exec(ctx, opts)
	if tty {
		ResetTerminal()
	}
	return err
}

// ResetTerminal turns off modes a crashed full-screen program may leave on (mouse tracking,
// bracketed paste, alternate screen) and shows the cursor.
func ResetTerminal() {
	fmt.Fprint(os.Stdout, "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[?1049l\x1b[?25h")
}

//go:build unix

package session

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// relayed reports whether a session can be relayed: both ends of this process are a terminal.
func relayed() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// relay runs run with a pseudo-terminal in place of this process's terminal, copying between the
// two: keys in (calling onPaste before a paste key goes through), output out, and size changes.
func relay(ctx context.Context, onPaste func(), run func(tty *os.File) error) error {
	return relayWith(ctx, os.Stdin, os.Stdout, onPaste, run)
}

// relayWith is relay for the terminal in (keys, size) and out.
func relayWith(ctx context.Context, in, out *os.File, onPaste func(), run func(tty *os.File) error) error {
	ptmx, tty, err := pty.Open()
	if err != nil {
		return err
	}
	defer ptmx.Close()
	resize := func() { _ = pty.InheritSize(in, ptmx) }
	resize()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer func() { signal.Stop(winch); close(winch) }()
	go func() {
		for range winch {
			resize()
		}
	}()

	old, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		tty.Close()
		return err
	}
	defer func() { _ = term.Restore(int(in.Fd()), old) }()

	output := make(chan struct{})
	go func() {
		_, _ = io.Copy(out, ptmx) // ends when the session's side is closed
		close(output)
	}()
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := in.Read(buf)
			if n > 0 {
				if isPasteKey(buf[:n]) {
					onPaste()
				}
				if _, werr := ptmx.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	err = run(tty)
	tty.Close()
	select { // the last output, then on
	case <-output:
	case <-time.After(time.Second):
	case <-ctx.Done():
	}
	return err
}

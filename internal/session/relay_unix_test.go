//go:build unix

package session

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"
)

// The relay passes keys and output between a terminal and a command in a pseudo-terminal of its
// own, and calls onPaste before a paste key reaches the command.
func TestRelay(t *testing.T) {
	user, term, err := pty.Open() // the user's terminal: we type into user, the relay reads term
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	defer user.Close()
	defer term.Close()

	var pasted atomic.Bool
	var seen atomic.Bool // the command got the paste key after onPaste ran
	onPaste := func() { pasted.Store(true) }
	done := make(chan error, 1)
	go func() {
		done <- relayWith(context.Background(), term, term, onPaste, func(tty *os.File) error {
			cmd := exec.Command("sh", "-c", `stty raw -echo; k=$(dd bs=1 count=6 2>/dev/null); printf 'got:%s' "$k" | tr '\026' '^'`)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
			err := cmd.Run()
			seen.Store(pasted.Load())
			return err
		})
	}()
	time.Sleep(300 * time.Millisecond) // the command sets its terminal up
	if _, err := user.Write([]byte("hello\x16")); err != nil {
		t.Fatal(err)
	}
	out := &lockedBuffer{} // the copy keeps writing while the test reads
	go func() { _, _ = io.Copy(out, user) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the relay didn't end")
	}
	time.Sleep(100 * time.Millisecond)
	if !strings.Contains(out.String(), "got:hello^") {
		t.Errorf("output %q", out.String())
	}
	if !pasted.Load() || !seen.Load() {
		t.Errorf("onPaste ran %v, before the command read the key %v", pasted.Load(), seen.Load())
	}
}

// lockedBuffer is a buffer written and read from different goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

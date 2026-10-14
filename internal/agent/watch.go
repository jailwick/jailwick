package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// WaitForSession returns once the session whose PID is in file has ended, or ctx is done. It
// gives up after start when no session ever wrote the file, so an unused container doesn't stay.
func WaitForSession(ctx context.Context, file string, start time.Duration, alive func(pid int) bool) {
	deadline := time.Now().Add(start)
	pid := 0
	for pid == 0 {
		if b, err := os.ReadFile(file); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		if pid == 0 && (time.Now().After(deadline) || !pause(ctx, 200*time.Millisecond)) {
			return
		}
	}
	for alive(pid) {
		if !pause(ctx, time.Second) {
			return
		}
	}
}

// Alive reports whether a process exists.
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// WriteSessionPID records this process as the container's session (it keeps its PID through exec).
func WriteSessionPID(file string) error {
	return os.WriteFile(file, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

// WaitProxy waits until the egress proxy named in proxyURL accepts connections. A session's
// proxy starts with it, so the session waits rather than meet a refused connection.
func WaitProxy(ctx context.Context, proxyURL string, timeout time.Duration) error {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return nil // no proxy configured: nothing to wait for
	}
	deadline := time.Now().Add(timeout)
	var d net.Dialer
	for {
		dctx, cancel := context.WithTimeout(ctx, time.Second)
		c, err := d.DialContext(dctx, "tcp", u.Host)
		cancel()
		if err == nil {
			_ = c.Close()
			return nil
		}
		if time.Now().After(deadline) || !pause(ctx, 200*time.Millisecond) {
			return fmt.Errorf("the egress proxy (%s) isn't answering: %w", u.Host, err)
		}
	}
}

// pause sleeps for d, or returns false when ctx ends first.
func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

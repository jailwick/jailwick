package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// lines collects what WatchStatus writes, safely across goroutines.
type lines struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lines) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Count(l.buf.String(), "\n")
}

// watch prints at once, after a change it was woken for, on the heartbeat, and nothing more.
func TestWatchStatus(t *testing.T) {
	var pid atomic.Int32
	pid.Store(1)
	snapshot := func() agentapi.Status { return agentapi.Status{Sessions: []agentapi.Session{{PID: int(pid.Load())}}} }
	kick := make(chan struct{})
	wait := func(ctx context.Context, max time.Duration) {
		select {
		case <-ctx.Done():
		case <-kick:
		case <-time.After(max):
		}
	}
	out := &lines{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- WatchStatus(ctx, out, snapshot, wait, 400*time.Millisecond) }()

	time.Sleep(50 * time.Millisecond)
	kick <- struct{}{} // woken, nothing changed: nothing printed
	time.Sleep(50 * time.Millisecond)
	if n := out.count(); n != 1 {
		t.Fatalf("unchanged: %d lines, want 1", n)
	}
	pid.Store(2)
	kick <- struct{}{}
	time.Sleep(50 * time.Millisecond)
	if n := out.count(); n != 2 {
		t.Fatalf("a change: %d lines, want 2", n)
	}
	time.Sleep(450 * time.Millisecond)
	if n := out.count(); n != 3 {
		t.Fatalf("the heartbeat: %d lines, want 3", n)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.buf.String(), `"pid":2`) {
		t.Errorf("JSON lines: %s", out.buf.String())
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("host gone") }

func TestWatchStatusEndsWithTheHost(t *testing.T) {
	nowait := func(context.Context, time.Duration) {}
	err := WatchStatus(context.Background(), failWriter{}, func() agentapi.Status { return agentapi.Status{} }, nowait, time.Second)
	if err == nil {
		t.Fatal("a write that fails ends watch")
	}
}

// The worktrees are listed again only when their files change.
func TestWorktreeSignature(t *testing.T) {
	repo := t.TempDir()
	head := filepath.Join(repo, ".git", "worktrees", "feat", "HEAD")
	if err := os.MkdirAll(filepath.Dir(head), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(head, []byte("ref: refs/heads/feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := worktreeSignature(repo)
	if before != worktreeSignature(repo) {
		t.Fatal("stable while nothing changes")
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(head, later, later); err != nil {
		t.Fatal(err)
	}
	if worktreeSignature(repo) == before {
		t.Error("a checkout in a worktree changes it")
	}
	reg := filepath.Join(repo, ".git", "jailwick")
	if err := os.MkdirAll(reg, 0o755); err != nil {
		t.Fatal(err)
	}
	mid := worktreeSignature(repo)
	if err := os.WriteFile(filepath.Join(reg, "worktrees.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if worktreeSignature(repo) == mid {
		t.Error("a slot change changes it")
	}
}

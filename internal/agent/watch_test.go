package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForSessionEndsWithTheProcess(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pid")
	os.WriteFile(file, []byte("42\n"), 0o644)
	var checks atomic.Int32
	alive := func(pid int) bool {
		if pid != 42 {
			t.Errorf("checked pid %d", pid)
		}
		return checks.Add(1) < 3 // alive twice, then gone
	}
	done := make(chan struct{})
	go func() { WaitForSession(context.Background(), file, time.Minute, alive); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("didn't return after the process ended")
	}
}

func TestWaitForSessionGivesUpWithoutASession(t *testing.T) {
	start := time.Now()
	WaitForSession(context.Background(), filepath.Join(t.TempDir(), "never"), 300*time.Millisecond,
		func(int) bool { t.Error("no PID, nothing to check"); return true })
	if time.Since(start) > 5*time.Second {
		t.Fatal("waited too long for a session that never started")
	}
}

func TestWaitForSessionStopsWithContext(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pid")
	os.WriteFile(file, []byte("42\n"), 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	WaitForSession(ctx, file, time.Minute, func(int) bool { return true })
}

func TestWaitProxy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx := context.Background()
	if err := WaitProxy(ctx, "http://"+ln.Addr().String(), time.Second); err != nil {
		t.Fatalf("listening proxy: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	if err := WaitProxy(ctx, "http://"+addr, 300*time.Millisecond); err == nil {
		t.Fatal("a closed port must time out")
	}
	if err := WaitProxy(ctx, "", time.Second); err != nil {
		t.Fatalf("no proxy configured: %v", err)
	}
}

func TestWriteSessionPIDAndAlive(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pid")
	if err := WriteSessionPID(file); err != nil {
		t.Fatal(err)
	}
	if !Alive(os.Getpid()) {
		t.Fatal("this process is alive")
	}
}

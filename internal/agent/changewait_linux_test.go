//go:build linux

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A wait ends soon after a file changes in a watched folder, or when ctx ends; not at its max.
func TestChangeWaiter(t *testing.T) {
	dir := t.TempDir()
	wait, closeWait := changeWaiter(func() []string { return []string{dir, filepath.Join(dir, "not-yet")} })
	defer closeWait()

	start := time.Now()
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.WriteFile(filepath.Join(dir, "123.json"), []byte(`{"status":"busy"}`), 0o644)
	}()
	wait(context.Background(), 10*time.Second)
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("woken after %v", took)
	}

	ctx, cancel := context.WithCancel(context.Background())
	start = time.Now()
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	wait(ctx, 10*time.Second)
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("cancelled after %v", took)
	}

	start = time.Now()
	wait(context.Background(), 200*time.Millisecond) // nothing changes: the max
	if took := time.Since(start); took < 150*time.Millisecond {
		t.Fatalf("returned after %v without a change", took)
	}
}

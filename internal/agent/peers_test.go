package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnProcess(t *testing.T) {
	if !ownProcess(os.Getpid()) {
		t.Fatal("this process is ours")
	}
	if ownProcess(1 << 30) {
		t.Fatal("a missing process isn't")
	}
}

func TestClearDirs(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"13.json", "13.abc.key", "sub/x.sock"} {
		p := filepath.Join(dir, f)
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	clearDirs(dir, filepath.Join(dir, "missing"))
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("left %v, %v", entries, err)
	}
}

// The shared folders must be mounted (not this container's memory), the user's, and closed to
// others: Claude Code refuses a sockets folder others could write to.
func TestSharedDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for fsType, ok := range map[string]bool{"": false, "tmpfs": false, "ext4": true} {
		if err := sharedDir(dir, fsType); (err == nil) != ok {
			t.Errorf("%q: %v", fsType, err)
		}
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if sharedDir(dir, "ext4") == nil {
		t.Error("open to others: refused")
	}
}

package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jailwick/jailwick/internal/agentapi"
)

func TestIsPasteKey(t *testing.T) {
	for in, want := range map[string]bool{
		"\x16":                   true, // Ctrl+V
		"ab\x16c":                true,
		"\x1b[118;5u":            true, // kitty keyboard protocol
		"\x1b[118;5:1u":          true, // with an event type
		"\x1b[27;5;118~":         true, // modifyOtherKeys
		"\x1b[200~\x1b[201~":     true, // an empty paste: the clipboard holds no text
		"\x1b[200~text\x1b[201~": false,
		"v":                      false,
		"\x1b[118u":              false, // v without Ctrl
		"\x1b[118;9u":            false, // Cmd+V (super), which pastes text
	} {
		if got := isPasteKey([]byte(in)); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
}

func TestPaster(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, agentapi.ClipboardFile)
	image := []byte("\x89PNG...")
	p := &paster{dir: dir, read: func(_ context.Context, f string) error { return os.WriteFile(f, image, 0o600) }}
	p.paste(context.Background())
	got, err := os.ReadFile(file)
	if err != nil || string(got) != string(image) {
		t.Fatalf("pasted %q, %v", got, err)
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v: the sandbox's user must read it", info.Mode().Perm())
	}

	// No image on the clipboard: the last one goes, so it is never pasted twice by mistake.
	p.read = func(context.Context, string) error { return errors.New("no image") }
	p.paste(context.Background())
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("an old image stayed: %v", err)
	}
	p.read = func(_ context.Context, f string) error { return os.WriteFile(f, nil, 0o600) }
	p.paste(context.Background())
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("an empty image is no image: %v", entries)
	}
}

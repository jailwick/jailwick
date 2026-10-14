package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWithNotifyChannel(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "settings.json")
	os.WriteFile(file, []byte(`{"model":"opus","permissions":{"allow":["Bash(ls)"]}}`), 0o644)
	args := []string{"--add-dir", "/x", "--settings", file, "--chrome"}

	got := WithNotifyChannel(args, "iterm2", dir)
	if got[3] == file || !slices.Equal(got[:3], args[:3]) || got[4] != "--chrome" {
		t.Fatalf("args: %q", got)
	}
	var s map[string]any
	data, _ := os.ReadFile(got[3])
	if json.Unmarshal(data, &s) != nil || s["preferredNotifChannel"] != "iterm2" || s["model"] != "opus" {
		t.Fatalf("settings: %s", data)
	}
	if args[3] != file {
		t.Fatal("the caller's arguments changed")
	}

	for _, c := range []string{"", "auto", "something-else"} {
		if got := WithNotifyChannel(args, c, dir); !slices.Equal(got, args) {
			t.Errorf("%q: %q", c, got)
		}
	}
	if got := WithNotifyChannel([]string{"--settings", filepath.Join(dir, "missing.json")}, "kitty", dir); got[1] != filepath.Join(dir, "missing.json") {
		t.Errorf("an unreadable file keeps the arguments: %q", got)
	}
}

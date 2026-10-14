package paths

import (
	"path/filepath"
	"testing"
)

func TestDataDir(t *testing.T) {
	t.Setenv("JAILWICK_DATA", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/me")
	if got := DataDir(); got != "/home/me/.local/share/jailwick" {
		t.Fatalf("default: %s", got)
	}
	t.Setenv("XDG_DATA_HOME", "/xdg")
	if got := DataDir(); got != "/xdg/jailwick" {
		t.Fatalf("XDG: %s", got)
	}
	t.Setenv("JAILWICK_DATA", "~/jw-data")
	if got := DataDir(); got != "/home/me/jw-data" || ProjectData("api") != filepath.Join("/home/me/jw-data", "projects", "api") {
		t.Fatalf("JAILWICK_DATA: %s", got)
	}
}

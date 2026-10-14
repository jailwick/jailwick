package server

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain keeps every test's state and data files (blocked records, discards, build failures) in
// a temporary folder, never the user's.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "jailwick-server-")
	if err != nil {
		panic(err)
	}
	for k, sub := range map[string]string{"JAILWICK_STATE": "state", "JAILWICK_DATA": "data"} {
		if err := os.Setenv(k, filepath.Join(dir, sub)); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

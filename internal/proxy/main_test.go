package proxy

import (
	"os"
	"testing"
)

// TestMain keeps the user's own presets out of the tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "jailwick-home-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("JAILWICK_HOME", dir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

package sandbox

import (
	"os"
	"testing"
)

// TestMain keeps every test's state and data files (allowlists, session folders, blocked
// records) in a temporary folder, never the user's.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "jailwick-state-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("JAILWICK_STATE", dir); err != nil {
		panic(err)
	}
	if err := os.Setenv("JAILWICK_DATA", dir+"/data"); err != nil { // kept data too (blocked records)
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

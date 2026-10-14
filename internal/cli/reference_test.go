package cli

import (
	"os"
	"strings"
	"testing"
)

// TestReferenceUpToDate fails when docs/CLI.md no longer matches the commands: run `make docs`.
func TestReferenceUpToDate(t *testing.T) {
	want, err := os.ReadFile("../../docs/CLI.md")
	if err != nil {
		t.Fatalf("docs/CLI.md missing: run make docs (%v)", err)
	}
	got := Reference()
	if got != string(want) {
		t.Fatal("docs/CLI.md is out of date: run make docs")
	}
	for _, c := range []string{"`jw claude`", "`jw wt rm`", "`jw allow`", "`jw app`", "`jw project add`"} {
		if !strings.Contains(got, c) {
			t.Errorf("reference misses %s", c)
		}
	}
}

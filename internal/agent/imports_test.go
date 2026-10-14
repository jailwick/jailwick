package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApproveCreatesAndKeeps(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".claude.json")
	a := claudeJSON{file: file}
	if err := a.Approve("/code/api"); err != nil {
		t.Fatal(err)
	}
	if _, approved, err := a.read("/code/api"); err != nil || !approved {
		t.Fatalf("a missing file is created approved: %v %v", approved, err)
	}

	must(t, os.WriteFile(file, []byte(`{"numStartups":12345678901234567890,"projects":{"/other":{"x":1}}}`), 0o600))
	if err := a.Approve("/code/api"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	for _, want := range []string{`"numStartups": 12345678901234567890`, `"/other"`, `"hasClaudeMdExternalIncludesApproved": true`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("lost or missing %s:\n%s", want, data)
		}
	}
	if fi, _ := os.Stat(file); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(file), ".claude.json.jailwick-*"))
	if len(leftovers) > 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}
}

// Once approved, the file Claude may be writing too is left alone.
func TestApproveLeavesApprovedFileAlone(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".claude.json")
	a := claudeJSON{file: file}
	if err := a.Approve("/code/api"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	must(t, os.Chtimes(file, old, old))
	if err := a.Approve("/code/api"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(file); !fi.ModTime().Equal(old) {
		t.Error("rewritten although already approved")
	}
}

func TestApproveRefusesInvalidJSON(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".claude.json")
	must(t, os.WriteFile(file, []byte(`{"projects":`), 0o600))
	if err := (claudeJSON{file: file}).Approve("/code/api"); err == nil {
		t.Fatal("invalid JSON passes")
	}
	if data, _ := os.ReadFile(file); string(data) != `{"projects":` {
		t.Fatalf("Claude's file was changed: %s", data)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

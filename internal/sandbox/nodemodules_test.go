package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

func TestPackageDirs(t *testing.T) {
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	for _, d := range []string{
		".", "web", "apps/admin", "apps/site", "packages/ui/lib", // found
		"node_modules/react", ".jailwick/worktrees/x", "a/b/c/d/e", // inside node_modules, hidden, too deep
		"generated/client", // git-ignored
	} {
		os.MkdirAll(filepath.Join(repo, d), 0o755)
		os.WriteFile(filepath.Join(repo, d, "package.json"), []byte("{}"), 0o644)
	}
	os.MkdirAll(filepath.Join(repo, "docs"), 0o755) // no package.json
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("generated/\n"), 0o644)

	p := config.Project{Dir: repo}
	if got := PackageDirs(p); !slices.Equal(got, []string{"apps/admin", "apps/site", "packages/ui/lib", "web"}) {
		t.Fatalf("found: %v", got)
	}
	// The project's own list wins: folders or patterns, existing folders only.
	p.NodeModules = []string{"apps/*", "missing", "docs"}
	if got := PackageDirs(p); !slices.Equal(got, []string{"apps/admin", "apps/site", "docs"}) {
		t.Fatalf("chosen: %v", got)
	}
}

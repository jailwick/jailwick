package sandbox

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
)

// maxPackageDepth is how deep PackageDirs looks for package folders below the repo's top.
const maxPackageDepth = 4

// skipDirs are never package folders, nor hold any worth finding.
var skipDirs = []string{"node_modules", "vendor", "dist", "build", "target", "coverage"}

// PackageDirs are the repo's package folders below its top whose node_modules the sandbox keeps
// apart from the Mac's (relative, sorted). They are the project's node_modules setting when it
// has one (folders or patterns like apps/*), else every folder with a package.json, 4 levels
// deep, leaving out hidden and git-ignored folders. The top's own node_modules is always apart.
func PackageDirs(p config.Project) []string {
	if len(p.NodeModules) > 0 {
		return chosenDirs(p.Dir, p.NodeModules)
	}
	return foundDirs(p.Dir)
}

// chosenDirs expands the folders a project names (patterns included) to existing folders.
func chosenDirs(repo string, names []string) []string {
	var out []string
	for _, n := range names {
		matches, _ := filepath.Glob(filepath.Join(repo, filepath.FromSlash(n)))
		for _, m := range matches {
			if st, err := os.Stat(m); err != nil || !st.IsDir() {
				continue
			}
			if rel, err := filepath.Rel(repo, m); err == nil && rel != "." && !slices.Contains(out, rel) {
				out = append(out, filepath.ToSlash(rel))
			}
		}
	}
	slices.Sort(out)
	return out
}

// foundDirs walks the repo for folders with a package.json.
func foundDirs(repo string) []string {
	var found []string
	_ = filepath.WalkDir(repo, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == repo {
			return nil
		}
		rel, _ := filepath.Rel(repo, path)
		name := d.Name()
		if strings.HasPrefix(name, ".") || slices.Contains(skipDirs, name) || strings.Count(rel, string(filepath.Separator)) >= maxPackageDepth {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	return notIgnored(repo, found)
}

// notIgnored drops the folders git ignores (one git call for all of them). git runs on the host
// here, in a repo the sandbox can write: --no-index and core.fsmonitor=false keep it from running
// anything the repo's settings could name (see docs/HOST-EXECUTION.md).
func notIgnored(repo string, dirs []string) []string {
	if len(dirs) == 0 {
		return nil
	}
	cmd := exec.Command("git", "-C", repo, "-c", "core.fsmonitor=false", "check-ignore", "--no-index", "--stdin")
	cmd.Stdin = strings.NewReader(strings.Join(dirs, "\n") + "\n")
	out, _ := cmd.Output() // exit 1: none ignored; not a repo: keep them all
	ignored := strings.Split(string(bytes.TrimSpace(out)), "\n")
	return slices.DeleteFunc(dirs, func(d string) bool { return slices.Contains(ignored, d) })
}

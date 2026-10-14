// Package paths resolves jailwick's folders on the host.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// Home is the user's home folder ("" if unknown).
func Home() string {
	h, _ := os.UserHomeDir()
	return h
}

// ConfigDir holds the user's configuration: $JAILWICK_HOME, else $XDG_CONFIG_HOME/jailwick,
// else ~/.config/jailwick (also on macOS, so dotfile managers find it).
func ConfigDir() string {
	if d := os.Getenv("JAILWICK_HOME"); d != "" {
		return Expand(d)
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "jailwick")
	}
	return filepath.Join(Home(), ".config", "jailwick")
}

// StateDir holds generated files (safe to delete): $JAILWICK_STATE, else
// $XDG_STATE_HOME/jailwick, else ~/.local/state/jailwick.
func StateDir() string {
	if d := os.Getenv("JAILWICK_STATE"); d != "" {
		return Expand(d)
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "jailwick")
	}
	return filepath.Join(Home(), ".local", "state", "jailwick")
}

// ProjectState is the generated-files folder of one project.
func ProjectState(project string) string { return filepath.Join(StateDir(), "projects", project) }

// DataDir holds data jailwick keeps and can't regenerate (a persistent blocked log, approvals,
// session records): $JAILWICK_DATA, else $XDG_DATA_HOME/jailwick, else ~/.local/share/jailwick.
// Unlike StateDir it is never "safe to delete"; a project's part goes with the project's data
// (jw destroy, jw project rm --purge).
func DataDir() string {
	if d := os.Getenv("JAILWICK_DATA"); d != "" {
		return Expand(d)
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "jailwick")
	}
	return filepath.Join(Home(), ".local", "share", "jailwick")
}

// ProjectData is the kept-data folder of one project.
func ProjectData(project string) string { return filepath.Join(DataDir(), "projects", project) }

// Expand replaces a leading ~ with the home folder and cleans the path.
func Expand(p string) string {
	if p == "~" {
		return Home()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(Home(), p[2:])
	}
	return p
}

// Real returns the absolute, symlink-resolved form of p (or the cleaned absolute path if it
// doesn't exist).
func Real(p string) string {
	p = Expand(p)
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// Within reports whether child is parent or inside it (both already cleaned).
func Within(child, parent string) bool {
	if child == parent {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// Tilde shortens a path under the home folder to ~/… for display.
func Tilde(p string) string {
	h := Home()
	if h != "" && Within(p, h) {
		if p == h {
			return "~"
		}
		return "~" + strings.TrimPrefix(p, h)
	}
	return p
}

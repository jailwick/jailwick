package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Homebrew names: the tap's formula (the CLI) and cask (the desktop app).
const (
	Formula = "jailwick/tap/jailwick"
	Cask    = "jailwick/tap/jailwick-app"
)

// Install is how the running jailwick was installed, and so how it updates.
type Install struct {
	// Method: homebrew (Update runs it), or download (the release page has it; also builds
	// from source).
	Method string `json:"method"`
	// Command is what updates it, for a terminal ("" for a download).
	Command string `json:"command,omitempty"`
	brew    string // brew's path
	formula bool   // the CLI is Homebrew's
	cask    bool   // the app is Homebrew's
}

// Homebrew reports whether Homebrew updates this jailwick.
func (i Install) Homebrew() bool { return i.Method == "homebrew" }

// brewPaths are where Homebrew lives when brew isn't on the PATH (an app opened from the Finder).
var brewPaths = []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew", "/home/linuxbrew/.linuxbrew/bin/brew"}

// Detect finds how the running executable was installed. It is Homebrew's when it is the
// formula's binary (in the Cellar) or the cask's app (installed to an Applications folder); the
// other half is updated along with it when Homebrew has it too, so they stay the same version.
func Detect() Install {
	exe, err := os.Executable()
	if err == nil {
		exe, _ = filepath.EvalSymlinks(exe)
	}
	return detect(exe, findBrew(), os.Getenv("HOME"))
}

func detect(exe, brew, home string) Install {
	download := Install{Method: "download"}
	if brew == "" || exe == "" || runtime.GOOS == "windows" {
		return download
	}
	prefix := filepath.Dir(filepath.Dir(brew)) // <prefix>/bin/brew
	formula := dirExists(filepath.Join(prefix, "Cellar", "jailwick"))
	cask := dirExists(filepath.Join(prefix, "Caskroom", "jailwick-app"))
	fromFormula := formula && strings.HasPrefix(exe, filepath.Join(prefix, "Cellar", "jailwick")+string(filepath.Separator))
	fromCask := false
	for _, apps := range []string{"/Applications", filepath.Join(home, "Applications")} {
		if strings.HasPrefix(exe, filepath.Join(apps, "jailwick.app")+string(filepath.Separator)) {
			fromCask = cask
		}
	}
	if !fromFormula && !fromCask {
		return download
	}
	in := Install{Method: "homebrew", brew: brew, formula: formula, cask: cask}
	var cmds []string
	for _, args := range in.commands() {
		cmds = append(cmds, "brew "+strings.Join(args, " "))
	}
	in.Command = strings.Join(cmds, " && ")
	return in
}

// commands are brew's arguments for the update: the CLI and the app, those Homebrew has.
func (i Install) commands() [][]string {
	var out [][]string
	if i.formula {
		out = append(out, []string{"upgrade", Formula})
	}
	if i.cask {
		out = append(out, []string{"upgrade", "--cask", Cask})
	}
	return out
}

// Upgrade runs Homebrew's update, writing its output to out. It doesn't restart anything: the
// running app or CLI goes on as it is, and sandboxes take the new helper at their next restart.
func (i Install) Upgrade(ctx context.Context, out io.Writer) error {
	if !i.Homebrew() {
		return fmt.Errorf("jailwick wasn't installed with Homebrew: download the new version from its release page")
	}
	for _, args := range i.commands() {
		fmt.Fprintf(out, "$ brew %s\n", strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, i.brew, args...)
		cmd.Stdout, cmd.Stderr = out, out
		cmd.Env = append(os.Environ(), "HOMEBREW_NO_ENV_HINTS=1", "HOMEBREW_NO_INSTALL_CLEANUP=1")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("brew %s: %w", strings.Join(args, " "), err)
		}
	}
	return nil
}

// findBrew is brew's path as found, not resolved: on Intel Macs /usr/local/bin/brew links into
// /usr/local/Homebrew, but the prefix (with the Cellar) is /usr/local.
func findBrew() string {
	if p, err := exec.LookPath("brew"); err == nil {
		return p
	}
	for _, p := range brewPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

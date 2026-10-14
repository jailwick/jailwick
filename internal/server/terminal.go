package server

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
)

// Terminals are the terminal apps OpenTerminal knows by name; config.yaml's `terminal` can also
// be a command template containing {cmd}.
var Terminals = []string{"terminal", "iterm2", "ghostty", "kitty", "wezterm"}

// Session is a session to start in a project: Claude (or a shell) in the main checkout or a
// worktree, or a saved conversation resumed in the folder it ran in.
type Session struct {
	Worktree string `json:"worktree"` // a worktree's name; "" for the main checkout
	Command  string `json:"command"`  // claude (default), shell, or folder: a plain terminal in Dir (no jailwick)
	Open     bool   `json:"open"`     // the whole internet for this session (claude only)
	Private  bool   `json:"private"`  // kept out of messages between sessions (claude only)
	Resume   string `json:"resume"`   // a conversation id to resume
	Dir      string `json:"dir"`      // the conversation's folder, for Resume; the folder, for folder
}

// SessionCommand is the shell command that starts s: what OpenTerminal runs, and what the app
// offers to copy for a terminal it can't open. It refuses a folder outside the project.
func SessionCommand(p config.Project, s Session) (string, error) {
	dir, run, err := sessionParts(p, s)
	if err != nil || run == "" {
		return "cd " + shellPath(dir), err
	}
	return "cd " + shellPath(dir) + " && " + run, nil
}

// sessionParts is where s starts and the jailwick command it runs there ("" for a plain
// terminal in the folder).
func sessionParts(p config.Project, s Session) (dir, run string, err error) {
	repo := paths.Real(p.Dir)
	dir = repo
	args := []string{"claude"}
	if s.Dir != "" && (s.Resume != "" || s.Command == "folder") {
		d := filepath.Clean(s.Dir)
		if d != repo && !paths.Within(d, repo) {
			return repo, "", badInput("dir: not in the project")
		}
		dir = d
	}
	switch {
	case s.Command == "folder":
		return dir, "", nil
	case s.Resume != "":
		if err := agentapi.CheckID(s.Resume); err != nil {
			return repo, "", err
		}
		args = append(args, s.Resume)
	case s.Command == "shell":
		args = []string{"shell"}
	}
	if s.Worktree != "" && s.Resume == "" {
		if wt, ok := worktreeDir(repo, s.Worktree); ok {
			dir = wt
		} else { // a new worktree: start from the repo and name it
			args = append([]string{"wt", s.Worktree}, args[1:]...)
		}
	}
	if args[0] != "shell" {
		if s.Open {
			args = append(args, "--open")
		}
		if s.Private {
			args = append(args, "--private")
		}
	}
	words := make([]string, len(args))
	for i, a := range args {
		words[i] = shellWord(a)
	}
	return dir, cliName() + " " + strings.Join(words, " "), nil
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9._/@:+=-]+$`)

// shellWord is s as one shell word: as is when nothing in it is special, else quoted.
func shellWord(s string) string {
	if plainWord.MatchString(s) {
		return s
	}
	return shellQuote(s)
}

// shellPath is a folder for cd: ~/… under the home folder (the ~ outside quotes, so it expands).
func shellPath(dir string) string {
	if rel, ok := strings.CutPrefix(dir, paths.Home()+"/"); ok {
		return "~/" + shellWord(rel)
	}
	return shellWord(dir)
}

// worktreeDir is an existing worktree's folder (in either worktree folder).
func worktreeDir(repo, name string) (string, bool) {
	for _, f := range agentapi.WorktreeFolders {
		dir := filepath.Join(repo, f, strings.ReplaceAll(name, "/", "-"))
		if _, err := os.Stat(dir); err == nil {
			return dir, true
		}
	}
	return "", false
}

// OpenTerminal starts a session in the user's terminal app: terminal is config.yaml's
// `terminal` (see Terminals; "" is macOS Terminal).
func OpenTerminal(terminal string, p config.Project, s Session) error {
	cmd, err := SessionCommand(p, s)
	if err != nil {
		return err
	}
	shell := []string{"bash", "-lc", cmd + "; exec $SHELL"}
	mac := runtime.GOOS == "darwin"
	switch terminal {
	case "", "terminal":
		if !mac {
			return errors.New("set the terminal in Settings (kitty, wezterm, or a command with {cmd})")
		}
		return osascript(`tell application "Terminal" to do script ` + appleString(cmd) + "\n" + `tell application "Terminal" to activate`)
	case "iterm2":
		return osascript(`tell application "iTerm" to create window with default profile command ` + appleString("bash -lc "+shellQuote(cmd+"; exec $SHELL")))
	case "ghostty":
		return exec.Command("open", append([]string{"-na", "Ghostty", "--args", "-e"}, shell...)...).Start()
	case "kitty":
		if mac { // an app started from the Finder doesn't have kitty on its PATH
			return exec.Command("open", append([]string{"-na", "kitty", "--args"}, shell...)...).Start()
		}
		return exec.Command("kitty", shell...).Start()
	case "wezterm":
		if mac {
			return exec.Command("open", append([]string{"-na", "WezTerm", "--args", "start", "--"}, shell...)...).Start()
		}
		return exec.Command("wezterm", append([]string{"start", "--"}, shell...)...).Start()
	}
	if !strings.Contains(terminal, "{cmd}") {
		return fmt.Errorf("terminal %q: use %s, or a command with {cmd}", terminal, strings.Join(Terminals, ", "))
	}
	return exec.Command("sh", "-c", strings.ReplaceAll(terminal, "{cmd}", shellQuote(cmd))).Start()
}

// CheckTerminal reports whether a `terminal` setting is one OpenTerminal can use.
func CheckTerminal(t string) error {
	if t == "" || slices.Contains(Terminals, t) || strings.Contains(t, "{cmd}") {
		return nil
	}
	return badInput(fmt.Sprintf("terminal %q: use %s, or a command with {cmd}", t, strings.Join(Terminals, ", ")))
}

// cliName is how a command line names jailwick: jw or jailwick when the shell finds them, else
// the full path (the desktop app is a different binary).
func cliName() string {
	for _, n := range []string{"jw", "jailwick"} {
		if _, err := exec.LookPath(n); err == nil {
			return n
		}
	}
	if exe, err := os.Executable(); err == nil && strings.HasSuffix(filepath.Base(exe), "jailwick") {
		return shellWord(exe)
	}
	for _, d := range []string{filepath.Join(paths.Home(), ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin"} {
		if f := filepath.Join(d, "jailwick"); fileExists(f) {
			return shellWord(f)
		}
	}
	return "jailwick"
}

func fileExists(f string) bool {
	_, err := os.Stat(f)
	return err == nil
}

func osascript(script string) error {
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("opening the terminal: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

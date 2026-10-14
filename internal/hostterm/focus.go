package hostterm

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Result says what Focus brought forward.
type Result struct {
	// Found is "tab" (the session's own window and tab are in front), "app" (the terminal app is,
	// but not necessarily that tab) or "" (nothing: Note says why).
	Found string `json:"found"`
	App   string `json:"app,omitempty"` // the terminal's name
	Note  string `json:"note,omitempty"`
}

// Terminal.app and iTerm2 are asked, by AppleScript, for the tab on the session's tty; it comes
// as an argument, never as script text. macOS asks once to let jailwick control the app.
const (
	terminalScript = `on run argv
	tell application "Terminal"
		repeat with w in windows
			repeat with t in tabs of w
				if tty of t is (item 1 of argv) then
					set selected of t to true
					set index of w to 1
					activate
					return "tab"
				end if
			end repeat
		end repeat
	end tell
	return ""
end run`
	itermScript = `on run argv
	tell application "iTerm2"
		repeat with w in windows
			repeat with t in tabs of w
				repeat with s in sessions of t
					if tty of s is (item 1 of argv) then
						select w
						tell t to select
						tell s to select
						activate
						return "tab"
					end if
				end repeat
			end repeat
		end repeat
	end tell
	return ""
end run`
)

// Focus brings the terminal of a session forward: its own tab where the terminal can be asked
// for it (Terminal, iTerm2, WezTerm, kitty with remote control), else the app.
func Focus(ctx context.Context, t Terminal) Result {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	name := t.Name()
	r := Result{App: name}
	if !t.Known() {
		r.Note = "This session didn't record its terminal: it was started by an older jailwick, or not from a terminal."
		return r
	}
	mac := runtime.GOOS == "darwin"
	tab, denied := false, false
	ask := func(script string) {
		if mac && t.TTY != "" {
			out, err := osascript(ctx, script, t.TTY)
			tab, denied = out == "tab", notAllowed(err)
		}
	}
	switch strings.ToLower(t.App) {
	case "apple_terminal":
		ask(terminalScript)
	case "iterm.app":
		ask(itermScript)
	case "wezterm":
		if bin := find("wezterm", "/Applications/WezTerm.app/Contents/MacOS/wezterm"); bin != "" && t.Pane != "" {
			tab = exec.CommandContext(ctx, bin, "cli", "activate-pane", "--pane-id", t.Pane).Run() == nil
		}
	case "kitty":
		if bin := find("kitty", "/Applications/kitty.app/Contents/MacOS/kitty"); bin != "" && t.Pane != "" && t.Socket != "" {
			tab = exec.CommandContext(ctx, bin, "@", "--to", t.Socket, "focus-window", "--match", "id:"+t.Pane).Run() == nil
		}
	}
	// The app to the front: for the tab just picked, and as all there is for the others.
	front := mac && t.Bundle != "" && exec.CommandContext(ctx, "open", "-b", t.Bundle).Run() == nil
	switch {
	case tab:
		r.Found = "tab"
	case front:
		r.Found = "app"
		r.Note = whyNotTab(t, name)
		if denied {
			r.Note = "jailwick isn't allowed to control " + name + ": allow it in System Settings → Privacy & Security → Automation, then try again."
		}
	default:
		r.Note = "Couldn't bring " + orTheTerminal(name) + " forward"
		if t.TTY != "" {
			r.Note += " (the session's tab is " + t.TTY + ")"
		}
		r.Note += ": it may have been closed, or it isn't one jailwick can ask."
	}
	return r
}

// whyNotTab says why only the app came forward.
func whyNotTab(t Terminal, name string) string {
	switch strings.ToLower(t.App) {
	case "apple_terminal", "iterm.app":
		return "No tab of " + name + " runs the session any more (" + t.TTY + "): it may have been closed."
	case "kitty":
		if t.Socket == "" {
			return "kitty can't be asked for one tab unless remote control is on (allow_remote_control and listen_on in kitty.conf)."
		}
		return "kitty didn't find the session's window: it may have been closed."
	case "wezterm":
		return "WezTerm didn't find the session's pane: it may have been closed."
	case "tmux":
		return "The session runs in tmux: switch to its window there."
	}
	return orTheTerminal(name) + " can't be asked for one tab: look for the session there" + inTTY(t) + "."
}

func inTTY(t Terminal) string {
	if t.TTY == "" {
		return ""
	}
	return " (its tab is " + t.TTY + ")"
}

func orTheTerminal(name string) string {
	if name == "" {
		return "The terminal"
	}
	return name
}

func osascript(ctx context.Context, script, arg string) (string, error) {
	cmd := exec.CommandContext(ctx, "osascript", "-e", script, "--", arg)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New(stderr.String())
	}
	return strings.TrimSpace(string(out)), nil
}

// notAllowed is osascript's error when the user hasn't let jailwick control the app (-1743).
func notAllowed(err error) bool { return err != nil && strings.Contains(err.Error(), "-1743") }

// find is a command on the PATH, else the first of paths that exists (an app started from the
// Finder doesn't have the terminals' commands on its PATH).
func find(name string, paths ...string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, p := range paths {
		if p, err := exec.LookPath(filepath.Clean(p)); err == nil {
			return p
		}
	}
	return ""
}

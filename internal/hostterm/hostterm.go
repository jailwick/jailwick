// Package hostterm knows the host terminal a session was started in, and brings it to the front.
//
// jw claude (and jw wt, jw resume) records its terminal as it starts a session: which app, the
// tty, and the app's own id for the tab where it has one. The record is a label on the session's
// container, set on the host (a sandbox can't change its labels), so the app can later bring that
// terminal forward: the app has no terminal of its own, the session's screen is that terminal's.
package hostterm

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Terminal is where a session runs on the host.
type Terminal struct {
	App    string // TERM_PROGRAM (Apple_Terminal, iTerm.app, WezTerm, ghostty, vscode…), or kitty
	Bundle string // the macOS app's bundle id (__CFBundleIdentifier): an app to bring forward
	TTY    string // /dev/ttys004, /dev/pts/3
	Pane   string // the app's id for the tab: WezTerm's pane, kitty's window
	Socket string // kitty's remote control address (KITTY_LISTEN_ON), unix: only
}

// Current is the terminal this process runs in (zero when it has none).
func Current() Terminal {
	return current(os.Getenv, ttyOf(os.Getpid()))
}

func current(env func(string) string, tty string) Terminal {
	t := Terminal{App: env("TERM_PROGRAM"), Bundle: env("__CFBundleIdentifier"), TTY: tty}
	switch {
	case env("KITTY_WINDOW_ID") != "":
		t.App, t.Pane, t.Socket = "kitty", env("KITTY_WINDOW_ID"), env("KITTY_LISTEN_ON")
	case env("WEZTERM_PANE") != "":
		t.App, t.Pane = "WezTerm", env("WEZTERM_PANE")
	}
	return t.valid()
}

// ttyOf is the terminal device a process runs on ("" for none), as ps names it.
func ttyOf(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "tty=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(out))
	if name == "" || name == "?" || name == "??" {
		return ""
	}
	return "/dev/" + name
}

// What each field may hold. A record is passed to osascript and terminal CLIs as arguments,
// never as script text, but it is checked anyway: what doesn't match is dropped.
var (
	reApp    = regexp.MustCompile(`^[A-Za-z0-9._ -]{1,40}$`)
	reBundle = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
	reTTY    = regexp.MustCompile(`^/dev/(ttys[0-9]{1,4}|pts/[0-9]{1,4})$`)
	rePane   = regexp.MustCompile(`^[0-9]{1,9}$`)
	reSocket = regexp.MustCompile(`^unix:/[A-Za-z0-9._/@-]{1,200}$`)
)

func (t Terminal) valid() Terminal {
	keep := func(v string, re *regexp.Regexp) string {
		if re.MatchString(v) {
			return v
		}
		return ""
	}
	return Terminal{App: keep(t.App, reApp), Bundle: keep(t.Bundle, reBundle), TTY: keep(t.TTY, reTTY),
		Pane: keep(t.Pane, rePane), Socket: keep(t.Socket, reSocket)}
}

// Known reports whether there is anything to find the terminal by.
func (t Terminal) Known() bool { return t != Terminal{} }

// Encode is the record as a container label's value; "" for an unknown terminal.
func (t Terminal) Encode() string {
	v := url.Values{}
	for k, s := range map[string]string{"app": t.App, "bundle": t.Bundle, "tty": t.TTY, "pane": t.Pane, "socket": t.Socket} {
		if s != "" {
			v.Set(k, s)
		}
	}
	return v.Encode()
}

// Decode reads a label's value (Encode); what isn't valid is dropped.
func Decode(s string) Terminal {
	v, err := url.ParseQuery(s)
	if err != nil {
		return Terminal{}
	}
	return Terminal{App: v.Get("app"), Bundle: v.Get("bundle"), TTY: v.Get("tty"), Pane: v.Get("pane"), Socket: v.Get("socket")}.valid()
}

// Name is the terminal app's name, for people ("" when unknown).
func (t Terminal) Name() string {
	switch strings.ToLower(t.App) {
	case "apple_terminal":
		return "Terminal"
	case "iterm.app":
		return "iTerm2"
	case "wezterm":
		return "WezTerm"
	case "kitty":
		return "kitty"
	case "ghostty":
		return "Ghostty"
	case "vscode":
		return "VS Code" // or a fork of it: Cursor, Windsurf…
	case "warpterminal":
		return "Warp"
	case "tmux":
		return "tmux"
	}
	return t.App
}

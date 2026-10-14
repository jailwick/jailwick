package notify

// A session tells you it waits for you through its terminal or through the app
// (config session_notifications). The terminal is better where it can show a notification: a
// click on it brings back the very tab the session runs in. Claude Code does it (its
// preferredNotifChannel), for the terminals below.

// native maps a terminal (TERM_PROGRAM, or Terminal's guess) to the Claude Code channel that
// shows a notification in it. WezTerm takes iTerm2's.
var native = map[string]string{
	"iTerm.app": "iterm2",
	"WezTerm":   "iterm2",
	"ghostty":   "ghostty",
	"kitty":     "kitty",
}

// Terminal names the terminal a session runs in from its environment: TERM_PROGRAM, or kitty's
// own variables (kitty doesn't set TERM_PROGRAM). "" when unknown.
func Terminal(env func(string) string) string {
	if t := env("TERM_PROGRAM"); t != "" {
		return t
	}
	if env("KITTY_WINDOW_ID") != "" || env("TERM") == "xterm-kitty" {
		return "kitty"
	}
	return ""
}

// Channel is the Claude Code notification channel a session in terminal gets, for a
// session_notifications mode: the terminal's own where it has one (auto, terminal); its bell
// where it hasn't (terminal); none where the app tells, or nobody does (auto without one, app,
// off).
func Channel(mode, terminal string) string {
	if mode == "app" || mode == "off" {
		return "notifications_disabled"
	}
	if c, ok := native[terminal]; ok {
		return c
	}
	if mode == "terminal" {
		return "terminal_bell"
	}
	return "notifications_disabled"
}

// AppNotifies reports whether the app tells about a session that waits, for a
// session_notifications mode and the channel the session got (Channel; "" from a session started
// before there were channels): in app mode, and in auto mode when its terminal doesn't.
func AppNotifies(mode, channel string) bool {
	switch mode {
	case "app":
		return true
	case "", "auto":
		for _, c := range native {
			if c == channel {
				return false
			}
		}
		return true
	}
	return false
}

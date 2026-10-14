package notify

import "testing"

func TestTerminal(t *testing.T) {
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"TERM_PROGRAM": "iTerm.app"}, "iTerm.app"},
		{map[string]string{"TERM": "xterm-kitty"}, "kitty"},
		{map[string]string{"KITTY_WINDOW_ID": "3"}, "kitty"},
		{map[string]string{"TERM": "xterm-256color"}, ""},
	} {
		if got := Terminal(func(k string) string { return c.env[k] }); got != c.want {
			t.Errorf("%v: %q, want %q", c.env, got, c.want)
		}
	}
}

func TestChannelAndAppNotifies(t *testing.T) {
	for _, c := range []struct {
		mode, terminal, channel string
		app                     bool
	}{
		{"auto", "iTerm.app", "iterm2", false},
		{"auto", "WezTerm", "iterm2", false},
		{"auto", "ghostty", "ghostty", false},
		{"auto", "kitty", "kitty", false},
		{"auto", "Apple_Terminal", "notifications_disabled", true}, // no notifications there: the app tells
		{"auto", "", "notifications_disabled", true},
		{"terminal", "Apple_Terminal", "terminal_bell", false},
		{"terminal", "iTerm.app", "iterm2", false},
		{"app", "iTerm.app", "notifications_disabled", true},
		{"off", "iTerm.app", "notifications_disabled", false},
	} {
		ch := Channel(c.mode, c.terminal)
		if ch != c.channel || AppNotifies(c.mode, ch) != c.app {
			t.Errorf("%s in %q: channel %q, app %v; want %q, %v", c.mode, c.terminal, ch, AppNotifies(c.mode, ch), c.channel, c.app)
		}
	}
	if !AppNotifies("auto", "") {
		t.Error("a session from before channels: the app tells, as it did")
	}
}

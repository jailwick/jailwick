package hostterm

import (
	"context"
	"testing"
)

func TestCurrent(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	iterm := current(env(map[string]string{"TERM_PROGRAM": "iTerm.app", "__CFBundleIdentifier": "com.googlecode.iterm2"}), "/dev/ttys004")
	if iterm != (Terminal{App: "iTerm.app", Bundle: "com.googlecode.iterm2", TTY: "/dev/ttys004"}) || iterm.Name() != "iTerm2" {
		t.Fatalf("iTerm2: %+v", iterm)
	}
	kitty := current(env(map[string]string{"TERM_PROGRAM": "", "KITTY_WINDOW_ID": "3", "KITTY_LISTEN_ON": "unix:/tmp/kitty-1"}), "")
	if kitty.App != "kitty" || kitty.Pane != "3" || kitty.Socket != "unix:/tmp/kitty-1" {
		t.Fatalf("kitty: %+v", kitty)
	}
	wez := current(env(map[string]string{"TERM_PROGRAM": "WezTerm", "WEZTERM_PANE": "12"}), "/dev/pts/2")
	if wez.Pane != "12" || wez.Name() != "WezTerm" {
		t.Fatalf("WezTerm: %+v", wez)
	}
	if (current(env(nil), "")).Known() {
		t.Fatal("no terminal: nothing known")
	}
}

// A record goes through a label and back; what isn't valid is dropped.
func TestEncodeDecode(t *testing.T) {
	in := Terminal{App: "Apple_Terminal", Bundle: "com.apple.Terminal", TTY: "/dev/ttys012"}
	if got := Decode(in.Encode()); got != in {
		t.Fatalf("round trip: %+v", got)
	}
	bad := Decode("app=x%22%3Bdo%20shell&tty=/etc/passwd&bundle=a;b&pane=1%3Brm&socket=tcp:1.2.3.4:1")
	if bad != (Terminal{}) {
		t.Fatalf("invalid fields kept: %+v", bad)
	}
	if (Terminal{}).Encode() != "" {
		t.Fatal("an unknown terminal has no label")
	}
}

func TestFocusUnknown(t *testing.T) {
	if r := Focus(context.Background(), Terminal{}); r.Found != "" || r.Note == "" {
		t.Fatalf("%+v", r)
	}
}

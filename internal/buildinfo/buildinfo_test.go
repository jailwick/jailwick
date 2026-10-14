package buildinfo

import (
	"regexp"
	"testing"
)

func TestVersion(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Fatalf("VERSION must be major.minor.patch: %q", Version)
	}
}

func TestString(t *testing.T) {
	for _, c := range []struct {
		in   Info
		want string
	}{
		{Info{Version: "0.2.5"}, "0.2.5"},
		{Info{Version: "0.2.5", Commit: "c53f880", Date: "2026-10-15"}, "0.2.5 (c53f880, 15 Oct 2026)"},
		{Info{Version: "0.2.5", Commit: "c53f880", Date: "2026-10-15", Modified: true}, "0.2.5 (c53f880 with changes, 15 Oct 2026)"},
	} {
		if got := c.in.String(); got != c.want {
			t.Errorf("%+v: %q, want %q", c.in, got, c.want)
		}
	}
}

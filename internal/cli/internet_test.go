package cli

import (
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/proxy"
)

func TestInternetLabel(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		in   *proxy.Internet
		want string
	}{
		{nil, ""},
		{&proxy.Internet{}, "allowlist"},
		{&proxy.Internet{Open: true}, "open (until closed)"},
		{&proxy.Internet{Open: true, Until: now.Add(14*time.Minute + 40*time.Second)}, "open (15m left)"},
		{&proxy.Internet{Open: true, Until: now.Add(2 * time.Hour)}, "open (2h left)"},
		{&proxy.Internet{Open: true, Until: now.Add(90 * time.Minute)}, "open (1h30m left)"},
		{&proxy.Internet{Open: true, Until: now.Add(20 * time.Second)}, "open (closing)"},
	} {
		if got := internetLabel(c.in, now); got != c.want {
			t.Errorf("internetLabel(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

package proxy

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Internet is whether a session's proxy lets it reach every destination (except the host and
// private networks, which stay refused), and until when.
type Internet struct {
	Open bool `json:"open"`
	// Until is when the opening ends; zero: until it is closed.
	Until time.Time `json:"until,omitzero"`
}

// MaxOpen is the longest timed opening.
const MaxOpen = 24 * time.Hour

// CheckDuration accepts 0 (until closed) or a duration from a minute to MaxOpen.
func CheckDuration(d time.Duration) error {
	if d != 0 && (d < time.Minute || d > MaxOpen) {
		return fmt.Errorf("open for 1m to %s, or 0 for until closed (got %s)", MaxOpen, d)
	}
	return nil
}

// OpenCommand opens a proxy for d (0: until closed). It runs in the proxy container.
func OpenCommand(d time.Duration) []string {
	return []string{"jailwick-proxy", "open", strconv.FormatInt(int64(d/time.Second), 10)}
}

// StatusCommand prints a proxy's open state. It runs in the proxy container.
var StatusCommand = []string{"jailwick-proxy", "status"}

// ParseStatus reads the output of StatusCommand. The end is taken relative to the container's own
// clock and placed on the host's, so a clock difference between the two doesn't shift it.
func ParseStatus(out string, hostNow time.Time) (Internet, error) {
	f := strings.Fields(out)
	if len(f) != 3 || (f[0] != "open" && f[0] != "closed") {
		return Internet{}, fmt.Errorf("unexpected proxy status %q", strings.TrimSpace(out))
	}
	until, err1 := strconv.ParseInt(f[1], 10, 64)
	now, err2 := strconv.ParseInt(f[2], 10, 64)
	if err1 != nil || err2 != nil {
		return Internet{}, fmt.Errorf("unexpected proxy status %q", strings.TrimSpace(out))
	}
	in := Internet{Open: f[0] == "open"}
	if in.Open && until > 0 {
		in.Until = hostNow.Add(time.Duration(until-now) * time.Second).Truncate(time.Second)
	}
	return in, nil
}

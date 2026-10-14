package proxy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/assets"
)

func TestCheckDuration(t *testing.T) {
	for d, ok := range map[time.Duration]bool{0: true, time.Minute: true, 15 * time.Minute: true, MaxOpen: true,
		30 * time.Second: false, MaxOpen + time.Minute: false, -time.Minute: false} {
		if err := CheckDuration(d); (err == nil) != ok {
			t.Errorf("CheckDuration(%s) = %v", d, err)
		}
	}
}

func TestParseStatus(t *testing.T) {
	host := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		out  string
		want Internet
	}{
		{"closed 0 1000\n", Internet{}},
		{"open 0 1000\n", Internet{Open: true}},
		// 15 minutes left on the container's clock, whatever its time is.
		{"open 1900 1000\n", Internet{Open: true, Until: host.Add(15 * time.Minute)}},
	}
	for _, c := range cases {
		got, err := ParseStatus(c.out, host)
		if err != nil || got != c.want {
			t.Errorf("ParseStatus(%q) = %+v, %v; want %+v", c.out, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "open", "maybe 0 1", "open x 1"} {
		if _, err := ParseStatus(bad, host); err == nil {
			t.Errorf("ParseStatus(%q): want an error", bad)
		}
	}
}

// The order of http_access rules is the security boundary: private destinations are refused
// before an open session is let through, and the allowlist only after the port rules.
func TestSquidRuleOrder(t *testing.T) {
	var rules []string
	for _, l := range strings.Split(string(assets.MustRead("images/proxy/squid.conf")), "\n") {
		if strings.HasPrefix(l, "http_access ") {
			rules = append(rules, strings.TrimPrefix(l, "http_access "))
		}
	}
	want := []string{"deny private_dst", "allow open_all", "deny !Safe_ports", "deny CONNECT !SSL_ports",
		"allow allowed_domains", "deny all"}
	if strings.Join(rules, "|") != strings.Join(want, "|") {
		t.Fatalf("http_access rules:\n%s\nwant:\n%s", strings.Join(rules, "\n"), strings.Join(want, "\n"))
	}
}

// proxyScript runs the image's jailwick-proxy with fake squid and nc, in a temp state folder.
func proxyScript(t *testing.T) (run func(args ...string) (string, error), state string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	state = filepath.Join(dir, "state")
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(state, 0o755)
	os.MkdirAll(bin, 0o755)
	script := filepath.Join(dir, "jailwick-proxy")
	os.WriteFile(script, assets.MustRead("images/proxy/jailwick-proxy"), 0o755)
	// squid -N runs until killed; squid -k only records the call.
	os.WriteFile(filepath.Join(bin, "squid"), []byte("#!/bin/sh\n"+
		"echo \"$@\" >> \""+state+"/squid-calls\"\n"+
		"case \"$1\" in -N) exec sleep 600;; esac\n"), 0o755)
	os.WriteFile(filepath.Join(bin, "nc"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	env := append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "JAILWICK_PROXY_STATE="+state)
	return func(args ...string) (string, error) {
		cmd := exec.Command("sh", append([]string{script}, args...)...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}, state
}

func TestProxyScriptOpenAndStatus(t *testing.T) {
	run, state := proxyScript(t)
	os.WriteFile(filepath.Join(state, "open-acl"), nil, 0o644) // as `run` leaves it

	if out, err := run("status"); err != nil || !strings.HasPrefix(out, "closed 0 ") {
		t.Fatalf("status: %q %v", out, err)
	}
	out, err := run("open", "900")
	if err != nil || !strings.HasPrefix(out, "open ") {
		t.Fatalf("open: %q %v", out, err)
	}
	in, err := ParseStatus(out, time.Now())
	if err != nil || !in.Open || time.Until(in.Until) < 14*time.Minute {
		t.Fatalf("open for 15m: %+v %v", in, err)
	}
	if acl, _ := os.ReadFile(filepath.Join(state, "open-acl")); !strings.Contains(string(acl), "0.0.0.0/0") {
		t.Fatalf("acl: %q", acl)
	}
	if calls, _ := os.ReadFile(filepath.Join(state, "squid-calls")); !strings.Contains(string(calls), "-k reconfigure") {
		t.Fatalf("squid wasn't reconfigured: %q", calls)
	}
	if out, _ := run("open", "0"); !strings.HasPrefix(out, "open 0 ") {
		t.Fatalf("until closed: %q", out)
	}
	if _, err := run("open", "-5"); err == nil {
		t.Fatal("a bad duration must fail")
	}
}

func TestProxyScriptRunClosesWhenTheWindowEnds(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the timer")
	}
	run, state := proxyScript(t)
	done := make(chan error, 1)
	go func() { _, err := run("run"); done <- err }()

	// Wait for run to start, then open with an end that has already passed.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if b, _ := os.ReadFile(filepath.Join(state, "open-until")); strings.TrimSpace(string(b)) == "0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run didn't start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	os.WriteFile(filepath.Join(state, "open-until"), []byte("1\n"), 0o644)
	os.WriteFile(filepath.Join(state, "open-acl"), []byte("0.0.0.0/0\n"), 0o644)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run should exit cleanly at the end of the window: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run kept going after the window ended")
	}
}

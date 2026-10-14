package agent

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Verify checks that the sandbox's boundaries hold. It changes nothing persistent. others are
// host:port addresses of other sandboxes' services, which must be out of reach.
func Verify(c *agentapi.Config, others []string) []agentapi.Check {
	v := &verifier{c: c, others: others}
	v.identity()
	v.filesystem()
	v.claudeLevels()
	v.peers()
	v.workspace()
	v.allowed()
	v.blocked()
	v.isolated()
	v.tooling()
	return v.checks
}

type verifier struct {
	c       *agentapi.Config
	others  []string
	section string
	checks  []agentapi.Check
}

func (v *verifier) add(ok bool, name, detail string) {
	v.checks = append(v.checks, agentapi.Check{Section: v.section, Name: name, OK: ok, Detail: detail})
}

func (v *verifier) expect(name string, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	v.add(err == nil, name, detail)
}

func (v *verifier) refuse(name string, err error) {
	if err == nil {
		v.add(false, name, "was allowed")
		return
	}
	v.add(true, name, "")
}

func (v *verifier) info(name string) {
	v.checks = append(v.checks, agentapi.Check{Section: v.section, Name: name, OK: true, Info: true})
}

func (v *verifier) identity() {
	v.section = "Identity"
	v.add(os.Getuid() != 0, fmt.Sprintf("runs as a normal user (uid %d)", os.Getuid()), "")
	v.refuse("sudo not available", exec.Command("sudo", "-n", "true").Run())
	status, _ := os.ReadFile("/proc/self/status")
	capEff := ""
	for _, l := range strings.Split(string(status), "\n") {
		if s, ok := strings.CutPrefix(l, "CapEff:"); ok {
			capEff = strings.TrimSpace(s)
		}
	}
	v.add(strings.Trim(capEff, "0") == "", "no Linux capabilities", "CapEff "+capEff)
}

func (v *verifier) filesystem() {
	v.section = "Files"
	v.expect("repo writable at "+v.c.Dir, writable(v.c.Dir))
	v.expect("node_modules is the sandbox's own, writable", writable(filepath.Join(v.c.Dir, "node_modules")))
	for _, nm := range v.c.NodeModules[min(1, len(v.c.NodeModules)):] { // the top's is checked above
		rel, _ := filepath.Rel(v.c.Dir, nm)
		v.expect(rel+" is the sandbox's own, writable", writable(nm))
	}

	allowed := map[string]bool{"/": true, "/etc/hosts": true, "/etc/hostname": true, "/etc/resolv.conf": true,
		"/sbin/docker-init": true, "/usr/sbin/docker-init": true, "/dev/init": true}
	for _, m := range v.c.Expected.Mounts {
		allowed[m] = true
	}
	var unexpected []string
	for _, m := range mounts() {
		if allowed[m] || under(m, "/proc", "/sys", "/dev") || v.isWorkspaceMount(m) {
			continue
		}
		unexpected = append(unexpected, m)
	}
	v.add(len(unexpected) == 0, "no unexpected mounts", strings.Join(unexpected, " "))

	var writableRO []string
	for _, m := range v.c.Expected.ReadOnly {
		if writable(m) == nil {
			writableRO = append(writableRO, m)
		}
	}
	v.add(len(writableRO) == 0, fmt.Sprintf("%d read-only mounts can't be written", len(v.c.Expected.ReadOnly)), strings.Join(writableRO, " "))

	sockets := []string{"/var/run/docker.sock", "/run/docker.sock", "/run/containerd/containerd.sock"}
	found := slices.DeleteFunc(slices.Clone(sockets), func(s string) bool { _, err := os.Stat(s); return err != nil })
	v.add(len(found) == 0, "no container runtime socket", strings.Join(found, " "))
	keys, _ := filepath.Glob(filepath.Join(v.c.Home, ".ssh", "id_*"))
	host, _ := filepath.Glob("/Users/*/.ssh")
	host2, _ := filepath.Glob("/home/*/.ssh")
	v.add(len(keys)+len(host)+len(host2) == 0, "no SSH keys or host .ssh folders", strings.Join(append(append(keys, host...), host2...), " "))
}

// claudeEntries are what Claude Code keeps at the top of ~/.claude, as of the version jailwick
// was checked against. Entries named "<entry>.<anything>" are
// variants of one (locks, backups, stamps).
var claudeEntries = []string{
	// The login's level: the account, its caches and Claude's own settings.
	".credentials.json", ".oauth_refresh.lock", ".claude.json", ".config.json", ".device-keys.json",
	".last-cleanup", ".last-update-result.json", "backups", "cache", "stats-cache.json", "state",
	"statsig", "remote-settings.json", "policy-limits.json", "settings.json", "keybindings.json",
	"plugins", "skills", "agents", "commands", "hooks", "output-styles", "workflows", "routines",
	"themes", "rules", "mcp-discovery-cache", "mcp-skill-archives", "usage-data", "uploads",
	"scheduled_tasks.json", "launch.json", "daemon.json", "CLAUDE.md",
	// Mounted from the project (claude-config) or kept per container.
	"projects", "file-history", "session-env", "todos", "tasks", "plans", "shell-snapshots",
	"paste-cache", "history.jsonl", "bridge-spawn", "jobs", "sessions", "debug", "logs", "daemon",
	"ide", "chrome", "downloads", "access-audit",
}

// claudeLevels reports entries of ~/.claude jailwick doesn't know. With the app login that
// folder is shared by every project using it, so a new Claude Code version could start keeping
// project data there; this makes it visible. It informs and never fails.
func (v *verifier) claudeLevels() {
	if v.c.ClaudeLogin != "app" {
		return
	}
	v.section = "Claude"
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = filepath.Join(v.c.Home, ".claude")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		v.expect("Claude's config folder readable", err)
		return
	}
	var unknown []string
	for _, e := range entries {
		if !knownClaudeEntry(e.Name()) {
			unknown = append(unknown, e.Name())
		}
	}
	if len(unknown) == 0 {
		v.add(true, "~/.claude holds only what jailwick knows (shared by projects on the app login)", "")
		return
	}
	v.info("~/.claude has entries jailwick doesn't know, shared by every project on the app login: " +
		strings.Join(unknown, ", "))
}

func knownClaudeEntry(name string) bool {
	for _, k := range claudeEntries {
		if name == k || strings.HasPrefix(name, k+".") {
			return true
		}
	}
	return false
}

func (v *verifier) isWorkspaceMount(m string) bool {
	w := v.c.Workspace
	if w == nil {
		return false
	}
	return slices.Contains(w.Members, m) || slices.Contains(w.Hidden, m) || m == w.Handoff || m == w.Notes
}

func (v *verifier) workspace() {
	w := v.c.Workspace
	if w == nil {
		return
	}
	v.section = "Workspace " + w.Name
	for _, m := range w.Members {
		entries, err := os.ReadDir(m)
		if err == nil && len(entries) == 0 {
			err = errors.New("empty")
		}
		v.expect("member "+strings.TrimPrefix(m, w.Root+"/")+" visible", err)
		v.refuse("member "+strings.TrimPrefix(m, w.Root+"/")+" read-only", writable(m))
	}
	var leaked []string
	for _, h := range w.Hidden {
		if st, err := os.Stat(h); err == nil {
			if st.IsDir() {
				if e, _ := os.ReadDir(h); len(e) > 0 {
					leaked = append(leaked, h)
				}
			} else if st.Size() > 0 {
				leaked = append(leaked, h)
			}
		}
	}
	v.add(len(leaked) == 0, fmt.Sprintf("%d hidden paths are empty", len(w.Hidden)), strings.Join(leaked, " "))
	entries, _ := os.ReadDir(w.Root)
	var extra []string
	for _, e := range entries {
		if !slices.Contains(w.RootList, e.Name()) {
			extra = append(extra, e.Name())
		}
	}
	v.add(len(extra) == 0, "the workspace root shows only the project and its members", strings.Join(extra, " "))
	if w.Handoff != "" {
		v.expect("handoff folder writable", writable(w.Handoff))
	}
	if w.Notes != "" {
		v.refuse("root notes read-only", writable(w.Notes))
	}
}

var httpClient = &http.Client{
	Timeout:   12 * time.Second,
	Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, TLSClientConfig: &tls.Config{}},
}

// get succeeds when the request completes with a status below 400.
func get(url string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// reach succeeds when the server answers at all (any status).
func reach(url string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func dial(addr string) error {
	c, err := net.DialTimeout("tcp", addr, 4*time.Second)
	if err == nil {
		c.Close()
	}
	return err
}

func (v *verifier) allowed() {
	v.section = "Network: allowed"
	v.expect("api.anthropic.com through the proxy", reach("https://api.anthropic.com"))
	for _, probe := range []struct{ host, url string }{
		{"registry.npmjs.org", "https://registry.npmjs.org/react"},
		{"github.com", "https://github.com"},
	} {
		if allowedHost(v.c.Expected.Hosts, probe.host) {
			v.expect(probe.host+" through the proxy", get(probe.url))
		}
	}
	for _, s := range v.c.Services {
		host := map[string]string{"postgres": "postgres:5432", "redis": "redis:6379", "mongo": "mongodb:27017"}[s]
		v.expect(s+" reachable", dial(host))
	}
	for _, s := range v.c.TestServices {
		switch s {
		case "postgres":
			v.expect("test postgres can create and drop databases", testPostgres())
		case "redis":
			v.expect("test redis reachable", dial("redis-test:6379"))
		case "mongo":
			v.expect("test mongodb reachable", dial("mongodb-test:27017"))
		}
	}
	for _, f := range v.c.Forwards {
		local, svc, _ := strings.Cut(f, ":")
		svc, _, _ = strings.Cut(svc, ":")
		v.expect("localhost:"+local+" forwards to "+svc, dial("127.0.0.1:"+local))
	}
}

func testPostgres() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, "postgres://test:test@postgres-test:5432/postgres")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	for _, q := range []string{"DROP DATABASE IF EXISTS jailwick_verify", "CREATE DATABASE jailwick_verify", "DROP DATABASE jailwick_verify"} {
		if _, err := conn.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (v *verifier) blocked() {
	v.section = "Network: blocked"
	v.refuse("a domain not on the allowlist (example.com)", get("https://example.com"))
	v.refuse("direct connections around the proxy (1.1.1.1:443)", dial("1.1.1.1:443"))
	v.refuse("the host, directly (host.docker.internal)", dial("host.docker.internal:80"))
	v.refuse("the host, through the proxy", get("http://host.docker.internal"))
	v.refuse("cloud metadata, through the proxy (169.254.169.254)", get("http://169.254.169.254/latest/meta-data/"))
}

// isolated checks the ways out that don't go through the proxy: name resolution (a query for
// <data>.attacker.example carries data out with no connection at all), IPv6, and the other
// sandboxes on this machine.
func (v *verifier) isolated() {
	v.section = "Network: no way around the proxy"
	v.refuse("outside names don't resolve in the sandbox (example.com)", lookup(net.DefaultResolver, "example.com"))
	v.refuse("public DNS servers can't be asked directly (1.1.1.1:53)", lookup(resolverAt("1.1.1.1:53"), "example.com"))
	v.refuse("public IPv6 addresses are unreachable ([2606:4700:4700::1111]:443)", dial("[2606:4700:4700::1111]:443"))
	if len(v.others) == 0 {
		v.info("other sandboxes: none running to check against")
	}
	for _, addr := range v.others {
		v.refuse("another sandbox's service ("+addr+")", dial(addr))
	}
}

// lookup succeeds when name resolves to an address.
func lookup(r *net.Resolver, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	addrs, err := r.LookupHost(ctx, name)
	if err == nil && len(addrs) == 0 {
		return errors.New("no address")
	}
	return err
}

// resolverAt asks one DNS server directly, around the sandbox's own resolver.
func resolverAt(server string) *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, server)
	}}
}

func (v *verifier) tooling() {
	v.section = "Tools and configuration"
	for _, t := range []string{"sh", "git", v.c.Command} {
		_, err := exec.LookPath(t)
		v.expect(t+" installed", err)
	}
	v.expect("~/.local writable (npm -g, uv tool install without root)", writable(filepath.Join(v.c.Home, ".local")))
	pointer, _ := os.ReadFile("/etc/claude-code/CLAUDE.md")
	v.add(strings.Contains(string(pointer), "@/etc/jailwick/shared/CLAUDE.md"), "shared instructions are loaded", "")
	// What the image was built with: jailwick's base, the project's stacks, Playwright.
	if want := v.c.Expected.Tools; len(want) > 0 {
		var missing []string
		for _, t := range want {
			if _, err := exec.LookPath(t); err != nil {
				missing = append(missing, t)
			}
		}
		v.add(len(missing) == 0, "the image's tools (base, stacks, Playwright)", strings.Join(missing, " "))
	}
	if b := v.c.Expected.Browser; b != "" {
		_ = os.WriteFile("/tmp/jailwick-verify.html", []byte(`<h1>jailwick</h1>`), 0o644)
		err := exec.Command("playwright", "screenshot", "--browser", b, "file:///tmp/jailwick-verify.html", "/tmp/jailwick-verify.png").Run()
		v.expect("Playwright takes a screenshot ("+b+")", err)
	}
	if _, err := exec.LookPath("java"); err == nil {
		v.info("Android toolchain present")
	}
}

func writable(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		f, err := os.CreateTemp(path, ".jailwick-verify-*")
		if err != nil {
			return err
		}
		f.Close()
		return os.Remove(f.Name())
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

func mounts() []string {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fs := strings.Fields(sc.Text()); len(fs) >= 2 {
			out = append(out, strings.ReplaceAll(fs[1], `\040`, " "))
		}
	}
	return out
}

// mountTypes maps mount targets to their filesystem type (the last mount at a target wins).
func mountTypes() map[string]string {
	data, _ := os.ReadFile("/proc/mounts")
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if fs := strings.Fields(line); len(fs) >= 3 {
			out[strings.ReplaceAll(fs[1], `\040`, " ")] = fs[2]
		}
	}
	return out
}

func under(p string, roots ...string) bool {
	for _, r := range roots {
		if p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}

// allowedHost reports whether an allowlist entry covers host (".x.com" covers x.com and subdomains).
func allowedHost(entries []string, host string) bool {
	for _, e := range entries {
		if e == host || strings.HasPrefix(e, ".") && (host == e[1:] || strings.HasSuffix(host, e)) {
			return true
		}
	}
	return false
}

package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strconv"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
)

// Inputs are the resolved pieces a sandbox is built from. Host paths are absolute.
type Inputs struct {
	Project    config.Project // with defaults (config.Project.Resolved)
	Image      string         // sandbox image reference
	ProxyImage string

	AgentBinary  string // host file mounted at AgentPath
	PointerFile  string // host file mounted as Claude's managed CLAUDE.md
	SharedDir    string // shared instructions, settings.json, mcp.json
	GeneratedDir string // per-project generated files (agent.json, workspace.md, tools.md)
	NotesDir     string // project notes folder ("" if none)
	UserDir      string // user's additions to the shared instructions ("" if none)
	AllowDir     string // proxy allowlist folder
	// ClipboardDir is a session's folder for the image the user pastes ("" for the project's own
	// container): the host writes it there when the user presses Ctrl+V.
	ClipboardDir string
	PostgresInit string // postgres init scripts folder
	UserMD       string // host CLAUDE.md shown read-only as the user's ("" if none)

	Timezone string // container timezone
	HostName string // name the host reaches the main container by, e.g. sandbox.jailwick-api.orb.local

	// NodeModules are the repo's package folders below its top (relative, sorted): each gets a
	// node_modules volume of its own, like the top-level one, so the Mac's and the sandbox's
	// installs (with native parts for each system) never mix.
	NodeModules []string

	// Secrets are passed as environment variables but never written into generated files.
	Secrets map[string]string
	// Workspace adds mounts and environment ("" members when the project has none).
	Workspace Extra
}

// Extra holds additions from other modules (workspaces) to the main container.
type Extra struct {
	Mounts []Mount
	Env    map[string]string
}

// Build returns the sandbox for a project.
func Build(in Inputs) *Sandbox {
	p := in.Project
	s := &Sandbox{
		Name:    Prefix + p.Name,
		Project: p.Name,
		Networks: []Network{
			{Name: SandboxNetwork, Internal: true},
			{Name: ProxyNetwork, Internal: true},
			{Name: EgressNetwork},
		},
		Volumes: []string{"main-node-modules", "claude-config", "npm-cache", "gradle-cache", "cache", "user-tools", PeersVolume},
	}
	for _, dir := range in.NodeModules {
		s.Volumes = append(s.Volumes, NodeModulesVolume(dir))
	}
	s.AppVolumes = appVolumes(p)
	s.Containers = append(s.Containers, claude(in), volumeInit(in), proxy(in))
	if p.Uses("postgres") {
		s.Containers = append(s.Containers, postgres(in))
		s.Volumes = append(s.Volumes, "pg-data")
	}
	if p.Uses("redis") {
		s.Containers = append(s.Containers, redis(in))
		s.Volumes = append(s.Volumes, "redis-data")
	}
	if p.Uses("mongo") {
		s.Containers = append(s.Containers, mongo(in))
		s.Volumes = append(s.Volumes, "mongo-data", "mongo-keyfile")
	}
	if p.UsesTest("postgres") {
		s.Containers = append(s.Containers, postgresTest(in))
	}
	if p.UsesTest("redis") {
		s.Containers = append(s.Containers, redisTest(in))
	}
	if p.UsesTest("mongo") {
		s.Containers = append(s.Containers, mongoTest(in))
	}
	s.Seal()
	return s
}

// serviceHosts are the in-sandbox host names that must bypass the proxy.
func serviceHosts(p config.Project) []string {
	h := []string{"localhost", "127.0.0.1", "::1", SandboxService, "postgres", "redis", "mongodb"}
	for _, t := range p.TestServices {
		h = append(h, testHost(t))
	}
	return h
}

func testHost(service string) string {
	if service == "mongo" {
		return "mongodb-test"
	}
	return service + "-test"
}

// TestURLs are the addresses of the enabled test services.
func TestURLs(p config.Project) map[string]string {
	out := map[string]string{}
	if p.UsesTest("postgres") {
		out["SANDBOX_TEST_POSTGRES_URL"] = "postgres://test:test@postgres-test:5432/postgres"
	}
	if p.UsesTest("redis") {
		out["SANDBOX_TEST_REDIS_URL"] = "redis://redis-test:6379"
	}
	if p.UsesTest("mongo") {
		out["SANDBOX_TEST_MONGO_URL"] = "mongodb://mongodb-test:27017/?replicaSet=rs0"
	}
	return out
}

func claude(in Inputs) Container {
	p := in.Project
	home := p.Image.Home
	noProxy := strings.Join(serviceHosts(p), ",")
	env := map[string]string{
		"HTTP_PROXY": ProxyURL, "HTTPS_PROXY": ProxyURL, "http_proxy": ProxyURL, "https_proxy": ProxyURL,
		"NO_PROXY": noProxy, "no_proxy": noProxy,
		"NODE_USE_ENV_PROXY":  "1",
		"NODE_OPTIONS":        "--max-old-space-size=" + p.NodeHeap(),
		"TERM":                "xterm-256color",
		"COLORTERM":           "truecolor",
		"LANG":                "C.UTF-8",
		"TZ":                  in.Timezone,
		"HOME":                home,
		"CLAUDE_CONFIG_DIR":   path.Join(home, ".claude"),
		"DISABLE_AUTOUPDATER": "1",
		"HISTFILE":            path.Join(home, ".cache", "bash_history"),
		// A command run in a worktree gets that worktree's ports, databases and Redis.
		"BASH_ENV": WorktreeEnvScript,
		// Tool telemetry would only be denied by the proxy.
		"DO_NOT_TRACK": "1", "CHECKPOINT_DISABLE": "1", "TURBO_TELEMETRY_DISABLED": "1",
		"ASTRO_TELEMETRY_DISABLED": "1", "NEXT_TELEMETRY_DISABLED": "1", "EXPO_NO_TELEMETRY": "1",
		"PROJECT_DIR":      p.Dir,
		"SANDBOX_PROJECT":  p.Name,
		"SANDBOX_SERVICES": strings.Join(p.Services, " "),
		"SANDBOX_HOST":     in.HostName,
		"HOST":             "0.0.0.0",
		// Vite answers 403 to host names it doesn't know.
		"__VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS": hostSuffix(in.HostName),
		"GIT_AUTHOR_NAME":                        p.Git.Name, "GIT_AUTHOR_EMAIL": p.Git.Email,
		"GIT_COMMITTER_NAME": p.Git.Name, "GIT_COMMITTER_EMAIL": p.Git.Email,
	}
	if p.Uses("postgres") {
		pg := p.Postgres
		env["DATABASE_URL"] = "postgres://" + pg.User + ":" + pg.Password + "@postgres:5432/" + pg.DB
		env["PGHOST"], env["PGUSER"], env["PGPASSWORD"], env["PGDATABASE"], env["POSTGRES_DB"] = "postgres", pg.User, pg.Password, pg.DB, pg.DB
	}
	if p.Uses("redis") {
		env["REDIS_URL"] = RedisURL(p, 0)
		if len(p.Redis.Passwords) > 0 {
			env["REDISCLI_AUTH"] = p.Redis.Passwords[0]
		}
	}
	if p.Uses("mongo") {
		env["MONGO_URL"] = "mongodb://" + p.Mongo.User + ":" + p.Mongo.Password + "@mongodb:27017/?authSource=admin&replicaSet=rs0&directConnection=true"
	}
	if len(p.TestServices) > 0 {
		env["SANDBOX_TEST_SERVICES"] = strings.Join(p.TestServices, " ")
		for k, v := range TestURLs(p) {
			env[k] = v
		}
	}
	for k, v := range in.Workspace.Env {
		env[k] = v
	}
	for k, v := range p.Env {
		env[k] = v
	}

	mounts := []Mount{
		{Type: Bind, Source: p.Dir, Target: p.Dir},
		{Type: Volume, Source: "main-node-modules", Target: path.Join(p.Dir, "node_modules")},
	}
	for _, dir := range in.NodeModules {
		mounts = append(mounts, Mount{Type: Volume, Source: NodeModulesVolume(dir), Target: path.Join(p.Dir, dir, "node_modules")})
	}
	mounts = append(mounts, claudeMounts(p, path.Join(home, ".claude"))...)
	mounts = append(mounts, []Mount{
		{Type: Volume, Source: "npm-cache", Target: path.Join(home, ".npm")},
		{Type: Volume, Source: "gradle-cache", Target: path.Join(home, ".gradle", "caches")},
		{Type: Volume, Source: "cache", Target: path.Join(home, ".cache")},
		{Type: Volume, Source: "user-tools", Target: path.Join(home, ".local")},
		{Type: Bind, Source: in.AgentBinary, Target: AgentPath, ReadOnly: true},
		{Type: Bind, Source: in.PointerFile, Target: "/etc/claude-code/CLAUDE.md", ReadOnly: true},
		{Type: Bind, Source: in.SharedDir, Target: "/etc/jailwick/shared", ReadOnly: true},
		{Type: Bind, Source: in.GeneratedDir, Target: "/etc/jailwick/generated", ReadOnly: true},
	}...)
	optional := []struct{ src, dst string }{
		{in.NotesDir, "/etc/jailwick/project"},
		{in.UserDir, "/etc/jailwick/user"},
		{in.UserMD, path.Join(home, ".claude", "CLAUDE.md")},
	}
	for _, o := range optional {
		if o.src != "" {
			mounts = append(mounts, Mount{Type: Bind, Source: o.src, Target: o.dst, ReadOnly: true})
		}
	}
	mounts = append(mounts, in.Workspace.Mounts...)
	mounts = append(mounts, peerMounts(home)...)

	deps := []Dependency{
		{Service: VolumeInit, Condition: Completed},
		{Service: ProxyService, Condition: Healthy},
	}
	for _, s := range p.Services {
		deps = append(deps, Dependency{Service: serviceContainer(s), Condition: Healthy})
	}
	for _, s := range p.TestServices {
		deps = append(deps, Dependency{Service: testHost(s), Condition: Healthy})
	}

	return Container{
		Name: SandboxService, Image: in.Image, Platform: p.Image.Platform,
		Hostname: SandboxService, User: p.Image.User, WorkDir: p.Dir,
		Entrypoint: []string{AgentPath, "init"},
		Init:       true, CapDrop: []string{"ALL"}, NoNewPrivileges: true,
		// The proxy network holds only this container and its proxy, so egress-proxy always
		// means its own proxy and no other container can use that proxy.
		Networks: []string{SandboxNetwork, ProxyNetwork},
		Env:      env, Mounts: mounts,
		Memory: p.Resources.Memory, CPUs: p.Resources.CPUs, Pids: p.Resources.Pids, Shm: p.Resources.Shm,
		DependsOn: deps,
		Secrets:   in.Secrets,
	}
}

// RedisURL is the Redis address for a database index (worktrees use their own index).
func RedisURL(p config.Project, index int) string {
	auth := ""
	if len(p.Redis.Passwords) > 0 {
		auth = ":" + p.Redis.Passwords[0] + "@"
	}
	return "redis://" + auth + "redis:6379/" + strconv.Itoa(index)
}

func serviceContainer(s string) string {
	if s == "mongo" {
		return "mongodb"
	}
	return s
}

func hostSuffix(host string) string {
	if i := strings.Index(host, "."); i > 0 {
		return host[i:]
	}
	return host
}

// Claude's config folder (~/.claude) is the login's volume: the project's claude-config with
// claude.login: project, the app's LoginVolume with claude.login: app. With the app login, what
// belongs to the project stays in claude-config, mounted over the login volume at these paths
// (all folders but history.jsonl). These are where it already is with the project login, so
// switching moves nothing.
var projectClaudePaths = []string{
	"projects", "file-history", "session-env", "todos", "plans", "shell-snapshots", "paste-cache",
	"history.jsonl",
}

const projectClaudeFile = "history.jsonl"

// appVolumes are the app volumes a project's sandboxes use.
func appVolumes(p config.Project) []string {
	if p.AppLogin() {
		return []string{LoginVolume}
	}
	return nil
}

// claudeMounts mounts Claude's config folder at dir for the project's login.
func claudeMounts(p config.Project, dir string) []Mount {
	if !p.AppLogin() {
		return []Mount{{Type: Volume, Source: "claude-config", Target: dir}}
	}
	mounts := []Mount{{Type: Volume, Source: LoginVolume, Target: dir}}
	for _, sub := range projectClaudePaths {
		mounts = append(mounts, Mount{Type: Volume, Source: "claude-config", Target: path.Join(dir, sub), Subpath: sub})
	}
	return mounts
}

// peerMounts share Claude Code's list of running sessions and their message sockets with the
// project's sessions (PeersVolume), so they can message each other. The list sits in the login
// folder, which projects may share: the mount keeps it the project's.
func peerMounts(home string) []Mount {
	return []Mount{
		{Type: Volume, Source: PeersVolume, Target: sessionList(home), Subpath: "sessions"},
		{Type: Volume, Source: PeersVolume, Target: agentapi.SocketsDir, Subpath: "sockets"},
	}
}

// sessionList is the folder of Claude Code's list of running sessions.
func sessionList(home string) string { return path.Join(home, ".claude", "sessions") }

// volumeInit prepares the named volumes before the main container starts: a fresh volume is
// root-owned, and a subpath must exist before it can be mounted. It runs as root with only the
// capabilities for that, and no network.
func volumeInit(in Inputs) Container {
	p := in.Project
	const root = "/jailwick/volumes"
	vols := []string{"main-node-modules", "claude-config"}
	script := []string{"set -e"}
	owned := []string{root + "/main-node-modules"}
	for _, dir := range in.NodeModules {
		v := NodeModulesVolume(dir)
		vols = append(vols, v)
		owned = append(owned, root+"/"+v)
	}
	if p.AppLogin() {
		vols = append(vols, LoginVolume)
		cfg := root + "/claude-config"
		var dirs []string
		for _, sub := range projectClaudePaths {
			owned = append(owned, cfg+"/"+sub)
			if sub != projectClaudeFile {
				dirs = append(dirs, cfg+"/"+sub)
			}
		}
		file := cfg + "/" + projectClaudeFile
		script = append(script,
			"mkdir -p "+strings.Join(dirs, " "),
			"[ -e "+file+" ] || touch "+file)
		owned = append(owned, cfg, root+"/"+LoginVolume)
	}
	// The user's alone (0700), as Claude Code makes them: it refuses a sockets folder others
	// could write to.
	vols = append(vols, PeersVolume)
	peers := []string{root + "/" + PeersVolume + "/sessions", root + "/" + PeersVolume + "/sockets"}
	script = append(script, "mkdir -p -m 700 "+strings.Join(peers, " "))
	owned = append(owned, peers...)
	// Not recursive: only what is created here; the rest is already the user's.
	script = append(script, "chown "+shellQuote(p.Image.User)+" "+strings.Join(owned, " "))

	mounts := make([]Mount, 0, len(vols))
	for _, v := range vols {
		mounts = append(mounts, Mount{Type: Volume, Source: v, Target: root + "/" + v})
	}
	return Container{
		Name: VolumeInit, Image: in.Image, Platform: p.Image.Platform, User: "root",
		Entrypoint: []string{"sh", "-c", strings.Join(script, "\n")},
		OneShot:    true, NoNetwork: true, NoNewPrivileges: true,
		// DAC_OVERRIDE: root creates folders in a volume the user already owns.
		CapDrop: []string{"ALL"}, CapAdd: []string{"CHOWN", "DAC_OVERRIDE"},
		Mounts: mounts,
	}
}

// shellQuote quotes s as one word for sh.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func proxy(in Inputs) Container {
	return Container{
		Name: ProxyService, Image: in.ProxyImage,
		CapDrop: []string{"ALL"}, NoNewPrivileges: true, ReadOnlyRoot: true,
		Tmpfs:    []string{"/tmp"},
		Networks: []string{ProxyNetwork, EgressNetwork},
		Mounts:   []Mount{{Type: Bind, Source: in.AllowDir, Target: "/etc/squid/allow", ReadOnly: true}},
		// Fast checks only while starting (the sandbox waits for it); the checks run for the
		// container's whole life, and squid.conf keeps them out of the access log.
		Health: &Health{
			Test:     []string{"CMD-SHELL", "nc -z 127.0.0.1 3128"},
			Interval: "30s", Retries: 3, StartPeriod: "30s", StartInterval: "1s",
		},
	}
}

// NodeModulesVolume names the node_modules volume of a package folder (relative to the repo).
func NodeModulesVolume(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return "nm-" + hex.EncodeToString(sum[:4])
}

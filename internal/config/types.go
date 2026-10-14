// Package config loads, validates and saves jailwick's YAML configuration.
//
//	$JAILWICK_HOME/config.yaml               Global
//	$JAILWICK_HOME/projects/<name>.yaml      Project
//	$JAILWICK_HOME/workspaces/<name>.yaml    Workspace
package config

// Version is the current config file format.
const Version = 1

// Global holds settings shared by all projects, and defaults for them.
type Global struct {
	Version int `yaml:"version"`
	// Runtime: docker, containerd or apple. Empty: detect.
	Runtime string `yaml:"runtime,omitempty"`
	// Terminal used by the app to open sessions (terminal, iterm2, ghostty, kitty, wezterm or a
	// command template with {cmd}).
	Terminal string `yaml:"terminal,omitempty"`
	// Secrets backend: keychain (macOS default), file, env.
	Secrets string      `yaml:"secrets,omitempty"`
	Git     GitIdentity `yaml:"git,omitempty"`
	Claude  Claude      `yaml:"claude,omitempty"`
	Image   ImageSource `yaml:"image,omitempty"`
	// Domains every sandbox may reach, on top of the built-in presets.
	Allow []string `yaml:"allow,omitempty"`
	// Notifications from the app (a session waiting for you, a start done or failed, a handoff,
	// a blocked host). Default: on.
	Notifications *bool `yaml:"notifications,omitempty"`
	// SessionNotifications: where a session tells you it waits for you (a permission, an
	// answer, or it finished). auto (default): its terminal when the terminal can show
	// notifications (iTerm2, Ghostty, kitty, WezTerm), else the app; terminal: always the
	// terminal (a bell where it can't do more); app: the app; off: none.
	SessionNotifications string `yaml:"session_notifications,omitempty"`
	// BlockedNotifications: the app tells you when a sandbox was refused a host. Default: on.
	BlockedNotifications *bool `yaml:"blocked_notifications,omitempty"`
	// Updates: jailwick asks GitHub once a day whether a newer version is out, and says so (the
	// app, jw version, jw doctor). Default: on. `jw update` checks either way.
	Updates *bool `yaml:"updates,omitempty"`
}

// SessionNotifyModes are the values of session_notifications.
var SessionNotifyModes = []string{"auto", "terminal", "app", "off"}

// NotificationsOn reports whether the app sends notifications.
func (g Global) NotificationsOn() bool { return g.Notifications == nil || *g.Notifications }

// SessionNotify is session_notifications, auto when unset.
func (g Global) SessionNotify() string {
	if g.SessionNotifications == "" {
		return "auto"
	}
	return g.SessionNotifications
}

// UpdatesOn reports whether jailwick checks for new versions by itself.
func (g Global) UpdatesOn() bool { return g.Updates == nil || *g.Updates }

// BlockedNotificationsOn reports whether the app tells about refused hosts.
func (g Global) BlockedNotificationsOn() bool {
	return g.BlockedNotifications == nil || *g.BlockedNotifications
}

// Project is one repo with its own sandbox.
type Project struct {
	Version int    `yaml:"version"`
	Name    string `yaml:"-"`
	// Dir is the repo, mounted read-write at the same path.
	Dir              string      `yaml:"dir"`
	Workspace        string      `yaml:"workspace,omitempty"`
	WorkspaceRelated []string    `yaml:"workspace_related,omitempty"`
	WorkspaceExclude []string    `yaml:"workspace_exclude,omitempty"`
	Runtime          string      `yaml:"runtime,omitempty"`
	Git              GitIdentity `yaml:"git,omitempty"`
	Timezone         string      `yaml:"timezone,omitempty"`
	Services         []string    `yaml:"services,omitempty"`
	TestServices     []string    `yaml:"test_services,omitempty"`
	Postgres         Postgres    `yaml:"postgres,omitempty"`
	Redis            Redis       `yaml:"redis,omitempty"`
	Mongo            Mongo       `yaml:"mongo,omitempty"`
	Test             TestConfig  `yaml:"test,omitempty"`
	// LocalForwards: "localport:service:port", so env files saying localhost:<port> work.
	LocalForwards []string `yaml:"local_forwards,omitempty"`
	Worktree      Worktree `yaml:"worktree,omitempty"`
	// Run starts the app (e.g. pnpm dev), from the app's Run button: in the main checkout, or in a
	// worktree with that worktree's ports ($PORT), databases and Redis.
	Run string `yaml:"run,omitempty"`
	// NodeModules are the package folders (relative to the repo; globs like apps/* allowed) whose
	// node_modules the sandbox keeps apart from the Mac's. Empty: found automatically (every
	// folder with a package.json, 4 levels deep). The repo's own node_modules always is.
	NodeModules []string  `yaml:"node_modules,omitempty"`
	Image       Image     `yaml:"image,omitempty"`
	Resources   Resources `yaml:"resources,omitempty"`
	Claude      Claude    `yaml:"claude,omitempty"`
	Agent       Agent     `yaml:"agent,omitempty"`
	// Extra environment for the sandbox (not secret).
	Env map[string]string `yaml:"env,omitempty"`
	// Names of secrets passed as environment variables (values in the secret store).
	SecretEnv []string `yaml:"secret_env,omitempty"`
	// Allowlist presets and domains for this project, on top of the global ones.
	AllowPresets []string `yaml:"allow_presets,omitempty"`
	Allow        []string `yaml:"allow,omitempty"`
	// Blocked: hosts kept blocked. They are refused like any host off the allowlist, and the app
	// no longer lists them as blocked requests or notifies about them. Allowing one removes it.
	Blocked []string `yaml:"blocked,omitempty"`
}

// Workspace is a folder of related repos.
type Workspace struct {
	Version   int      `yaml:"version"`
	Name      string   `yaml:"-"`
	Root      string   `yaml:"root"`
	Members   []Member `yaml:"members,omitempty"`
	RootNotes string   `yaml:"root_notes,omitempty"`
	Exclude   []string `yaml:"exclude,omitempty"`
	Handoff   string   `yaml:"handoff,omitempty"`
}

// Member is a folder of a workspace: a name under the root, or a name shown for another path.
type Member struct {
	Name string `yaml:"name"`
	Path string `yaml:"path,omitempty"`
}

type GitIdentity struct {
	Name  string `yaml:"name,omitempty"`
	Email string `yaml:"email,omitempty"`
}

type Postgres struct {
	User     string    `yaml:"user,omitempty"`
	Password string    `yaml:"password,omitempty"`
	DB       string    `yaml:"db,omitempty"`
	Image    string    `yaml:"image,omitempty"`
	Timezone string    `yaml:"timezone,omitempty"`
	ExtraDBs []ExtraDB `yaml:"extra_dbs,omitempty"`
}

// ExtraDB is an additional database, optionally owned by its own login role.
type ExtraDB struct {
	Name     string `yaml:"name"`
	User     string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type Redis struct {
	Image string `yaml:"image,omitempty"`
	// Passwords accepted for the default user; the first is used in REDIS_URL. Empty: no auth.
	Passwords []string `yaml:"passwords,omitempty"`
}

type Mongo struct {
	Image    string `yaml:"image,omitempty"`
	User     string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type TestConfig struct {
	PostgresImage string `yaml:"postgres_image,omitempty"`
	RedisImage    string `yaml:"redis_image,omitempty"`
	MongoImage    string `yaml:"mongo_image,omitempty"`
	TmpfsSize     string `yaml:"tmpfs_size,omitempty"`
}

type Worktree struct {
	// Files copied from the main checkout into new worktrees (DB names rewritten).
	CopyFiles []string `yaml:"copy_files,omitempty"`
	// Commands run in a new worktree after it is set up.
	Setup []string `yaml:"setup,omitempty"`
}

// ImageSource is where a sandbox image comes from; the global config can set it for every
// project. At most one of Base, Dockerfile, Name.
type ImageSource struct {
	// Base: build jailwick's project layer FROM this image instead of the jailwick base.
	Base string `yaml:"base,omitempty"`
	// Dockerfile (+ Context): build this as the base, then jailwick's layer on top.
	Dockerfile string `yaml:"dockerfile,omitempty"`
	Context    string `yaml:"context,omitempty"`
	// Name: a prebuilt image used as is.
	Name string `yaml:"name,omitempty"`
	// User and Home of the sandbox user in the image.
	User string `yaml:"user,omitempty"`
	Home string `yaml:"home,omitempty"`

	ClaudeCodeVersion string `yaml:"claude_code_version,omitempty"`
	Platform          string `yaml:"platform,omitempty"`
}

// Image is a project's sandbox image: its source, and what jailwick's layer adds to it.
type Image struct {
	ImageSource `yaml:",inline"`
	// Stacks are toolchains in the image, by name (internal/stacks), each at a version ("" for the
	// stack's default).
	Stacks map[string]string `yaml:"stacks,omitempty"`
	// Playwright, its MCP server and these browsers; no browsers: no Playwright.
	Playwright  Playwright `yaml:"playwright,omitempty"`
	AptPackages []string   `yaml:"apt_packages,omitempty"`
	Android     bool       `yaml:"android,omitempty"`
}

type Playwright struct {
	Browsers []string `yaml:"browsers,omitempty"`
	Versions []string `yaml:"versions,omitempty"`
}

type Resources struct {
	Memory     string `yaml:"memory,omitempty"`
	CPUs       string `yaml:"cpus,omitempty"`
	Pids       int    `yaml:"pids,omitempty"`
	Shm        string `yaml:"shm,omitempty"`
	NodeHeapMB int    `yaml:"node_heap_mb,omitempty"`
}

type Claude struct {
	// UserMD: host file shown read-only as the user's CLAUDE.md. "none" disables it.
	UserMD string `yaml:"user_md,omitempty"`
	// YOLO starts sessions with --dangerously-skip-permissions. Off unless set.
	YOLO *bool `yaml:"yolo,omitempty"`
	// MCP: "default" (Playwright when the project has browsers), "none", or a host path to an
	// mcp.json.
	MCP string `yaml:"mcp,omitempty"`
	// Login: "app" (one sign-in shared by every project that uses it) or "project" (the
	// project's own). Default: app.
	Login string `yaml:"login,omitempty"`
	// Chrome lets sessions use Claude in Chrome (the user's own browser). Off unless set.
	Chrome *bool `yaml:"chrome,omitempty"`
	// Profiles: named Claude settings layers (<config>/claude/profiles/<name>.json), in order.
	Profiles []string `yaml:"profiles,omitempty"`
}

type Agent struct {
	// Command started for sessions. Default: claude.
	Command string `yaml:"command,omitempty"`
}

// Package agentapi is the contract between the host and jailwick-agent inside a sandbox: the
// configuration the host writes, and the JSON the agent prints.
package agentapi

import "time"

// ConfigFile is where the agent reads its configuration (a read-only mount of the host's
// generated folder, so edits apply to the next command without a restart).
const ConfigFile = "/etc/jailwick/generated/agent.json"

// A session container (one per session, see spec.BuildSession) carries its session id in
// SessionEnv. Its agent writes the session's PID to SessionPIDFile, and the container ends when
// that process does.
// A session's pasted image: the host writes it to ClipboardFile in ClipboardDir (a read-only mount
// of a folder of the session's own) when the user presses Ctrl+V, and the agent hands it to the
// agent CLI, answering as xclip and wl-paste (mounted under those names at ClipboardTools).
const (
	ClipboardDir  = "/run/jailwick/clipboard"
	ClipboardFile = "image.png"
)

// ClipboardTools are where the agent is mounted as the clipboard commands the agent CLI runs.
var ClipboardTools = []string{"/usr/local/bin/xclip", "/usr/local/bin/wl-paste"}

const (
	SessionEnv     = "JAILWICK_SESSION"
	SessionPIDFile = "/tmp/jailwick-session.pid"
	// PrivateEnv is set in a private session's container: it is kept out of messages between
	// sessions (see spec.SessionOptions).
	PrivateEnv = "JAILWICK_PRIVATE"
	// NotifyEnv carries a session's notification channel, chosen on the host for the terminal it
	// runs in (Claude Code's preferredNotifChannel: iterm2, kitty, ghostty, terminal_bell or
	// notifications_disabled). The agent applies it to the session's settings, and reports it
	// back (Session.Notify) so the app notifies only for sessions their terminal doesn't.
	NotifyEnv = "JAILWICK_NOTIFY"
	// SocketsDir is where Claude Code puts the socket other sessions message it through
	// ($XDG_RUNTIME_DIR or the temp folder, then cc-socks; neither is set in a sandbox).
	SocketsDir = "/tmp/cc-socks"
)

// Contract is the version of this contract. The host and the agent can be of different jailwick
// versions: the agent of a running sandbox stays until its restart. Within one contract, changes
// are additive only (new optional fields, new commands); anything else is a new contract. A host
// that finds an older agent asks for a restart instead of misreading its answers.
const Contract = 1

// Version is what `jailwick-agent version -json` prints.
type Version struct {
	Contract int    `json:"contract"`
	Build    string `json:"build"` // the hash of the agent's bytes
}

// Config is written by the host for each sandbox.
type Config struct {
	Contract     int       `json:"contract"` // the host's Contract
	Project      string    `json:"project"`
	Dir          string    `json:"dir"`
	Home         string    `json:"home"`
	Command      string    `json:"command"`                // agent CLI started for sessions
	ClaudeLogin  string    `json:"claude_login,omitempty"` // "app" or "project" (claude.login)
	HostName     string    `json:"host_name"`
	Services     []string  `json:"services,omitempty"`
	TestServices []string  `json:"test_services,omitempty"`
	Postgres     *Postgres `json:"postgres,omitempty"`
	Redis        *Redis    `json:"redis,omitempty"`
	Forwards     []string  `json:"forwards,omitempty"` // localport:host:port
	CopyFiles    []string  `json:"copy_files,omitempty"`
	Setup        []string  `json:"setup,omitempty"`
	Run          string    `json:"run,omitempty"` // the project's run command (Run button)
	// NodeModules are the node_modules folders kept apart from the host's (absolute; the top's first).
	NodeModules []string `json:"node_modules,omitempty"`
	// Workspace is set for projects in a workspace (verify checks it).
	Workspace *Workspace `json:"workspace,omitempty"`
	// Expected lists what verify expects to find.
	Expected Expected `json:"expected"`
}

type Postgres struct {
	User     string   `json:"user"`
	Password string   `json:"password"`
	DB       string   `json:"db"`
	ExtraDBs []string `json:"extra_dbs,omitempty"`
}

type Redis struct {
	Passwords []string `json:"passwords,omitempty"`
}

type Workspace struct {
	Name     string   `json:"name"`
	Root     string   `json:"root"`
	Members  []string `json:"members"`   // container paths, read-only
	Hidden   []string `json:"hidden"`    // container paths that must appear empty
	RootList []string `json:"root_list"` // entries expected under the root
	Handoff  string   `json:"handoff,omitempty"`
	Notes    string   `json:"notes,omitempty"` // root notes file, read-only
}

type Expected struct {
	Mounts   []string `json:"mounts"`   // every mount target the container should have
	ReadOnly []string `json:"readonly"` // targets that must not be writable
	Hosts    []string `json:"hosts"`    // domains that must be reachable through the proxy
	Tools    []string `json:"tools"`    // commands the image has: jailwick's base, stacks, Playwright
	Browser  string   `json:"browser"`  // Playwright's browser, for a screenshot; "": no Playwright
}

// Status is printed by `jailwick-agent status --json`.
type Status struct {
	Sessions  []Session  `json:"sessions"`
	Worktrees []Worktree `json:"worktrees"`
	Runs      []Run      `json:"runs,omitempty"` // the run command, where it runs
	// EmptyNodeModules are the folders of NodeModules with nothing installed yet (relative to the
	// repo, "." for its top): the sandbox's own installs start empty.
	EmptyNodeModules []string `json:"empty_node_modules,omitempty"`
}

// Run is the project's run command running in the main checkout or a worktree.
type Run struct {
	Worktree string    `json:"worktree,omitempty"` // "" = main checkout
	PID      int       `json:"pid"`
	Started  time.Time `json:"started"`
	Running  bool      `json:"running"`        // false: it ended (its output is still there)
	Port     int       `json:"port,omitempty"` // the worktree's first port; 0 in the main checkout
}

// Changes is what a checkout changed: a worktree against the main checkout's branch (Base), from
// where they part; the main checkout against its last commit. Diff is one file's, when asked.
type Changes struct {
	Base    string        `json:"base,omitempty"`
	Head    string        `json:"head"`
	Commits []Commit      `json:"commits,omitempty"`
	Files   []ChangedFile `json:"files"`
	Diff    string        `json:"diff,omitempty"`
	DiffCut bool          `json:"diff_cut,omitempty"` // the diff was longer than shown
}

type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

// ChangedFile is one changed file: added, modified, deleted, renamed or untracked.
type ChangedFile struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
	Binary  bool   `json:"binary,omitempty"`
}

// WorktreeAdded is a worktree the app created (wt add): ready to work in, no session started.
type WorktreeAdded struct {
	Name   string   `json:"name"`
	Branch string   `json:"branch"`
	Port   int      `json:"port"`
	DBs    []string `json:"dbs,omitempty"`
	// SetupFailed: one of the project's setup commands failed (the worktree is there; the
	// output says which).
	SetupFailed bool `json:"setup_failed,omitempty"`
}

// WorktreeCheck is what removing a worktree would take away, asked before it is removed.
type WorktreeCheck struct {
	Name    string `json:"name"`
	Branch  string `json:"branch"`
	Missing bool   `json:"missing,omitempty"` // its folder is gone
	Changed int    `json:"changed"`           // files with uncommitted changes (lost with the folder)
	// Into is the branch "merged" means: the main checkout's ("" when it has none, detached).
	Into   string `json:"into,omitempty"`
	Merged bool   `json:"merged"` // every commit of the branch is in Into
	Ahead  int    `json:"ahead"`  // commits of the branch not in Into
	// Pushed: the branch has an upstream with every one of its commits (deleting it loses nothing).
	Pushed bool `json:"pushed"`
}

// RunLog is the end of a run's output.
type RunLog struct {
	Text string `json:"text"`
}

// Session is one agent process.
type Session struct {
	PID      int       `json:"pid"`
	Dir      string    `json:"dir"`
	Worktree string    `json:"worktree,omitempty"` // "" = main checkout
	Started  time.Time `json:"started"`
	Name     string    `json:"name,omitempty"`   // from Claude's session registry, best effort
	State    string    `json:"state,omitempty"`  // busy / idle, best effort
	ID       string    `json:"id,omitempty"`     // conversation id, best effort
	Notify   string    `json:"notify,omitempty"` // its notification channel (NotifyEnv); "" before 0.2.9
}

// Worktree is one jailwick worktree and its resources.
type Worktree struct {
	Name    string   `json:"name"`
	Dir     string   `json:"dir"`
	Branch  string   `json:"branch"`
	Slot    int      `json:"slot"`
	Port    int      `json:"port"`
	DBs     []string `json:"dbs,omitempty"`
	RedisDB int      `json:"redis_db"`
	Missing bool     `json:"missing,omitempty"`
	// Unmanaged is a worktree in a worktree folder that jailwick has no slot for (made by git or a
	// tool directly): it shares the main checkout's ports, databases and Redis.
	Unmanaged bool `json:"unmanaged,omitempty"`
	Sessions  int  `json:"sessions"`
}

// Conversation is one saved conversation (history).
type Conversation struct {
	ID     string    `json:"id"`
	Dir    string    `json:"dir"`
	Where  string    `json:"where"` // "main checkout", "wt <name>" or the folder
	When   time.Time `json:"when"`
	Prompt string    `json:"prompt"`
	Name   string    `json:"name,omitempty"` // the name given with /rename, if any
	Gone   bool      `json:"gone,omitempty"`
	File   string    `json:"file,omitempty"`
}

// Check is one verify result.
type Check struct {
	Section string `json:"section"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Info    bool   `json:"info,omitempty"` // informational, never a failure
	Detail  string `json:"detail,omitempty"`
}

// Port is a listening TCP port and the process that owns it.
type Port struct {
	Port    int    `json:"port"`
	Address string `json:"address"`
	PID     int    `json:"pid,omitempty"`
	Command string `json:"command,omitempty"`
	Where   string `json:"where"` // main checkout, wt <name>, forward, other
}

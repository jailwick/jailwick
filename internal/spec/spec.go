// Package spec describes a sandbox independently of the container runtime. The builder turns a
// project's configuration into a Sandbox; runtime drivers turn a Sandbox into containers.
package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
)

// Names used across jailwick.
const (
	Prefix         = "jailwick-"
	LabelProject   = "dev.jailwick.project"
	LabelSpecHash  = "dev.jailwick.spec-hash"
	SandboxService = "sandbox" // the project's main container: shells, commands, verify
	// LegacySandboxService is what the main container was called before 0.2.5; a sandbox still
	// running with it is replaced at its next start (see sandbox.Manager.Up).
	LegacySandboxService = "claude"
	ProxyService         = "egress-proxy"
	SandboxNetwork       = "sandbox" // sandbox containers and services; no route out
	ProxyNetwork         = "proxy"   // one agent container and its own proxy, nothing else
	EgressNetwork        = "egress"  // proxies only: the way out
	AgentPath            = "/opt/jailwick/bin/jailwick-agent"
	// WorktreeEnvScript gives commands in a worktree that worktree's environment (BASH_ENV).
	WorktreeEnvScript = "/etc/jailwick/shared/worktree-env.sh"
	ProxyURL          = "http://egress-proxy:3128"
	// LabelChrome marks a session that may use Claude in Chrome.
	LabelChrome = "dev.jailwick.chrome"
	// VolumeInit prepares the named volumes (folders, ownership) before the others start.
	VolumeInit = "volume-init"
	// LoginVolume holds the app's Claude login, shared by every project with claude.login: app.
	// It is an app volume: never project-scoped, never removed with a project. Its name must never
	// change: every version finds the login there, so a new name would sign everyone out.
	LoginVolume = Prefix + "claude-login"
	// PeersVolume is where a project's sessions find each other: Claude Code's list of running
	// sessions (~/.claude/sessions) and the sockets their messages go through (agentapi.SocketsDir), shared
	// by the project's container and its sessions' (see BuildSession). The main container's agent
	// empties it at every start: its PID namespace is new then, so no session from before is alive.
	PeersVolume = "peers"
	// LabelPrivate marks a session kept out of messages between sessions.
	LabelPrivate = "dev.jailwick.private"
	// LabelTerminal records the host terminal a session runs in (hostterm.Terminal.Encode), for
	// the app to bring it forward.
	LabelTerminal = "dev.jailwick.terminal"
	// LabelNotify is where a session tells it waits (its notification channel).
	LabelNotify = "dev.jailwick.notify"
)

// ContainerName is the runtime name of a sandbox service's container (Compose's naming).
func ContainerName(sandbox, service string) string { return sandbox + "-" + service + "-1" }

// Sandbox is everything one project runs, or one of its sessions (see BuildSession).
type Sandbox struct {
	Name    string // runtime project name: jailwick-<project>, or jailwick-<project>--<session>
	Project string
	// Parent is the sandbox whose volumes and external networks this one uses ("" for a project).
	Parent   string
	Networks []Network
	Volumes  []string // named volumes, project-scoped by the runtime (the parent's when Parent is set)
	// AppVolumes are named volumes shared by every sandbox, used by their own name. The runtime
	// creates them when missing and never removes them.
	AppVolumes []string `json:",omitempty"`
	Containers []Container
}

type Network struct {
	Name     string
	Internal bool // no route to the internet or the host
	External bool // the parent sandbox's network of that name
}

// Container is one service of the sandbox.
type Container struct {
	Name       string // service name, e.g. "claude", "postgres"
	Image      string
	Platform   string
	Hostname   string
	User       string
	WorkDir    string
	Entrypoint []string
	Command    []string
	Init       bool // run a minimal init as PID 1 (reaps zombies, forwards signals)
	// OneShot containers run to completion before the others start (e.g. volume ownership).
	OneShot bool
	// Ephemeral containers are never restarted: they end with their work (a session).
	Ephemeral bool `json:",omitempty"` // omitted: no hash change for other containers
	// SharePIDs is a service of the parent sandbox whose PID namespace this container joins:
	// its processes see each other's, and end with that container.
	SharePIDs       string `json:",omitempty"`
	CapDrop         []string
	CapAdd          []string
	NoNewPrivileges bool
	ReadOnlyRoot    bool
	Networks        []string // empty with NoNetwork: no network at all
	NoNetwork       bool
	Env             map[string]string
	Mounts          []Mount
	Tmpfs           []string // "path" or "path:size=1g"
	Memory          string
	CPUs            string
	Pids            int
	Shm             string
	Health          *Health
	DependsOn       []Dependency
	Labels          map[string]string
	// Secrets are environment variables kept out of generated files (drivers pass them at
	// start). Only a digest of each value enters the hash.
	Secrets map[string]string `json:"-"`
}

type MountType string

const (
	Bind   MountType = "bind"
	Volume MountType = "volume"
)

type Mount struct {
	Type     MountType
	Source   string // host path (bind) or volume name
	Target   string
	ReadOnly bool
	// Subpath mounts only this path inside the volume (it must exist).
	Subpath string `json:",omitempty"` // omitted: no hash change for other containers
}

type Health struct {
	Test        []string // CMD-SHELL form: {"CMD-SHELL", "..."}
	Interval    string
	Retries     int
	StartPeriod string
	// StartInterval is the time between checks until the first success (during StartPeriod);
	// Interval applies after that. Omitted from the hash when empty, so adding the field changed
	// no other container's hash.
	StartInterval string `json:",omitempty"`
}

type Condition string

const (
	Started   Condition = "service_started"
	Healthy   Condition = "service_healthy"
	Completed Condition = "service_completed_successfully"
)

type Dependency struct {
	Service   string
	Condition Condition
}

// Container returns the named container, or nil.
func (s *Sandbox) Container(name string) *Container {
	for i := range s.Containers {
		if s.Containers[i].Name == name {
			return &s.Containers[i]
		}
	}
	return nil
}

// Hash is a stable digest of a container's definition (labels excluded), used to detect
// pending changes the same way on every runtime.
func (c Container) Hash() string {
	c.Labels = nil
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(c) // maps encode with sorted keys: stable
	for _, k := range slices.Sorted(maps.Keys(c.Secrets)) {
		v := sha256.Sum256([]byte(c.Secrets[k]))
		h.Write([]byte(k))
		h.Write(v[:])
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// Seal sets the spec-hash and project labels on every container.
func (s *Sandbox) Seal() {
	for i := range s.Containers {
		c := &s.Containers[i]
		if c.Labels == nil {
			c.Labels = map[string]string{}
		}
		c.Labels[LabelProject] = s.Project
		c.Labels[LabelSpecHash] = c.Hash()
	}
}

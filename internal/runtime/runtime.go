// Package runtime starts and inspects sandboxes on a container runtime. Each runtime is a
// Driver; the rest of jailwick only talks to this interface.
package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/jailwick/jailwick/internal/spec"
)

// Driver is one container runtime.
type Driver interface {
	// Name is docker, containerd or apple.
	Name() string
	// Check verifies the runtime is installed and running.
	Check(ctx context.Context) error
	// Info describes the runtime for doctor and status.
	Info(ctx context.Context) Info

	// Up creates or starts the sandbox. Running containers are recreated only when
	// opts.Recreate is set (that ends their sessions).
	Up(ctx context.Context, sb *spec.Sandbox, opts UpOptions) error
	// Down stops and removes the sandbox's containers; with volumes, also its data.
	Down(ctx context.Context, sandbox string, volumes bool) error
	// Containers lists the sandbox's containers (any state). For a project's sandbox that
	// includes its sessions' containers (labelled spec.LabelSession).
	Containers(ctx context.Context, sandbox string) ([]Container, error)
	// Restart restarts one container (stop, then start; a short grace period).
	Restart(ctx context.Context, container string) error
	// Stop stops one container; its restart policy leaves it stopped.
	Stop(ctx context.Context, container string) error
	// Unpause resumes paused containers.
	Unpause(ctx context.Context, containers ...string) error
	// Sandboxes lists the projects that have containers, running or not.
	Sandboxes(ctx context.Context) ([]string, error)
	// AllContainers lists every sandbox's containers (projects' and sessions', any state) in
	// one call; LabelProject says whose each is.
	AllContainers(ctx context.Context) ([]Container, error)

	Exec(ctx context.Context, opts ExecOptions) error
	Logs(ctx context.Context, sandbox, service string, opts LogOptions) error
	// Stats shows CPU and memory of the sandboxes' running containers; once: a single snapshot.
	Stats(ctx context.Context, sandboxes []string, once bool, w io.Writer) error

	Build(ctx context.Context, opts BuildOptions) error
	// ImageID returns the local image ID, or "" when the image isn't present.
	ImageID(ctx context.Context, ref string) (string, error)
	ImageLabels(ctx context.Context, ref string) (map[string]string, error)
	ImagesWithLabel(ctx context.Context, label string) ([]Image, error)
	// ImagesNamed lists the images present of those named (pulled ones, not built by jailwick).
	ImagesNamed(ctx context.Context, refs []string) ([]Image, error)
	RemoveImage(ctx context.Context, ref string) error
	// ContainerImages returns the image IDs of jailwick's containers, in any state.
	ContainerImages(ctx context.Context) ([]string, error)

	// Networks lists the runtime's networks, everyone's (they share its address pools).
	Networks(ctx context.Context) ([]Network, error)
	RemoveNetwork(ctx context.Context, name string) error
}

// Network is one of the runtime's networks.
type Network struct {
	Name string
	// Addressed: it takes an address range from the runtime's pools (bridge networks do; host and
	// none don't).
	Addressed bool
	// Project is the Compose project it belongs to ("" for none): jailwick's are its sandboxes'.
	Project string
}

// Watcher is a driver that can keep its answers current from the runtime's events: while Watch
// runs, container listings and image lookups are answered without asking the runtime, and on
// (when not nil) is told of every jailwick container's event. A long-running jailwick (the app)
// runs it; a one-shot command doesn't need to. It returns when ctx ends, or ErrNoEvents when the
// runtime gives no events (the driver then asks directly).
type Watcher interface {
	Watch(ctx context.Context, on func(ContainerEvent)) error
}

// Info describes a runtime.
type Info struct {
	Name     string
	Version  string
	Platform string // e.g. "OrbStack", "Docker Desktop"
	// HostSuffix: the host reaches container <c> of sandbox <s> at <c>.<s><HostSuffix>
	// (OrbStack: ".orb.local"). Empty: no such names.
	HostSuffix string
	Arch       string // VM architecture: arm64 or amd64
}

// Container is the state of one sandbox container.
type Container struct {
	Name    string // runtime container name
	Service string
	State   string // running, exited, created, paused, restarting, …
	Health  string // healthy, unhealthy, starting or ""
	// Restarts is how often the runtime restarted it after it ended (its restart policy);
	// ExitCode is how its last run ended; Policy is its restart policy ("no": it runs once).
	Restarts int
	ExitCode int
	Policy   string
	ImageID  string
	Labels   map[string]string
	Created  time.Time
}

// Running reports whether the container is up.
func (c Container) Running() bool { return c.State == "running" }

type UpOptions struct {
	Recreate      bool // recreate containers whose definition changed
	RemoveOrphans bool
	// Only recreates just these services, without their dependencies (e.g. one that crashed).
	Only []string
	// Env is extra environment for the runtime process (secret values).
	Env map[string]string
	Out io.Writer
}

type ExecOptions struct {
	Container string
	User      string
	WorkDir   string
	Env       map[string]string
	Cmd       []string
	TTY       bool // interactive terminal
	// Terminal runs the command in this terminal (one end of a pseudo-terminal), its controlling
	// terminal, instead of the process's own; it implies TTY.
	Terminal *os.File
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

type LogOptions struct {
	Follow bool
	Tail   int
	// Since: only lines after this time; Timestamps: each line starts with its time (RFC 3339,
	// nanoseconds), to read on from the last one.
	Since      time.Time
	Timestamps bool
	Out        io.Writer
}

type BuildOptions struct {
	Context    string // folder
	Dockerfile string // path, default <Context>/Dockerfile
	Tag        string
	Platform   string
	Args       map[string]string
	Labels     map[string]string
	Pull       bool // pull the FROM image again (only for images from a registry)
	NoCache    bool // run every step again
	Out        io.Writer
}

type Image struct {
	Ref     string
	ID      string
	Size    string
	Created string
	Labels  map[string]string
}

// ContainerName is the runtime name of a sandbox service's container.
func ContainerName(sandbox, service string) string { return spec.ContainerName(sandbox, service) }

// Open returns the driver by name; empty detects the first available one.
func Open(name, stateDir string) (Driver, error) {
	switch name {
	case "docker":
		return newCLI("docker", "docker", stateDir), nil
	case "containerd":
		return newCLI("containerd", "nerdctl", stateDir), nil
	case "apple":
		return newApple(stateDir), nil
	case "":
		for _, c := range []struct{ name, bin string }{{"docker", "docker"}, {"containerd", "nerdctl"}, {"apple", "container"}} {
			if _, err := exec.LookPath(c.bin); err == nil {
				return Open(c.name, stateDir)
			}
		}
		return nil, fmt.Errorf("no container runtime found: install OrbStack, Docker Desktop, Colima, nerdctl or Apple container")
	}
	return nil, fmt.Errorf("unknown runtime %q (use docker, containerd or apple)", name)
}

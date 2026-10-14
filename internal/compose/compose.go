// Package compose renders a sandbox spec as a Compose file, used by the docker and containerd
// drivers. Output is deterministic: the same spec gives byte-identical files.
package compose

import (
	"bytes"
	"maps"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/jailwick/jailwick/internal/spec"
)

// SecretVar is the name of the process environment variable carrying a secret to Compose.
func SecretVar(key string) string { return "JAILWICK_SECRET_ENV_" + key }

type file struct {
	Name     string             `yaml:"name"`
	Services map[string]service `yaml:"services"`
	Networks map[string]network `yaml:"networks"`
	Volumes  map[string]named   `yaml:"volumes,omitempty"`
}

// named is a volume, or an external one (the parent sandbox's) by its runtime name.
type named struct {
	External bool   `yaml:"external,omitempty"`
	Name     string `yaml:"name,omitempty"`
}

type network struct {
	Internal bool   `yaml:"internal,omitempty"`
	External bool   `yaml:"external,omitempty"`
	Name     string `yaml:"name,omitempty"`
}

type service struct {
	Image       string                `yaml:"image"`
	Platform    string                `yaml:"platform,omitempty"`
	Hostname    string                `yaml:"hostname,omitempty"`
	User        string                `yaml:"user,omitempty"`
	WorkingDir  string                `yaml:"working_dir,omitempty"`
	Entrypoint  []string              `yaml:"entrypoint,omitempty"`
	Command     []string              `yaml:"command,omitempty"`
	Init        bool                  `yaml:"init,omitempty"`
	Restart     string                `yaml:"restart"`
	CapDrop     []string              `yaml:"cap_drop,omitempty"`
	CapAdd      []string              `yaml:"cap_add,omitempty"`
	SecurityOpt []string              `yaml:"security_opt,omitempty"`
	ReadOnly    bool                  `yaml:"read_only,omitempty"`
	NetworkMode string                `yaml:"network_mode,omitempty"`
	Pid         string                `yaml:"pid,omitempty"`
	Networks    []string              `yaml:"networks,omitempty"`
	DependsOn   map[string]dependency `yaml:"depends_on,omitempty"`
	Environment map[string]string     `yaml:"environment,omitempty"`
	Volumes     []volume              `yaml:"volumes,omitempty"`
	Tmpfs       []string              `yaml:"tmpfs,omitempty"`
	MemLimit    string                `yaml:"mem_limit,omitempty"`
	CPUs        string                `yaml:"cpus,omitempty"`
	PidsLimit   int                   `yaml:"pids_limit,omitempty"`
	ShmSize     string                `yaml:"shm_size,omitempty"`
	Healthcheck *healthcheck          `yaml:"healthcheck,omitempty"`
	Logging     *logging              `yaml:"logging,omitempty"`
	Labels      map[string]string     `yaml:"labels,omitempty"`
}

type dependency struct {
	Condition string `yaml:"condition"`
}

type volume struct {
	Type     string        `yaml:"type"`
	Source   string        `yaml:"source"`
	Target   string        `yaml:"target"`
	ReadOnly bool          `yaml:"read_only,omitempty"`
	Bind     *bind         `yaml:"bind,omitempty"`
	Volume   *volumeOption `yaml:"volume,omitempty"`
}

type volumeOption struct {
	Subpath string `yaml:"subpath"`
}

type bind struct {
	CreateHostPath bool `yaml:"create_host_path"`
}

type healthcheck struct {
	Test          []string `yaml:"test"`
	Interval      string   `yaml:"interval,omitempty"`
	Retries       int      `yaml:"retries,omitempty"`
	StartPeriod   string   `yaml:"start_period,omitempty"`
	StartInterval string   `yaml:"start_interval,omitempty"`
}

type logging struct {
	Driver  string            `yaml:"driver"`
	Options map[string]string `yaml:"options"`
}

// rotatedLogs keeps each container's log bounded. json-file is set explicitly: `jw logs` and
// `jw blocked` read logs back, which some daemon defaults (syslog, journald) don't allow.
var rotatedLogs = &logging{Driver: "json-file", Options: map[string]string{"max-size": "10m", "max-file": "3"}}

// Render returns the Compose file and the environment the compose process needs (secrets,
// referenced as ${…} so their values never touch the disk).
func Render(s *spec.Sandbox) ([]byte, map[string]string, error) {
	f := file{
		Name:     s.Name,
		Services: map[string]service{},
		Networks: map[string]network{},
		Volumes:  map[string]named{},
	}
	// The parent's resources are referenced by their runtime names: <parent>_<name>.
	external := func(name string) string { return s.Parent + "_" + name }
	for _, n := range s.Networks {
		if n.External {
			f.Networks[n.Name] = network{External: true, Name: external(n.Name)}
		} else {
			f.Networks[n.Name] = network{Internal: n.Internal}
		}
	}
	for _, v := range s.Volumes {
		if s.Parent != "" {
			f.Volumes[v] = named{External: true, Name: external(v)}
		} else {
			f.Volumes[v] = named{}
		}
	}
	// App volumes belong to no sandbox: external, so Compose never creates or removes them.
	for _, v := range s.AppVolumes {
		f.Volumes[v] = named{External: true, Name: v}
	}
	procEnv := map[string]string{}
	for _, c := range s.Containers {
		sv := toService(c, procEnv)
		if c.SharePIDs != "" {
			sv.Pid = "container:" + spec.ContainerName(s.Parent, c.SharePIDs)
		}
		f.Services[c.Name] = sv
	}
	var buf bytes.Buffer
	buf.WriteString("# Generated by jailwick. Don't edit: changes are overwritten.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), procEnv, nil
}

func toService(c spec.Container, procEnv map[string]string) service {
	sv := service{
		Image:      c.Image,
		Platform:   c.Platform,
		Hostname:   c.Hostname,
		User:       c.User,
		WorkingDir: esc(c.WorkDir),
		Entrypoint: escAll(c.Entrypoint),
		Command:    escAll(c.Command),
		Init:       c.Init,
		Restart:    "unless-stopped",
		CapDrop:    c.CapDrop,
		CapAdd:     c.CapAdd,
		ReadOnly:   c.ReadOnlyRoot,
		Networks:   c.Networks,
		Tmpfs:      c.Tmpfs,
		MemLimit:   c.Memory,
		CPUs:       c.CPUs,
		PidsLimit:  c.Pids,
		ShmSize:    c.Shm,
		Logging:    rotatedLogs,
		Labels:     c.Labels,
	}
	if c.OneShot || c.Ephemeral {
		sv.Restart = "no"
	}
	if c.NoNewPrivileges {
		sv.SecurityOpt = []string{"no-new-privileges:true"}
	}
	if c.NoNetwork {
		sv.NetworkMode = "none"
	}
	if len(c.DependsOn) > 0 {
		sv.DependsOn = map[string]dependency{}
		for _, d := range c.DependsOn {
			sv.DependsOn[d.Service] = dependency{Condition: string(d.Condition)}
		}
	}
	if len(c.Env)+len(c.Secrets) > 0 {
		sv.Environment = map[string]string{}
		for k, v := range c.Env {
			sv.Environment[k] = esc(v)
		}
		for _, k := range slices.Sorted(maps.Keys(c.Secrets)) {
			sv.Environment[k] = "${" + SecretVar(k) + "}"
			procEnv[SecretVar(k)] = c.Secrets[k]
		}
	}
	for _, m := range c.Mounts {
		v := volume{Type: string(m.Type), Source: esc(m.Source), Target: esc(m.Target), ReadOnly: m.ReadOnly}
		if m.Type == spec.Bind {
			// A missing host path is an error, never an empty folder created on the host.
			v.Bind = &bind{CreateHostPath: false}
		}
		if m.Subpath != "" {
			v.Volume = &volumeOption{Subpath: esc(m.Subpath)}
		}
		sv.Volumes = append(sv.Volumes, v)
	}
	if h := c.Health; h != nil {
		sv.Healthcheck = &healthcheck{Test: escAll(h.Test), Interval: h.Interval, Retries: h.Retries,
			StartPeriod: h.StartPeriod, StartInterval: h.StartInterval}
	}
	return sv
}

// esc protects literal dollar signs from Compose interpolation.
func esc(s string) string { return strings.ReplaceAll(s, "$", "$$") }

func escAll(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = esc(s)
	}
	return out
}

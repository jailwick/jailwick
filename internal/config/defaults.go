package config

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/jailwick/jailwick/internal/presets"
)

// Defaults for unset project settings.
const (
	DefaultPostgresImage = "postgres:17-alpine"
	DefaultRedisImage    = "redis:7-alpine"
	DefaultMongoImage    = "mongo:8"
	DefaultUser          = "node"
	DefaultHome          = "/home/node"
	DefaultAgentCommand  = "claude"
)

// Claude logins: where a sandbox keeps Claude's sign-in.
const (
	LoginApp     = "app"     // one sign-in for every project that uses it
	LoginProject = "project" // the project's own
)

// Logins are the valid claude.login values.
var Logins = []string{LoginApp, LoginProject}

// Known service names.
var ServiceNames = []string{"postgres", "redis", "mongo"}

// Resolved returns a copy of p with defaults filled in, taking global defaults first.
// Callers use the result for generation; the saved file stays as the user wrote it.
func (p Project) Resolved(g Global) Project {
	r := p
	r.Services = slices.Clone(p.Services)
	r.TestServices = slices.Clone(p.TestServices)
	or := func(dst *string, vals ...string) {
		for _, v := range vals {
			if *dst != "" {
				return
			}
			*dst = v
		}
	}
	or(&r.Git.Name, g.Git.Name, "jailwick")
	or(&r.Git.Email, g.Git.Email, "jailwick@localhost")
	or(&r.Postgres.User, "app")
	or(&r.Postgres.Password, "app")
	or(&r.Postgres.DB, "app")
	or(&r.Postgres.Image, DefaultPostgresImage)
	or(&r.Postgres.Timezone, "UTC")
	or(&r.Redis.Image, DefaultRedisImage)
	or(&r.Mongo.Image, DefaultMongoImage)
	or(&r.Mongo.User, "app")
	or(&r.Mongo.Password, "app")
	or(&r.Test.PostgresImage, r.Postgres.Image)
	or(&r.Test.RedisImage, r.Redis.Image)
	or(&r.Test.MongoImage, r.Mongo.Image)
	or(&r.Test.TmpfsSize, "1g")

	img := &r.Image
	if img.Base == "" && img.Dockerfile == "" && img.Name == "" {
		img.Base, img.Dockerfile, img.Context, img.Name = g.Image.Base, g.Image.Dockerfile, g.Image.Context, g.Image.Name
	}
	or(&img.User, g.Image.User, DefaultUser)
	or(&img.Home, g.Image.Home, DefaultHome)
	or(&img.ClaudeCodeVersion, g.Image.ClaudeCodeVersion, "latest")
	or(&img.Platform, g.Image.Platform)

	or(&r.Resources.Memory, "12g")
	or(&r.Resources.CPUs, "6")
	or(&r.Resources.Shm, "1g")
	if r.Resources.Pids == 0 {
		r.Resources.Pids = 4096
	}
	if r.Resources.NodeHeapMB == 0 {
		r.Resources.NodeHeapMB = 4096
	}

	or(&r.Claude.UserMD, g.Claude.UserMD)
	or(&r.Claude.MCP, g.Claude.MCP, "default")
	if r.Claude.YOLO == nil {
		r.Claude.YOLO = g.Claude.YOLO
	}
	or(&r.Claude.Login, g.Claude.Login, LoginApp)
	if r.Claude.Chrome == nil {
		r.Claude.Chrome = g.Claude.Chrome
	}
	if r.Claude.Profiles == nil {
		r.Claude.Profiles = slices.Clone(g.Claude.Profiles)
	}
	or(&r.Agent.Command, DefaultAgentCommand)
	return r
}

// YOLO reports whether sessions skip Claude's permission prompts (off by default).
func (p Project) YOLO() bool { return p.Claude.YOLO != nil && *p.Claude.YOLO }

// Chrome reports whether sessions use Claude in Chrome by default (off by default).
func (p Project) Chrome() bool { return p.Claude.Chrome != nil && *p.Claude.Chrome }

// AppLogin reports whether the project uses the app's Claude login (the default).
func (p Project) AppLogin() bool { return p.Claude.Login != LoginProject }

// AddAllowPresets adds allowlist presets to the project; they must exist.
func (p *Project) AddAllowPresets(names ...string) error {
	if err := presets.Check(names); err != nil {
		return err
	}
	for _, n := range names {
		if !slices.Contains(p.AllowPresets, n) {
			p.AllowPresets = append(p.AllowPresets, n)
		}
	}
	return nil
}

// RemoveAllowPresets removes allowlist presets from the project. Presets that are always on
// can't be removed.
func (p *Project) RemoveAllowPresets(names ...string) error {
	for _, n := range names {
		if pr, ok := presets.Get(n); ok && pr.Always {
			return fmt.Errorf("%s is always on: every sandbox needs it", n)
		}
	}
	p.AllowPresets = slices.DeleteFunc(p.AllowPresets, func(n string) bool { return slices.Contains(names, n) })
	return nil
}

// Uses reports whether the project runs the dev service s.
func (p Project) Uses(s string) bool { return slices.Contains(p.Services, s) }

// UsesTest reports whether the project runs the test service s.
func (p Project) UsesTest(s string) bool { return slices.Contains(p.TestServices, s) }

// NodeHeap is the Node heap limit as a flag value.
func (p Project) NodeHeap() string { return strconv.Itoa(p.Resources.NodeHeapMB) }

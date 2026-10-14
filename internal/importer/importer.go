// Package importer converts a csb setup (projects/*.env, workspaces/*.env, config/ notes, the
// proxy allowlist) into jailwick configuration. It only reads the csb folder.
package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/detect"
	"github.com/jailwick/jailwick/internal/envfile"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/presets"
	"github.com/jailwick/jailwick/internal/proxy"
)

// Plan is what an import would write.
type Plan struct {
	Projects   []Project
	Workspaces []Workspace
	Allow      []string // allowlist entries beyond the built-in presets
	Skipped    []string // with reasons
}

// Project is one converted project, its secrets (names → values) and notes.
type Project struct {
	Config  config.Project
	Secrets map[string]string
	Notes   []byte
}

// Workspace is one converted workspace and its notes.
type Workspace struct {
	Config config.Workspace
	Notes  []byte
}

// secretKeys are moved to the secret store, never into YAML.
var secretKeys = []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "GH_TOKEN"}

// Read builds the plan from a csb folder.
func Read(setup string) (*Plan, error) {
	if _, err := os.Stat(filepath.Join(setup, "projects")); err != nil {
		return nil, fmt.Errorf("%s doesn't look like a csb setup (no projects/ folder)", setup)
	}
	plan := &Plan{}
	files, _ := filepath.Glob(filepath.Join(setup, "projects", "*.env"))
	slices.Sort(files)
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".env")
		if strings.HasPrefix(name, "_") {
			continue
		}
		env, err := parseEnv(f)
		if err != nil {
			plan.Skipped = append(plan.Skipped, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		p := convertProject(name, env)
		if err := p.Config.Validate(); err != nil {
			plan.Skipped = append(plan.Skipped, err.Error())
			continue
		}
		p.Notes = notes(filepath.Join(setup, "config", "projects", name, "notes.md"))
		plan.Projects = append(plan.Projects, p)
	}
	wfiles, _ := filepath.Glob(filepath.Join(setup, "workspaces", "*.env"))
	slices.Sort(wfiles)
	for _, f := range wfiles {
		name := strings.TrimSuffix(filepath.Base(f), ".env")
		if strings.HasPrefix(name, "_") {
			continue
		}
		env, err := parseEnv(f)
		if err != nil {
			plan.Skipped = append(plan.Skipped, fmt.Sprintf("workspace %s: %v", name, err))
			continue
		}
		w := Workspace{Config: convertWorkspace(name, env)}
		w.Notes = notes(filepath.Join(setup, "config", "workspaces", name, "notes.md"))
		plan.Workspaces = append(plan.Workspaces, w)
	}
	if data, err := os.ReadFile(filepath.Join(setup, "proxy", "allow", "allowlist.txt")); err == nil {
		covered, extra := fromAllowlist(assets.ParseDomains(data))
		plan.Allow = extra
		// The projects keep what csb allowed them, plus what their repos call for.
		for i := range plan.Projects {
			p := &plan.Projects[i].Config
			narrow, _ := presets.Split(detect.Repo(paths.Real(p.Dir)).Presets)
			p.AllowPresets = presets.Explicit(append(covered, narrow...))
		}
	}
	return plan, nil
}

// parseEnv reads KEY=value lines: the last assignment wins, quotes and a leading ~ removed.
func parseEnv(file string) (map[string]string, error) {
	env, err := envfile.Read(file)
	for k, v := range env {
		env[k] = paths.Expand(v)
	}
	return env, err
}

func fields(s string) []string { return strings.Fields(s) }

func convertProject(name string, env map[string]string) Project {
	get := func(k string) string { return env[k] }
	p := config.Project{Name: name, Dir: get("PROJECT_DIR"), Workspace: get("WORKSPACE")}
	p.WorkspaceRelated = fields(get("WORKSPACE_RELATED"))
	p.WorkspaceExclude = fields(get("WORKSPACE_EXCLUDE"))
	if n := get("GIT_AUTHOR_NAME"); n != "" && n != "Claude Sandbox" {
		p.Git.Name = n
	}
	if e := get("GIT_AUTHOR_EMAIL"); e != "" && e != "claude@sandbox.local" {
		p.Git.Email = e
	}
	p.Timezone = get("SANDBOX_TZ")

	// csb ran postgres and redis when SANDBOX_SERVICES was unset.
	services, set := env["SANDBOX_SERVICES"]
	if !set {
		services = "postgres redis"
	}
	if services != "none" {
		p.Services = fields(services)
	}
	p.TestServices = fields(get("SANDBOX_TEST_SERVICES"))

	if p.Uses("postgres") {
		p.Postgres = config.Postgres{User: get("POSTGRES_USER"), Password: get("POSTGRES_PASSWORD"), DB: get("POSTGRES_DB"),
			Image: get("POSTGRES_IMAGE"), Timezone: get("POSTGRES_TIMEZONE")}
		for _, e := range fields(get("POSTGRES_EXTRA_DBS")) {
			parts := strings.SplitN(e, ":", 3)
			x := config.ExtraDB{Name: parts[0]}
			if len(parts) == 3 {
				x.User, x.Password = parts[1], parts[2]
			}
			p.Postgres.ExtraDBs = append(p.Postgres.ExtraDBs, x)
		}
	}
	if p.Uses("redis") {
		p.Redis.Image = get("REDIS_IMAGE")
		pws := fields(get("REDIS_PASSWORDS"))
		if first := get("REDIS_PASSWORD_FIRST"); first != "" {
			pws = append([]string{first}, slices.DeleteFunc(pws, func(s string) bool { return s == first })...)
		}
		p.Redis.Passwords = pws
	}
	if p.Uses("mongo") {
		p.Mongo = config.Mongo{Image: get("MONGO_IMAGE"), User: get("MONGO_USER"), Password: get("MONGO_PASSWORD")}
	}
	p.Test = config.TestConfig{PostgresImage: get("SANDBOX_TEST_POSTGRES_IMAGE"), RedisImage: get("SANDBOX_TEST_REDIS_IMAGE"),
		MongoImage: get("SANDBOX_TEST_MONGO_IMAGE"), TmpfsSize: get("SANDBOX_TEST_TMPFS_SIZE")}
	p.LocalForwards = fields(get("LOCAL_FORWARDS"))
	p.Worktree.CopyFiles = fields(get("WT_COPY_FILES"))

	if v := get("CLAUDE_CODE_VERSION"); v != "latest" {
		p.Image.ClaudeCodeVersion = v
	}
	p.Image.Playwright.Browsers = fields(get("PLAYWRIGHT_BROWSERS"))
	p.Image.Playwright.Versions = fields(get("PLAYWRIGHT_PROJECT_VERSIONS"))
	p.Image.AptPackages = fields(get("EXTRA_APT_PACKAGES"))
	p.Image.Android = get("INSTALL_ANDROID") == "true"
	if pl := get("CLAUDE_PLATFORM"); pl == "linux/amd64" && !p.Image.Android {
		p.Image.Platform = pl
	}
	p.Resources.Memory = get("CLAUDE_MEM_LIMIT")
	p.Resources.CPUs = get("CLAUDE_CPUS")
	if n, err := strconv.Atoi(get("NODE_MAX_HEAP_MB")); err == nil && n != 4096 {
		p.Resources.NodeHeapMB = n
	}
	p.Claude.UserMD = get("CLAUDE_USER_MD")
	// csb started sessions with --dangerously-skip-permissions; keep that for imported projects.
	yes := true
	p.Claude.YOLO = &yes

	secrets := map[string]string{}
	for _, k := range secretKeys {
		if v := get(k); v != "" {
			secrets[k] = v
		}
	}
	return Project{Config: p, Secrets: secrets}
}

func convertWorkspace(name string, env map[string]string) config.Workspace {
	w := config.Workspace{Name: name, Root: env["WORKSPACE_ROOT"], RootNotes: env["WORKSPACE_ROOT_NOTES"],
		Exclude: fields(env["WORKSPACE_EXCLUDE"]), Handoff: env["WORKSPACE_HANDOFF"]}
	for _, m := range fields(env["WORKSPACE_MEMBERS"]) {
		if n, p, ok := strings.Cut(m, "="); ok {
			w.Members = append(w.Members, config.Member{Name: n, Path: p})
		} else {
			w.Members = append(w.Members, config.Member{Name: m})
		}
	}
	return w
}

// notes reads a notes file, pointing csb's in-container paths and commands at jailwick's.
func notes(file string) []byte {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	r := strings.NewReplacer("/etc/sandbox/", "/etc/jailwick/", "csb restart", "jw restart", "csb up", "jw up",
		"make restart", "jw restart", "EXTRA_APT_PACKAGES", "image.apt_packages")
	return []byte(r.Replace(string(data)))
}

// fromAllowlist splits the csb allowlist (which applied to every project): the presets that cover
// some of its entries, and the entries no preset covers.
func fromAllowlist(entries []string) (covered, extra []string) {
	for _, e := range entries {
		host := strings.TrimPrefix(e, ".")
		var by []string
		for _, p := range presets.Catalogue() {
			// A preset covers a .domain entry only if it has that exact entry.
			if slices.ContainsFunc(p.Hosts, func(h string) bool { return proxy.Covers(h, host) && (!strings.HasPrefix(e, ".") || h == e) }) {
				by = append(by, p.Name)
			}
		}
		if len(by) == 0 {
			extra = append(extra, e)
		}
		covered = append(covered, by...)
	}
	return covered, extra
}

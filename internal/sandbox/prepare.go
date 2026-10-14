// Package sandbox runs the life cycle of a project's sandbox: it prepares the generated files,
// makes sure images exist, starts and restarts containers, and reports their state.
package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/claudesettings"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/secrets"
	"github.com/jailwick/jailwick/internal/spec"
	"github.com/jailwick/jailwick/internal/workspace"
)

// Prepared is a project ready to start: its spec and what sessions need.
type Prepared struct {
	Project config.Project // resolved
	Spec    *spec.Sandbox
	Images  image.Result
	// AddDirs are workspace members passed to the agent as extra working folders.
	AddDirs []string
	// MCPConfig and Settings are container paths for the agent's flags.
	MCPConfig string
	Settings  string
	// Inputs built the spec; sessions build their own sandboxes from them.
	Inputs spec.Inputs
	// HostSuffix is the runtime's host name suffix ("" without host names).
	HostSuffix string
}

// Prepare writes the generated files, ensures images and builds the spec. It never starts
// anything.
func (m *Manager) Prepare(ctx context.Context, name string, imgOpts image.Options) (*Prepared, error) {
	raw, err := m.Store.Project(name)
	if err != nil {
		return nil, err
	}
	if err := raw.Validate(); err != nil {
		return nil, err
	}
	p := raw.Resolved(m.Global)
	p.Dir = paths.Real(p.Dir)

	info := m.Driver.Info(ctx)
	images, err := m.images().Ensure(ctx, p, imgOpts)
	if err != nil {
		return nil, err
	}

	stateDir := paths.ProjectState(p.Name)
	genDir := filepath.Join(stateDir, "generated")
	ws, err := workspace.Generate(m.Store, p, filepath.Join(m.StateDir, "mask"))
	if err != nil {
		return nil, err
	}
	if ws != nil && m.Log != nil {
		for _, w := range ws.Warnings {
			m.Log("%s", w)
		}
	}

	files := fileSet{}
	shared := filepath.Join(m.StateDir, "shared")
	for n, data := range assets.Dir("claude") {
		if n != "pointer.md" && n != "settings.json" { // the settings are merged per project
			files[filepath.Join(shared, n)] = data
		}
	}
	files[filepath.Join(shared, "worktree-env.sh")] = assets.MustRead("shell/worktree-env.sh")
	pointer := filepath.Join(m.StateDir, "claude-pointer.md")
	files[pointer] = assets.MustRead("claude/pointer.md")
	pgInit := filepath.Join(m.StateDir, "postgres-init")
	files[filepath.Join(pgInit, "10-extra-dbs.sh")] = assets.MustRead("postgres/10-extra-dbs.sh")

	tools := toolsetOf(p, images.JailwickBase)
	files[filepath.Join(genDir, "tools.md")] = tools.notes() // nil removes it
	if ws != nil {
		files[filepath.Join(genDir, "workspace.md")] = ws.Markdown
	} else {
		files[filepath.Join(genDir, "workspace.md")] = nil
	}

	allow, err := m.allowlist(p, false)
	if err != nil {
		return nil, err
	}
	files[allowFile(p.Name, "")] = proxy.File(allow)
	settings, err := claudesettings.Build(m.Store, p)
	if err != nil {
		return nil, err
	}
	files[filepath.Join(genDir, "settings.json")] = settings

	mcp, err := mcpConfig(p, tools, genDir, files)
	if err != nil {
		return nil, err
	}
	notesDir := m.Store.ProjectNotesDir(p.Name)

	agentBin, err := m.agentBinary(info.Arch)
	if err != nil {
		return nil, err
	}
	for _, d := range []string{notesDir, m.Store.ClaudeDir(), genDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}

	hostName := "localhost"
	if info.HostSuffix != "" {
		hostName = spec.SandboxService + "." + spec.Prefix + p.Name + info.HostSuffix
	}
	userMD, err := userMD(p)
	if err != nil {
		return nil, err
	}
	secretVals, err := secrets.Resolve(m.Secrets, p.Name, append(append([]string{}, secrets.Known...), p.SecretEnv...))
	if err != nil {
		return nil, err
	}

	in := spec.Inputs{
		Project: p, Image: images.Sandbox, ProxyImage: images.Proxy,
		AgentBinary: agentBin, PointerFile: pointer, SharedDir: shared, GeneratedDir: genDir,
		NotesDir: notesDir, UserDir: m.Store.ClaudeDir(), AllowDir: allowDir(p.Name, ""), PostgresInit: pgInit,
		UserMD: userMD, Timezone: hostutil.Timezone(p.Timezone), HostName: hostName,
		NodeModules: PackageDirs(p), Secrets: secretVals,
	}
	var addDirs []string
	if ws != nil {
		in.Workspace = ws.Extra
		addDirs = ws.AddDirs
	}
	sb := spec.Build(in)

	agentCfg := agentConfig(p, hostName, sb, ws, allow, tools)
	agentCfg.NodeModules = []string{filepath.Join(p.Dir, "node_modules")}
	for _, d := range in.NodeModules {
		agentCfg.NodeModules = append(agentCfg.NodeModules, filepath.Join(p.Dir, d, "node_modules"))
	}
	data, _ := json.MarshalIndent(agentCfg, "", "  ")
	files[filepath.Join(genDir, "agent.json")] = data
	if err := files.write(); err != nil {
		return nil, err
	}
	return &Prepared{
		Project: p, Spec: sb, Images: images, AddDirs: addDirs, MCPConfig: mcp, Settings: settingsFile,
		Inputs: in, HostSuffix: info.HostSuffix,
	}, nil
}

func (m *Manager) images() *image.Builder {
	return &image.Builder{Driver: m.Driver, StateDir: m.StateDir, Out: m.Out, Log: m.Log}
}

// settingsFile is where the main container finds the project's merged Claude settings.
const settingsFile = "/etc/jailwick/generated/settings.json"

// allowlist is a sandbox's egress allowlist; chrome adds Claude in Chrome's relay (a session that
// opted in).
func (m *Manager) allowlist(p config.Project, chrome bool) ([]string, error) {
	return proxy.List(m.sources(p, chrome))
}

// sources are what a sandbox's allowlist is made of.
func (m *Manager) sources(p config.Project, chrome bool) proxy.Sources {
	var user []string
	if data, err := os.ReadFile(m.Store.AllowFile()); err == nil {
		user = assets.ParseDomains(data)
	}
	return proxy.Sources{Presets: p.AllowPresets, Global: m.Global.Allow, User: user, Project: p.Allow, Chrome: chrome}
}

// sessionState holds a session's own files: <state>/projects/<p>/sessions/<id>.
func sessionState(project, id string) string {
	return filepath.Join(paths.ProjectState(project), "sessions", id)
}

// allowDir is the folder a proxy reads its allowlist from: the project's, or a session's own
// (session != "").
func allowDir(project, session string) string {
	if session == "" {
		return filepath.Join(paths.ProjectState(project), "allow")
	}
	return filepath.Join(sessionState(project, session), "allow")
}

// ClipboardDir is the host folder a session's pasted image goes to (mounted read-only in its
// container at agentapi.ClipboardDir).
func ClipboardDir(project, session string) string {
	return filepath.Join(sessionState(project, session), "clipboard")
}

func allowFile(project, session string) string {
	return filepath.Join(allowDir(project, session), "allowlist.txt")
}

// mcpConfig picks the MCP servers file: Playwright when the project has browsers, none, or the
// user's own file.
func mcpConfig(p config.Project, tools toolset, genDir string, files fileSet) (string, error) {
	switch v := p.Claude.MCP; {
	case v == "none" || (v == "default" && tools.mcp() == nil):
		return "/etc/jailwick/shared/mcp-none.json", nil
	case v == "default":
		files[filepath.Join(genDir, "mcp.json")] = tools.mcp()
		return "/etc/jailwick/generated/mcp.json", nil
	default:
		data, err := os.ReadFile(paths.Expand(v))
		if err != nil {
			return "", fmt.Errorf("claude.mcp: %w", err)
		}
		files[filepath.Join(genDir, "mcp.json")] = data
		return "/etc/jailwick/generated/mcp.json", nil
	}
}

// agentBinary writes the embedded agent for the runtime's architecture to a content-addressed
// file, so a new jailwick version shows up as a pending change.
func (m *Manager) agentBinary(arch string) (string, error) {
	if arch == "" {
		arch = hostutil.Arch()
	}
	data, err := assets.Agent(arch)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	file := filepath.Join(m.StateDir, "bin", "jailwick-agent-"+arch+"-"+hex.EncodeToString(sum[:6]))
	if !hostutil.Exists(file) {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return "", err
		}
		if err := config.WriteFileAtomic(file, data, 0o755); err != nil {
			return "", err
		}
	}
	// Where this agent comes from, for the pending changes a restart would apply. The first
	// jailwick build that wrote these bytes names them.
	if info := assets.AgentInfoOf(); info != nil && !hostutil.Exists(file+".json") {
		if data, err := json.Marshal(info); err == nil {
			_ = config.WriteFileAtomic(file+".json", data, 0o644)
		}
	}
	return file, nil
}

// agentInfoAt reads the AgentInfo written next to an agent file; nil when there is none.
func agentInfoAt(file string) *assets.AgentInfo {
	data, err := os.ReadFile(file + ".json")
	if err != nil {
		return nil
	}
	return assets.ParseAgentInfo(data)
}

func userMD(p config.Project) (string, error) {
	switch v := p.Claude.UserMD; v {
	case "none":
		return "", nil
	case "":
		f := filepath.Join(paths.Home(), ".claude", "CLAUDE.md")
		if hostutil.Exists(f) {
			return paths.Real(f), nil
		}
		return "", nil
	default:
		f := paths.Expand(v)
		if !hostutil.Exists(f) {
			return "", fmt.Errorf("claude.user_md: %s doesn't exist", v)
		}
		return paths.Real(f), nil
	}
}

func agentConfig(p config.Project, host string, sb *spec.Sandbox, ws *workspace.Result, allow []string, tools toolset) agentapi.Config {
	c := agentapi.Config{
		Contract: agentapi.Contract, Project: p.Name, Dir: p.Dir, Home: p.Image.Home, Command: p.Agent.Command, HostName: host,
		ClaudeLogin: p.Claude.Login,
		Services:    p.Services, TestServices: p.TestServices,
		Forwards: p.LocalForwards, CopyFiles: p.Worktree.CopyFiles, Setup: p.Worktree.Setup, Run: p.Run,
	}
	if p.Uses("postgres") {
		pg := &agentapi.Postgres{User: p.Postgres.User, Password: p.Postgres.Password, DB: p.Postgres.DB}
		for _, e := range p.Postgres.ExtraDBs {
			pg.ExtraDBs = append(pg.ExtraDBs, e.Name)
		}
		c.Postgres = pg
	}
	if p.Uses("redis") {
		c.Redis = &agentapi.Redis{Passwords: p.Redis.Passwords}
	}
	if ws != nil {
		c.Workspace = ws.Info
	}
	claude := sb.Container(spec.SandboxService)
	for _, mt := range claude.Mounts {
		c.Expected.Mounts = append(c.Expected.Mounts, mt.Target)
		if mt.ReadOnly {
			c.Expected.ReadOnly = append(c.Expected.ReadOnly, mt.Target)
		}
	}
	for _, t := range claude.Tmpfs {
		target, _, _ := strings.Cut(t, ":")
		c.Expected.Mounts = append(c.Expected.Mounts, target)
	}
	c.Expected.Hosts = allow
	c.Expected.Tools = tools.tools()
	c.Expected.Browser = tools.browser()
	return c
}

// fileSet maps host paths to content; nil content removes the file. Files are rewritten only
// when they change.
type fileSet map[string][]byte

func (fs fileSet) write() error {
	for file, data := range fs {
		if data == nil {
			if err := os.Remove(file); err != nil && !errors.Is(err, errNotExist) {
				return err
			}
			continue
		}
		if old, err := os.ReadFile(file); err == nil && bytes.Equal(old, data) {
			continue
		}
		perm := os.FileMode(0o644)
		if strings.HasSuffix(file, ".sh") {
			perm = 0o755
		}
		if err := config.WriteFileAtomic(file, data, perm); err != nil {
			return err
		}
	}
	return nil
}

var errNotExist = fs.ErrNotExist

// Allowlist returns a project's current egress allowlist.
func (m *Manager) Allowlist(name string) ([]string, error) {
	raw, err := m.Store.Project(name)
	if err != nil {
		return nil, err
	}
	return m.allowlist(raw.Resolved(m.Global), false)
}

// AllowGroups returns where a project's allowlist entries come from.
func (m *Manager) AllowGroups(name string) ([]proxy.Group, error) {
	raw, err := m.Store.Project(name)
	if err != nil {
		return nil, err
	}
	return proxy.Groups(m.sources(raw.Resolved(m.Global), false))
}

// ReloadProxy rewrites a project's allowlists (its own and each session's) and makes its running
// proxies read them.
func (m *Manager) ReloadProxy(ctx context.Context, name string) error {
	raw, err := m.Store.Project(name)
	if err != nil {
		return err
	}
	p := raw.Resolved(m.Global)
	boxes, err := m.Boxes(ctx, name)
	if err != nil {
		return err
	}
	plain, err := m.allowlist(p, false)
	if err != nil {
		return err
	}
	withChrome, err := m.allowlist(p, true)
	if err != nil {
		return err
	}
	files := fileSet{allowFile(name, ""): proxy.File(plain)}
	for _, b := range boxes {
		allow := plain
		if b.Chrome {
			allow = withChrome
		}
		files[allowFile(name, b.ID)] = proxy.File(allow)
	}
	if err := files.write(); err != nil {
		return err
	}
	proxies, err := m.Proxies(ctx, name)
	if err != nil {
		return err
	}
	var errs []error
	for _, c := range proxies {
		errs = append(errs, m.Driver.Exec(ctx, runtime.ExecOptions{
			Container: c,
			Cmd:       []string{"squid", "-k", "reconfigure", "-f", "/etc/squid/squid.conf"},
			Stdout:    io.Discard, Stderr: io.Discard,
		}))
	}
	return errors.Join(errs...)
}

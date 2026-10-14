package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/presets"
	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/secrets"
	"github.com/jailwick/jailwick/internal/spec"
	"github.com/jailwick/jailwick/internal/workspace"
)

// ---- views ----

type RuntimeView struct {
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Platform string `json:"platform,omitempty"`
	Arch     string `json:"arch,omitempty"`
	URLs     bool   `json:"urls"` // per-container host names (OrbStack)
	Error    string `json:"error,omitempty"`
}

type ProjectSummary struct {
	Name      string   `json:"name"`
	Dir       string   `json:"dir"`
	Workspace string   `json:"workspace,omitempty"`
	Services  []string `json:"services"`
	Running   bool     `json:"running"`
	Sessions  int      `json:"sessions"`
	Busy      int      `json:"busy"`
	Error     string   `json:"error,omitempty"`
	// BuildFailed: the last start's image build failed (the project page has its output).
	BuildFailed bool `json:"build_failed,omitempty"`
}

// Overview is what the configuration says, without asking the runtime: it answers at once.
// Live has the rest.
type Overview struct {
	Version    string           `json:"version"`
	Projects   []ProjectSummary `json:"projects"`
	Workspaces []string         `json:"workspaces"`
}

// Live is what the runtime reports: the projects whose sandbox is up, by name, and the starts,
// restarts and stops under way (Ops, by project): while one runs its containers come and go.
type Live struct {
	Runtime  RuntimeView            `json:"runtime"`
	Projects map[string]ProjectLive `json:"projects"`
	Ops      map[string]string      `json:"ops"`
	// Health: the projects whose sandbox is paused, recovering, or has containers not doing
	// their work.
	Health map[string]sandbox.Health `json:"health"`
}

type ProjectLive struct {
	Sessions int `json:"sessions"`
	Busy     int `json:"busy"`
}

type ProjectDetail struct {
	ProjectSummary
	Config     any                 `json:"config"` // the resolved project, with its YAML key names
	Containers []runtime.Container `json:"containers"`
	Pending    []string            `json:"pending"` // one line per container
	Changes    []sandbox.Change    `json:"changes"` // the same, item by item, before and after
	List       []sandbox.Session   `json:"session_list"`
	Worktrees  []agentapi.Worktree `json:"worktrees"`
	// WorktreeJobs: worktrees being made from the app, or made with a problem.
	WorktreeJobs []WorktreeJob   `json:"worktree_jobs"`
	Runs         []agentapi.Run  `json:"runs"` // the run command, where it runs
	NodeModules  NodeModulesView `json:"node_modules"`
	URLBase      string          `json:"url_base"` // the project's container: http://host (append :port)
	Allow        []string        `json:"allow"`
	// AllowGroups: where the allowlist comes from (each preset, the project, the user, config.yaml).
	AllowGroups []proxy.Group     `json:"allow_groups"`
	Files       map[string]string `json:"files"`           // config, notes
	Commands    map[string]string `json:"commands"`        // claude, shell: the commands that start them, to copy
	Build       *BuildView        `json:"build,omitempty"` // an image being built for a start
	Op          string            `json:"op,omitempty"`    // starting, restarting or stopping
	Health      sandbox.Health    `json:"health"`          // paused, recovering, containers not doing their work
}

type SessionView struct {
	Project string `json:"project"`
	sandbox.Session
}

type NetworkView struct {
	User    []string         `json:"user"`
	Presets []presets.Preset `json:"presets"`
	// PresetProblems: preset files of the user's that can't be read (left out of Presets).
	PresetProblems string              `json:"preset_problems,omitempty"`
	ProjectPresets map[string][]string `json:"project_presets"` // allow_presets, by project
	Projects       map[string][]string `json:"projects"`
	Blocked        []proxy.Denied      `json:"blocked"`
}

type WorkspaceView struct {
	Config   any                 `json:"config"` // YAML key names
	Members  []MemberView        `json:"members"`
	Projects []string            `json:"projects"`
	Handoffs []workspace.Handoff `json:"handoffs"`
	Project  string              `json:"project,omitempty"` // whose view hidden is
	Hidden   []string            `json:"hidden"`
	Rules    []string            `json:"rules"`
	Error    string              `json:"error,omitempty"`
}

// MemberView is a member folder: where it is, and the project it is, if any.
type MemberView struct {
	Name    string `json:"name"`
	Dir     string `json:"dir"`
	Project string `json:"project,omitempty"`
}

type SettingsView struct {
	Global      any      `json:"global"` // YAML key names
	ConfigDir   string   `json:"config_dir"`
	StateDir    string   `json:"state_dir"`
	SecretStore string   `json:"secret_store"`
	Secrets     []string `json:"secrets"` // names set for all projects
	Version     string   `json:"version"`
	Profiles    []string `json:"profiles"` // Claude settings profiles, for claude.profiles
	// LoginDefault is claude.login in config.yaml (app unless set); Logins each project's choice.
	LoginDefault string         `json:"login_default"`
	Logins       []ProjectLogin `json:"logins"`
}

// ProjectLogin is the Claude login a project uses: its own choice, or the default.
type ProjectLogin struct {
	Project string `json:"project"`
	Login   string `json:"login"` // claude.login in the project's file; empty: the default
	Uses    string `json:"uses"`  // what it resolves to: app or project
}

// ---- handlers ----

func (s *Server) runtimeView(ctx context.Context) (RuntimeView, *sandbox.Manager) {
	m, err := s.env.Manager(ctx, "")
	if err != nil {
		return RuntimeView{Name: s.env.RuntimeName(""), Error: err.Error()}, nil
	}
	i := m.Driver.Info(ctx)
	return RuntimeView{Name: i.Name, Version: i.Version, Platform: i.Platform, Arch: i.Arch, URLs: i.HostSuffix != ""}, m
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	ps, errs := s.env.Store.Projects()
	out := Overview{Version: s.version, Workspaces: append([]string{}, s.env.Store.Workspaces()...), Projects: []ProjectSummary{}}
	for _, p := range ps {
		v := summary(p)
		if b := s.builds.of(p.Name); b != nil && b.Failed {
			v.BuildFailed = true
		}
		out.Projects = append(out.Projects, v)
	}
	for _, e := range errs {
		out.Projects = append(out.Projects, ProjectSummary{Name: "?", Error: e.Error()})
	}
	writeJSON(w, out)
}

func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rt, m := s.runtimeView(ctx)
	out := Live{Runtime: rt, Projects: map[string]ProjectLive{}, Ops: s.opsUnderWay(), Health: map[string]sandbox.Health{}}
	if m != nil {
		all, err := s.runningSessions(ctx, m)
		if err != nil {
			out.Runtime.Error = err.Error() // the runtime stopped since it was checked
		}
		out.Health = s.health()
		for p, ss := range all {
			var pl ProjectLive
			pl.Sessions, pl.Busy = countSessions(ss)
			out.Projects[p] = pl
		}
	}
	writeJSON(w, out)
}

// runningSessions is every running sandbox's sessions, by project. The notifier and the app's
// pages all ask every few seconds, and each listing asks every sandbox (an exec into each
// container): they share one listing for sessionsFresh, and a caller that comes during a listing
// waits for it rather than starting another. The result is shared: read it, don't change it.
func (s *Server) runningSessions(ctx context.Context, m *sandbox.Manager) (map[string][]sandbox.Session, error) {
	c := &s.listing
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < sessionsFresh {
		return c.all, c.err
	}
	c.all, c.health, c.err = s.listSessions(ctx, m)
	c.at = time.Now()
	if ctx.Err() != nil { // a caller that went away mustn't leave an empty answer for the others
		c.at = time.Time{}
	}
	return c.all, c.err
}

// sessionsFresh is how long a listing of the sessions is shared.
const sessionsFresh = 2 * time.Second

// sessionsCache is the last listing of the sessions (see runningSessions).
type sessionsCache struct {
	mu     sync.Mutex
	at     time.Time
	all    map[string][]sandbox.Session
	health map[string]sandbox.Health // the projects whose health has something to show
	err    error
}

// health is the projects' health from the last listing (see runningSessions).
func (s *Server) health() map[string]sandbox.Health {
	s.listing.mu.Lock()
	defer s.listing.mu.Unlock()
	if s.listing.health == nil {
		return map[string]sandbox.Health{}
	}
	return s.listing.health
}

// listSessions asks every running sandbox for its sessions, in parallel, after one listing of all
// containers; that listing also gives each project's health.
func (s *Server) listSessions(ctx context.Context, m *sandbox.Manager) (map[string][]sandbox.Session, map[string]sandbox.Health, error) {
	all, err := m.ProjectContainers(ctx)
	if err != nil {
		return nil, nil, err
	}
	health := map[string]sandbox.Health{}
	for p, cs := range all {
		if h := sandbox.HealthOf(cs); h.Main != "" || len(h.Problems) > 0 {
			health[p] = h
		}
	}
	out := make(map[string][]sandbox.Session, len(all))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for p, cs := range all {
		if !sandbox.MainRunning(cs) {
			continue
		}
		wg.Go(func() {
			ss := m.SessionsOf(ctx, p, cs)
			mu.Lock()
			out[p] = ss
			mu.Unlock()
		})
	}
	wg.Wait()
	return out, health, nil
}

// yamlView re-encodes a configuration value with its YAML key names (what users see in their
// files), so the UI and the files speak the same names.
func yamlView(v any) any {
	data, err := config.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	if yaml.Unmarshal(data, &out) != nil {
		return nil
	}
	return out
}

func summary(p config.Project) ProjectSummary {
	sv := p.Services
	if sv == nil {
		sv = []string{}
	}
	return ProjectSummary{Name: p.Name, Dir: paths.Tilde(p.Dir), Workspace: p.Workspace, Services: sv}
}

func countSessions(ss []sandbox.Session) (n, busy int) {
	for _, x := range ss {
		if x.State == "busy" {
			busy++
		}
	}
	return len(ss), busy
}

func (s *Server) manager(w http.ResponseWriter, r *http.Request) (*sandbox.Manager, string, bool) {
	name := r.PathValue("name")
	if _, err := s.env.Store.Project(name); err != nil {
		apiError(w, err)
		return nil, "", false
	}
	m, err := s.env.Manager(r.Context(), name)
	if err != nil {
		apiError(w, err)
		return nil, "", false
	}
	return m, name, true
}

// withStatus adds what the runtime reports. The lists stay lists, never null, even when the
// runtime has nothing (a stopped project has no containers): the UI reads them as they come.
func (d *ProjectDetail) withStatus(st *sandbox.State) {
	d.Running = st.Running
	d.Health = st.Health
	if st.Containers != nil {
		d.Containers = st.Containers
	}
	if st.Pending != nil {
		d.Pending = st.Pending
	}
	if st.Changes != nil {
		d.Changes = st.Changes
	}
}

// project answers with a project's settings and, when the runtime answers, its sandbox; without
// a runtime it still answers, with the reason in `error`.
func (s *Server) project(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	raw, err := s.env.Store.Project(name)
	if err != nil {
		apiError(w, err)
		return
	}
	ctx := r.Context()
	g, _ := s.env.Global()
	d := ProjectDetail{ProjectSummary: summary(raw), Config: yamlView(raw.Resolved(g)), Pending: []string{},
		Changes: []sandbox.Change{}, List: []sandbox.Session{}, Worktrees: []agentapi.Worktree{}, Runs: []agentapi.Run{}, Containers: []runtime.Container{}}
	d.Build = s.builds.of(name)
	d.Op = s.opOf(name)
	d.WorktreeJobs = s.wt.of(name)
	d.NodeModules = NodeModulesView{Auto: len(raw.NodeModules) == 0, Setting: nonNil(raw.NodeModules),
		Dirs: nonNil(sandbox.PackageDirs(raw)), Empty: []string{}}
	d.NodeModules.Used = len(raw.NodeModules) > 0 || len(d.NodeModules.Dirs) > 0 || fileExists(filepath.Join(raw.Dir, "package.json"))
	d.Commands = map[string]string{}
	for _, c := range []string{"claude", "shell"} {
		d.Commands[c], _ = SessionCommand(raw, Session{Command: c})
	}
	d.Files = map[string]string{
		"config": paths.Tilde(s.env.Store.ProjectFile(name)),
		"notes":  paths.Tilde(filepath.Join(s.env.Store.ProjectNotesDir(name), "notes.md")),
	}
	// The allowlist comes from the configuration alone.
	cfgOnly := &sandbox.Manager{Store: s.env.Store, Global: g}
	if d.Allow, err = cfgOnly.Allowlist(name); err != nil || d.Allow == nil {
		d.Allow = []string{}
	}
	if d.AllowGroups, err = cfgOnly.AllowGroups(name); err != nil || d.AllowGroups == nil {
		d.AllowGroups = []proxy.Group{}
	}
	m, err := s.env.Manager(ctx, name)
	if err != nil {
		d.Error = err.Error()
		writeJSON(w, d)
		return
	}
	if st, err := m.Status(ctx, name); err != nil {
		d.Error = err.Error()
	} else {
		d.withStatus(st)
	}
	if d.Running {
		if ss, err := m.Sessions(ctx, name); err == nil && ss != nil {
			d.List = m.WithInternet(ctx, name, ss)
			d.Sessions, d.Busy = countSessions(ss)
		}
		if as, err := m.AgentStatus(ctx, name); err == nil {
			if as.Worktrees != nil {
				d.Worktrees = as.Worktrees
			}
			if as.Runs != nil {
				d.Runs = as.Runs
			}
			if as.EmptyNodeModules != nil {
				d.NodeModules.Empty = as.EmptyNodeModules
			}
		}
	}
	info := m.Driver.Info(ctx)
	if info.HostSuffix != "" {
		d.URLBase = "http://" + spec.SandboxService + "." + spec.Prefix + name + info.HostSuffix
	}
	writeJSON(w, d)
}

func (s *Server) up(w http.ResponseWriter, r *http.Request)      { s.act(w, r, ActStart) }
func (s *Server) restart(w http.ResponseWriter, r *http.Request) { s.act(w, r, ActRestart) }

// down is like jw down: refused (409) while sessions run, unless force (?force=1) ends them.
func (s *Server) down(w http.ResponseWriter, r *http.Request) { s.act(w, r, ActStop) }

// rebuild builds the request's project's images again (?fresh=1: without the cache, pulling the
// bases again); see Server.Rebuild. 423 while another op runs.
func (s *Server) rebuild(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.manager(w, r); !ok {
		return
	}
	if err := s.Rebuild(r.PathValue("name"), r.URL.Query().Get("fresh") == "1"); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// act answers a start, restart or stop of the request's project (see Server.Act).
func (s *Server) act(w http.ResponseWriter, r *http.Request, action string) {
	if _, _, ok := s.manager(w, r); !ok {
		return
	}
	name := r.PathValue("name")
	st, err := s.Act(name, action, r.URL.Query().Get("force") == "1")
	switch {
	case err != nil:
		apiError(w, err) // 409 while sessions run, unless force; 423 while another op runs
	case st == nil:
		writeJSON(w, map[string]bool{"ok": true})
	default:
		writeJSON(w, st)
	}
}

// startService starts one service of the project's sandbox again (sandbox.StartService): one
// that stopped, crashed or was paused.
func (s *Server) startService(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	if err := m.StartService(r.Context(), name, r.PathValue("service")); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) stopSessions(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	if err := m.StopSessions(r.Context(), name); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 || tail > 5000 {
		tail = 300
	}
	var buf bytes.Buffer
	err := m.Driver.Logs(r.Context(), spec.Prefix+name, r.URL.Query().Get("service"), runtime.LogOptions{Tail: tail, Out: &buf})
	if err != nil && buf.Len() == 0 {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]string{"text": buf.String()})
}

// agentJSON runs an agent command in a running sandbox and decodes its output.
func agentJSON(ctx context.Context, m *sandbox.Manager, project string, v any, args ...string) error {
	return agentError(ctx, project, m.AgentJSON(ctx, sandbox.MainContainer(project), v, args...))
}

// agentError says why an agent command in a project's sandbox failed, for the page.
func agentError(ctx context.Context, project string, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("%s's sandbox didn't answer in time; try again", project)
	case errors.Is(err, sandbox.ErrAgentAnswer):
		return err
	case errors.Is(err, sandbox.ErrAgentOlder):
		return fmt.Errorf("%s: %w (Restart applies it)", project, err)
	case err != nil:
		return errors.New(project + " isn't running")
	}
	return nil
}

// historyTimeout bounds listing conversations: the page waits for it.
const historyTimeout = 30 * time.Second

func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	checks, err := m.Verify(r.Context(), name)
	if err != nil {
		apiError(w, agentError(r.Context(), name, err))
		return
	}
	writeJSON(w, checks)
}

func (s *Server) ports(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	ports, err := m.Ports(r.Context(), name)
	if err != nil {
		apiError(w, err)
		return
	}
	if ports == nil {
		ports = []sandbox.Port{} // nothing listening, not "not loaded yet"
	}
	writeJSON(w, ports)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	var convs []agentapi.Conversation // an older agent prints null for none
	ctx, cancel := context.WithTimeout(r.Context(), historyTimeout)
	defer cancel()
	if err := agentJSON(ctx, m, name, &convs, "history", "-n", "100"); err != nil {
		apiError(w, err)
		return
	}
	p, err := s.env.Store.Project(name)
	if err != nil {
		apiError(w, err)
		return
	}
	out := make([]ConversationView, 0, len(convs))
	for _, c := range convs {
		v := ConversationView{Conversation: c}
		s := Session{Resume: c.ID, Dir: c.Dir}
		v.Command, _ = SessionCommand(p, s)
		if dir, run, err := sessionParts(p, s); err == nil {
			v.Run, v.Folder = run, paths.Tilde(dir)
		}
		out = append(out, v)
	}
	writeJSON(w, out)
}

// ConversationView is a saved conversation with the command that resumes it in its folder
// (Command), and the same in two parts: the folder (Folder, ~/…) and what to run there (Run).
type ConversationView struct {
	agentapi.Conversation
	Command string `json:"command,omitempty"`
	Run     string `json:"run,omitempty"`
	Folder  string `json:"folder,omitempty"`
}

// notes and saveNotes serve a project's notes.md; workspaces have their own (workspaceNotes).
func (s *Server) notes(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.env.Store.Project(name); err != nil {
		apiError(w, err)
		return
	}
	readNotes(w, s.env.Store.ProjectNotesDir(name))
}

func (s *Server) saveNotes(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.env.Store.Project(name); err != nil {
		apiError(w, err)
		return
	}
	writeNotes(w, r, s.env.Store.ProjectNotesDir(name))
}

// readNotes answers with the notes.md of a notes folder: {text, path}.
func readNotes(w http.ResponseWriter, dir string) {
	file := filepath.Join(dir, "notes.md")
	data, _ := os.ReadFile(file)
	writeJSON(w, map[string]string{"text": string(data), "path": paths.Tilde(file)})
}

// writeNotes saves the request's {text} as the notes.md of a notes folder.
func writeNotes(w http.ResponseWriter, r *http.Request, dir string) {
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		apiError(w, err)
		return
	}
	if err := config.WriteFileAtomic(filepath.Join(dir, "notes.md"), []byte(body.Text), 0o644); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) terminal(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.env.Store.Project(name)
	if err != nil {
		apiError(w, err)
		return
	}
	var body Session
	if err := readJSON(r, &body); err != nil {
		apiError(w, err)
		return
	}
	// A conversation a session holds isn't resumed again (the page may not have caught up).
	if body.Resume != "" {
		if m, err := s.env.Manager(r.Context(), name); err == nil {
			if running, err := m.RunningConversations(r.Context(), name); err == nil {
				if err := sandbox.CheckResume(running, body.Resume); err != nil {
					apiError(w, err)
					return
				}
			}
		}
	}
	g, _ := s.env.Global()
	if err := OpenTerminal(g.Terminal, p, body); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := []SessionView{}
	m, err := s.env.Manager(ctx, "")
	if err != nil {
		writeJSON(w, out)
		return
	}
	all, _ := s.runningSessions(ctx, m)
	for p, ss := range all {
		for _, x := range m.WithInternet(ctx, p, ss) { // the page shows it, and opens or closes it
			out = append(out, SessionView{Project: p, Session: x})
		}
	}
	slices.SortFunc(out, func(a, b SessionView) int { return b.Started.Compare(a.Started) })
	writeJSON(w, out)
}

func (s *Server) network(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := NetworkView{Presets: presets.Catalogue(), Projects: map[string][]string{}, ProjectPresets: map[string][]string{},
		Blocked: []proxy.Denied{}, User: []string{}}
	if _, err := presets.Custom(); err != nil {
		v.PresetProblems = err.Error()
	}
	if data, err := os.ReadFile(s.env.Store.AllowFile()); err == nil {
		v.User = assets.ParseDomains(data)
	}
	ps, _ := s.env.Store.Projects()
	for _, p := range ps {
		if len(p.Allow) > 0 {
			v.Projects[p.Name] = p.Allow
		}
		if len(p.AllowPresets) > 0 {
			v.ProjectPresets[p.Name] = p.AllowPresets
		}
	}
	acc := map[string]*proxy.Denied{}
	for _, p := range ps { // from each project's records: running or not (see blockedOf)
		hosts, _ := s.blockedOf(ctx, p.Name)
		one := map[string]*proxy.Denied{}
		for i := range hosts {
			one[hosts[i].Host] = &hosts[i]
		}
		proxy.MergeDenied(acc, one)
	}
	v.Blocked = proxy.SortDenied(acc)
	writeJSON(w, v)
}

// allowRequest changes an allowlist: a domain (a project's, or every sandbox's), or a preset of
// a project's allow_presets.
type allowRequest struct {
	Domain  string   `json:"domain"`
	Domains []string `json:"domains"` // several at once, the proxies reloaded once
	Preset  string   `json:"preset"`
	Project string   `json:"project"` // empty: every sandbox (domains only)
}

func (s *Server) allow(w http.ResponseWriter, r *http.Request) {
	s.changeAllowlist(w, r,
		func(list []string, d string) []string { return proxy.Minimize(append(list, d)) },
		func(p *config.Project, name string) error { return p.AddAllowPresets(name) })
}

func (s *Server) deny(w http.ResponseWriter, r *http.Request) {
	s.changeAllowlist(w, r,
		func(list []string, d string) []string {
			return slices.DeleteFunc(list, func(e string) bool { return e == d })
		},
		func(p *config.Project, name string) error { return p.RemoveAllowPresets(name) })
}

func (s *Server) changeAllowlist(w http.ResponseWriter, r *http.Request,
	domains func([]string, string) []string, presetsOf func(*config.Project, string) error) {
	var req allowRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	var changed string
	var err error
	switch {
	case req.Preset != "":
		changed, err = req.Preset, s.changePresets(req, presetsOf)
	case len(req.Domains) > 0:
		var done []string
		for _, d := range req.Domains {
			one := req
			one.Domain = d
			var c string
			if c, err = s.changeDomains(one, domains); err != nil {
				break
			}
			done = append(done, c)
		}
		changed = strings.Join(done, ", ")
	default:
		changed, err = s.changeDomains(req, domains)
	}
	if err != nil {
		apiError(w, err)
		return
	}
	var affected []string
	if req.Project != "" {
		affected = []string{req.Project}
	}
	writeJSON(w, map[string]any{"domain": changed, "reloaded": s.reloadProxies(r.Context(), affected)})
}

// changePresets changes a project's allow_presets.
func (s *Server) changePresets(req allowRequest, change func(*config.Project, string) error) error {
	if req.Project == "" {
		return badInput("a preset is added to a project: name the project")
	}
	p, err := s.env.Store.Project(req.Project)
	if err != nil {
		return err
	}
	if err := change(&p, req.Preset); err != nil {
		return badInput(err.Error())
	}
	return s.env.Store.SaveProject(p)
}

// changeDomains changes a project's own domains, or the user's allowlist; it returns the domain.
func (s *Server) changeDomains(req allowRequest, change func([]string, string) []string) (string, error) {
	d, err := proxy.Normalize(req.Domain)
	if err != nil {
		return "", err
	}
	if req.Project != "" {
		p, err := s.env.Store.Project(req.Project)
		if err != nil {
			return "", err
		}
		p.Allow = change(p.Allow, d)
		// Allowed now: no longer kept blocked.
		p.Blocked = slices.DeleteFunc(p.Blocked, func(h string) bool { return proxy.Covers(d, h) })
		return d, s.env.Store.SaveProject(p)
	}
	var list []string
	if data, err := os.ReadFile(s.env.Store.AllowFile()); err == nil {
		list = assets.ParseDomains(data)
	}
	list = change(list, d)
	text := "# Your domains, allowed in every jailwick sandbox.\n" + strings.Join(list, "\n") + "\n"
	return d, config.WriteFileAtomic(s.env.Store.AllowFile(), []byte(text), 0o644)
}

// reloadProxies makes the running sandboxes of projects (all running ones when empty) read their
// allowlists again, and returns those it reloaded.
func (s *Server) reloadProxies(ctx context.Context, projects []string) []string {
	reloaded := []string{}
	m, err := s.env.Manager(ctx, "")
	if err != nil {
		return reloaded
	}
	running, _ := m.Running(ctx)
	for _, p := range running {
		if len(projects) > 0 && !slices.Contains(projects, p) {
			continue
		}
		if m.ReloadProxy(ctx, p) == nil {
			reloaded = append(reloaded, p)
		}
	}
	return reloaded
}

// memberViews are a workspace's members with the projects they are.
func memberViews(w config.Workspace, ps []config.Project) []MemberView {
	out := []MemberView{}
	for _, m := range w.Members {
		dir := workspace.MemberDir(w, m)
		v := MemberView{Name: strings.TrimSuffix(m.Name, "/"), Dir: paths.Tilde(dir)}
		for _, p := range ps {
			if paths.Real(p.Dir) == dir {
				v.Project = p.Name
			}
		}
		out = append(out, v)
	}
	return out
}

// workspace answers with a workspace: its settings, projects, handoffs, and what is hidden in its
// members as ?project= (default: its first project) sees them; without projects, as any would.
func (s *Server) workspace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	wc, err := s.env.Store.Workspace(name)
	if err != nil {
		apiError(w, err)
		return
	}
	v := WorkspaceView{Config: yamlView(wc), Projects: []string{}, Handoffs: []workspace.Handoff{}, Hidden: []string{},
		Rules: workspace.ApplyRules(workspace.DefaultRules, wc.Exclude)}
	if h := workspace.HandoffDir(wc); h != "" {
		if hs, err := workspace.ListHandoffs(h); err == nil && hs != nil {
			v.Handoffs = hs
		}
	}
	ps, _ := s.env.Store.Projects()
	v.Members = memberViews(wc, ps)
	var seen *config.Project
	for _, p := range ps {
		if p.Workspace == name {
			v.Projects = append(v.Projects, p.Name)
			if seen == nil || p.Name == r.URL.Query().Get("project") {
				seen = &p
			}
		}
	}
	if seen == nil {
		v.Hidden = append(v.Hidden, workspace.Hidden(wc)...)
		writeJSON(w, v)
		return
	}
	v.Project = seen.Name
	seen.Dir = paths.Real(seen.Dir)
	res, err := workspace.Generate(s.env.Store, *seen, filepath.Join(paths.StateDir(), "mask"))
	if err != nil {
		v.Error = err.Error()
		writeJSON(w, v)
		return
	}
	v.Rules = res.Rules
	for _, h := range res.Info.Hidden {
		v.Hidden = append(v.Hidden, strings.TrimPrefix(h, res.Info.Root+"/"))
	}
	writeJSON(w, v)
}

func (s *Server) images(w http.ResponseWriter, r *http.Request) {
	m, err := s.env.Manager(r.Context(), "")
	if err != nil {
		apiError(w, err)
		return
	}
	uses, err := m.Images(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, uses)
}

// pruneImages removes the images jailwick built that nothing uses (jw image prune).
func (s *Server) pruneImages(w http.ResponseWriter, r *http.Request) {
	m, err := s.env.Manager(r.Context(), "")
	if err != nil {
		apiError(w, err)
		return
	}
	removed, err := m.PruneImages(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	if removed == nil {
		removed = []image.Use{}
	}
	writeJSON(w, map[string]any{"removed": removed})
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	g, err := s.env.Global()
	if err != nil {
		apiError(w, err)
		return
	}
	v := SettingsView{Global: yamlView(g), ConfigDir: paths.Tilde(s.env.Store.Dir), StateDir: paths.Tilde(paths.StateDir()),
		Version: s.version, Secrets: []string{}, Profiles: []string{}}
	if names, err := s.env.Store.Profiles(); err == nil && names != nil {
		v.Profiles = names
	}
	v.LoginDefault, v.Logins = config.LoginApp, []ProjectLogin{}
	if g.Claude.Login != "" {
		v.LoginDefault = g.Claude.Login
	}
	ps, _ := s.env.Store.Projects()
	for _, p := range ps {
		v.Logins = append(v.Logins, ProjectLogin{Project: p.Name, Login: p.Claude.Login, Uses: p.Resolved(g).Claude.Login})
	}
	if st, err := s.env.Secrets(); err == nil {
		v.SecretStore = st.Name()
		if keys, err := st.Keys(secrets.Global); err == nil {
			v.Secrets = keys
		}
	}
	writeJSON(w, v)
}

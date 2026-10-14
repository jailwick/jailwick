package server

import (
	"net/http"
	"strings"

	"github.com/jailwick/jailwick/internal/claudesettings"
	"github.com/jailwick/jailwick/internal/presets"
	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/secrets"
)

// What the app changes besides the allowlists: a project's Claude settings, secrets and the
// user's own presets. Each change is validated as the CLI validates it.

// ClaudeChange changes a project's Claude settings; fields left out stay as they are.
type ClaudeChange struct {
	Login    *string   `json:"login"` // app, project, or "" for the default (config.yaml)
	Chrome   *bool     `json:"chrome"`
	Profiles *[]string `json:"profiles"`
}

func (s *Server) saveClaude(w http.ResponseWriter, r *http.Request) {
	var req ClaudeChange
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	p, err := s.env.Store.Project(r.PathValue("name"))
	if err != nil {
		apiError(w, err)
		return
	}
	if req.Login != nil {
		p.Claude.Login = *req.Login
	}
	if req.Chrome != nil {
		p.Claude.Chrome = req.Chrome
	}
	if req.Profiles != nil {
		p.Claude.Profiles = *req.Profiles
		if len(p.Claude.Profiles) == 0 {
			p.Claude.Profiles = nil
		}
	}
	if err := p.Validate(); err != nil {
		apiError(w, err)
		return
	}
	g, err := s.env.Global()
	if err != nil {
		apiError(w, err)
		return
	}
	if _, err := claudesettings.Layers(s.env.Store, p.Resolved(g)); err != nil { // the profiles exist
		apiError(w, badInput(err.Error()))
		return
	}
	if err := s.env.Store.SaveProject(p); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// saveNotifications changes the notification settings in config.yaml, those given: the app's
// notifications on or off (notifications), where sessions tell they wait
// (session_notifications), and refused hosts (blocked_notifications).
func (s *Server) saveNotifications(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On       *bool   `json:"on"`       // notifications
		Sessions *string `json:"sessions"` // session_notifications
		Blocked  *bool   `json:"blocked"`  // blocked_notifications
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	g, err := s.env.Store.Global()
	if err != nil {
		apiError(w, err)
		return
	}
	if req.On != nil {
		g.Notifications = req.On
	}
	if req.Sessions != nil {
		g.SessionNotifications = *req.Sessions
	}
	if req.Blocked != nil {
		g.BlockedNotifications = req.Blocked
	}
	if err := g.Validate(); err != nil {
		apiError(w, err)
		return
	}
	if err := s.env.Store.SaveGlobal(g); err != nil {
		apiError(w, err)
		return
	}
	s.env.Reload()
	writeJSON(w, map[string]bool{"ok": true})
}

// saveTerminal sets the terminal app sessions open in (config.yaml's terminal).
func (s *Server) saveTerminal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Terminal string `json:"terminal"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	if err := CheckTerminal(req.Terminal); err != nil {
		apiError(w, err)
		return
	}
	g, err := s.env.Store.Global()
	if err != nil {
		apiError(w, err)
		return
	}
	g.Terminal = req.Terminal
	if err := s.env.Store.SaveGlobal(g); err != nil {
		apiError(w, err)
		return
	}
	s.env.Reload()
	writeJSON(w, map[string]bool{"ok": true})
}

// saveDefaultLogin sets the Claude login projects use unless they choose (claude.login in
// config.yaml).
func (s *Server) saveDefaultLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login string `json:"login"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	g, err := s.env.Store.Global()
	if err != nil {
		apiError(w, err)
		return
	}
	g.Claude.Login = req.Login
	if err := g.Validate(); err != nil {
		apiError(w, err)
		return
	}
	if err := s.env.Store.SaveGlobal(g); err != nil {
		apiError(w, err)
		return
	}
	s.env.Reload()
	writeJSON(w, map[string]bool{"ok": true})
}

// SecretView is one secret's name and scope; values are never sent.
type SecretView struct {
	Name    string `json:"name"`
	Project string `json:"project,omitempty"` // empty: all projects
}

// SecretsView lists the secrets set, where they are kept, and the names jailwick knows.
type SecretsView struct {
	Store     string       `json:"store"`
	ReadOnly  bool         `json:"read_only"` // the env store: set in the environment, not here
	Secrets   []SecretView `json:"secrets"`
	Known     []string     `json:"known"`
	Overrides []string     `json:"overrides"` // the names that replace Claude's login
}

func (s *Server) secretsList(w http.ResponseWriter, r *http.Request) {
	st, err := s.env.Secrets()
	if err != nil {
		apiError(w, err)
		return
	}
	v := SecretsView{Store: st.Name(), ReadOnly: st.Name() == "env", Secrets: []SecretView{},
		Known: secrets.Known, Overrides: secrets.LoginOverrides}
	add := func(scope, project string) {
		keys, _ := st.Keys(scope)
		for _, k := range keys {
			v.Secrets = append(v.Secrets, SecretView{Name: k, Project: project})
		}
	}
	add(secrets.Global, "")
	ps, _ := s.env.Store.Projects()
	for _, p := range ps {
		add(p.Name, p.Name)
	}
	writeJSON(w, v)
}

type secretRequest struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Project string `json:"project"` // empty: all projects
}

func (s *Server) setSecret(w http.ResponseWriter, r *http.Request) {
	var req secretRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	req.Name = strings.ToUpper(strings.TrimSpace(req.Name))
	switch err := secrets.CheckKey(req.Name); {
	case err != nil:
		apiError(w, badInput(err.Error()))
		return
	case req.Value == "":
		apiError(w, badInput("empty value"))
		return
	}
	s.changeSecret(w, req.Project, func(st secrets.Store, scope string) error { return st.Set(scope, req.Name, req.Value) })
}

func (s *Server) unsetSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.changeSecret(w, r.URL.Query().Get("project"), func(st secrets.Store, scope string) error { return st.Delete(scope, name) })
}

// changeSecret changes a secret of a project's (or of all projects) in the secret store.
func (s *Server) changeSecret(w http.ResponseWriter, project string, change func(secrets.Store, string) error) {
	scope := secrets.Global
	if project != "" {
		if _, err := s.env.Store.Project(project); err != nil {
			apiError(w, err)
			return
		}
		scope = project
	}
	st, err := s.env.Secrets()
	if err != nil {
		apiError(w, err)
		return
	}
	if err := change(st, scope); err != nil {
		apiError(w, badInput(err.Error()))
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// PresetsView is the preset catalogue: built-in, then the user's own.
type PresetsView struct {
	Presets  []presets.Preset `json:"presets"`
	Problems string           `json:"problems,omitempty"` // preset files of the user's left out
}

func (s *Server) presetsList(w http.ResponseWriter, _ *http.Request) {
	v := PresetsView{Presets: presets.Catalogue()}
	if _, err := presets.Custom(); err != nil {
		v.Problems = err.Error()
	}
	writeJSON(w, v)
}

type presetRequest struct {
	Description string `json:"description"`
	Hosts       string `json:"hosts"` // separated by commas, spaces or new lines
}

func (s *Server) savePreset(w http.ResponseWriter, r *http.Request) {
	var req presetRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	name := r.PathValue("name")
	hosts, err := proxy.ParseList(req.Hosts)
	if err == nil {
		err = presets.SaveCustom(name, strings.TrimSpace(req.Description), hosts)
	}
	if err != nil {
		apiError(w, badInput(err.Error()))
		return
	}
	reloaded := []string{}
	if users := s.env.Store.ProjectsUsingPreset(name); len(users) > 0 {
		reloaded = s.reloadProxies(r.Context(), users)
	}
	writeJSON(w, map[string]any{"name": name, "hosts": hosts, "reloaded": reloaded})
}

func (s *Server) deletePreset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if users := s.env.Store.ProjectsUsingPreset(name); len(users) > 0 {
		apiError(w, badInput(strings.Join(users, ", ")+" uses the "+name+" preset: remove it there first"))
		return
	}
	if err := presets.DeleteCustom(name); err != nil {
		apiError(w, badInput(err.Error()))
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// saveDefaultImage sets the image projects build on unless they name their own (config.yaml's
// image: base, user, home; an empty base is jailwick's own). A project's next start or restart
// builds on it.
func (s *Server) saveDefaultImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Base string `json:"base"`
		User string `json:"user"`
		Home string `json:"home"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	g, err := s.env.Store.Global()
	if err != nil {
		apiError(w, err)
		return
	}
	g.Image.Base, g.Image.User, g.Image.Home = req.Base, req.User, req.Home
	if req.Base != "" {
		g.Image.Dockerfile, g.Image.Context, g.Image.Name = "", "", "" // one source
	}
	if err := g.Validate(); err != nil {
		apiError(w, err)
		return
	}
	if err := s.env.Store.SaveGlobal(g); err != nil {
		apiError(w, err)
		return
	}
	s.env.Reload()
	writeJSON(w, map[string]bool{"ok": true})
}

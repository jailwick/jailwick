// Package projectsetup is the guided creation of a project, shared by `jw project add` and the
// app: a draft prefilled from what the repo's files show, then the fields of Steps, then the file.
package projectsetup

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/detect"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/presets"
	"github.com/jailwick/jailwick/internal/stacks"
	"github.com/jailwick/jailwick/internal/workspace"
)

// Store is what the setup needs from the config store.
type Store interface {
	Project(name string) (config.Project, error)
	Projects() ([]config.Project, []error)
	workspace.Workspaces
}

// Draft is a project being set up.
type Draft struct {
	Detected detect.Suggestion
	Project  config.Project
	// Hints are things worth knowing that the flow doesn't ask about.
	Hints []string
	// EnvFile is the env file the logins were read from (see UseEnvFile); it isn't saved.
	EnvFile string
	// Workspaces are those whose root holds the repo: the project can join one.
	Workspaces []string
	// Folder is whether the repo's folder becomes a member of the workspace it joins, shown to the
	// other projects (FolderShow, the default), or not (FolderHide). See workspaceFolderField.
	Folder string
	// Shown are the workspaces whose members already show the repo's folder.
	Shown map[string]bool
	// ImageSource is the image answer as given (see imageFields); "" until given, then it is
	// read from the project.
	ImageSource string
}

// New starts a draft for a repo folder: checks it can be a project and prefills from detection.
func New(store Store, dir string) (*Draft, error) {
	dir = paths.Real(paths.Expand(strings.TrimSpace(dir)))
	if err := config.CheckMountable(dir); err != nil {
		return nil, problem("%v", err)
	}
	ps, _ := store.Projects()
	for _, p := range ps {
		if paths.Real(p.Dir) == dir {
			return nil, problem("%s is already the project %q", paths.Tilde(dir), p.Name)
		}
	}
	s := detect.Repo(dir)
	p := config.Project{Name: config.NameFrom(dir, "project"), Dir: dir, Services: s.Services}
	if p.Uses("postgres") && s.Postgres.DB != "" {
		p.Postgres = config.Postgres{User: s.Postgres.User, Password: s.Postgres.Password, DB: s.Postgres.DB}
	}
	if s.RedisPass != "" {
		p.Redis.Passwords = []string{s.RedisPass}
	}
	if s.MongoUser != "" {
		p.Mongo = config.Mongo{User: s.MongoUser, Password: s.MongoPass}
	}
	p.LocalForwards = s.Forwards
	p.Worktree.CopyFiles = s.EnvFiles
	if len(s.Stacks) > 0 {
		p.Image.Stacks = s.Stacks
	}
	if usesPlaywright(s) {
		p.Image.Playwright.Browsers = []string{"chromium"}
		p.Image.Playwright.Versions = s.Playwright
	}
	// Broad presets aren't chosen; the network step names them (networkNote).
	narrow, _ := presets.Split(s.Presets)
	for _, c := range stacks.Chosen(p.Image.Stacks) { // what the stacks download through
		stackNarrow, _ := presets.Split(c.Presets)
		narrow = append(narrow, stackNarrow...)
	}
	p.AllowPresets = presets.Explicit(narrow)

	d := &Draft{Detected: s, Project: p, EnvFile: s.LoginsFrom, Workspaces: workspace.Containing(store, dir),
		Folder: FolderShow, Shown: map[string]bool{}}
	for _, n := range d.Workspaces {
		if w, err := store.Workspace(n); err == nil {
			if _, missing := workspace.MissingMember(w, p); !missing {
				d.Shown[n] = true
			}
		}
	}
	if len(d.Workspaces) == 1 { // the folder is in a workspace: it joins unless told not to
		d.Project.Workspace = d.Workspaces[0]
	}
	if s.Android {
		d.Hints = append(d.Hints, "Found an android/ folder: for Android builds set image.android: true (jw project edit).")
	}
	return d, nil
}

// AddsMember reports whether saving the project makes its folder a member of its workspace.
func (d *Draft) AddsMember() bool {
	return d.Project.Workspace != "" && d.Folder != FolderHide && !d.Shown[d.Project.Workspace]
}

// Value is a field's current value: a string, or a []string for Multi and List fields.
func (d *Draft) Value(f Field) any { return f.Get(d) }

// Visible reports whether a field applies, given the other values.
func (d *Draft) Visible(f Field) bool { return Form.Visible(d, f) }

// Set checks and stores a field's value (see setup.Form.Set).
func (d *Draft) Set(f Field, v any) error { return Form.Set(d, f, v) }

// Check reports whether Set would accept the value, without changing the draft.
func (d *Draft) Check(f Field, v any) error { return Form.Check(d, f, v) }

// SetAll sets values by field key, in the order of the steps.
func (d *Draft) SetAll(values map[string]any) error { return Form.SetAll(d, values) }

// View describes every step with the draft's current values.
func (d *Draft) View() []StepView { return Form.View(d) }

// UseEnvFile takes the logins of an env file (relative to the repo, or absolute): they replace
// the service logins, and its services and forwards are added. "" forgets the file.
func (d *Draft) UseEnvFile(file string) error {
	d.EnvFile = file
	if file == "" {
		return nil
	}
	path := paths.Expand(file)
	if !filepath.IsAbs(path) {
		path = filepath.Join(d.Project.Dir, path)
	}
	e, err := detect.ReadEnv(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return problem("Read logins from: there is no %s", file)
	case err != nil:
		return problem("Read logins from: %v", err)
	}
	if len(e.Services) == 0 {
		return problem("Read logins from: no local database login in %s", file)
	}
	p := &d.Project // new slices: drafts share the old ones (Check)
	p.Services = union(p.Services, e.Services)
	p.LocalForwards = union(p.LocalForwards, e.Forwards)
	if e.Postgres.User != "" || e.Postgres.DB != "" {
		p.Postgres.User, p.Postgres.Password, p.Postgres.DB = e.Postgres.User, e.Postgres.Password, e.Postgres.DB
	}
	if e.RedisPass != "" {
		p.Redis.Passwords = []string{e.RedisPass}
	}
	if e.MongoUser != "" {
		p.Mongo.User, p.Mongo.Password = e.MongoUser, e.MongoPass
	}
	return nil
}

// union is a new list of a's values then b's that a lacks.
func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, v := range b {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// Finish returns the project to save: settings of services that aren't used are dropped, then
// the whole project is validated and its name checked against existing projects.
func (d *Draft) Finish(store Store) (config.Project, error) {
	p := d.Project
	tidy(&p)
	if err := d.checkImage(); err != nil {
		return p, err
	}
	if err := p.Validate(); err != nil {
		return p, err
	}
	if err := checkWorkspace(store, p); err != nil {
		return p, err
	}
	if _, err := store.Project(p.Name); err == nil {
		return p, problem("a project named %q exists; pick another name", p.Name)
	}
	return p, nil
}

// tidy drops what the answers left without use: settings of services the project doesn't use,
// the always-on presets (they apply anyway), Playwright without browsers, forwards to services
// it doesn't use.
func tidy(p *config.Project) {
	if !p.Uses("postgres") {
		p.Postgres = config.Postgres{}
	}
	if !p.Uses("redis") {
		p.Redis = config.Redis{}
	}
	if !p.Uses("mongo") {
		p.Mongo = config.Mongo{}
	}
	p.AllowPresets = presets.Explicit(p.AllowPresets) // the always-on presets stay
	if len(p.Image.Playwright.Browsers) == 0 {
		p.Image.Playwright.Versions = nil // no browsers, no Playwright
	}
	p.LocalForwards = slices.DeleteFunc(slices.Clone(p.LocalForwards), func(fw string) bool {
		_, host, _, err := config.ParseForward(fw)
		return err == nil && !p.Uses(strings.Replace(host, "mongodb", "mongo", 1))
	})
}

// checkWorkspace checks the project against the workspace it is in, if any.
func checkWorkspace(store Store, p config.Project) error {
	if p.Workspace == "" {
		return nil
	}
	w, err := store.Workspace(p.Workspace)
	if err == nil {
		err = workspace.Check(w, p)
	}
	if err != nil {
		return problem("Workspace: %s", strings.TrimPrefix(err.Error(), "workspace: "))
	}
	return nil
}

// usesPlaywright reports whether the repo's tests use Playwright.
func usesPlaywright(s detect.Suggestion) bool {
	return len(s.Playwright) > 0 || slices.ContainsFunc(s.Facts.Packages, func(p string) bool {
		return p == "@playwright/test" || p == "playwright"
	})
}

// problem is a problem with the answers. (Not Form.Problem: the fields' own functions use it, and
// Form is built from them.)
func problem(format string, a ...any) error {
	return &config.Problems{What: what, List: []string{fmt.Sprintf(format, a...)}}
}

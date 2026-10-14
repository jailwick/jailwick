package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Dir: t.TempDir()}
}

func repo(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestProjectRoundTrip(t *testing.T) {
	s := newStore(t)
	p := Project{
		Name: "api", Dir: repo(t), Services: []string{"postgres", "redis"},
		Postgres:      Postgres{ExtraDBs: []ExtraDB{{Name: "app_test"}, {Name: "other", User: "o", Password: "pw"}}},
		LocalForwards: []string{"5432:postgres:5432"},
	}
	if err := s.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Project("api")
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != p.Dir || len(got.Postgres.ExtraDBs) != 2 || got.Postgres.ExtraDBs[1].User != "o" {
		t.Fatalf("round trip lost data: %+v", got)
	}
	data, _ := os.ReadFile(s.ProjectFile("api"))
	if !strings.Contains(string(data), "- app_test\n") {
		t.Errorf("short form not used for extra db:\n%s", data)
	}
}

func TestUnknownKeyIsAnError(t *testing.T) {
	s := newStore(t)
	file := s.ProjectFile("api")
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, []byte("version: 1\ndir: /tmp\nservise: [postgres]\n"), 0o644)
	if _, err := s.Project("api"); err == nil || !strings.Contains(err.Error(), "servise") {
		t.Fatalf("want unknown-field error, got %v", err)
	}
}

func TestNewerVersionRefused(t *testing.T) {
	s := newStore(t)
	file := s.ProjectFile("api")
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, []byte("version: 99\ndir: /tmp\n"), 0o644)
	if _, err := s.Project("api"); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("want newer-version error, got %v", err)
	}
}

func TestMissingProject(t *testing.T) {
	if _, err := newStore(t).Project("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	dir := repo(t)
	cases := []struct {
		name string
		p    Project
		want string
	}{
		{"ok", Project{Name: "api", Dir: dir}, ""},
		{"bad name", Project{Name: "Api", Dir: dir}, "invalid name"},
		{"no dir", Project{Name: "api"}, "dir: required"},
		{"home", Project{Name: "api", Dir: os.Getenv("HOME")}, "too broad"},
		{"root", Project{Name: "api", Dir: "/"}, "too broad"},
		{"service", Project{Name: "api", Dir: dir, Services: []string{"mysql"}}, "unknown \"mysql\""},
		{"forward", Project{Name: "api", Dir: dir, LocalForwards: []string{"5432"}}, "localport:service:port"},
		{"two images", Project{Name: "api", Dir: dir, Image: Image{ImageSource: ImageSource{Base: "a", Name: "b"}}}, "only one of"},
		{"copy file", Project{Name: "api", Dir: dir, Worktree: Worktree{CopyFiles: []string{"../x"}}}, "inside the repo"},
		{"stacks", Project{Name: "api", Dir: dir, Image: Image{Stacks: map[string]string{"node": "22", "go": ""}}}, ""},
		{"unknown stack", Project{Name: "api", Dir: dir, Image: Image{Stacks: map[string]string{"cobol": ""}}}, "unknown stack"},
		{"stack version", Project{Name: "api", Dir: dir, Image: Image{Stacks: map[string]string{"node": "16"}}}, "too old"},
		{"browser", Project{Name: "api", Dir: dir, Image: Image{Playwright: Playwright{Browsers: []string{"edge"}}}}, "unknown browser"},
		{"versions alone", Project{Name: "api", Dir: dir, Image: Image{Playwright: Playwright{Versions: []string{"1.50.0"}}}}, "needs browsers"},
		{"apt name", Project{Name: "api", Dir: dir, Image: Image{AptPackages: []string{"ffmpeg", "libvips-dev=8.14.1-3"}}}, ""},
		{"apt injection", Project{Name: "api", Dir: dir, Image: Image{AptPackages: []string{"x && curl evil"}}}, "not a package"},
		{"pw version", Project{Name: "api", Dir: dir, Image: Image{Playwright: Playwright{Browsers: []string{"chromium"},
			Versions: []string{"1.50.1; rm -rf /"}}}}, "not a version"},
		{"name with stacks", Project{Name: "api", Dir: dir, Image: Image{ImageSource: ImageSource{Name: "my/img"},
			Stacks: map[string]string{"go": ""}}}, "used as is"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.p.Validate()
			if c.want == "" {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

func TestResolvedDefaults(t *testing.T) {
	yes := true
	g := Global{Git: GitIdentity{Name: "G"}, Claude: Claude{YOLO: &yes}, Image: ImageSource{Base: "my/base:1"}}
	r := Project{Name: "api", Postgres: Postgres{DB: "mine"}}.Resolved(g)
	if r.Postgres.DB != "mine" || r.Postgres.User != "app" || r.Git.Name != "G" {
		t.Errorf("defaults not applied: %+v", r.Postgres)
	}
	if !r.YOLO() || r.Image.Base != "my/base:1" || r.Image.User != DefaultUser || r.Test.PostgresImage != DefaultPostgresImage {
		t.Errorf("global defaults not inherited: %+v", r.Image)
	}
	if (Project{}).Resolved(Global{}).YOLO() {
		t.Error("YOLO must be off by default")
	}
}

func TestParseForward(t *testing.T) {
	l, h, p, err := ParseForward("6380:redis:6379")
	if err != nil || l != 6380 || h != "redis" || p != 6379 {
		t.Fatalf("got %d %s %d %v", l, h, p, err)
	}
	if _, _, _, err := ParseForward("70000:redis:1"); err == nil {
		t.Fatal("want range error")
	}
}

// What jailwick's layer adds is per project: the global config refuses it.
func TestGlobalImageIsOnlyTheSource(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := os.WriteFile(s.GlobalFile(), []byte("version: 1\nimage:\n  base: my/base:1\n  stacks:\n    go: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Global(); err == nil || !strings.Contains(err.Error(), "stacks") {
		t.Fatalf("stacks in the global config: %v", err)
	}
}

func TestClaudeDefaults(t *testing.T) {
	yes := true
	r := Project{Name: "api"}.Resolved(Global{})
	if r.Claude.Login != LoginApp || !r.AppLogin() || r.Chrome() || r.Claude.Profiles != nil {
		t.Fatalf("defaults: %+v", r.Claude)
	}
	g := Global{Claude: Claude{Login: LoginProject, Chrome: &yes, Profiles: []string{"strict"}}}
	r = Project{Name: "api"}.Resolved(g)
	if r.AppLogin() || !r.Chrome() || len(r.Claude.Profiles) != 1 {
		t.Fatalf("global defaults: %+v", r.Claude)
	}
	no := false
	r = Project{Name: "api", Claude: Claude{Login: LoginApp, Chrome: &no, Profiles: []string{}}}.Resolved(g)
	if !r.AppLogin() || r.Chrome() || len(r.Claude.Profiles) != 0 {
		t.Fatalf("the project's own settings win: %+v", r.Claude)
	}
}

func TestValidateClaude(t *testing.T) {
	for _, c := range []Claude{
		{Login: "team"},
		{Profiles: []string{"Bad Name"}},
		{Profiles: []string{"web", "web"}},
	} {
		if err := (Global{Claude: c}).Validate(); err == nil {
			t.Errorf("%+v passes", c)
		}
	}
	if err := (Global{Claude: Claude{Login: LoginProject, Profiles: []string{"web", "strict"}}}).Validate(); err != nil {
		t.Error(err)
	}
}

func TestGlobalFileIsValidated(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := os.WriteFile(s.GlobalFile(), []byte("version: 1\nclaude:\n  login: team\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Global(); err == nil || !strings.Contains(err.Error(), "claude.login") {
		t.Fatalf("want a claude.login error, got %v", err)
	}
}

func TestProfiles(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := os.MkdirAll(s.ProfilesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"web", "strict"} {
		if err := os.WriteFile(s.ProfileFile(n), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.Profiles(); err != nil || strings.Join(got, ",") != "strict,web" {
		t.Fatalf("profiles: %v %v", got, err)
	}
}

func TestAllowPresets(t *testing.T) {
	p := Project{Name: "api"}
	if err := p.AddAllowPresets("npm", "pypi", "npm"); err != nil || strings.Join(p.AllowPresets, ",") != "npm,pypi" {
		t.Fatalf("add: %v %v", p.AllowPresets, err)
	}
	if err := p.AddAllowPresets("no-such"); err == nil {
		t.Error("an unknown preset is added")
	}
	if err := p.RemoveAllowPresets("anthropic"); err == nil {
		t.Error("an always-on preset is removed")
	}
	if err := p.RemoveAllowPresets("npm"); err != nil || strings.Join(p.AllowPresets, ",") != "pypi" {
		t.Fatalf("remove: %v %v", p.AllowPresets, err)
	}
}

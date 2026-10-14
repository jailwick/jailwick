package projectsetup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

type fakeStore []config.Project

func (s fakeStore) Project(name string) (config.Project, error) {
	for _, p := range s {
		if p.Name == name {
			return p, nil
		}
	}
	return config.Project{}, config.ErrNotFound
}

func (s fakeStore) Projects() ([]config.Project, []error) { return s, nil }

func (fakeStore) Workspaces() []string { return nil }

func (fakeStore) Workspace(name string) (config.Workspace, error) {
	return config.Workspace{}, config.ErrNotFound
}

func repo(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for f, data := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func field(t *testing.T, key string) Field {
	t.Helper()
	f, ok := Form.Field(key)
	if !ok {
		t.Fatalf("no field %q", key)
	}
	return f
}

func isProblem(err error) bool {
	var pr *config.Problems
	return errors.As(err, &pr)
}

func TestDraftPrefillsFromExampleEnv(t *testing.T) {
	dir := repo(t, "My Shop", map[string]string{
		".env.example": "DATABASE_URL=postgres://dev:devpw@localhost:5433/shop\n",
	})
	d, err := New(fakeStore{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if d.Project.Name != "my-shop" || !slices.Equal(d.Project.Services, []string{"postgres"}) {
		t.Fatalf("name/services: %q %v", d.Project.Name, d.Project.Services)
	}
	if d.Value(field(t, "postgres.user")) != "dev" || d.Value(field(t, "postgres.db")) != "shop" {
		t.Fatalf("postgres login not prefilled: %+v", d.Project.Postgres)
	}
	if !slices.Equal(d.Value(field(t, "local_forwards")).([]string), []string{"5433:postgres:5432"}) {
		t.Fatalf("forwards: %v", d.Project.LocalForwards)
	}
}

func TestDraftRejectsKnownFolder(t *testing.T) {
	dir := repo(t, "api", nil)
	_, err := New(fakeStore{{Name: "api", Dir: dir}}, dir)
	if !isProblem(err) {
		t.Fatalf("want a problem for a folder that is already a project, got %v", err)
	}
}

func TestSetChecksValues(t *testing.T) {
	d, err := New(fakeStore{}, repo(t, "x", nil))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		key string
		v   any
	}{
		{"name", "Not Valid"},
		{"local_forwards", []string{"5433-postgres"}},
		{"services", []string{"mysql"}},
	}
	for _, c := range cases {
		if err := d.Set(field(t, c.key), c.v); !isProblem(err) {
			t.Errorf("%s=%v: want a problem, got %v", c.key, c.v, err)
		}
	}
	// A list field given text (a flag, one input) is split on commas.
	if err := d.Set(field(t, "services"), "postgres, redis"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Project.Services, []string{"postgres", "redis"}) {
		t.Fatalf("comma list: %v", d.Project.Services)
	}
}

func TestVisible(t *testing.T) {
	d, _ := New(fakeStore{}, repo(t, "x", nil))
	pg, fw := field(t, "postgres.user"), field(t, "local_forwards")
	if d.Visible(pg) || d.Visible(fw) {
		t.Fatal("postgres fields and forwards hide without services")
	}
	_ = d.Set(field(t, "services"), []string{"postgres"})
	if !d.Visible(pg) || !d.Visible(fw) || d.Visible(field(t, "redis.password")) {
		t.Fatal("visibility follows the services")
	}
}

func TestSetAllFromJSONAndFinish(t *testing.T) {
	dir := repo(t, "shop", map[string]string{
		".env": "DATABASE_URL=postgres://a:b@localhost:5433/c\nREDIS_URL=redis://:pw@localhost:6380/0\n",
	})
	d, err := New(fakeStore{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any // as the API decodes it
	_ = json.Unmarshal([]byte(`{"services":["postgres"],"worktree.setup":["pnpm install"," "],"name":"shop-2"}`), &values)
	if err := d.SetAll(values); err != nil {
		t.Fatal(err)
	}
	p, err := d.Finish(fakeStore{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "shop-2" || len(p.Redis.Passwords) != 0 || !slices.Equal(p.LocalForwards, []string{"5433:postgres:5432"}) {
		t.Fatalf("redis settings and forwards should go with the service: %+v", p)
	}
	if !slices.Equal(p.Worktree.Setup, []string{"pnpm install"}) || p.Postgres.User != "a" {
		t.Fatalf("project: %+v", p)
	}

	if err := d.SetAll(map[string]any{"nope": "x"}); !isProblem(err) {
		t.Fatalf("unknown key: %v", err)
	}
	if _, err := d.Finish(fakeStore{{Name: "shop-2"}}); !isProblem(err) {
		t.Fatalf("taken name: %v", err)
	}
}

func TestViewHasEveryField(t *testing.T) {
	d, _ := New(fakeStore{}, repo(t, "x", nil))
	n := 0
	for _, s := range d.View() {
		for _, f := range s.Fields {
			n++
			if f.Value == nil {
				t.Errorf("%s: value must not be null in JSON", f.Key)
			}
		}
	}
	if n != len(Form.Fields()) {
		t.Fatalf("view has %d fields, want %d", n, len(Form.Fields()))
	}
}

func TestFormIsWellFormed(t *testing.T) {
	if err := Form.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultName(t *testing.T) {
	for in, want := range map[string]string{"/x/My_App": "my-app", "/x/---": "project", "/x/API": "api", "/x/a--b": "a-b"} {
		if got := config.NameFrom(in, "project"); got != want {
			t.Errorf("NameFrom(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPresetsFromTheRepo(t *testing.T) {
	dir := repo(t, "shop", map[string]string{
		"package.json": `{"dependencies": {"@prisma/client": "5", "@sentry/node": "8"}}`,
	})
	d, err := New(fakeStore{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Narrow presets are chosen; sentry is broad, so it's only mentioned, on the network step.
	if got := d.Value(field(t, "allow_presets")); !slices.Equal(got.([]string), []string{"anthropic", "npm", "prisma"}) {
		t.Fatalf("presets: %v", got)
	}
	if len(d.Hints) != 0 {
		t.Fatalf("the folder step names no presets: %v", d.Hints)
	}
	if n := field(t, "allow_presets").Note(d); !strings.Contains(n, "sentry") {
		t.Fatalf("the network step must say why sentry isn't chosen: %q", n)
	}
	// Even an empty choice keeps the agent's own preset.
	if err := d.Set(field(t, "allow_presets"), []string{}); err != nil {
		t.Fatal(err)
	}
	p, err := d.Finish(fakeStore{})
	if err != nil || !slices.Equal(p.AllowPresets, []string{"anthropic"}) {
		t.Fatalf("finish: %v %v", p.AllowPresets, err)
	}
	if err := d.Set(field(t, "allow_presets"), []string{"nope"}); !isProblem(err) {
		t.Fatalf("unknown preset: %v", err)
	}
}

func TestToolsetFromTheRepo(t *testing.T) {
	dir := repo(t, "web", map[string]string{
		"package.json":   `{"devDependencies": {"@playwright/test": "1.50.1"}, "engines": {"node": "^22.3.0"}}`,
		"pyproject.toml": "[project]\nname = \"x\"\n",
	})
	d, err := New(fakeStore{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Value(field(t, "image.stacks")).([]string), []string{"node", "python"}) ||
		d.Value(field(t, "image.stacks.node")) != "22" || d.Value(field(t, "image.stacks.python")) != "" {
		t.Fatalf("detected stacks and versions: %v", d.Project.Image.Stacks)
	}
	img := d.Project.Image.Playwright
	if !slices.Equal(img.Browsers, []string{"chromium"}) || !slices.Equal(img.Versions, []string{"1.50.1"}) {
		t.Fatalf("a repo testing with Playwright gets chromium: %+v", img)
	}
	if !slices.Contains(d.Project.AllowPresets, "pypi") || !slices.Contains(d.Project.AllowPresets, "npm") {
		t.Fatalf("the stacks' presets are ticked: %v", d.Project.AllowPresets)
	}
	if !d.Visible(field(t, "image.stacks.node")) || d.Visible(field(t, "image.stacks.go")) {
		t.Fatal("a version is asked only for a chosen stack")
	}

	// Choosing Go keeps the other versions; a draft that was only checked keeps its own.
	if err := d.Check(field(t, "image.stacks"), []string{"go"}); err != nil {
		t.Fatal(err)
	}
	if len(d.Project.Image.Stacks) != 2 {
		t.Fatalf("Check changed the draft: %v", d.Project.Image.Stacks)
	}
	if err := d.SetAll(map[string]any{"image.stacks": []any{"node", "go"}, "image.stacks.go": "1.25",
		"image.stacks.rust": "1.82", "allow_presets": []any{"npm"}}); err != nil {
		t.Fatal(err)
	}
	if got := d.Project.Image.Stacks; len(got) != 2 || got["node"] != "22" || got["go"] != "1.25" {
		t.Fatalf("stacks: %v (a version of an unchosen stack is ignored)", got)
	}
	if err := d.Set(field(t, "image.stacks.node"), "16"); err == nil || !isProblem(err) {
		t.Fatalf("too old a version is refused: %v", err)
	}
	if note := field(t, "allow_presets").Note(d); !strings.Contains(note, "go, which aren't ticked") {
		t.Fatalf("the network step says what the stacks need: %q", note)
	}

	// No browsers, no Playwright: its versions go too.
	if err := d.Set(field(t, "image.playwright.browsers"), []string{}); err != nil {
		t.Fatal(err)
	}
	p, err := d.Finish(fakeStore{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Image.Playwright.Versions) != 0 {
		t.Fatalf("versions without browsers: %+v", p.Image.Playwright)
	}
}

func TestEnvFileGivesTheLogins(t *testing.T) {
	dir := repo(t, "shop", map[string]string{
		".env":    "DATABASE_URL=postgres://dev:devpw@localhost:5432/shop\n",
		"app.env": "DB_CONNECTION=pgsql\nDB_HOST=localhost\nDB_USERNAME=lara\nDB_PASSWORD=secret\nDB_DATABASE=store\nREDIS_PASSWORD=r\n",
		"no.env":  "APP_NAME=shop\n",
	})
	d, err := New(fakeStore{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	f := field(t, "env_file")
	if got := d.Value(f); got != ".env" {
		t.Fatalf("prefilled with the file the logins came from: %q", got)
	}
	if err := d.Set(f, "app.env"); err != nil {
		t.Fatal(err)
	}
	pg := d.Project.Postgres
	if pg.User != "lara" || pg.Password != "secret" || pg.DB != "store" || !slices.Equal(d.Project.Redis.Passwords, []string{"r"}) {
		t.Fatalf("logins: %+v %v", pg, d.Project.Redis.Passwords)
	}
	if !slices.Equal(d.Project.Services, []string{"postgres", "redis"}) {
		t.Fatalf("services: %v", d.Project.Services)
	}
	// Answers given after the file still win: SetAll applies fields in order.
	if err := d.SetAll(map[string]any{"env_file": "app.env", "postgres.user": "mine"}); err != nil || d.Project.Postgres.User != "mine" {
		t.Fatalf("%v %q", err, d.Project.Postgres.User)
	}
	if err := d.Check(f, "no.env"); !isProblem(err) {
		t.Fatalf("a file without a login is a problem: %v", err)
	}
	if err := d.Check(f, "missing.env"); !isProblem(err) {
		t.Fatalf("a missing file is a problem: %v", err)
	}
	if _, err := d.Finish(fakeStore{}); err != nil {
		t.Fatal(err)
	}
}

func TestJoinsTheWorkspaceItIsIn(t *testing.T) {
	store := &config.Store{Dir: t.TempDir()}
	dir := repo(t, "api", nil)
	if err := store.SaveWorkspace(config.Workspace{Name: "shop", Root: filepath.Dir(dir)}); err != nil {
		t.Fatal(err)
	}
	d, err := New(store, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Value(field(t, "workspace")); !slices.Equal(got.([]string), []string{"shop"}) {
		t.Fatalf("the workspace whose root holds the repo is ticked: %v", got)
	}
	if p, err := d.Finish(store); err != nil || p.Workspace != "shop" {
		t.Fatalf("%+v %v", p.Workspace, err)
	}
	if err := d.Set(field(t, "workspace"), []string{}); err != nil || d.Project.Workspace != "" {
		t.Fatalf("unticked: %v", err)
	}
	// Elsewhere, there is nothing to ask.
	other, _ := New(fakeStore{}, repo(t, "web", nil))
	if Form.Visible(other, field(t, "workspace")) {
		t.Fatal("no workspace holds it: the field doesn't apply")
	}
}

// Joining shows its folder to the workspace's other projects unless it keeps to itself; a folder
// already a member has nothing to add.
func TestWorkspaceFolderChoice(t *testing.T) {
	store := &config.Store{Dir: t.TempDir()}
	dir := repo(t, "api", nil)
	if err := store.SaveWorkspace(config.Workspace{Name: "shop", Root: filepath.Dir(dir)}); err != nil {
		t.Fatal(err)
	}
	d, _ := New(store, dir)
	f := field(t, "workspace.folder")
	if !d.Visible(f) || d.Value(f) != FolderShow || !d.AddsMember() {
		t.Fatalf("shown by default: %v %v", d.Value(f), d.AddsMember())
	}
	if err := d.Set(f, FolderHide); err != nil || d.AddsMember() {
		t.Fatalf("keeps to itself: %v", err)
	}
	d.Set(f, FolderShow)
	d.Set(field(t, "workspace"), []string{})
	if d.Visible(f) || d.AddsMember() {
		t.Fatal("in no workspace: nothing to choose")
	}

	store.SaveWorkspace(config.Workspace{Name: "shop", Root: filepath.Dir(dir), Members: []config.Member{{Name: "api"}}})
	d, _ = New(store, dir)
	if d.AddsMember() || f.Note(d) == "" {
		t.Fatal("already a member: nothing to add, and the note says so")
	}
}

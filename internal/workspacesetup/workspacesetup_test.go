package workspacesetup

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

// fixture is a root with the repos api, web and docs (api and web are projects; web is in another
// workspace) and a plain folder.
func fixture(t *testing.T) (*config.Store, string) {
	t.Helper()
	store := &config.Store{Dir: t.TempDir()}
	root := filepath.Join(t.TempDir(), "shop")
	for _, d := range []string{"api/.git", "web/.git", "docs/.git", "assets"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# shop"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []config.Project{{Name: "api", Dir: filepath.Join(root, "api")}, {Name: "web", Dir: filepath.Join(root, "web"), Workspace: "other"}} {
		if err := store.SaveProject(p); err != nil {
			t.Fatal(err)
		}
	}
	return store, root
}

func field(t *testing.T, key string) Field {
	t.Helper()
	f, ok := Form.Field(key)
	if !ok {
		t.Fatalf("no field %q", key)
	}
	return f
}

func TestFormIsWellFormed(t *testing.T) {
	if err := Form.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestNewPrefills(t *testing.T) {
	store, root := fixture(t)
	d, err := New(store, root)
	if err != nil {
		t.Fatal(err)
	}
	w := d.Workspace
	if w.Name != "shop" || w.Handoff != ".handoff" || w.RootNotes != "CLAUDE.md" {
		t.Fatalf("defaults: %+v", w)
	}
	if got := field(t, "members").Get(d); !slices.Equal(got.([]string), []string{"api", "docs", "web"}) {
		t.Fatalf("the repos are the members: %v", got)
	}
	// web is in another workspace: offered, but it can't be chosen.
	if !slices.Equal(d.Projects, []string{"api"}) {
		t.Fatalf("projects: %v", d.Projects)
	}
	if err := d.SetAll(map[string]any{"projects": []string{"api", "web"}}); err == nil {
		t.Fatal("a project of another workspace can't join")
	}
	if err := d.SetAll(map[string]any{"members": []string{"api", "assets"}, "members_elsewhere": []string{"/tmp"}}); err != nil {
		t.Fatal(err)
	}
	if n := len(d.Workspace.Members); n != 3 || d.Workspace.Members[2].Name != "tmp" {
		t.Fatalf("members: %+v", d.Workspace.Members)
	}
}

func TestNewRefuses(t *testing.T) {
	store, root := fixture(t)
	home, _ := os.UserHomeDir()
	if _, err := New(store, home); err == nil {
		t.Fatal("the home folder is too broad")
	}
	d, _ := New(store, root)
	if _, err := d.Save(store); err != nil {
		t.Fatal(err)
	}
	if _, err := New(store, root); err == nil || !strings.Contains(err.Error(), "already the workspace") {
		t.Fatalf("a root is one workspace: %v", err)
	}
}

func TestSaveLoadEditRemove(t *testing.T) {
	store, root := fixture(t)
	d, _ := New(store, root)
	d.Workspace.Handoff = "api/.handoff" // inside a repo: refused before saving
	if _, err := d.Save(store); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("problems show before saving: %v", err)
	}
	d.Workspace.Handoff = ".handoff"
	changed, err := d.Save(store)
	if err != nil || !slices.Equal(changed, []string{"api"}) {
		t.Fatalf("save: %v %v", changed, err)
	}

	e, err := Load(store, "shop")
	if err != nil || !slices.Equal(e.Projects, []string{"api"}) {
		t.Fatalf("load: %+v %v", e, err)
	}
	if err := e.SetAll(map[string]any{"name": "renamed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Save(store); err != nil {
		t.Fatal(err)
	}
	if p, _ := store.Project("api"); p.Workspace != "renamed" {
		t.Fatalf("renaming moves the projects along: %+v", p)
	}
	e, _ = Load(store, "renamed")
	if err := e.SetAll(map[string]any{"projects": []string{}}); err != nil {
		t.Fatal(err)
	}
	if changed, err := e.Save(store); err != nil || !slices.Equal(changed, []string{"api"}) {
		t.Fatalf("api leaves: %v %v", changed, err)
	}

	if _, err := Remove(store, "renamed"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Workspace("renamed"); !errors.Is(err, config.ErrNotFound) {
		t.Fatalf("removed: %v", err)
	}
}

func TestMembership(t *testing.T) {
	store, root := fixture(t)
	d, _ := New(store, root)
	if _, err := d.Save(store); err != nil {
		t.Fatal(err)
	}
	m, err := MembershipOf(store, "api")
	if err != nil || m.Workspace != "shop" || !slices.Equal(m.Members, []string{"docs", "web"}) || !slices.Equal(m.Candidates, []string{"shop"}) {
		t.Fatalf("%+v %v", m, err)
	}
	m.Related, m.Exclude = []string{"docs"}, []string{"drafts/"}
	if _, err := SetMembership(store, "api", m); err != nil {
		t.Fatal(err)
	}
	m.Related = []string{"nope"}
	if _, err := SetMembership(store, "api", m); err == nil {
		t.Fatal("related must be members")
	}
	if p, _ := store.Project("api"); !slices.Equal(p.WorkspaceRelated, []string{"docs"}) {
		t.Fatalf("a refused change saves nothing: %+v", p)
	}
	if _, err := SetMembership(store, "api", Membership{}); err != nil {
		t.Fatal(err)
	}
	if p, _ := store.Project("api"); p.Workspace != "" || p.WorkspaceRelated != nil {
		t.Fatalf("leaving clears the settings: %+v", p)
	}
	if _, err := SetMembership(store, "api", Membership{Workspace: "elsewhere"}); err == nil {
		t.Fatal("only a workspace whose root holds the project")
	}
}

// A repo added under the root after the workspace was made: joining makes its folder a member when
// asked, so the other projects see it after a restart; leaving takes it out only when asked.
func TestJoiningAddsTheFolderAsMember(t *testing.T) {
	store, root := fixture(t)
	d, _ := New(store, root)
	if _, err := d.Save(store); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(root, "public", ".git"), 0o755)
	if err := store.SaveProject(config.Project{Name: "pub", Dir: filepath.Join(root, "public")}); err != nil {
		t.Fatal(err)
	}
	isMember := func() bool {
		w, _ := store.Workspace("shop")
		return slices.ContainsFunc(w.Members, func(m config.Member) bool { return m.Name == "public" })
	}
	if isMember() {
		t.Fatal("made after the workspace: not a member yet")
	}
	if restart, err := SetMembership(store, "pub", Membership{Workspace: "shop"}); err != nil || isMember() || restart != nil {
		t.Fatalf("joined, keeping to itself: member %v, restart %v, %v", isMember(), restart, err)
	}
	SetMembership(store, "pub", Membership{})
	restart, err := SetMembership(store, "pub", Membership{Workspace: "shop", ShowFolder: true})
	if err != nil || !isMember() || !slices.Equal(restart, []string{"api"}) {
		t.Fatalf("joined: member %v, restart %v, %v", isMember(), restart, err)
	}
	if restart, _ := SetMembership(store, "pub", Membership{Workspace: "shop", Exclude: []string{"x/"}}); restart != nil {
		t.Fatalf("already a member: nothing to restart for: %v", restart)
	}
	if _, err := SetMembership(store, "pub", Membership{}); err != nil || !isMember() {
		t.Fatalf("left, folder kept: %v %v", isMember(), err)
	}
	SetMembership(store, "pub", Membership{Workspace: "shop", ShowFolder: true})
	restart, err = SetMembership(store, "pub", Membership{RemoveFolder: true})
	if err != nil || isMember() || !slices.Equal(restart, []string{"api"}) {
		t.Fatalf("left with its folder: member %v, restart %v, %v", isMember(), restart, err)
	}
}

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

func TestApply(t *testing.T) {
	store := &config.Store{Dir: t.TempDir()}
	root := t.TempDir()
	for _, n := range []string{"api", "web", "cli"} {
		p := config.Project{Name: n, Dir: filepath.Join(root, n)}
		if err := os.MkdirAll(p.Dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if n == "cli" {
			p.Workspace = "other"
		}
		if err := store.SaveProject(p); err != nil {
			t.Fatal(err)
		}
	}
	project := func(n string) config.Project {
		p, err := store.Project(n)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	// Create: api and web join.
	w := config.Workspace{Name: "shop", Root: root}
	changed, err := Apply(store, Change{After: &w, Projects: []string{"api", "web"}})
	if err != nil || !slices.Equal(changed, []string{"api", "web"}) {
		t.Fatalf("create: %v %v", changed, err)
	}
	p := project("api")
	p.WorkspaceExclude = []string{"secrets/"}
	if err := store.SaveProject(p); err != nil {
		t.Fatal(err)
	}

	// Edit: web leaves, api stays untouched.
	changed, err = Apply(store, Change{Before: "shop", After: &w, Projects: []string{"api"}})
	if err != nil || !slices.Equal(changed, []string{"web"}) || project("web").Workspace != "" {
		t.Fatalf("edit: %v %v", changed, err)
	}

	// Rename: api moves with its settings, the old file and notes move too.
	if err := os.MkdirAll(store.WorkspaceNotesDir("shop"), 0o755); err != nil {
		t.Fatal(err)
	}
	renamed := w
	renamed.Name = "store"
	if _, err := Apply(store, Change{Before: "shop", After: &renamed, Projects: []string{"api"}}); err != nil {
		t.Fatal(err)
	}
	if p := project("api"); p.Workspace != "store" || len(p.WorkspaceExclude) != 1 {
		t.Fatalf("rename keeps the project's settings: %+v", p)
	}
	if _, err := store.Workspace("shop"); !errors.Is(err, config.ErrNotFound) {
		t.Fatalf("the old file is gone: %v", err)
	}
	if _, err := os.Stat(store.WorkspaceNotesDir("store")); err != nil {
		t.Fatalf("notes moved: %v", err)
	}

	// Remove: api leaves with its settings cleared; cli, in another workspace, is untouched.
	changed, err = Apply(store, Change{Before: "store"})
	if err != nil || !slices.Equal(changed, []string{"api"}) {
		t.Fatalf("remove: %v %v", changed, err)
	}
	if p := project("api"); p.Workspace != "" || p.WorkspaceExclude != nil || project("cli").Workspace != "other" {
		t.Fatalf("after remove: %+v", p)
	}
	if store.Workspaces() != nil {
		t.Fatalf("no workspace left: %v", store.Workspaces())
	}
}

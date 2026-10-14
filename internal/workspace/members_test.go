package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

// A folder becomes a member named by its path from the root; a name another member has gets a
// free one, with the path; a member already showing the folder is left alone.
func TestWithMember(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"api", "apps/web", "web"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	w := config.Workspace{Name: "shop", Root: root, Members: []config.Member{{Name: "web"}}}
	if _, added := WithMember(w, filepath.Join(root, "web")); added {
		t.Fatal("already a member")
	}
	w, added := WithMember(w, filepath.Join(root, "api"))
	if !added || w.Members[1] != (config.Member{Name: "api"}) {
		t.Fatalf("right under the root: %+v", w.Members)
	}
	w, added = WithMember(w, filepath.Join(root, "apps", "web"))
	if !added || w.Members[2] != (config.Member{Name: "apps/web"}) {
		t.Fatalf("deeper: %+v", w.Members)
	}
	os.MkdirAll(filepath.Join(root, "x", "api"), 0o755)
	w.Members = append(w.Members, config.Member{Name: "x/api", Path: "elsewhere"}) // a name taken by another folder
	w, added = WithMember(w, filepath.Join(root, "x", "api"))
	if !added || w.Members[4] != (config.Member{Name: "api-2", Path: "x/api"}) {
		t.Fatalf("its name taken: %+v", w.Members)
	}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, added := WithMember(w, t.TempDir()); added {
		t.Fatal("a folder outside the root isn't added")
	}
}

// jw doctor's check: a project of the workspace whose folder isn't a member.
func TestMissingMember(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "api"), 0o755)
	os.MkdirAll(filepath.Join(root, "public"), 0o755)
	w := config.Workspace{Name: "shop", Root: root, Members: []config.Member{{Name: "api"}}}
	if _, missing := MissingMember(w, config.Project{Dir: filepath.Join(root, "api")}); missing {
		t.Fatal("a member")
	}
	if m, missing := MissingMember(w, config.Project{Dir: filepath.Join(root, "public")}); !missing || m.Name != "public" {
		t.Fatalf("not a member: %+v %v", m, missing)
	}
	if len(w.Members) != 1 {
		t.Fatal("checking changes nothing")
	}
}

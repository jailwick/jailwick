package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func call(t *testing.T, s *Server, method, url, body string, out any) int {
	t.Helper()
	w := do(s, method, url, func(r *http.Request) {
		r.Body = io.NopCloser(strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	})
	if out != nil && w.Code < 300 {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, url, err, w.Body)
		}
	}
	return w.Code
}

// The app creates, edits and removes a workspace with the same rules as the CLI.
func TestWorkspaceSetup(t *testing.T) {
	s := testServer(t, "")
	api, _ := s.env.Store.Project("api")
	root := filepath.Dir(api.Dir)
	os.MkdirAll(filepath.Join(api.Dir, ".git"), 0o755)
	rootJSON, _ := json.Marshal(root)

	var draft DraftView
	if code := call(t, s, "POST", "/api/workspaces/draft", `{"root":`+string(rootJSON)+`}`, &draft); code != 200 {
		t.Fatalf("draft: %d", code)
	}
	if len(draft.Steps) == 0 || len(draft.Problems) != 0 || !strings.Contains(draft.YAML, "- api") {
		t.Fatalf("draft: %+v", draft)
	}
	// A bad answer is a problem in the draft, not a failed request.
	call(t, s, "POST", "/api/workspaces/draft", `{"root":`+string(rootJSON)+`,"values":{"handoff":"api/x"}}`, &draft)
	if len(draft.Problems) != 1 {
		t.Fatalf("problems: %v", draft.Problems)
	}

	var saved changedView
	if code := call(t, s, "POST", "/api/workspaces", `{"root":`+string(rootJSON)+`,"values":{"name":"shop"}}`, &saved); code != 200 ||
		saved.Name != "shop" || len(saved.Changed) != 1 {
		t.Fatalf("create: %d %+v", code, saved)
	}
	if code := call(t, s, "POST", "/api/workspaces", `{"root":`+string(rootJSON)+`}`, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("a root is one workspace: %d", code)
	}
	if code := call(t, s, "PUT", "/api/workspaces/shop", `{"values":{"projects":[]}}`, &saved); code != 200 || len(saved.Changed) != 1 {
		t.Fatalf("edit: %d %+v", code, saved)
	}
	if code := call(t, s, "DELETE", "/api/workspaces/shop", ``, &saved); code != 200 {
		t.Fatalf("remove: %d", code)
	}
	if code := call(t, s, "DELETE", "/api/workspaces/shop", ``, nil); code != http.StatusNotFound {
		t.Fatalf("remove again: %d", code)
	}
}

// A project's own side of its workspace, the workspace's notes, and a handoff request.
func TestWorkspaceDetails(t *testing.T) {
	s := testServer(t, "")
	api, _ := s.env.Store.Project("api")
	root := filepath.Dir(api.Dir)
	os.MkdirAll(filepath.Join(root, "docs", "drafts"), 0o755)
	rootJSON, _ := json.Marshal(root)
	if code := call(t, s, "POST", "/api/workspaces", `{"root":`+string(rootJSON)+`,"values":{"name":"shop","members":["api","docs"]}}`, nil); code != 200 {
		t.Fatalf("create: %d", code)
	}

	var m struct {
		Workspace                 string
		Members, Related, Exclude []string
	}
	if call(t, s, "GET", "/api/projects/api/workspace", ``, &m); m.Workspace != "shop" || len(m.Members) != 1 {
		t.Fatalf("membership: %+v", m)
	}
	if code := call(t, s, "PUT", "/api/projects/api/workspace", `{"workspace":"shop","related":["nope"]}`, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("related must be members: %d", code)
	}
	if call(t, s, "PUT", "/api/projects/api/workspace", `{"workspace":"shop","exclude":["drafts/"]}`, &m); len(m.Exclude) != 1 {
		t.Fatalf("saved: %+v", m)
	}

	var v WorkspaceView
	if call(t, s, "GET", "/api/workspaces/shop?project=api", ``, &v); v.Project != "api" || len(v.Hidden) != 1 || v.Hidden[0] != "docs/drafts" {
		t.Fatalf("hidden as api sees it: %+v", v)
	}

	if code := call(t, s, "PUT", "/api/workspaces/shop/notes", `{"text":"# shop"}`, nil); code != 200 {
		t.Fatalf("notes: %d", code)
	}
	var notes struct{ Text string }
	if call(t, s, "GET", "/api/workspaces/shop/notes", ``, &notes); notes.Text != "# shop" {
		t.Fatalf("notes: %+v", notes)
	}

	req := filepath.Join(root, ".handoff", "to-api", "1.md")
	os.MkdirAll(filepath.Dir(req), 0o755)
	os.WriteFile(req, []byte("---\nfrom: docs\nsubject: Hi\n---\nbody\n"), 0o644)
	var h struct {
		Meta struct{ From string }
		Body string
	}
	if code := call(t, s, "GET", "/api/workspaces/shop/handoff?file="+req, ``, &h); code != 200 || h.Meta.From != "docs" || h.Body != "body\n" {
		t.Fatalf("handoff: %d %+v", code, h)
	}
	if code := call(t, s, "GET", "/api/workspaces/shop/handoff?file="+api.Dir+"/x.md", ``, nil); code != http.StatusNotFound {
		t.Fatalf("only requests of the handoff folder: %d", code)
	}

	// Renamed: the project moves along, and the members say which folder is which project.
	var saved changedView
	if code := call(t, s, "PUT", "/api/workspaces/shop", `{"values":{"name":"store"}}`, &saved); code != 200 || saved.Name != "store" {
		t.Fatalf("rename: %d %+v", code, saved)
	}
	if call(t, s, "GET", "/api/workspaces/store", ``, &v); len(v.Members) != 2 || v.Members[0].Project != "api" || v.Members[1].Project != "" {
		t.Fatalf("members: %+v", v.Members)
	}
}

// The project's node_modules folders: found automatically, or its own list from the page.
func TestNodeModulesSetting(t *testing.T) {
	s := testServer(t, "")
	api, _ := s.env.Store.Project("api")
	os.MkdirAll(filepath.Join(api.Dir, "web"), 0o755)
	os.WriteFile(filepath.Join(api.Dir, "web", "package.json"), []byte("{}"), 0o644)

	var d struct{ NodeModules NodeModulesView `json:"node_modules"` }
	if call(t, s, "GET", "/api/projects/api", ``, &d); !d.NodeModules.Auto || len(d.NodeModules.Dirs) != 1 || d.NodeModules.Dirs[0] != "web" {
		t.Fatalf("found: %+v", d.NodeModules)
	}
	if code := call(t, s, "PUT", "/api/projects/api/node-modules", `{"dirs":["../outside"]}`, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("outside the repo is refused: %d", code)
	}
	if code := call(t, s, "PUT", "/api/projects/api/node-modules", `{"dirs":["web","apps/*"]}`, nil); code != 200 {
		t.Fatalf("save: %d", code)
	}
	if call(t, s, "GET", "/api/projects/api", ``, &d); d.NodeModules.Auto || len(d.NodeModules.Setting) != 2 {
		t.Fatalf("chosen: %+v", d.NodeModules)
	}
}

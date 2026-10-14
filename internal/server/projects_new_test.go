package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/hostutil"
)

func post(s *Server, url, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", url, strings.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "shop")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, ".env.example"), []byte("DATABASE_URL=postgres://dev:pw@localhost:5433/shop\n"), 0o644)
	return dir
}

func TestDraftProject(t *testing.T) {
	s := testServer(t, "")
	dir := newRepo(t)

	w := post(s, "/api/projects/draft", `{"dir":"`+dir+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("draft: %d %s", w.Code, w.Body)
	}
	var v DraftView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Steps) == 0 || len(v.Problems) != 0 || !strings.Contains(v.YAML, "db: shop") {
		t.Fatalf("draft view: %+v", v)
	}

	// A bad answer is a problem next to the steps, not an error.
	w = post(s, "/api/projects/draft", `{"dir":"`+dir+`","values":{"name":"Bad Name"}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "invalid name") {
		t.Fatalf("bad value: %d %s", w.Code, w.Body)
	}
}

func TestDraftProjectBadInput(t *testing.T) {
	s := testServer(t, "")
	for body, want := range map[string]int{
		`not json`:                       http.StatusBadRequest,
		`{"dir":""}`:                     http.StatusBadRequest,
		`{"dir":"/no/such/folder/here"}`: http.StatusUnprocessableEntity,
	} {
		if w := post(s, "/api/projects/draft", body); w.Code != want {
			t.Errorf("%s: got %d, want %d (%s)", body, w.Code, want, w.Body)
		}
	}
}

func TestCreateProject(t *testing.T) {
	s := testServer(t, "")
	dir := newRepo(t)

	w := post(s, "/api/projects", `{"dir":"`+dir+`","values":{"name":"shop","worktree.setup":["pnpm install"]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	p, err := s.env.Store.Project("shop")
	if err != nil || p.Postgres.DB != "shop" || len(p.Worktree.Setup) != 1 {
		t.Fatalf("saved project: %+v %v", p, err)
	}
	// The same folder again is refused.
	if w := post(s, "/api/projects", `{"dir":"`+dir+`","values":{"name":"shop-2"}}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("second create: %d %s", w.Code, w.Body)
	}
	// So is a bad value, instead of saving without it.
	if w := post(s, "/api/projects", `{"dir":"`+newRepo(t)+`","values":{"services":["mysql"]}}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad value: %d %s", w.Code, w.Body)
	}
}

func TestPickFolder(t *testing.T) {
	s := testServer(t, "")
	repo := newRepo(t)
	var gotStart string
	pick := func(path string, err error) {
		s.PickFolder = func(_ context.Context, _, start string) (string, error) {
			gotStart = start
			return path, err
		}
	}

	pick(repo, nil)
	w := post(s, "/api/pick-folder", `{"start":"`+filepath.Dir(repo)+`"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"path":"`+repo+`"`) {
		t.Fatalf("pick: %d %s", w.Code, w.Body)
	}
	if gotStart != filepath.Dir(repo) {
		t.Errorf("the dialog opens in %q, want the given folder", gotStart)
	}

	pick("", nil) // cancelled; a missing start falls back to the home folder
	w = post(s, "/api/pick-folder", `{"start":"/no/such/folder"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"path":""`) || gotStart == "/no/such/folder" {
		t.Fatalf("cancel: %d %s (start %q)", w.Code, w.Body, gotStart)
	}

	pick("", hostutil.ErrNoFolderPicker)
	if w = post(s, "/api/pick-folder", `{}`); w.Code != http.StatusNotImplemented {
		t.Fatalf("no picker: %d %s", w.Code, w.Body)
	}
}

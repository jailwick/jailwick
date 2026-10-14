package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/core"
	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
)

func testServer(t *testing.T, token string) *Server {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(t.TempDir(), "api")
	os.MkdirAll(repo, 0o755)
	store := &config.Store{Dir: dir}
	// An unknown runtime keeps the tests independent of Docker.
	if err := store.SaveGlobal(config.Global{Runtime: "none-for-tests", Secrets: "file"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProject(config.Project{Name: "api", Dir: repo, Services: []string{"postgres"}}); err != nil {
		t.Fatal(err)
	}
	ui := fstest.MapFS{"index.html": {Data: []byte("<html>jailwick</html>")}}
	return New(&core.Env{Store: store}, ui, token, "test")
}

func do(s *Server, method, url string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, nil)
	if mutate != nil {
		mutate(r)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestTokenAndHost(t *testing.T) {
	s := testServer(t, "secret")
	local := func(r *http.Request) { r.Host = "127.0.0.1:7788" }

	if w := do(s, "GET", "/api/overview", local); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: got %d", w.Code)
	}
	w := do(s, "GET", "/?token=secret", local)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Set-Cookie"), "HttpOnly") {
		t.Fatalf("token link: got %d %q", w.Code, w.Header().Get("Set-Cookie"))
	}
	withCookie := func(r *http.Request) {
		local(r)
		r.AddCookie(&http.Cookie{Name: cookieName, Value: "secret"})
	}
	if w := do(s, "GET", "/api/overview", withCookie); w.Code != http.StatusOK {
		t.Fatalf("with cookie: got %d", w.Code)
	}
	rebinding := func(r *http.Request) {
		withCookie(r)
		r.Host = "evil.example:7788"
	}
	if w := do(s, "GET", "/api/overview", rebinding); w.Code != http.StatusForbidden {
		t.Fatalf("foreign host: got %d", w.Code)
	}
	if w := do(s, "GET", "/api/overview", func(r *http.Request) { local(r); r.AddCookie(&http.Cookie{Name: cookieName, Value: "wrong"}) }); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: got %d", w.Code)
	}
}

func TestOverviewWithoutRuntime(t *testing.T) {
	s := testServer(t, "")
	w := do(s, "GET", "/api/overview", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body)
	}
	var ov Overview
	if err := json.Unmarshal(w.Body.Bytes(), &ov); err != nil {
		t.Fatal(err)
	}
	if len(ov.Projects) != 1 || ov.Projects[0].Name != "api" || ov.Projects[0].Running {
		t.Fatalf("unexpected overview: %+v", ov)
	}
	// The app reads .length on lists: none of them may be null.
	if strings.Contains(w.Body.String(), "null") {
		t.Fatalf("overview has a null list: %s", w.Body)
	}
}

func TestLiveWithoutRuntime(t *testing.T) {
	s := testServer(t, "")
	_, end, err := s.ops.begin("api", opRestarting) // the page shows it, whatever the runtime says
	if err != nil {
		t.Fatal(err)
	}
	defer end()
	w := do(s, "GET", "/api/live", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body)
	}
	var lv Live
	if err := json.Unmarshal(w.Body.Bytes(), &lv); err != nil {
		t.Fatal(err)
	}
	if lv.Runtime.Error == "" || lv.Projects == nil || len(lv.Projects) != 0 || lv.Ops["api"] != opRestarting {
		t.Fatalf("unexpected live state: %s", w.Body)
	}
}

func TestNotFoundAndStatic(t *testing.T) {
	s := testServer(t, "")
	if w := do(s, "GET", "/api/projects/nope/notes", nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown project: got %d", w.Code)
	}
	if w := do(s, "GET", "/projects/api", nil); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "jailwick") {
		t.Fatalf("client route should serve index.html: %d", w.Code)
	}
	if w := do(s, "GET", "/api/unknown", nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown api: got %d", w.Code)
	}
}

func TestNotesRoundTrip(t *testing.T) {
	s := testServer(t, "")
	r := httptest.NewRequest("PUT", "/api/projects/api/notes", strings.NewReader(`{"text":"# api\nrun: pnpm dev\n"}`))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w = do(s, "GET", "/api/projects/api/notes", nil)
	if !strings.Contains(w.Body.String(), "pnpm dev") {
		t.Fatalf("notes not saved: %s", w.Body)
	}
}

// Without a runtime the project page still gets the project, with the reason in error.
func TestProjectWithoutRuntime(t *testing.T) {
	s := testServer(t, "")
	w := do(s, "GET", "/api/projects/api", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body)
	}
	var d ProjectDetail
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Name != "api" || d.Error == "" || d.Files["config"] == "" || d.List == nil || len(d.Allow) == 0 {
		t.Fatalf("detail: %+v", d)
	}
	if strings.Contains(w.Body.String(), "null") {
		t.Fatalf("the page reads .length on lists: none may be null: %s", w.Body)
	}
	if w := do(s, "GET", "/api/projects/nope", nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown project: %d", w.Code)
	}
}

// A stopped project: the runtime answers with no containers (nil). The page must still get lists.
func TestProjectDetailStopped(t *testing.T) {
	d := ProjectDetail{Pending: []string{}, List: []sandbox.Session{}, Worktrees: []agentapi.Worktree{},
		Containers: []runtime.Container{}, Allow: []string{}}
	d.withStatus(&sandbox.State{Name: "api"})
	if d.Running || d.Containers == nil || d.Pending == nil {
		t.Fatalf("a stopped project must answer with empty lists, not null: %+v", d)
	}
}

// buildingDriver builds by printing a line, and lets the test look at the page meanwhile.
type buildingDriver struct {
	runtime.Driver
	during func()
	fail   error
}

func (d *buildingDriver) Build(_ context.Context, o runtime.BuildOptions) error {
	fmt.Fprintln(o.Out, "#5 [2/4] apt-get install")
	d.during()
	return d.fail
}
func (d *buildingDriver) ImageID(context.Context, string) (string, error) { return "", nil }
func (d *buildingDriver) ImageLabels(context.Context, string) (map[string]string, error) {
	return nil, nil
}

// A start's image builds show on the project page while it runs (on the op's own context: see ops).
func TestBuildProgress(t *testing.T) {
	b := builds{by: map[string]BuildView{}}
	ctx, done := b.watch(context.Background(), "api")
	var seen []BuildView
	d := &buildingDriver{during: func() { seen = append(seen, *b.of("api")) }}
	img := &image.Builder{Driver: d, StateDir: t.TempDir()}
	p := config.Project{Name: "api", Image: config.Image{ImageSource: config.ImageSource{ClaudeCodeVersion: "2.1.0", User: "node"}}}
	if _, err := img.Ensure(ctx, p, image.Options{}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || !strings.HasPrefix(seen[0].Step, "Setting up the network filter") ||
		!strings.HasPrefix(seen[2].Step, "Setting up the sandbox") || seen[2].Line != "#5 [2/4] apt-get install" ||
		seen[2].Done != 2 || seen[2].Total != 4 ||
		!strings.HasSuffix(seen[2].Log, ".log") {
		t.Fatalf("the page sees each build's step and latest line: %+v", seen)
	}
	done(nil)
	if b.of("api") != nil {
		t.Fatal("finished builds are forgotten")
	}

	// A failed build stays for the page, with its last lines, until the next start.
	ctx, done = b.watch(context.Background(), "api")
	d.fail = errors.New("exit status 1")
	_, err := (&image.Builder{Driver: d, StateDir: t.TempDir()}).Ensure(ctx, p, image.Options{})
	done(err)
	if v := b.of("api"); v == nil || !v.Failed || len(v.Tail) == 0 {
		t.Fatalf("the failure stays: %+v", v)
	}
	_, done = b.watch(context.Background(), "api")
	if b.of("api") != nil {
		t.Fatal("a new start forgets the failure")
	}
	done(nil)
}

// Running sessions stop a restart or a stop until the page asks: 409.
func TestSessionsRunningIsAConflict(t *testing.T) {
	w := httptest.NewRecorder()
	apiError(w, fmt.Errorf("%w: 2 in api", sandbox.ErrSessionsRunning))
	if w.Code != http.StatusConflict {
		t.Fatalf("got %d", w.Code)
	}
}

// Resuming a running conversation is a conflict, like stopping with sessions running.
func TestConversationRunningIsConflict(t *testing.T) {
	w := httptest.NewRecorder()
	apiError(w, sandbox.CheckResume(map[string]sandbox.Session{"a1": {}}, "a1"))
	if w.Code != http.StatusConflict {
		t.Fatalf("got %d", w.Code)
	}
}

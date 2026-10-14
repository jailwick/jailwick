package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func send(s *Server, method, url, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

// editServer is a test server whose own presets go to its configuration folder.
func editServer(t *testing.T) *Server {
	t.Helper()
	s := testServer(t, "")
	t.Setenv("JAILWICK_HOME", s.env.Store.Dir)
	return s
}

func TestCustomPresetsAPI(t *testing.T) {
	s := editServer(t)
	if w := send(s, "PUT", "/api/presets/acme", `{"description":"Acme","hosts":"api.acme.dev, .acme-cdn.net\nhttps://docs.acme.dev/x"}`); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	if w := send(s, "PUT", "/api/presets/npm", `{"hosts":"x.dev"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a built-in name: %d %s", w.Code, w.Body)
	}
	if w := send(s, "PUT", "/api/presets/bad", `{"hosts":"not a host"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad hosts: %d %s", w.Code, w.Body)
	}

	// A project adds it like any preset; it can't be deleted while used.
	if w := send(s, "POST", "/api/network/allow", `{"preset":"acme","project":"api"}`); w.Code != http.StatusOK {
		t.Fatalf("allow preset: %d %s", w.Code, w.Body)
	}
	p, _ := s.env.Store.Project("api")
	if !slices.Contains(p.AllowPresets, "acme") {
		t.Fatalf("allow_presets: %v", p.AllowPresets)
	}
	if w := send(s, "POST", "/api/network/allow", `{"preset":"acme"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a preset needs a project: %d", w.Code)
	}
	if w := send(s, "DELETE", "/api/presets/acme", ""); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "api uses") {
		t.Fatalf("delete while used: %d %s", w.Code, w.Body)
	}
	if w := send(s, "POST", "/api/network/deny", `{"preset":"anthropic","project":"api"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("an always-on preset is removed: %d", w.Code)
	}
	if w := send(s, "POST", "/api/network/deny", `{"preset":"acme","project":"api"}`); w.Code != http.StatusOK {
		t.Fatalf("deny preset: %d %s", w.Code, w.Body)
	}
	if w := send(s, "DELETE", "/api/presets/acme", ""); w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}

	if w := do(s, "GET", "/api/presets", nil); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"npm"`) {
		t.Fatalf("presets: %d", w.Code)
	}
	var n NetworkView
	w := do(s, "GET", "/api/network", nil)
	if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil || n.ProjectPresets == nil {
		t.Fatalf("network: %v %s", err, w.Body)
	}
}

func TestSaveClaudeSettings(t *testing.T) {
	s := editServer(t)
	if w := send(s, "PATCH", "/api/projects/api/claude", `{"login":"project","chrome":true,"profiles":[]}`); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	p, _ := s.env.Store.Project("api")
	if p.Claude.Login != "project" || !p.Chrome() || p.Claude.Profiles != nil {
		t.Fatalf("saved: %+v", p.Claude)
	}
	// Only what is sent changes.
	if w := send(s, "PATCH", "/api/projects/api/claude", `{"chrome":false}`); w.Code != http.StatusOK {
		t.Fatalf("chrome only: %d %s", w.Code, w.Body)
	}
	if p, _ = s.env.Store.Project("api"); p.Claude.Login != "project" || p.Chrome() {
		t.Fatalf("partial change: %+v", p.Claude)
	}
	if w := send(s, "PATCH", "/api/projects/api/claude", `{"login":"team"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad login: %d %s", w.Code, w.Body)
	}
	if w := send(s, "PATCH", "/api/projects/api/claude", `{"login":"app","profiles":["missing"]}`); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), `no profile \"missing\"`) {
		t.Fatalf("missing profile: %d %s", w.Code, w.Body)
	}
}

func TestSecretsAPI(t *testing.T) {
	s := editServer(t)
	if w := send(s, "PUT", "/api/secrets", `{"name":"claude_code_oauth_token","value":"tok"}`); w.Code != http.StatusOK {
		t.Fatalf("set global: %d %s", w.Code, w.Body)
	}
	if w := send(s, "PUT", "/api/secrets", `{"name":"GH_TOKEN","value":"gh","project":"api"}`); w.Code != http.StatusOK {
		t.Fatalf("set for a project: %d %s", w.Code, w.Body)
	}
	for _, bad := range []string{`{"name":"bad name","value":"x"}`, `{"name":"X","value":""}`, `{"name":"X","value":"x","project":"nope"}`} {
		if w := send(s, "PUT", "/api/secrets", bad); w.Code == http.StatusOK {
			t.Errorf("%s accepted", bad)
		}
	}
	w := do(s, "GET", "/api/secrets", nil)
	if strings.Contains(w.Body.String(), "tok") || strings.Contains(w.Body.String(), `"gh"`) {
		t.Fatalf("a value was sent: %s", w.Body)
	}
	var v SecretsView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	want := []SecretView{{Name: "CLAUDE_CODE_OAUTH_TOKEN"}, {Name: "GH_TOKEN", Project: "api"}}
	if !slices.Equal(v.Secrets, want) || v.Store != "file" || len(v.Overrides) == 0 {
		t.Fatalf("list: %+v", v)
	}
	if w := send(s, "DELETE", "/api/secrets/GH_TOKEN?project=api", ""); w.Code != http.StatusOK {
		t.Fatalf("unset: %d %s", w.Code, w.Body)
	}
	if w := send(s, "DELETE", "/api/secrets/GH_TOKEN?project=api", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("unset again: %d %s", w.Code, w.Body)
	}
}

func TestDefaultLogin(t *testing.T) {
	s := editServer(t)
	var v SettingsView
	get := func() {
		t.Helper()
		v = SettingsView{}
		if err := json.Unmarshal(do(s, "GET", "/api/settings", nil).Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
	}
	get()
	if v.LoginDefault != "app" || len(v.Logins) != 1 || v.Logins[0] != (ProjectLogin{Project: "api", Uses: "app"}) {
		t.Fatalf("defaults: %+v", v)
	}
	if w := send(s, "PATCH", "/api/settings/claude", `{"login":"project"}`); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	get()
	if v.LoginDefault != "project" || v.Logins[0].Uses != "project" || v.Logins[0].Login != "" {
		t.Fatalf("after: %+v", v)
	}
	if w := send(s, "PATCH", "/api/settings/claude", `{"login":"team"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad login: %d %s", w.Code, w.Body)
	}
}

func TestSaveNotifications(t *testing.T) {
	s := testServer(t, "")
	if w := send(s, "PATCH", "/api/settings/notifications", `{"sessions":"terminal","blocked":false}`); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	g, _ := s.env.Store.Global()
	if g.SessionNotify() != "terminal" || g.BlockedNotificationsOn() || !g.NotificationsOn() {
		t.Fatalf("%+v", g)
	}
	if w := send(s, "PATCH", "/api/settings/notifications", `{"sessions":"sometimes"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a bad mode: %d", w.Code)
	}
	if w := send(s, "PATCH", "/api/settings/notifications", `{"on":false}`); w.Code != http.StatusOK {
		t.Fatalf("%d", w.Code)
	}
	if g, _ := s.env.Store.Global(); g.NotificationsOn() || g.SessionNotify() != "terminal" {
		t.Fatalf("only what was given changes: %+v", g)
	}
}

func TestSaveDefaultImage(t *testing.T) {
	s := testServer(t, "")
	if w := send(s, "PUT", "/api/settings/image", `{"base":"eclipse-temurin:21-jdk","user":"app","home":"/home/app"}`); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	g, _ := s.env.Store.Global()
	if g.Image.Base != "eclipse-temurin:21-jdk" || g.Image.User != "app" || g.Image.Home != "/home/app" {
		t.Fatalf("%+v", g.Image)
	}
	p, _ := s.env.Store.Project("api")
	if r := p.Resolved(g); r.Image.Base != "eclipse-temurin:21-jdk" {
		t.Fatalf("a project without its own image builds on the default: %+v", r.Image)
	}
	for _, bad := range []string{`{"base":"Not An Image"}`, `{"base":"x","user":"Root"}`, `{"base":"x","home":"rel/path"}`} {
		if w := send(s, "PUT", "/api/settings/image", bad); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	if w := post(s, "/api/images/check", `{"ref":"Not An Image"}`); w.Code != http.StatusBadRequest {
		t.Errorf("checking a bad name: %d", w.Code)
	}
}

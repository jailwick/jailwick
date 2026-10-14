package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProjectSettings(t *testing.T) {
	s := testServer(t, "")
	w := post(s, "/api/projects/api/settings/draft", `{"values":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var d DraftView
	json.Unmarshal(w.Body.Bytes(), &d)
	var ids []string
	for _, st := range d.Steps {
		ids = append(ids, st.ID)
		for _, f := range st.Fields {
			if f.Key == "name" || f.Key == "workspace" {
				t.Errorf("%s is in the settings", f.Key)
			}
		}
	}
	if strings.Join(ids, " ") != "services image toolset network worktrees resources" || len(d.Problems) != 0 {
		t.Fatalf("steps %v, problems %v", ids, d.Problems)
	}

	if w := send(s, "PUT", "/api/projects/api/settings", `{"values":{"run":"pnpm dev","resources.memory":"8g"}}`); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	p, _ := s.env.Store.Project("api")
	if p.Run != "pnpm dev" || p.Resources.Memory != "8g" {
		t.Fatalf("%+v", p)
	}
	if w := send(s, "PUT", "/api/projects/api/settings", `{"values":{"resources.cpus":"none"}}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a bad value: %d", w.Code)
	}
}

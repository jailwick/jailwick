package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLanguages(t *testing.T) {
	dir := t.TempDir()
	for f, data := range map[string]string{
		"composer.json":            "{}",
		"api/Api.csproj":           "<Project/>",
		"web/app.csproj":           "<Project/>", // .NET once
		"node_modules/x/mix.exs":   "",           // a dependency's: not the repo's
		"a/b/c/d/too-deep/mix.exs": "",
		"deps.edn":                 "{}", // Clojure: the java stack
	} {
		p := filepath.Join(dir, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(data), 0o644)
	}
	got := Languages(dir)
	if len(got) != 2 || got[0].Name != "PHP" || got[0].File != "composer.json" || got[1].Name != ".NET" {
		t.Fatalf("%+v", got)
	}
	if s := Repo(dir); len(s.Languages) != 2 || s.Stacks["java"] != "" {
		t.Fatalf("Repo: languages %+v, stacks %v", s.Languages, s.Stacks)
	} else if _, ok := s.Stacks["java"]; !ok {
		t.Fatal("deps.edn should tick the java stack")
	}
}

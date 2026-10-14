package stacks

import (
	"slices"
	"strings"
	"testing"
)

func TestCatalogue(t *testing.T) {
	if !slices.Equal(Names(), []string{"node", "python", "go", "rust", "bun", "java", "ruby"}) {
		t.Fatalf("layer order: %v", Names())
	}
	for _, s := range All() {
		if s.Description == "" || len(s.Tools) == 0 || len(s.Presets) == 0 || s.Notes == "" {
			t.Errorf("%s: needs a description, tools, presets and a note", s.Name)
		}
		for _, v := range append([]string{s.Default}, s.Versions...) {
			if err := s.Check(v); err != nil {
				t.Errorf("%s: its own version %q is refused: %v", s.Name, v, err)
			}
		}
	}
}

func TestCheck(t *testing.T) {
	for _, ok := range []map[string]string{
		nil, {"node": ""}, {"node": "22.11.0", "go": "1.25.3"}, {"python": "3.12"},
		{"rust": "1.82"}, {"rust": "nightly-2026-01-15"}, {"bun": "1.2"}, {"java": "17"}, {"ruby": "system"},
	} {
		if err := Check(ok); err != nil {
			t.Errorf("Check(%v): %v", ok, err)
		}
	}
	for msg, chosen := range map[string]map[string]string{
		"unknown stack":    {"elixir": "1.17"},
		"is too old":       {"node": "16"},
		"is not a version": {"go": "latest"},
	} {
		if err := Check(chosen); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("Check(%v) = %v, want %q", chosen, err, msg)
		}
	}
}

func TestChosen(t *testing.T) {
	got := Chosen(map[string]string{"bun": "", "node": "22"})
	if len(got) != 2 || got[0].Name != "node" || got[0].Version != "22" || got[1].Name != "bun" || got[1].Version != "1" {
		t.Fatalf("in layer order, defaults filled in: %+v", got)
	}
}

// A version past the newest listed by more than one release doesn't exist yet.
func TestCheckRefusesVersionsThatDontExist(t *testing.T) {
	node, _ := Get("node")
	python, _ := Get("python")
	golang, _ := Get("go")
	rust, _ := Get("rust")
	for _, c := range []struct {
		s  Stack
		v  string
		ok bool
	}{
		{node, "34", false}, {node, "27", true}, {node, "26", true}, {node, "24.11.1", true},
		{python, "3.20", false}, {python, "3.15", true},
		{golang, "1.40", false}, {golang, "1.28", true},
		{rust, "1.99", true}, {rust, "stable", true},
	} {
		if err := c.s.Check(c.v); (err == nil) != c.ok {
			t.Errorf("%s %s: %v", c.s.Name, c.v, err)
		}
	}
}

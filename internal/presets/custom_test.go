package presets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCustomPresets(t *testing.T) {
	t.Setenv("JAILWICK_HOME", t.TempDir())
	if err := SaveCustom("acme", "Acme's services", []string{"api.acme.dev", ".acme-cdn.net"}); err != nil {
		t.Fatal(err)
	}
	p, ok := Get("acme")
	if !ok || p.Category != CustomCategory || !slices.Equal(p.Hosts, []string{"api.acme.dev", ".acme-cdn.net"}) {
		t.Fatalf("get: %+v %v", p, ok)
	}
	if err := Check([]string{"npm", "acme"}); err != nil {
		t.Fatal(err)
	}
	if last := Catalogue()[len(Catalogue())-1]; last.Name != "acme" {
		t.Errorf("custom presets come last: %s", last.Name)
	}
	if err := DeleteCustom("acme"); err != nil {
		t.Fatal(err)
	}
	if _, ok := Get("acme"); ok {
		t.Error("still there after delete")
	}
	if err := DeleteCustom("acme"); err == nil {
		t.Error("deleting a missing preset passes")
	}
}

func TestCustomPresetChecks(t *testing.T) {
	t.Setenv("JAILWICK_HOME", t.TempDir())
	for _, c := range []struct {
		name  string
		hosts []string
		want  string
	}{
		{"npm", []string{"x.dev"}, "built-in"},
		{"Bad Name", []string{"x.dev"}, "invalid preset name"},
		{"mine", nil, "no hosts"},
		{"mine", []string{"not a host"}, "not a domain"},
	} {
		if err := SaveCustom(c.name, "", c.hosts); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %v: %v, want %q", c.name, c.hosts, err, c.want)
		}
	}
	// A broken file is reported, and left out of the catalogue.
	dir := CustomDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("hosts: [nope]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Custom(); err == nil || !strings.Contains(err.Error(), "broken.yaml") {
		t.Errorf("broken file: %v", err)
	}
	if _, ok := Get("broken"); ok {
		t.Error("a broken preset is in the catalogue")
	}
}

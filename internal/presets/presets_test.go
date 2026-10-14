package presets

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// TestMain keeps the user's own presets out: the tests see the built-in catalogue only, unless
// they write presets of their own.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "jailwick-home-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("JAILWICK_HOME", dir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// The catalogue is data: these checks are its schema.
func TestCatalogueIsWellFormed(t *testing.T) {
	all := Catalogue()
	if len(all) < 30 {
		t.Fatalf("only %d presets", len(all))
	}
	always := 0
	for _, p := range all {
		where := "preset " + p.Name
		if p.Description == "" || !slices.Contains(Categories, p.Category) || len(p.Hosts) == 0 {
			t.Errorf("%s: needs a description, a known category and hosts: %+v", where, p)
		}
		if p.Docs != "" && !strings.HasPrefix(p.Docs, "https://") {
			t.Errorf("%s: docs must be an https link", where)
		}
		for _, h := range p.Hosts {
			if !hostRE.MatchString(h) {
				t.Errorf("%s: %q is not a host or .domain", where, h)
			}
			// verify probes example.com and expects it refused.
			if Covers(h, "example.com") {
				t.Errorf("%s: %q admits example.com", where, h)
			}
			for _, other := range p.Hosts {
				if other != h && Covers(h, strings.TrimPrefix(other, ".")) {
					t.Errorf("%s: %q already covers %q (the proxy refuses overlaps)", where, h, other)
				}
			}
		}
		if p.Always {
			always++
		}
		detects := len(p.Detect.Files)+len(p.Detect.Packages)+len(p.Detect.Remotes) > 0
		if p.Category == "docs" || p.Category == "editors" || p.Always {
			if detects {
				t.Errorf("%s: %s presets are chosen by hand, not detected", where, p.Category)
			}
		} else if !detects {
			t.Errorf("%s: needs detection rules", where)
		}
	}
	if always != 1 || !mustGet(t, "anthropic").Always {
		t.Errorf("only anthropic is always on")
	}
}

func mustGet(t *testing.T, name string) Preset {
	t.Helper()
	p, ok := Get(name)
	if !ok {
		t.Fatalf("no preset %q", name)
	}
	return p
}

func TestMatch(t *testing.T) {
	got := func(f Facts) []string {
		var names []string
		for _, p := range Match(f) {
			names = append(names, p.Name)
		}
		return names
	}
	cases := []struct {
		facts Facts
		want  []string
	}{
		{Facts{Files: []string{"package.json"}, Packages: []string{"@prisma/client", "@sentry/node"}}, []string{"npm", "prisma", "sentry"}},
		{Facts{Files: []string{"go.mod"}, Packages: []string{"github.com/stripe/stripe-go/v82"}, Remotes: []string{"github.com"}}, []string{"github", "go", "stripe"}},
		{Facts{Files: []string{"api.csproj", "Cargo.toml"}}, []string{"crates", "nuget"}},
		{Facts{Files: []string{"requirements-dev.txt"}, Packages: []string{"openai"}}, []string{"openai", "pypi"}},
		{Facts{Files: []string{"README.md"}}, nil},
	}
	for _, c := range cases {
		g := got(c.facts)
		slices.Sort(g)
		if !slices.Equal(g, c.want) {
			t.Errorf("Match(%+v) = %v, want %v", c.facts, g, c.want)
		}
	}
}

func TestCovering(t *testing.T) {
	var names []string
	for _, p := range Covering("objects.githubusercontent.com") {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"github"}) {
		t.Fatalf("Covering: %v", names)
	}
	if len(Covering("evil.example.org")) != 0 {
		t.Fatal("no preset covers an unknown host")
	}
	if err := Check([]string{"npm", "nope"}); err == nil {
		t.Fatal("unknown names are refused")
	}
}

func TestExplicitAndSplit(t *testing.T) {
	if got := Explicit([]string{"pypi", "npm", "npm", "nope"}); !slices.Equal(got, []string{"anthropic", "npm", "pypi"}) {
		t.Fatalf("Explicit: %v", got)
	}
	if got := Explicit(nil); !slices.Equal(got, []string{"anthropic"}) {
		t.Fatalf("Explicit(nil) must not be empty: %v", got)
	}
	narrow, broad := Split([]string{"npm", "github", "sentry"})
	if !slices.Equal(narrow, []string{"npm"}) || !slices.Equal(broad, []string{"github", "sentry"}) {
		t.Fatalf("Split: %v %v", narrow, broad)
	}
}

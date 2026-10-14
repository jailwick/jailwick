package proxy

import (
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/presets"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"https://API.Stripe.com/v1/x": "api.stripe.com",
		"api.stripe.com:443":          "api.stripe.com",
		"*.stripe.com":                ".stripe.com",
		".stripe.com":                 ".stripe.com",
	} {
		if got, err := Normalize(in); err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "localhost", "10.0.0.1", "a b.com"} {
		if _, err := Normalize(bad); err == nil {
			t.Errorf("Normalize(%q) should fail", bad)
		}
	}
}

func TestMinimize(t *testing.T) {
	got := Minimize([]string{"api.github.com", ".github.com", "github.com", "a.example.com", ".example.com", ".x.example.com", "z.io", "z.io"})
	want := []string{".example.com", ".github.com", "z.io"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestListPresets(t *testing.T) {
	l, err := List(Sources{Presets: []string{"npm"}, Project: []string{"API.stripe.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(l, "api.anthropic.com") || !slices.Contains(l, "registry.npmjs.org") || !slices.Contains(l, "api.stripe.com") {
		t.Fatalf("missing entries: %v", l)
	}
	if slices.Contains(l, "pypi.org") {
		t.Fatal("unselected preset included")
	}
	if _, err := List(Sources{Presets: []string{"nope"}}); err == nil {
		t.Fatal("want unknown preset error")
	}
	// Without presets, only the always-on ones: the agent can reach its API, nothing else.
	bare, err := List(Sources{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(bare, "api.anthropic.com") || slices.Contains(bare, "registry.npmjs.org") {
		t.Fatalf("allowlist without presets: %v", bare)
	}
}

func TestParseDenied(t *testing.T) {
	log := `1760000000.100      0 172.18.0.3 TCP_DENIED/403 3893 CONNECT api.stripe.com:443 - HIER_NONE/- text/html
1760000100.100      0 172.18.0.3 TCP_DENIED/403 3893 GET http://foo.dev/x - HIER_NONE/- text/html
1760000200.100      0 172.18.0.3 TCP_TUNNEL/200 3893 CONNECT api.anthropic.com:443 - HIER_DIRECT/1.2.3.4 -
1760000300.100      0 172.18.0.3 TCP_DENIED/403 3893 CONNECT api.stripe.com:443 - HIER_NONE/- text/html
1760000300.100      0 172.18.0.3 TCP_DENIED/403 3893 CONNECT example.com:443 - HIER_NONE/- text/html`
	acc := map[string]*Denied{}
	ParseDenied(strings.NewReader(log), "api", acc)
	ParseDenied(strings.NewReader(log), "web", acc)
	got := SortDenied(acc)
	if len(got) != 2 || got[0].Host != "api.stripe.com" || got[0].Count != 4 || len(got[0].Projects) != 2 {
		t.Fatalf("unexpected: %+v", got)
	}
}

// The sandbox picks the hosts it tries: what isn't a host name is left out of the list.
func TestParseDeniedIgnoresCraftedHosts(t *testing.T) {
	log := "1760000000.1 0 172.18.0.3 TCP_DENIED/403 3893 CONNECT evil\x1b]52;c;eA==\x07.com:443 - HIER_NONE/- text/html\n" +
		"1760000000.1 0 172.18.0.3 TCP_DENIED/403 3893 CONNECT [2001:db8::1]:443 - HIER_NONE/- text/html\n" +
		"1760000000.1 0 172.18.0.3 TCP_DENIED/403 3893 CONNECT Cdn.Example.org:443 - HIER_NONE/- text/html"
	acc := map[string]*Denied{}
	ParseDenied(strings.NewReader(log), "api", acc)
	got := SortDenied(acc)
	hosts := []string{}
	for _, d := range got {
		hosts = append(hosts, d.Host)
	}
	slices.Sort(hosts)
	if !slices.Equal(hosts, []string{"[2001:db8::1]", "cdn.example.org"}) {
		t.Fatalf("%q", hosts)
	}
}

// Claude in Chrome's relay is allowed only for a session that opted in, never through a preset.
func TestChromeHost(t *testing.T) {
	without, err := List(Sources{})
	if err != nil {
		t.Fatal(err)
	}
	with, err := List(Sources{Chrome: true})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(without, ChromeHost) || !slices.Contains(with, ChromeHost) {
		t.Fatalf("without %v, with %v", without, with)
	}
	for _, p := range presets.Catalogue() {
		for _, h := range p.Hosts {
			if Covers(h, ChromeHost) {
				t.Errorf("preset %s allows %s", p.Name, ChromeHost)
			}
		}
	}
}

func TestParseList(t *testing.T) {
	got, err := ParseList("api.acme.dev, .acme-cdn.net\nhttps://Docs.Acme.dev/x\n\napi.acme.dev")
	if err != nil || !slices.Equal(got, []string{"api.acme.dev", ".acme-cdn.net", "docs.acme.dev"}) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := ParseList("ok.dev, not a host"); err == nil || !strings.Contains(err.Error(), "not, a, host") {
		t.Errorf("bad entries: %v", err)
	}
	if _, err := ParseList(" , \n"); err == nil {
		t.Error("an empty list passes")
	}
}

func TestGroups(t *testing.T) {
	gs, err := Groups(Sources{Presets: []string{"npm"}, User: []string{"a.dev"}, Project: []string{"b.dev"}, Chrome: true})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range gs {
		got = append(got, g.Kind+":"+g.Name)
	}
	if strings.Join(got, " ") != "preset:anthropic preset:npm user: project: chrome:" {
		t.Fatalf("groups: %v", got)
	}
	if _, err := Groups(Sources{Presets: []string{"nope"}}); err == nil {
		t.Error("an unknown preset passes")
	}
}

package detect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func repoWith(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPresetsFromRepo(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"node with prisma and sentry", map[string]string{
			"package.json": `{"dependencies": {"@prisma/client": "^5"}, "devDependencies": {"@sentry/node": "8"}}`,
		}, []string{"npm", "prisma", "sentry"}},
		{"go with stripe, in a subfolder", map[string]string{
			"services/api/go.mod": "module x\n\ngo 1.23\n\nrequire (\n\tgithub.com/stripe/stripe-go/v82 v82.1.0 // indirect\n)\n",
		}, []string{"go", "stripe"}},
		{"rust", map[string]string{
			"Cargo.toml":          "[package]\nname = \"x\"\n\n[dependencies]\nserde = \"1\"\n",
			"rust-toolchain.toml": "[toolchain]\nchannel = \"1.80\"\n",
		}, []string{"crates", "rust"}},
		{"python with openai", map[string]string{
			"pyproject.toml": "[project]\nname = \"x\"\ndependencies = [\n  \"OpenAI>=1.0\",\n  \"httpx\",\n]\n",
		}, []string{"openai", "pypi"}},
		{"poetry with supabase", map[string]string{
			"pyproject.toml": "[tool.poetry.dependencies]\npython = \"^3.12\"\nsupabase = \"^2\"\n",
		}, []string{"pypi", "supabase"}},
		{"github remote", map[string]string{
			".git/config": "[remote \"origin\"]\n\turl = git@github.com:acme/shop.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n",
		}, []string{"github"}},
		{"nothing", map[string]string{"README.md": "# hi\n"}, nil},
	}
	for _, c := range cases {
		got := Repo(repoWith(t, c.files)).Presets
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: presets %v, want %v", c.name, got, c.want)
		}
	}
}

func TestManifestReaders(t *testing.T) {
	cases := []struct {
		read func([]byte) []string
		data string
		want []string
	}{
		{requirements, "# deps\nDjango==5.0 ; python_version>'3'\n-r base.txt\nrequests[socks]>=2\n", []string{"django", "requests"}},
		{gemfile, "source 'https://rubygems.org'\ngem 'rails', '~> 7'\n  gem \"stripe\"\n", []string{"rails", "stripe"}},
		{goMod, "module x\nrequire golang.org/x/net v0.30.0\n", []string{"golang.org/x/net"}},
		{composerJSON, `{"require": {"php": ">=8", "stripe/stripe-php": "^13"}}`, []string{"php", "stripe/stripe-php"}},
		{cargoToml, "[dev-dependencies]\ntokio = { version = \"1\" }\n[features]\nfull = []\n", []string{"tokio"}},
		{packageJSON, `not json`, nil},
	}
	for i, c := range cases {
		got := c.read([]byte(c.data))
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("case %d: %v, want %v", i, got, c.want)
		}
	}
}

func TestRemoteHost(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/acme/shop.git":  "github.com",
		"git@gitlab.com:acme/shop.git":      "gitlab.com",
		"ssh://git@bitbucket.org/acme/shop": "bitbucket.org",
		"/local/path/to/repo":               "",
	} {
		if got := remoteHost(in); got != want {
			t.Errorf("remoteHost(%q) = %q, want %q", in, got, want)
		}
	}
}

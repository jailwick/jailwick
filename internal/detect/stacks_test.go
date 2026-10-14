package detect

import (
	"maps"
	"testing"
)

func TestStacksFromRepo(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  map[string]string
	}{
		{"nothing", map[string]string{"README.md": "# hi\n"}, map[string]string{}},
		{"maven", map[string]string{"pom.xml": "<project/>"}, map[string]string{"java": ""}},
		{"gradle with a java version", map[string]string{"build.gradle.kts": "", ".java-version": "21.0.2\n"}, map[string]string{"java": "21"}},
		{"sdkman", map[string]string{"gradlew": "", ".sdkmanrc": "java=17.0.9-tem\n"}, map[string]string{"java": "17"}},
		{"java 8: the default", map[string]string{"pom.xml": "", ".java-version": "1.8\n"}, map[string]string{"java": ""}},
		{"tool-versions java and ruby", map[string]string{".tool-versions": "java temurin-21.0.1+12\nruby 3.3.0\n"}, map[string]string{"java": "21", "ruby": ""}},
		{"gemfile", map[string]string{"Gemfile": "source 'https://rubygems.org'\n", ".ruby-version": "3.3.0\n"}, map[string]string{"ruby": ""}},
		{"nvmrc", map[string]string{"package.json": `{}`, ".nvmrc": "v22.11.0\n"}, map[string]string{"node": "22.11.0"}},
		{"nvmrc codename", map[string]string{".nvmrc": "lts/iron\n"}, map[string]string{"node": "20"}},
		{"nvmrc lts", map[string]string{".nvmrc": "lts/*\n"}, map[string]string{"node": ""}},
		{"engines exact range", map[string]string{"package.json": `{"engines": {"node": "^20.10.0"}}`}, map[string]string{"node": "20"}},
		{"engines open range", map[string]string{"package.json": `{"engines": {"node": ">=18"}}`}, map[string]string{"node": ""}},
		{"engines too new for the default", map[string]string{"package.json": `{"engines": {"node": ">=26"}}`}, map[string]string{"node": "26"}},
		{"volta wins over engines", map[string]string{"package.json": `{"engines": {"node": "20"}, "volta": {"node": "22.1.0"}}`}, map[string]string{"node": "22.1.0"}},
		{"too old: the default", map[string]string{".node-version": "16\n"}, map[string]string{"node": ""}},
		{"bun", map[string]string{"package.json": `{"packageManager": "bun@1.2.3+sha256.abc"}`}, map[string]string{"node": "", "bun": "1.2.3"}},
		{"bun lockfile", map[string]string{"bun.lock": "{}"}, map[string]string{"bun": ""}},
		{"python file", map[string]string{".python-version": "3.12.4\n", "requirements.txt": "flask\n"}, map[string]string{"python": "3.12.4"}},
		{"requires-python open", map[string]string{"pyproject.toml": "[project]\nrequires-python = \">=3.11\"\n"}, map[string]string{"python": ""}},
		{"requires-python pinned", map[string]string{"pyproject.toml": "[project]\nrequires-python = \"~=3.11\"\n"}, map[string]string{"python": "3.11"}},
		{"poetry", map[string]string{"pyproject.toml": "[tool.poetry.dependencies]\npython = \"^3.12\"\n"}, map[string]string{"python": ""}},
		{"go toolchain wins", map[string]string{"go.mod": "module x\n\ngo 1.24\n\ntoolchain go1.25.3\n"}, map[string]string{"go": "1.25.3"}},
		{"go line", map[string]string{"go.mod": "module x\n\ngo 1.26.0\n"}, map[string]string{"go": "1.26.0"}},
		{"rust toml", map[string]string{"Cargo.toml": "[package]\n", "rust-toolchain.toml": "[toolchain]\nchannel = \"1.82\"\n"}, map[string]string{"rust": "1.82"}},
		{"rust plain", map[string]string{"rust-toolchain": "nightly-2026-01-15\n"}, map[string]string{"rust": "nightly-2026-01-15"}},
		{"tool-versions", map[string]string{".tool-versions": "nodejs 22.11.0\ngolang 1.25.3\npython 3.13.1\n"},
			map[string]string{"node": "22.11.0", "go": "1.25.3", "python": "3.13.1"}},
		{"monorepo: the root wins", map[string]string{
			".nvmrc": "22\n", "package.json": `{}`,
			"apps/web/.nvmrc": "20\n", "apps/web/package.json": `{}`,
			"services/api/go.mod": "module api\n\ngo 1.25.1\n",
		}, map[string]string{"node": "22", "go": "1.25.1"}},
	}
	for _, c := range cases {
		got := Repo(repoWith(t, c.files)).Stacks
		if !maps.Equal(got, c.want) {
			t.Errorf("%s: stacks %v, want %v", c.name, got, c.want)
		}
	}
}

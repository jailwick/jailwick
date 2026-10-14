package detect

import (
	"bufio"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Dependency names from package manifests, for preset detection. Each reader is small and
// forgiving: a manifest it can't read adds nothing.

// manifest returns the reader for a manifest file name, or nil.
func manifest(name string) func(data []byte) []string {
	switch {
	case name == "package.json":
		return packageJSON
	case name == "composer.json":
		return composerJSON
	case name == "go.mod":
		return goMod
	case name == "Cargo.toml":
		return cargoToml
	case name == "pyproject.toml":
		return pyproject
	case name == "Gemfile":
		return gemfile
	case strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt"):
		return requirements
	}
	return nil
}

type npmManifest struct {
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

func packageJSON(data []byte) []string {
	var m npmManifest
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	return keys(m.Dependencies, m.DevDependencies, m.PeerDependencies, m.OptionalDependencies)
}

// playwrightVersions are the exact @playwright/test versions a package.json pins.
func playwrightVersions(data []byte) []string {
	var m npmManifest
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	var out []string
	for _, deps := range []map[string]string{m.Dependencies, m.DevDependencies} {
		if v := strings.TrimLeft(deps["@playwright/test"], "^~="); semverRE.MatchString(v) {
			out = append(out, v)
		}
	}
	return out
}

func composerJSON(data []byte) []string {
	var m struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
	}
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	return keys(m.Require, m.RequireDev)
}

func keys(maps ...map[string]string) []string {
	var out []string
	for _, m := range maps {
		for k := range m {
			out = append(out, k)
		}
	}
	return out
}

// goMod reads the module paths of `require` lines and blocks.
func goMod(data []byte) []string {
	var out []string
	block := false
	for _, line := range lines(data) {
		switch {
		case line == "require (":
			block = true
		case block && line == ")":
			block = false
		case block:
			out = append(out, firstField(line))
		case strings.HasPrefix(line, "require "):
			out = append(out, firstField(strings.TrimPrefix(line, "require ")))
		}
	}
	return out
}

var tomlKeyRE = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*=`)

// cargoToml reads the crate names of the dependency tables.
func cargoToml(data []byte) []string {
	var out []string
	in := false
	for _, line := range lines(data) {
		if strings.HasPrefix(line, "[") {
			in = strings.HasSuffix(strings.Trim(line, "[]"), "dependencies")
			continue
		}
		if m := tomlKeyRE.FindStringSubmatch(line); in && m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

var (
	pep508RE = regexp.MustCompile(`^["']?([A-Za-z0-9][A-Za-z0-9._-]*)`)
	gemRE    = regexp.MustCompile(`^gem\s+["']([^"']+)["']`)
	pySepRE  = regexp.MustCompile(`[-_.]+`)
	semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// pyproject reads [project] dependencies (PEP 621) and Poetry's dependency tables.
func pyproject(data []byte) []string {
	var out []string
	inList, inPoetry := false, false
	for _, line := range lines(data) {
		switch {
		case strings.HasPrefix(line, "["):
			inList = false
			table := strings.Trim(line, "[]")
			inPoetry = strings.HasPrefix(table, "tool.poetry") && strings.HasSuffix(table, "dependencies")
		case strings.HasPrefix(line, "dependencies") && strings.Contains(line, "["):
			inList = !strings.Contains(line, "]")
			_, rest, _ := strings.Cut(line, "[")
			for _, item := range strings.Split(rest, ",") {
				out = appendPython(out, item)
			}
		case inList:
			inList = !strings.HasPrefix(line, "]")
			out = appendPython(out, line)
		case inPoetry:
			if m := tomlKeyRE.FindStringSubmatch(line); m != nil && m[1] != "python" {
				out = append(out, pythonName(m[1]))
			}
		}
	}
	return out
}

func requirements(data []byte) []string {
	var out []string
	for _, line := range lines(data) {
		if !strings.HasPrefix(line, "-") {
			out = appendPython(out, line)
		}
	}
	return out
}

func appendPython(out []string, item string) []string {
	if m := pep508RE.FindStringSubmatch(strings.TrimSpace(item)); m != nil {
		out = append(out, pythonName(m[1]))
	}
	return out
}

// pythonName normalizes a distribution name (PEP 503): lowercase, runs of -_. as one dash.
func pythonName(n string) string {
	return pySepRE.ReplaceAllString(strings.ToLower(n), "-")
}

func gemfile(data []byte) []string {
	var out []string
	for _, line := range lines(data) {
		if m := gemRE.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

// lines returns the trimmed lines without comments (#) or blanks.
func lines(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "#")
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func firstField(s string) string {
	f := strings.Fields(strings.Split(s, "//")[0])
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// gitRemotes returns the hosts of a repo's git remotes, from .git/config.
func gitRemotes(repo string) []string {
	data, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range lines(data) {
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "url" {
			continue
		}
		if h := remoteHost(strings.TrimSpace(value)); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// remoteHost is the host of a git remote: https://host/…, ssh://git@host/…, or git@host:….
func remoteHost(remote string) string {
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		return strings.ToLower(u.Hostname())
	}
	if _, rest, ok := strings.Cut(remote, "@"); ok {
		host, _, _ := strings.Cut(rest, ":")
		return strings.ToLower(host)
	}
	return ""
}

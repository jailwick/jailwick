// Package presets is the catalogue of allowlist presets: named groups of hosts a sandbox may reach
// (a package registry, a code host, a service), with what in a repo suggests each one. The
// catalogue is data, one file per preset in internal/assets/presets.
package presets

import (
	"bytes"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"

	"github.com/jailwick/jailwick/internal/assets"
)

// Categories, in the order they are shown; the user's own presets come last.
var Categories = []string{"agent", "code", "packages", "toolchains", "services", "docs", "editors", CustomCategory}

// Preset is one group of hosts.
type Preset struct {
	Name        string   `yaml:"-" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Category    string   `yaml:"category" json:"category"`
	Hosts       []string `yaml:"hosts" json:"hosts"` // ".example.com": the domain and its subdomains
	Detect      Detect   `yaml:"detect" json:"-"`
	// Broad says why the preset can receive data from anyone (uploads, events): an exfiltration
	// path. Detection suggests broad presets but doesn't choose them.
	Broad  string `yaml:"broad" json:"broad,omitempty"`
	Docs   string `yaml:"docs" json:"docs,omitempty"`
	Always bool   `yaml:"always" json:"always,omitempty"` // in every allowlist (the agent itself)
}

// Detect says what in a repo suggests a preset.
type Detect struct {
	Files    []string `yaml:"files"`    // file names or globs (*.csproj)
	Packages []string `yaml:"packages"` // dependency names; a trailing * matches a prefix
	Remotes  []string `yaml:"remotes"`  // hosts of git remotes
}

// Facts are what a repo shows (see detect.Repo).
type Facts struct {
	Files    []string // base names of the files found
	Packages []string // dependency names from the manifests found
	Remotes  []string // hosts of the git remotes
}

var builtins = sync.OnceValue(func() []Preset {
	var out []Preset
	for file, data := range assets.Dir("presets") {
		name, ok := strings.CutSuffix(file, ".yaml")
		if !ok {
			continue
		}
		var p Preset
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&p); err != nil {
			panic(fmt.Sprintf("presets: %s: %v", file, err)) // embedded data, checked by the tests
		}
		p.Name = name
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Preset) int {
		if c := slices.Index(Categories, a.Category) - slices.Index(Categories, b.Category); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
})

// catalogue is the built-in presets, then the user's valid ones (read each time, so a change is
// seen at once; they are a few small files).
func catalogue() []Preset {
	own, _ := Custom() // invalid files are reported by Custom's other callers
	return append(slices.Clip(builtins()), own...)
}

// Catalogue returns every preset, by category, then name: the built-in ones, then the user's.
func Catalogue() []Preset { return catalogue() }

// Get returns a preset by name.
func Get(name string) (Preset, bool) {
	list := catalogue()
	i := slices.IndexFunc(list, func(p Preset) bool { return p.Name == name })
	if i < 0 {
		return Preset{}, false
	}
	return list[i], true
}

// builtin returns a built-in preset by name.
func builtin(name string) (Preset, bool) {
	i := slices.IndexFunc(builtins(), func(p Preset) bool { return p.Name == name })
	if i < 0 {
		return Preset{}, false
	}
	return builtins()[i], true
}

// Names returns every preset name, sorted.
func Names() []string {
	var out []string
	for _, p := range catalogue() {
		out = append(out, p.Name)
	}
	slices.Sort(out)
	return out
}

// Check reports unknown preset names.
func Check(names []string) error {
	for _, n := range names {
		if _, ok := Get(n); !ok {
			return fmt.Errorf("unknown allowlist preset %q (jw proxy presets lists them)", n)
		}
	}
	return nil
}

// Covers reports whether an allowlist entry admits a host: ".example.com" admits example.com
// and its subdomains, any other entry only itself.
func Covers(entry, host string) bool {
	if strings.HasPrefix(entry, ".") {
		return host == entry[1:] || strings.HasSuffix(host, entry)
	}
	return host == entry
}

// Covering returns the presets that admit a host.
func Covering(host string) []Preset {
	var out []Preset
	for _, p := range catalogue() {
		if slices.ContainsFunc(p.Hosts, func(e string) bool { return Covers(e, host) }) {
			out = append(out, p)
		}
	}
	return out
}

// Match returns the presets a repo's facts suggest, in catalogue order. Presets that are always
// on are left out.
func Match(f Facts) []Preset {
	var out []Preset
	for _, p := range catalogue() {
		if !p.Always && p.Detect.matches(f) {
			out = append(out, p)
		}
	}
	return out
}

func (d Detect) matches(f Facts) bool {
	for _, pattern := range d.Files {
		if slices.ContainsFunc(f.Files, func(name string) bool { ok, _ := path.Match(pattern, name); return ok }) {
			return true
		}
	}
	for _, want := range d.Packages {
		prefix, wildcard := strings.CutSuffix(want, "*")
		if slices.ContainsFunc(f.Packages, func(have string) bool {
			return have == want || (wildcard && strings.HasPrefix(have, prefix))
		}) {
			return true
		}
	}
	for _, host := range d.Remotes {
		if slices.Contains(f.Remotes, host) {
			return true
		}
	}
	return false
}

// Explicit returns the presets to write in a project file: the always-on presets first, then
// the given ones in catalogue order, without duplicates or unknown names. It is never empty,
// so a written list can't read as "not chosen yet".
func Explicit(names []string) []string {
	var out []string
	for _, p := range catalogue() {
		if p.Always || slices.Contains(names, p.Name) {
			out = append(out, p.Name)
		}
	}
	return out
}

// Split separates the broad presets (they accept data from anyone) from the others.
func Split(names []string) (narrow, broad []string) {
	for _, n := range names {
		if p, ok := Get(n); ok && p.Broad != "" {
			broad = append(broad, n)
		} else {
			narrow = append(narrow, n)
		}
	}
	return narrow, broad
}

// BroadHint names the broad presets a repo calls for, which aren't chosen for it, in plain words.
func BroadHint(broad []string) string {
	if len(broad) == 1 {
		return broad[0] + " isn't ticked: it lets the sandbox send data anywhere on it. Tick it if the project needs it."
	}
	return strings.Join(broad[:len(broad)-1], ", ") + " and " + broad[len(broad)-1] +
		" aren't ticked: they let the sandbox send data anywhere on them. Tick them if the project needs them."
}

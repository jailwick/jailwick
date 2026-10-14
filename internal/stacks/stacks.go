// Package stacks is the catalogue of toolchains a project can have in its sandbox image: each is
// a layer of its own on jailwick's base, installed by a script in internal/assets/images/stacks,
// at a version the project chooses (detected from the repo).
package stacks

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Stack is one toolchain.
type Stack struct {
	Name        string
	Title       string // its name for people ("Node.js")
	Description string
	Default     string   // version when the project gives none
	Versions    []string // suggestions, newest first
	Presets     []string // allowlist presets it needs at runtime (package downloads)
	Tools       []string // commands verify expects in the sandbox
	Notes       string   // tips for Claude, after "Description (version)." in tools.md
	pattern     *regexp.Regexp
	minMajor    int // lowest major version accepted (0: any)
	// release is which dotted part of a version counts its releases (0 for node's 24, 1 for
	// python's 3.14 and go's 1.27); with it, a version past the newest one listed (Versions[0])
	// by more than one release is refused: it doesn't exist yet ("node 34"). -1: not checked.
	release int
}

// catalogue is in layer order: a change to an earlier stack rebuilds the later ones.
var catalogue = []Stack{
	{
		Name: "node", Title: "Node.js", Description: "Node.js with npm, pnpm and yarn (corepack), tsx and TypeScript",
		Default: "24", Versions: []string{"26", "24", "22", "20"},
		Presets: []string{"npm"}, Tools: []string{"node", "npm", "pnpm", "yarn", "tsx", "tsc"},
		Notes:   "Quick TS/JS: `tsx script.ts`.",
		pattern: regexp.MustCompile(`^\d+(\.\d+\.\d+)?$`), minMajor: 18, release: 0,
	},
	{
		Name: "python", Title: "Python", Description: "Python with uv",
		Default: "3.14", Versions: []string{"3.14", "3.13", "3.12", "3.11"},
		Presets: []string{"pypi"}, Tools: []string{"python3", "uv", "uvx"},
		Notes: "Quick Python with dependencies: `uv run --with <pkg> python script.py`; tools: " +
			"`uv tool install <pkg>`.",
		pattern: regexp.MustCompile(`^3\.\d+(\.\d+)?$`), release: 1,
	},
	{
		Name: "go", Title: "Go", Description: "Go",
		Default: "1.27", Versions: []string{"1.27", "1.26", "1.25"},
		Presets: []string{"go"}, Tools: []string{"go", "gofmt"},
		Notes: "`go install` puts binaries in ~/.local/bin; a newer toolchain named in go.mod is " +
			"downloaded through the proxy.",
		pattern: regexp.MustCompile(`^1\.\d+(\.\d+)?$`), release: 1,
	},
	{
		Name: "rust", Title: "Rust", Description: "Rust with rustup, cargo, rustfmt and clippy",
		Default: "stable", Versions: []string{"stable", "beta", "nightly"},
		Presets: []string{"crates", "rust"}, Tools: []string{"rustup", "cargo", "rustc"},
		Notes: "`cargo install` puts binaries in ~/.local/bin; a rust-toolchain.toml's toolchain is " +
			"installed on first use.",
		pattern: regexp.MustCompile(`^(stable|beta|nightly(-\d{4}-\d{2}-\d{2})?|1\.\d+(\.\d+)?)$`), release: -1,
	},
	{
		Name: "bun", Title: "Bun", Description: "Bun",
		Default: "1", Versions: []string{"1"},
		Presets: []string{"npm"}, Tools: []string{"bun", "bunx"},
		Notes:   "`bun add -g` puts binaries in ~/.local/bin.",
		pattern: regexp.MustCompile(`^1(\.\d+){0,2}$`), release: -1,
	},
	// Added after the first five: layers of projects using those stay as they were.
	{
		Name: "java", Title: "Java", Description: "Java: Eclipse Temurin's JDK (build with the project's mvnw or gradlew)",
		Default: "21", Versions: []string{"25", "21", "17"},
		Presets: []string{"maven"}, Tools: []string{"java", "javac", "jar"},
		Notes: "Build with the project's `./mvnw` or `./gradlew`; Maven and Gradle already know the proxy, and their " +
			"downloads stay in ~/.cache and ~/.gradle between sessions.",
		pattern: regexp.MustCompile(`^\d+$`), minMajor: 11, release: 0,
	},
	{
		Name: "ruby", Title: "Ruby", Description: "Ruby with bundler: the system's (3.1 on jailwick's base)",
		Default: "system", Versions: []string{"system"},
		Presets: []string{"rubygems"}, Tools: []string{"ruby", "gem", "bundle"},
		Notes: "Gems install into ~/.local/share/gem. Another Ruby version: a base image like ruby:3.3-bookworm " +
			"(the project's Settings, Image).",
		pattern: regexp.MustCompile(`^system$`), release: -1,
	},
}

// All returns the stacks in layer order.
func All() []Stack { return slices.Clone(catalogue) }

// Get returns a stack by name.
func Get(name string) (Stack, bool) {
	i := slices.IndexFunc(catalogue, func(s Stack) bool { return s.Name == name })
	if i < 0 {
		return Stack{}, false
	}
	return catalogue[i], true
}

// Names returns the stack names in layer order.
func Names() []string {
	out := make([]string, len(catalogue))
	for i, s := range catalogue {
		out[i] = s.Name
	}
	return out
}

// Version is the version to install: v, or the stack's default when v is empty.
func (s Stack) Version(v string) string {
	if v == "" {
		return s.Default
	}
	return v
}

// Check reports whether v ("" for the default) is a version this stack can install.
func (s Stack) Check(v string) error {
	if v == "" {
		return nil
	}
	if !s.pattern.MatchString(v) {
		return fmt.Errorf("%s: %q is not a version (e.g. %s)", s.Name, v, strings.Join(s.Versions, ", "))
	}
	if major, _ := strconv.Atoi(strings.SplitN(v, ".", 2)[0]); s.minMajor > 0 && major < s.minMajor {
		return fmt.Errorf("%s: %s is too old (%d or later)", s.Name, v, s.minMajor)
	}
	if s.release >= 0 && len(s.Versions) > 0 {
		newest, ok1 := releaseOf(s.Versions[0], s.release)
		got, ok2 := releaseOf(v, s.release)
		if ok1 && ok2 && got > newest+1 {
			return fmt.Errorf("%s: %s doesn't exist yet (the newest is %s)", s.Name, v, s.Versions[0])
		}
	}
	return nil
}

// releaseOf is the number of a version's dotted part (see Stack.release).
func releaseOf(v string, part int) (int, bool) {
	parts := strings.Split(v, ".")
	if part >= len(parts) {
		return 0, false
	}
	n, err := strconv.Atoi(parts[part])
	return n, err == nil
}

// Check reports unknown stacks and versions that can't be installed.
func Check(chosen map[string]string) error {
	for _, name := range slices.Sorted(maps.Keys(chosen)) {
		s, ok := Get(name)
		if !ok {
			return fmt.Errorf("unknown stack %q (stacks: %s)", name, strings.Join(Names(), ", "))
		}
		if err := s.Check(chosen[name]); err != nil {
			return err
		}
	}
	return nil
}

// Chosen returns the chosen stacks in layer order, each with the version to install.
func Chosen(chosen map[string]string) []Choice {
	var out []Choice
	for _, s := range catalogue {
		if v, ok := chosen[s.Name]; ok {
			out = append(out, Choice{Stack: s, Version: s.Version(v)})
		}
	}
	return out
}

// Choice is a stack at the version to install.
type Choice struct {
	Stack
	Version string
}

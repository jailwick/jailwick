package detect

import (
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Language is a language the repo uses that no stack installs, with ways to get it: a Debian or
// Ubuntu image that has it (Base), and the Debian packages that bring it to jailwick's base (Apt;
// none when Debian's are missing or too old to suggest).
type Language struct {
	Name string   `json:"name"`
	File string   `json:"file"` // the file that shows it
	Base string   `json:"base"`
	Apt  []string `json:"apt,omitempty"`
}

// languageRule finds a language by file name (globs allowed).
type languageRule struct {
	names []string
	lang  Language
}

// languages, in the order they are suggested.
var languages = []languageRule{
	{[]string{"composer.json"}, Language{Name: "PHP", Base: "php:8.3-cli-bookworm", Apt: []string{"php-cli", "composer"}}},
	{[]string{"mix.exs"}, Language{Name: "Elixir", Base: "elixir:1.17", Apt: []string{"elixir"}}},
	{[]string{"rebar.config"}, Language{Name: "Erlang", Base: "erlang:27", Apt: []string{"erlang"}}},
	{[]string{"*.csproj", "*.fsproj", "*.sln", "global.json"}, Language{Name: ".NET", Base: "mcr.microsoft.com/dotnet/sdk:8.0"}},
	{[]string{"Package.swift"}, Language{Name: "Swift", Base: "swift:6.0-jammy"}},
	{[]string{"pubspec.yaml"}, Language{Name: "Dart", Base: "dart:stable"}},
	{[]string{"stack.yaml", "cabal.project", "*.cabal"}, Language{Name: "Haskell", Base: "haskell:9.8"}},
	{[]string{"CMakeLists.txt"}, Language{Name: "C/C++ with CMake", Apt: []string{"cmake"}}},
}

// addLanguage notes a language a file of the repo shows, once per language.
func (s *Suggestion) addLanguage(name string) {
	for _, l := range languages {
		if !slices.ContainsFunc(l.names, func(p string) bool { ok, _ := path.Match(p, name); return ok }) {
			continue
		}
		if !slices.ContainsFunc(s.Languages, func(x Language) bool { return x.Name == l.lang.Name }) {
			found := l.lang
			found.File = name
			s.Languages = append(s.Languages, found)
			// In the catalogue's order, whatever order the files came in: the first suggests the base.
			slices.SortStableFunc(s.Languages, func(a, b Language) int { return languageRank(a) - languageRank(b) })
		}
		return
	}
}

func languageRank(l Language) int {
	return slices.IndexFunc(languages, func(x languageRule) bool { return x.lang.Name == l.Name })
}

// Languages finds just the languages a repo uses that no stack installs (see Repo, which finds
// them with everything else): by file name, to the same depth, skipping the same folders.
func Languages(dir string) []Language {
	var s Suggestion
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			if rel != "." && (skipDir(d.Name()) || strings.Count(rel, string(filepath.Separator)) >= 3) {
				return filepath.SkipDir
			}
			return nil
		}
		s.addLanguage(d.Name())
		return nil
	})
	return s.Languages
}

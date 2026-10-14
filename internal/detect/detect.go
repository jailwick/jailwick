// Package detect inspects a repo to suggest project settings: the services its env files point
// at on localhost, git-ignored env files worth copying into worktrees, its toolchains (stacks) and
// their versions, Playwright versions, and the allowlist presets its files, dependencies and git
// remotes call for.
// Example env files (.env.example, .env.sample, .env.template) count too, so a fresh clone without
// a .env still gets its services; values from real env files win.
package detect

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/presets"
)

// Suggestion is what detection found.
type Suggestion struct {
	Services []string
	Forwards []string // localport:service:port, for each localhost URL that names a port
	EnvFiles []string // git-ignored env files (relative), candidates for worktree.copy_files
	// EnvCandidates are all the env files found (relative), examples included.
	EnvCandidates []string
	// LoginsFrom is the env file (relative) the database logins came from, "" when none did.
	LoginsFrom string
	Logins
	// Stacks the repo uses, with the version it names ("" for the stack's default).
	Stacks map[string]string
	// Playwright versions from package.json files (@playwright/test).
	Playwright []string
	Android    bool
	// Presets are the allowlist presets the repo calls for (see presets.Match), broad ones included.
	Presets []string
	// Languages the repo uses that no stack installs, with ways to get them (see Language).
	Languages []Language
	Facts     presets.Facts
}

var (
	urlRE       = regexp.MustCompile(`(?i)\b(postgres(?:ql)?|redis|rediss|mongodb(?:\+srv)?)://[^\s"'` + "`" + `]+`)
	defaultPort = map[string]int{"postgres": 5432, "redis": 6379, "mongo": 27017}
)

// Repo scans a repo folder (depth 3, skipping dependencies and VCS folders).
func Repo(dir string) Suggestion {
	var s Suggestion
	var examples []string
	found := stackFindings{}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			if rel != "." && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			if strings.Count(rel, string(filepath.Separator)) >= 3 {
				return filepath.SkipDir
			}
			if d.Name() == "android" {
				s.Android = true
			}
			return nil
		}
		name := d.Name()
		s.Facts.Files = append(s.Facts.Files, name)
		s.addLanguage(name)
		found.add(name, strings.Count(rel, string(filepath.Separator)), func() ([]byte, bool) {
			data, err := os.ReadFile(path)
			return data, err == nil
		})
		if read := manifest(name); read != nil {
			if data, err := os.ReadFile(path); err == nil {
				s.Facts.Packages = append(s.Facts.Packages, read(data)...)
				if name == "package.json" {
					s.Playwright = append(s.Playwright, playwrightVersions(data)...)
				}
			}
		}
		switch {
		case strings.HasPrefix(name, ".env") && isExample(name):
			examples = append(examples, rel)
		case strings.HasPrefix(name, ".env"):
			s.scanEnv(dir, rel)
			if ignored(dir, rel) {
				s.EnvFiles = append(s.EnvFiles, rel)
			}
		}
		return nil
	})
	// After the real env files, so their logins win.
	for _, rel := range examples {
		s.scanEnv(dir, rel)
	}
	slices.Sort(s.Services)
	s.Services = slices.Compact(s.Services)
	slices.Sort(s.Playwright)
	s.Playwright = slices.Compact(s.Playwright)
	slices.Sort(s.EnvFiles)
	s.Stacks = found.chosen()
	s.Facts.Remotes = gitRemotes(dir)
	for _, p := range presets.Match(s.Facts) {
		s.Presets = append(s.Presets, p.Name)
	}
	return s
}

// scanEnv adds what an env file says. The first file to give a service's login wins.
func (s *Suggestion) scanEnv(dir, rel string) {
	s.EnvCandidates = append(s.EnvCandidates, rel)
	e, err := ReadEnv(filepath.Join(dir, rel))
	if err != nil {
		return
	}
	s.Services = append(s.Services, e.Services...)
	for _, f := range e.Forwards {
		if !slices.Contains(s.Forwards, f) {
			s.Forwards = append(s.Forwards, f)
		}
	}
	before := s.Logins
	if s.Postgres.User == "" && s.Postgres.DB == "" {
		s.Postgres = e.Postgres
	}
	if s.RedisPass == "" {
		s.RedisPass = e.RedisPass
	}
	if s.MongoUser == "" {
		s.MongoUser, s.MongoPass = e.MongoUser, e.MongoPass
	}
	if s.LoginsFrom == "" && s.Logins != before {
		s.LoginsFrom = rel
	}
}

func isExample(name string) bool {
	for _, w := range []string{"example", "sample", "template"} {
		if strings.Contains(name, w) {
			return true
		}
	}
	return false
}

func isLocal(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }

func service(scheme string) string {
	switch {
	case strings.HasPrefix(scheme, "postgres"):
		return "postgres"
	case strings.HasPrefix(scheme, "redis"):
		return "redis"
	}
	return "mongo"
}

func host(svc string) string {
	if svc == "mongo" {
		return "mongodb"
	}
	return svc
}

func ignored(repo, rel string) bool {
	return exec.Command("git", "-C", repo, "check-ignore", "-q", rel).Run() == nil
}

// skipDir reports whether a folder holds dependencies, builds or VCS data rather than the repo's own
// files.
func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", ".venv", "venv", "vendor", "dist", "build", ".next", "target", ".claude":
		return true
	}
	return false
}

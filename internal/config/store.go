package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/jailwick/jailwick/internal/paths"
)

// ErrNotFound is returned when a project or workspace file doesn't exist.
var ErrNotFound = errors.New("not found")

// Store reads and writes the configuration folder.
type Store struct{ Dir string }

// Open returns the store at the configured folder ($JAILWICK_HOME or ~/.config/jailwick).
func Open() *Store { return &Store{Dir: paths.ConfigDir()} }

func (s *Store) GlobalFile() string { return filepath.Join(s.Dir, "config.yaml") }
func (s *Store) ProjectFile(name string) string {
	return filepath.Join(s.Dir, "projects", name+".yaml")
}
func (s *Store) WorkspaceFile(name string) string {
	return filepath.Join(s.Dir, "workspaces", name+".yaml")
}

// ProjectNotesDir holds notes.md (and optional settings.json / mcp.json) for one project.
func (s *Store) ProjectNotesDir(name string) string {
	return filepath.Join(s.Dir, "notes", "projects", name)
}

// WorkspaceNotesDir holds notes.md for one workspace.
func (s *Store) WorkspaceNotesDir(name string) string {
	return filepath.Join(s.Dir, "notes", "workspaces", name)
}

// ClaudeDir holds the user's additions to the shared sandbox instructions (CLAUDE.md) and the
// Claude settings layers shared by projects.
func (s *Store) ClaudeDir() string { return filepath.Join(s.Dir, "claude") }

// ClaudeSettingsFile is the app-wide Claude settings layer, applied to every project.
func (s *Store) ClaudeSettingsFile() string { return filepath.Join(s.ClaudeDir(), "settings.json") }

// ProfilesDir holds the named Claude settings layers that projects pick with claude.profiles.
func (s *Store) ProfilesDir() string { return filepath.Join(s.ClaudeDir(), "profiles") }

// ProfileFile is one named Claude settings layer.
func (s *Store) ProfileFile(name string) string {
	return filepath.Join(s.ProfilesDir(), name+".json")
}

// ProjectSettingsFile is a project's own Claude settings layer, applied last.
func (s *Store) ProjectSettingsFile(name string) string {
	return filepath.Join(s.ProjectNotesDir(name), "settings.json")
}

// ProjectsUsingPreset lists the projects whose allow_presets name a preset. Unreadable project
// files are skipped (jw doctor reports them).
func (s *Store) ProjectsUsingPreset(name string) []string {
	ps, _ := s.Projects()
	var out []string
	for _, p := range ps {
		if slices.Contains(p.AllowPresets, name) {
			out = append(out, p.Name)
		}
	}
	return out
}

// Profiles lists the named Claude settings layers, sorted.
func (s *Store) Profiles() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(s.ProfilesDir(), "*.json"))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	slices.Sort(names)
	return names, nil
}

// AllowFile is the user's own allowlist, added to every sandbox.
func (s *Store) AllowFile() string { return filepath.Join(s.Dir, "allowlist.txt") }

// Global loads config.yaml; a missing file gives the defaults.
func (s *Store) Global() (Global, error) {
	var g Global
	err := readYAML(s.GlobalFile(), &g)
	if errors.Is(err, ErrNotFound) {
		return Global{Version: Version}, nil
	}
	if err != nil {
		return g, err
	}
	return g, g.Validate()
}

func (s *Store) SaveGlobal(g Global) error {
	g.Version = Version
	return writeYAML(s.GlobalFile(), g)
}

// Project loads one project file.
func (s *Store) Project(name string) (Project, error) {
	if err := ValidName(name); err != nil {
		return Project{}, err
	}
	var p Project
	if err := readYAML(s.ProjectFile(name), &p); err != nil {
		if errors.Is(err, ErrNotFound) {
			return p, fmt.Errorf("project %q: %w", name, ErrNotFound)
		}
		return p, err
	}
	p.Name = name
	p.Dir = paths.Expand(p.Dir)
	return p, nil
}

// Projects loads every project, sorted by name. Files that fail to load are returned in errs.
func (s *Store) Projects() (ps []Project, errs []error) {
	for _, name := range s.names("projects") {
		p, err := s.Project(name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		ps = append(ps, p)
	}
	return ps, errs
}

// SaveProject validates and writes a project file.
func (s *Store) SaveProject(p Project) error {
	if err := p.Validate(); err != nil {
		return err
	}
	p.Version = Version
	p.Dir = paths.Tilde(p.Dir)
	return writeYAML(s.ProjectFile(p.Name), p)
}

func (s *Store) DeleteProject(name string) error {
	err := os.Remove(s.ProjectFile(name))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("project %q: %w", name, ErrNotFound)
	}
	return err
}

// Workspace loads one workspace file.
func (s *Store) Workspace(name string) (Workspace, error) {
	if err := ValidName(name); err != nil {
		return Workspace{}, err
	}
	var w Workspace
	if err := readYAML(s.WorkspaceFile(name), &w); err != nil {
		if errors.Is(err, ErrNotFound) {
			return w, fmt.Errorf("workspace %q: %w", name, ErrNotFound)
		}
		return w, err
	}
	w.Name = name
	w.Root = paths.Expand(w.Root)
	return w, nil
}

// Workspaces lists workspace names.
func (s *Store) Workspaces() []string { return s.names("workspaces") }

func (s *Store) SaveWorkspace(w Workspace) error {
	if err := w.Validate(); err != nil {
		return err
	}
	w.Version = Version
	w.Root = paths.Tilde(w.Root)
	return writeYAML(s.WorkspaceFile(w.Name), w)
}

func (s *Store) names(kind string) []string {
	entries, _ := os.ReadDir(filepath.Join(s.Dir, kind))
	var out []string
	for _, e := range entries {
		if n, ok := strings.CutSuffix(e.Name(), ".yaml"); ok && !e.IsDir() && ValidName(n) == nil {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// readYAML decodes a file strictly: unknown keys are errors, so typos don't go unnoticed.
func readYAML(file string, v any) error {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: %w", paths.Tilde(file), err)
	}
	if ver := versionOf(v); ver > Version {
		return fmt.Errorf("%s: written by a newer jailwick (format %d); upgrade jailwick", paths.Tilde(file), ver)
	}
	return nil
}

func versionOf(v any) int {
	switch c := v.(type) {
	case *Global:
		return c.Version
	case *Project:
		return c.Version
	case *Workspace:
		return c.Version
	}
	return 0
}

// Marshal renders a configuration value as YAML with two-space indentation.
func Marshal(v any) ([]byte, error) {
	switch c := v.(type) {
	case Project:
		c.Version = Version
		c.Dir = paths.Tilde(c.Dir)
		v = c
	case Workspace:
		c.Version = Version
		c.Root = paths.Tilde(c.Root)
		v = c
	case Global:
		c.Version = Version
		v = c
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeYAML writes atomically (temp file + rename) so a crash never leaves half a file.
func writeYAML(file string, v any) error {
	data, err := Marshal(v)
	if err != nil {
		return err
	}
	return WriteFileAtomic(file, data, 0o644)
}

// WriteFileAtomic writes data to file via a temp file in the same folder.
func WriteFileAtomic(file string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

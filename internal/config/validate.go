package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/presets"
	"github.com/jailwick/jailwick/internal/stacks"
)

// Browsers Playwright can install.
var Browsers = []string{"chromium", "firefox", "webkit"}

// They go into the image's Dockerfile as written.
var (
	aptRE    = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*(=[A-Za-z0-9.+~:-]+)?$`)
	semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
)

// validateLayer checks what jailwick's layer adds to the image.
func validateLayer(img Image, pr *Problems) {
	if err := stacks.Check(img.Stacks); err != nil {
		pr.add("image.stacks: %v", err)
	}
	for _, b := range img.Playwright.Browsers {
		if !slices.Contains(Browsers, b) {
			pr.add("image.playwright.browsers: unknown browser %q (%s)", b, strings.Join(Browsers, ", "))
		}
	}
	for _, v := range img.Playwright.Versions {
		if !semverRE.MatchString(v) {
			pr.add("image.playwright.versions: %q is not a version (like 1.50.1)", v)
		}
	}
	for _, a := range img.AptPackages {
		if !aptRE.MatchString(a) {
			pr.add("image.apt_packages: %q is not a package name (optionally =version)", a)
		}
	}
	if len(img.Playwright.Versions) > 0 && len(img.Playwright.Browsers) == 0 {
		pr.add("image.playwright.versions: needs browsers (without browsers there is no Playwright)")
	}
	adds := len(img.Stacks) > 0 || len(img.Playwright.Browsers) > 0 || len(img.AptPackages) > 0 || img.Android
	if img.Name != "" && adds {
		pr.add("image.name is used as is, so stacks, playwright, apt_packages and android would be ignored: " +
			"use image.base to build jailwick's layer on it")
	}
}

var (
	nameRE    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	forwardRE = regexp.MustCompile(`^(\d{1,5}):([a-z0-9][a-z0-9.-]*):(\d{1,5})$`)
)

// ValidName checks a project or workspace name. "--" is reserved: it separates a project from a
// session in runtime names (jailwick-<project>--<session>).
// NameFrom derives a name from a folder's: lowercase, with every run of other characters (dashes
// included) turned into one dash; fallback when nothing is left.
func NameFrom(dir, fallback string) string {
	n := strings.Trim(nonName.ReplaceAllString(strings.ToLower(filepath.Base(dir)), "-"), "-")
	if len(n) > 40 {
		n = strings.Trim(n[:40], "-")
	}
	if n == "" {
		n = fallback
	}
	return n
}

var nonName = regexp.MustCompile(`[^a-z0-9]+`)

func ValidName(n string) error {
	if !nameRE.MatchString(n) || len(n) > 40 || strings.Contains(n, "--") {
		return fmt.Errorf("invalid name %q: use lowercase letters, digits and single dashes (max 40)", n)
	}
	return nil
}

// Problems collects validation errors for one file.
type Problems struct {
	What string
	List []string
}

func (p *Problems) add(format string, a ...any) { p.List = append(p.List, fmt.Sprintf(format, a...)) }

func (p *Problems) err() error {
	if len(p.List) == 0 {
		return nil
	}
	return p
}

func (p *Problems) Error() string {
	if len(p.List) == 1 {
		return p.What + ": " + p.List[0]
	}
	return p.What + ":\n  - " + strings.Join(p.List, "\n  - ")
}

// Validate checks a project before anything is generated or started.
func (p Project) Validate() error {
	pr := &Problems{What: "project " + p.Name}
	if err := ValidName(p.Name); err != nil {
		pr.add("%v", err)
	}
	switch {
	case p.Dir == "":
		pr.add("dir: required (the repo folder)")
	default:
		if err := CheckMountable(p.Dir); err != nil {
			pr.add("dir: %v", err)
		}
	}
	for _, s := range p.Services {
		if !slices.Contains(ServiceNames, s) {
			pr.add("services: unknown %q (use %s)", s, strings.Join(ServiceNames, ", "))
		}
	}
	for _, s := range p.TestServices {
		if !slices.Contains(ServiceNames, s) {
			pr.add("test_services: unknown %q (use %s)", s, strings.Join(ServiceNames, ", "))
		}
	}
	for _, f := range p.LocalForwards {
		if _, _, _, err := ParseForward(f); err != nil {
			pr.add("local_forwards: %v", err)
		}
	}
	if p.Workspace != "" {
		if err := ValidName(p.Workspace); err != nil {
			pr.add("workspace: %v", err)
		}
	} else if len(p.WorkspaceRelated) > 0 {
		pr.add("workspace_related: set without workspace")
	}
	validateImage(p.Image, pr)
	if df := p.DockerfilePath(); df != "" {
		if _, err := os.Stat(df); err != nil {
			pr.add("image.dockerfile: %s isn't there (a path in the repo, or an absolute one)", p.Image.Dockerfile)
		}
	}
	validateLayer(p.Image, pr)
	for _, f := range p.Worktree.CopyFiles {
		if filepath.IsAbs(f) || slices.Contains(strings.Split(filepath.ToSlash(f), "/"), "..") {
			pr.add("worktree.copy_files: %q must be a path inside the repo", f)
		}
	}
	for _, d := range p.NodeModules {
		_, globErr := filepath.Match(d, "")
		parts := strings.Split(filepath.ToSlash(d), "/")
		if d == "" || filepath.IsAbs(d) || slices.Contains(parts, "..") || globErr != nil {
			pr.add("node_modules: %q must be a folder (or a pattern like apps/*) inside the repo", d)
		}
	}
	for _, e := range p.Postgres.ExtraDBs {
		if e.Name == "" {
			pr.add("postgres.extra_dbs: entry without a name")
		}
	}
	if err := presets.Check(p.AllowPresets); err != nil {
		pr.add("allow_presets: %v", err)
	}
	for _, h := range p.Blocked {
		if h == "" || strings.ContainsAny(h, " \t/") {
			pr.add("blocked: %q must be a host name, like tracker.example.com", h)
		}
	}
	validateClaude(p.Claude, pr)
	return pr.err()
}

// Validate checks the global defaults that can't wait until a project uses them.
func (g Global) Validate() error {
	pr := &Problems{What: "config.yaml"}
	validateClaude(g.Claude, pr)
	validateImage(Image{Base: g.Image.Base, Dockerfile: g.Image.Dockerfile, Name: g.Image.Name, User: g.Image.User,
		Home: g.Image.Home, Platform: g.Image.Platform}, pr)
	if g.SessionNotifications != "" && !slices.Contains(SessionNotifyModes, g.SessionNotifications) {
		pr.add("session_notifications: %q is not one of %s", g.SessionNotifications, strings.Join(SessionNotifyModes, ", "))
	}
	return pr.err()
}

// validateClaude checks the claude settings shared by projects and the global defaults.
func validateClaude(c Claude, pr *Problems) {
	if c.Login != "" && !slices.Contains(Logins, c.Login) {
		pr.add("claude.login: unknown %q (use %s)", c.Login, strings.Join(Logins, " or "))
	}
	for i, n := range c.Profiles {
		if err := ValidName(n); err != nil {
			pr.add("claude.profiles: %v", err)
		} else if slices.Contains(c.Profiles[:i], n) {
			pr.add("claude.profiles: %q is listed twice", n)
		}
	}
}

// Validate checks a workspace file (paths are checked when a sandbox is generated).
func (w Workspace) Validate() error {
	pr := &Problems{What: "workspace " + w.Name}
	if err := ValidName(w.Name); err != nil {
		pr.add("%v", err)
	}
	if w.Root == "" {
		pr.add("root: required (the folder that contains the repos)")
	}
	seen := map[string]bool{}
	for _, m := range w.Members {
		if m.Name == "" || strings.Contains(m.Name, "..") || filepath.IsAbs(m.Name) {
			pr.add("members: invalid name %q", m.Name)
		}
		if seen[m.Name] {
			pr.add("members: %q listed twice", m.Name)
		}
		seen[m.Name] = true
	}
	return pr.err()
}

// ParseForward splits "localport:host:port".
func ParseForward(f string) (local int, host string, port int, err error) {
	m := forwardRE.FindStringSubmatch(f)
	if m == nil {
		return 0, "", 0, fmt.Errorf("%q: use localport:service:port, e.g. 5432:postgres:5432", f)
	}
	local, _ = strconv.Atoi(m[1])
	port, _ = strconv.Atoi(m[3])
	if local < 1 || local > 65535 || port < 1 || port > 65535 {
		return 0, "", 0, fmt.Errorf("%q: port out of range", f)
	}
	return local, m[2], port, nil
}

// CheckMountable refuses folders too broad to give a sandbox: /, the home folder, its parents,
// and system or personal folders. The path must exist and be a directory.
func CheckMountable(dir string) error {
	real := paths.Real(dir)
	st, err := os.Stat(real)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: no such folder", dir)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a folder", dir)
	}
	if IsBroadPath(real) {
		return fmt.Errorf("refusing to mount %s: too broad, pick the repo folder itself", real)
	}
	return nil
}

// IsBroadPath reports paths that must never be mounted into a sandbox.
func IsBroadPath(real string) bool {
	home := paths.Real(paths.Home())
	if real == "/" || real == home || paths.Within(home, real) {
		return true
	}
	// System folders and everything in them.
	for _, p := range []string{"/etc", "/private/etc", "/System", "/Library", "/usr", "/bin", "/sbin", "/proc", "/sys", "/dev", "/boot", "/root"} {
		if paths.Within(real, p) {
			return true
		}
	}
	// Folders that hold many unrelated things (their subfolders are fine).
	for _, p := range []string{"/Users", "/home", "/Volumes", "/private", "/private/var", "/var", "/tmp", "/private/tmp", "/opt", "/mnt", "/srv"} {
		if real == p {
			return true
		}
	}
	for _, p := range []string{"Desktop", "Documents", "Downloads", "projects", "src", "code", "dev"} {
		if real == filepath.Join(home, p) {
			return true
		}
	}
	for _, p := range []string{"Library", ".ssh", ".claude", ".config", ".gnupg", ".aws", ".docker", ".kube"} {
		if paths.Within(real, filepath.Join(home, p)) {
			return true
		}
	}
	return false
}

// UnmarshalYAML accepts "name" or {name, user, password}.
func (e *ExtraDB) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		e.Name = n.Value
		return nil
	}
	type plain ExtraDB
	return decodeStrict(n, (*plain)(e))
}

// UnmarshalYAML accepts "name" or {name, path}.
func (m *Member) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		m.Name = n.Value
		return nil
	}
	type plain Member
	return decodeStrict(n, (*plain)(m))
}

// MarshalYAML writes the short form when only the name is set.
func (e ExtraDB) MarshalYAML() (any, error) {
	if e.User == "" && e.Password == "" {
		return e.Name, nil
	}
	type plain ExtraDB
	return plain(e), nil
}

func (m Member) MarshalYAML() (any, error) {
	if m.Path == "" {
		return m.Name, nil
	}
	type plain Member
	return plain(m), nil
}

func decodeStrict(n *yaml.Node, v any) error {
	if n.Kind != yaml.MappingNode {
		return errors.New("expected a name or a mapping")
	}
	known := map[string]bool{}
	for _, f := range yamlKeys(v) {
		known[f] = true
	}
	for i := 0; i < len(n.Content); i += 2 {
		if k := n.Content[i].Value; !known[k] {
			return fmt.Errorf("line %d: unknown field %q", n.Content[i].Line, k)
		}
	}
	return n.Decode(v)
}

// yamlKeys lists the yaml keys of a struct pointer's fields.
func yamlKeys(v any) []string {
	t := reflect.TypeOf(v).Elem()
	var out []string
	for i := range t.NumField() {
		tag := strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0]
		if tag != "" && tag != "-" {
			out = append(out, tag)
		}
	}
	return out
}

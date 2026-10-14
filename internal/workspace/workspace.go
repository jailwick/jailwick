// Package workspace shows a project's related repos read-only in its sandbox, hides their
// secrets, and sets up the handoff folder shared by the workspace's sandboxes.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/spec"
	"github.com/jailwick/jailwick/internal/untrusted"
)

// MaskWarn is the number of hidden paths above which jailwick suggests broader rules.
const MaskWarn = 300

// Result is what a workspace adds to a project's sandbox.
type Result struct {
	Extra    spec.Extra
	AddDirs  []string // members, for the agent's --add-dir
	Markdown []byte   // generated instructions (workspace.md)
	Info     *agentapi.Workspace
	Rules    []string
	Warnings []string
}

// Generate returns nil for projects without a workspace. maskDir holds the empty file and
// folder mounted over hidden paths.
func Generate(store *config.Store, p config.Project, maskDir string) (*Result, error) {
	if p.Workspace == "" {
		return nil, nil
	}
	w, err := store.Workspace(p.Workspace)
	if err != nil {
		return nil, fmt.Errorf("project %s: %w (create it with: jw workspace add)", p.Name, err)
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	g := &generator{store: store, p: p, w: w, maskDir: maskDir}
	return g.run()
}

// Check reports whether workspace w can show project p, with the checks a start makes, without
// touching anything: the problems a setup shows before saving.
func Check(w config.Workspace, p config.Project) error {
	_, err := resolve(w, p)
	return err
}

// layout is what a workspace shows one project, resolved and checked.
type layout struct {
	root, dir string
	members   []member
	notes     string // file shown as <root>/CLAUDE.md; "" for none
	handoff   string // folder shared by the workspace's sandboxes; "" for none
}

// member is a folder shown read-only: from src on the host, at target in the sandbox.
type member struct{ src, target string }

func resolve(w config.Workspace, p config.Project) (layout, error) {
	l := layout{root: paths.Real(w.Root), dir: paths.Real(p.Dir)}
	if st, err := os.Stat(l.root); err != nil || !st.IsDir() {
		return l, fail("root %s doesn't exist", w.Root)
	}
	if l.root == "/" || strings.ContainsAny(l.root, " \t") {
		return l, fail("root %q isn't supported (/ or a path with spaces)", l.root)
	}
	if l.dir == l.root || !paths.Within(l.dir, l.root) {
		return l, fail("project %s (%s) must be inside the workspace root %s", p.Name, paths.Tilde(l.dir), paths.Tilde(l.root))
	}
	if err := l.resolveMembers(w, p); err != nil {
		return l, err
	}
	if err := l.resolveNotes(w); err != nil {
		return l, err
	}
	return l, l.resolveHandoff(w)
}

func (l *layout) resolveMembers(w config.Workspace, p config.Project) error {
	names := map[string]bool{}
	for _, m := range w.Members {
		names[strings.TrimSuffix(m.Name, "/")] = true
	}
	for _, r := range p.WorkspaceRelated {
		if !names[r] {
			return fail("workspace_related: %q is not a member of %s", r, w.Name)
		}
	}
	for _, m := range w.Members {
		name, src := strings.TrimSuffix(m.Name, "/"), MemberDir(w, m)
		target := filepath.Join(l.root, name)
		if src == l.dir || target == l.dir {
			continue // the project's own repo is already mounted read-write
		}
		if len(p.WorkspaceRelated) > 0 && !slices.Contains(p.WorkspaceRelated, name) {
			continue
		}
		if st, err := os.Stat(src); err != nil || !st.IsDir() {
			return fail("member %s: folder %s not found", name, paths.Tilde(src))
		}
		if strings.ContainsAny(src, " \t") {
			return fail("member %s: paths with spaces aren't supported", name)
		}
		if config.IsBroadPath(src) {
			return fail("member %s: refusing to show %s; point it at one repo", name, src)
		}
		for _, pair := range [][2]string{{target, l.dir}, {l.dir, target}, {src, l.dir}, {l.dir, src}} {
			if paths.Within(pair[0], pair[1]) {
				return fail("member %s overlaps the project's own repo", name)
			}
		}
		l.members = append(l.members, member{src: src, target: target})
	}
	return nil
}

func (l *layout) resolveNotes(w config.Workspace) error {
	if w.RootNotes == "" {
		return nil
	}
	file := paths.Expand(w.RootNotes)
	if !filepath.IsAbs(file) {
		file = filepath.Join(l.root, file)
	}
	if st, err := os.Stat(file); err != nil || st.IsDir() {
		return fail("root_notes: no such file %s", paths.Tilde(file))
	}
	l.notes = paths.Real(file)
	return nil
}

// HandoffDir is the workspace's handoff folder on the host; "" when it has none.
func HandoffDir(w config.Workspace) string {
	if w.Handoff == "" {
		return ""
	}
	h := paths.Expand(w.Handoff)
	if !filepath.IsAbs(h) {
		h = filepath.Join(paths.Real(w.Root), h)
	}
	return filepath.Clean(h)
}

// MemberDir is a member's folder on the host: its path, or its name under the root.
func MemberDir(w config.Workspace, m config.Member) string {
	root := paths.Real(w.Root)
	src := filepath.Join(root, strings.TrimSuffix(m.Name, "/"))
	if m.Path != "" {
		if src = paths.Expand(m.Path); !filepath.IsAbs(src) {
			src = filepath.Join(root, src)
		}
	}
	return paths.Real(src)
}

// Hidden lists what the workspace's rules hide in its members, relative to the root: what every
// project of it sees hidden (a project's own rules aside).
func Hidden(w config.Workspace) []string {
	rules := ApplyRules(DefaultRules, w.Exclude)
	var out []string
	for _, m := range w.Members {
		name, src := strings.TrimSuffix(m.Name, "/"), MemberDir(w, m)
		for _, h := range FindHidden(src, rules) {
			out = append(out, filepath.Join(name, strings.TrimPrefix(h, src+"/")))
		}
	}
	return out
}

func (l *layout) resolveHandoff(w config.Workspace) error {
	h := HandoffDir(w)
	if h == "" {
		return nil
	}
	if h == l.root || config.IsBroadPath(h) || strings.ContainsAny(h, " \t") {
		return fail("handoff %s: use a folder of its own (e.g. .handoff)", h)
	}
	for _, t := range append([]string{l.dir}, l.targets()...) {
		if paths.Within(t, h) || paths.Within(h, t) {
			return fail("handoff %s overlaps %s; use a folder outside the repos", h, t)
		}
	}
	l.handoff = h
	return nil
}

func (l *layout) targets() []string {
	out := make([]string, len(l.members))
	for i, m := range l.members {
		out[i] = m.target
	}
	return out
}

type generator struct {
	store   *config.Store
	p       config.Project
	w       config.Workspace
	maskDir string
	root    string
	res     Result
	info    agentapi.Workspace
	members []string // container targets
	notes   bool
}

func fail(format string, a ...any) error { return fmt.Errorf("workspace: "+format, a...) }

func (g *generator) run() (*Result, error) {
	l, err := resolve(g.w, g.p)
	if err != nil {
		return nil, err
	}
	g.root = l.root
	g.info = agentapi.Workspace{Name: g.w.Name, Root: g.root}
	g.addRoot(strings.Split(strings.TrimPrefix(l.dir, g.root+"/"), "/")[0])
	g.res.Rules = ApplyRules(ApplyRules(DefaultRules, g.w.Exclude), g.p.WorkspaceExclude)
	if err := g.ensureMasks(); err != nil {
		return nil, err
	}
	g.addMembers(l.members)
	g.addRootNotes(l.notes)
	handoff, err := g.addHandoff(l.handoff)
	if err != nil {
		return nil, err
	}
	notesDir := g.store.WorkspaceNotesDir(g.w.Name)
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return nil, err
	}
	g.mount(notesDir, "/etc/jailwick/workspace", true)
	g.res.Extra.Env = map[string]string{
		"SANDBOX_WORKSPACE": g.w.Name, "SANDBOX_WORKSPACE_ROOT": g.root, "SANDBOX_HANDOFF": handoff,
	}
	if g.notes {
		data, _ := os.ReadFile(g.info.Notes)
		sum := sha256.Sum256(data)
		// A single-file mount keeps the content from container start; the digest turns an edit
		// into a pending change.
		g.res.Extra.Env["SANDBOX_ROOT_NOTES_DIGEST"] = hex.EncodeToString(sum[:6])
	}
	slices.Sort(g.info.RootList)
	g.info.RootList = slices.Compact(g.info.RootList)
	g.info.Members = g.members
	g.res.AddDirs = g.members
	g.res.Info = &g.info
	g.res.Markdown = g.markdown(l.dir, handoff)
	return &g.res, nil
}

func (g *generator) addRoot(entry string) { g.info.RootList = append(g.info.RootList, entry) }

func (g *generator) mount(src, dst string, readOnly bool) {
	g.res.Extra.Mounts = append(g.res.Extra.Mounts, spec.Mount{Type: spec.Bind, Source: src, Target: dst, ReadOnly: readOnly})
}

func (g *generator) ensureMasks() error {
	if err := os.MkdirAll(filepath.Join(g.maskDir, "dir"), 0o755); err != nil {
		return err
	}
	f := filepath.Join(g.maskDir, "file")
	if _, err := os.Stat(f); err != nil {
		return os.WriteFile(f, nil, 0o444)
	}
	return nil
}

// addMembers mounts the members read-only, with masks over their hidden paths.
func (g *generator) addMembers(members []member) {
	for _, m := range members {
		g.mount(m.src, m.target, true)
		g.members = append(g.members, m.target)
		g.addRoot(strings.Split(strings.TrimPrefix(m.target, g.root+"/"), "/")[0])
		for _, h := range FindHidden(m.src, g.res.Rules) {
			mask := filepath.Join(g.maskDir, "file")
			if st, err := os.Stat(h); err == nil && st.IsDir() {
				mask = filepath.Join(g.maskDir, "dir")
			}
			t := filepath.Join(m.target, strings.TrimPrefix(h, m.src+"/"))
			g.mount(mask, t, true)
			g.info.Hidden = append(g.info.Hidden, t)
		}
	}
	if n := len(g.info.Hidden); n > MaskWarn {
		g.res.Warnings = append(g.res.Warnings, fmt.Sprintf("%d paths hidden in %s's members; broader rules (e.g. a folder/) in exclude would be lighter", n, g.p.Name))
	}
}

func (g *generator) addRootNotes(file string) {
	if file == "" {
		return
	}
	target := filepath.Join(g.root, "CLAUDE.md")
	g.mount(file, target, true)
	g.info.Notes = target
	g.notes = true
	g.addRoot("CLAUDE.md")
}

// addHandoff sets up the folder every sandbox of the workspace can write (requests between
// sessions). It is created on the host with a README when missing.
func (g *generator) addHandoff(h string) (string, error) {
	if h == "" {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Join(h, "done"), 0o755); err != nil {
		return "", fail("can't create the handoff folder %s: %v", h, err)
	}
	// The sandbox writes this folder: the README is only ever created, never written through
	// whatever is at its path (a link could point anywhere on the host).
	if _, err := untrusted.CreateFile(h, "README.md", []byte(handoffReadme(g.w.Name)), 0o644); err != nil {
		return "", err
	}
	h = paths.Real(h)
	g.mount(h, h, false)
	g.info.Handoff = h
	if paths.Within(h, g.root) {
		g.addRoot(strings.Split(strings.TrimPrefix(h, g.root+"/"), "/")[0])
	}
	return h, nil
}

// RepoID names a project's inbox: its folder under the root, with / replaced by -.
func RepoID(root, dir string) string {
	return strings.ReplaceAll(strings.TrimPrefix(dir, root+"/"), "/", "-")
}

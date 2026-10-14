package workspace

import (
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// DefaultRules hide secrets in every member, plus its own worktrees (duplicate copies) and its
// node_modules (built for the host, not the sandbox).
var DefaultRules = []string{".env", ".env.*", "!.env.example", "*.pem", "*.key", "*.p12", "*.pfx", "id_rsa*", "id_ed25519*", ".jailwick/", ".claude/worktrees/", "node_modules/"}

// ApplyRules edits a rule list: "x" or "+x" adds, "-x" removes a rule, "-*" removes all.
func ApplyRules(rules []string, edits []string) []string {
	out := slices.Clone(rules)
	for _, e := range edits {
		switch {
		case e == "-*":
			out = nil
		case strings.HasPrefix(e, "-") && len(e) > 1:
			out = slices.DeleteFunc(out, func(r string) bool { return r == e[1:] })
		case strings.HasPrefix(e, "+") && len(e) > 1:
			out = append(out, e[1:])
		case e != "":
			out = append(out, e)
		}
	}
	return out
}

type rule struct {
	pattern  string
	dirOnly  bool
	anchored bool // contains a slash: matched against the path from the member's root
}

func parse(r string) rule {
	x := rule{pattern: r}
	if strings.HasSuffix(x.pattern, "/") {
		x.dirOnly, x.pattern = true, strings.TrimSuffix(x.pattern, "/")
	}
	if strings.Contains(x.pattern, "/") {
		x.anchored, x.pattern = true, strings.TrimPrefix(x.pattern, "/")
	}
	return x
}

func (r rule) match(rel string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}
	target := path.Base(rel)
	if r.anchored {
		target = rel
	}
	ok, _ := path.Match(r.pattern, target)
	return ok
}

// FindHidden returns the paths in dir matching the rules (a .gitignore subset), sorted:
//
//	name, *.log   that name anywhere     a/b, /a   that path from the root
//	x/            folders only           !x        keep x visible
//
// A matched folder is hidden as a whole. .git and symlinks are never matched (a symlink only
// leads to paths that are mounted, which the rules cover themselves).
func FindHidden(dir string, rules []string) []string {
	var pos, neg []rule
	for _, r := range rules {
		if n, ok := strings.CutPrefix(r, "!"); ok {
			neg = append(neg, parse(n))
		} else {
			pos = append(pos, parse(r))
		}
	}
	if len(pos) == 0 {
		return nil
	}
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return nil
		}
		if d.Name() == ".git" && d.IsDir() {
			return filepath.SkipDir
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, dir+"/"))
		isDir := d.IsDir()
		if !matchAny(pos, rel, isDir) {
			return nil
		}
		if !matchAny(neg, rel, isDir) {
			out = append(out, p)
		}
		if isDir {
			return filepath.SkipDir
		}
		return nil
	})
	slices.Sort(out)
	return out
}

func matchAny(rules []rule, rel string, isDir bool) bool {
	for _, r := range rules {
		if r.match(rel, isDir) {
			return true
		}
	}
	return false
}

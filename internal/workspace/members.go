package workspace

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
)

// A workspace's projects are among its members: joining one makes the project's folder a member,
// so the workspace's other projects see it (read-only) as it sees theirs. A project's workspace
// and the workspace's members are kept in two files; every way of joining goes through
// AddMember so they agree. Leaving asks whether the folder stays a member (RemoveMember).

// memberFor is the index of w's member showing the folder dir, or -1.
func memberFor(w config.Workspace, dir string) int {
	dir = paths.Real(dir)
	return slices.IndexFunc(w.Members, func(m config.Member) bool { return MemberDir(w, m) == dir })
}

// WithMember is w with the folder dir among its members, and whether it had to be added. A new
// member is named by its path from the root ("api", "apps/web"), so sandboxes show it where it is
// on the host; when another member has that name, it gets a free one ("api-2") with its path.
func WithMember(w config.Workspace, dir string) (config.Workspace, bool) {
	if memberFor(w, dir) >= 0 {
		return w, false
	}
	root, dir := paths.Real(w.Root), paths.Real(dir)
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return w, false // not in the workspace: a start refuses it anyway
	}
	m := config.Member{Name: filepath.ToSlash(rel)}
	taken := func(n string) bool {
		return slices.ContainsFunc(w.Members, func(x config.Member) bool { return strings.TrimSuffix(x.Name, "/") == n })
	}
	for i := 2; taken(m.Name); i++ {
		m.Name, m.Path = fmt.Sprintf("%s-%d", filepath.Base(dir), i), filepath.ToSlash(rel)
	}
	w.Members = append(slices.Clone(w.Members), m)
	return w, true
}

// MissingMember is the member that would show a project's folder in its workspace, when the
// folder isn't one yet (a project put in it by editing its file): the other projects can't see
// it. ok is false when it is a member, or isn't in the workspace's root.
func MissingMember(w config.Workspace, p config.Project) (m config.Member, ok bool) {
	next, added := WithMember(w, p.Dir)
	if !added {
		return config.Member{}, false
	}
	return next.Members[len(next.Members)-1], true
}

// AddMember makes the folder of a project its workspace's member, saving the workspace. It
// reports whether it was added (false: already one, or the project is in no workspace).
func AddMember(store *config.Store, p config.Project) (bool, error) {
	if p.Workspace == "" {
		return false, nil
	}
	w, err := store.Workspace(p.Workspace)
	if err != nil {
		return false, err
	}
	w, added := WithMember(w, p.Dir)
	if !added {
		return false, nil
	}
	if err := w.Validate(); err != nil {
		return false, err
	}
	return true, store.SaveWorkspace(w)
}

// RemoveMember takes the folder dir out of a workspace's members, saving the workspace; the
// folder itself is untouched. It reports whether it was a member.
func RemoveMember(store *config.Store, name, dir string) (bool, error) {
	w, err := store.Workspace(name)
	if err != nil {
		return false, err
	}
	i := memberFor(w, dir)
	if i < 0 {
		return false, nil
	}
	w.Members = slices.Delete(slices.Clone(w.Members), i, i+1)
	return true, store.SaveWorkspace(w)
}

// Others are the projects of a workspace other than project: the ones that see a member
// added or removed after their next restart.
func Others(store *config.Store, workspace, project string) []string {
	ps, _ := store.Projects()
	var out []string
	for _, p := range ps {
		if p.Workspace == workspace && p.Name != project {
			out = append(out, p.Name)
		}
	}
	return out
}

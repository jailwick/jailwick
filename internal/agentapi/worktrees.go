package agentapi

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var idRE = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// CheckID validates a conversation id to resume (or last, pick).
func CheckID(id string) error {
	if !idRE.MatchString(id) {
		return fmt.Errorf("invalid conversation id %q", id)
	}
	return nil
}

// WorktreeFolder is where jailwick puts worktrees, relative to the repo: <repo>/.jailwick/worktrees/
// <name>. The whole .jailwick folder is jailwick's, kept out of git by the repo's local exclude.
const WorktreeFolder = ".jailwick/worktrees"

// LegacyWorktreeFolder is where worktrees were made before 0.2.5. Those are still recognised;
// new ones go to WorktreeFolder.
const LegacyWorktreeFolder = ".claude/worktrees"

// WorktreeFolders are both, newest first.
var WorktreeFolders = []string{WorktreeFolder, LegacyWorktreeFolder}

// WorktreeOf names the worktree folder that path is in, "" for the main checkout of repo.
func WorktreeOf(repo, path string) string {
	for _, f := range WorktreeFolders {
		if rel, ok := strings.CutPrefix(path, filepath.Join(repo, f)+"/"); ok {
			name, _, _ := strings.Cut(rel, "/")
			return name
		}
	}
	return ""
}

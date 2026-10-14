package agent

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// What a checkout changed, for review in the app: a worktree against the main checkout's branch
// (from where they part, so commits and uncommitted edits alike), or the main checkout against
// its last commit. git runs here in the sandbox, never on the host: a repo the sandbox wrote can
// make git run commands (hooks, fsmonitor, diff drivers), which must not happen on the host.

// maxDiff bounds one file's diff.
const maxDiff = 512 << 10

// safeGit runs git with the settings that could run commands turned off.
func safeGit(dir string, args ...string) ([]byte, error) {
	base := []string{"-C", dir, "-c", "core.fsmonitor=false", "-c", "core.pager=cat", "-c", "diff.external=",
		"--no-pager"}
	cmd := exec.Command("git", append(base, args...)...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(errOut.String()))
	}
	return out.Bytes(), nil
}

// Changes describes what the main checkout ("") or a worktree changed. With file, it also
// carries that file's diff.
func Changes(c *agentapi.Config, worktree, file string) (agentapi.Changes, error) {
	dir, base := c.Dir, "HEAD"
	var ch agentapi.Changes
	if worktree != "" {
		branch, _, err := Resolve(c, worktree)
		if err != nil {
			return ch, err
		}
		dir = worktreeDir(c, branch)
		mainBranch, err := safeGit(c.Dir, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return ch, err
		}
		ch.Base = strings.TrimSpace(string(mainBranch))
		mb, err := safeGit(dir, "merge-base", ch.Base, "HEAD")
		if err != nil {
			return ch, err
		}
		base = strings.TrimSpace(string(mb))
		if log, err := safeGit(dir, "log", "--format=%h%x00%s", base+"..HEAD"); err == nil {
			for _, l := range strings.Split(strings.TrimSpace(string(log)), "\n") {
				if h, s, ok := strings.Cut(l, "\x00"); ok {
					ch.Commits = append(ch.Commits, agentapi.Commit{Hash: h, Subject: s})
				}
			}
		}
	}
	if head, err := safeGit(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		ch.Head = strings.TrimSpace(string(head))
	}
	// Tracked files, from the base to the working tree: commits and uncommitted edits together.
	numstat, err := safeGit(dir, "diff", "--no-ext-diff", "--no-textconv", "--numstat", "-z", base)
	if err != nil {
		return ch, err
	}
	status, _ := safeGit(dir, "diff", "--no-ext-diff", "--no-textconv", "--name-status", "-z", base)
	kinds := nameStatus(status)
	for _, f := range parseNumstat(numstat) {
		f.Status = kinds[f.Path]
		ch.Files = append(ch.Files, f)
	}
	if untracked, err := safeGit(dir, "ls-files", "--others", "--exclude-standard", "-z"); err == nil {
		for _, p := range strings.Split(strings.TrimRight(string(untracked), "\x00"), "\x00") {
			if p != "" {
				ch.Files = append(ch.Files, agentapi.ChangedFile{Path: p, Status: "untracked"})
			}
		}
	}
	if file != "" {
		diff, err := safeGit(dir, "diff", "--no-ext-diff", "--no-textconv", "--no-color", base, "--", file)
		if err != nil {
			return ch, err
		}
		if len(diff) > maxDiff {
			diff, ch.DiffCut = diff[:maxDiff], true
		}
		ch.Diff = string(diff)
	}
	return ch, nil
}

// parseNumstat reads `git diff --numstat -z`: added, deleted, path (binary files have "-").
func parseNumstat(out []byte) []agentapi.ChangedFile {
	var files []agentapi.ChangedFile
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		parts := strings.SplitN(fields[i], "\t", 3)
		if len(parts) != 3 {
			continue
		}
		f := agentapi.ChangedFile{Path: parts[2]}
		if parts[2] == "" && i+2 < len(fields) { // a rename: old and new paths follow
			f.Path = fields[i+2]
			i += 2
		}
		f.Added, _ = strconv.Atoi(parts[0])
		f.Deleted, _ = strconv.Atoi(parts[1])
		f.Binary = parts[0] == "-"
		files = append(files, f)
	}
	return files
}

// nameStatus reads `git diff --name-status -z` into a kind per path.
func nameStatus(out []byte) map[string]string {
	words := map[byte]string{'A': "added", 'M': "modified", 'D': "deleted", 'R': "renamed", 'C': "copied", 'T': "type changed"}
	kinds := map[string]string{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		code := fields[i]
		if code == "" || i+1 >= len(fields) {
			continue
		}
		path := fields[i+1]
		i++
		if code[0] == 'R' || code[0] == 'C' { // old, then new path
			if i+1 < len(fields) {
				path = fields[i+1]
				i++
			}
		}
		kinds[path] = words[code[0]]
	}
	return kinds
}

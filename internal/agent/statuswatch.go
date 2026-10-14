package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Status is what `status` reports about this container: its sessions, worktrees, runs and the
// node_modules folders with nothing installed.
func Status(c *agentapi.Config) agentapi.Status {
	return agentapi.Status{Sessions: Sessions(c), Worktrees: ListWorktrees(c), Runs: Runs(c),
		EmptyNodeModules: EmptyNodeModules(c)}
}

// The app follows a container through `watch` instead of asking `status` every few seconds: one
// long-running exec, which prints the status when it changes. It sleeps until a folder the status
// comes from changes (inotify): Claude's session list (rewritten as a session starts, ends, works
// or waits), the runs, the worktrees and slots, the node_modules folders. The heartbeat also looks
// again, catching what no file says (a process gone without its file). The worktrees run git:
// they're listed again only when their files changed.

// Watch timings: the longest watch stays silent (the host takes silence for a hung stream), and
// looks again regardless; the timer where there's no inotify; the pause that makes a burst of
// changes one look; the longest the worktrees go without being listed again.
const (
	WatchHeartbeat = 30 * time.Second
	WatchEvery     = 2 * time.Second
	settle         = 300 * time.Millisecond
	worktreesEvery = 30 * time.Second
)

// Watch prints the container's status as JSON lines, as it changes, until the host goes away
// or ctx ends.
func Watch(ctx context.Context, c *agentapi.Config, out io.Writer) error {
	wait, closeWait := changeWaiter(func() []string { return watchedDirs(c) })
	defer closeWait()
	return WatchStatus(ctx, out, StatusSource(c), wait, WatchHeartbeat)
}

// watchedDirs are the folders the status comes from.
func watchedDirs(c *agentapi.Config) []string {
	git := filepath.Join(c.Dir, ".git")
	dirs := []string{filepath.Join(claudeDir(c.Home), "sessions"), runDir,
		filepath.Join(git, "worktrees"), filepath.Join(git, "jailwick")}
	for _, f := range agentapi.WorktreeFolders {
		dirs = append(dirs, filepath.Join(c.Dir, f))
	}
	return append(dirs, c.NodeModules...)
}

// timerWaiter looks every WatchEvery: where inotify isn't there.
func timerWaiter(func() []string) (func(ctx context.Context, max time.Duration), func()) {
	return func(ctx context.Context, max time.Duration) {
		select {
		case <-ctx.Done():
		case <-time.After(min(max, WatchEvery)):
		}
	}, func() {}
}

// WatchStatus prints snapshot as a JSON line now, then whenever it changes (looking again when
// wait returns: something changed, or heartbeat passed), and at least every heartbeat. It
// returns when out can't be written (the host went away) or ctx ends.
func WatchStatus(ctx context.Context, out io.Writer, snapshot func() agentapi.Status, wait func(context.Context, time.Duration), heartbeat time.Duration) error {
	var last []byte
	var sent time.Time
	for {
		data, err := json.Marshal(snapshot())
		if err != nil {
			return err
		}
		if !bytes.Equal(data, last) || time.Since(sent) >= heartbeat {
			if _, err := out.Write(append(data, '\n')); err != nil {
				return err
			}
			last, sent = data, time.Now()
		}
		if wait(ctx, heartbeat-time.Since(sent)); ctx.Err() != nil {
			return nil
		}
	}
}

// StatusSource builds the status for WatchStatus: sessions, runs and node_modules each time, the
// worktrees only when worktreeSignature changed or worktreesEvery passed.
func StatusSource(c *agentapi.Config) func() agentapi.Status {
	var sig string
	var listed time.Time
	var worktrees []agentapi.Worktree
	return func() agentapi.Status {
		if now := worktreeSignature(c.Dir); now != sig || time.Since(listed) >= worktreesEvery {
			worktrees, sig, listed = ListWorktrees(c), now, time.Now()
		}
		return agentapi.Status{Sessions: Sessions(c), Worktrees: worktrees, Runs: Runs(c),
			EmptyNodeModules: EmptyNodeModules(c)}
	}
}

// worktreeSignature changes when the repo's worktrees may have: git's worktree records (one
// folder each, whose HEAD changes with the branch), jailwick's slot registry, and the worktree
// folders. Only stats: no git.
func worktreeSignature(repo string) string {
	var b strings.Builder
	stamp := func(path string) {
		if info, err := os.Stat(path); err == nil {
			b.WriteString(path)
			b.WriteByte(' ')
			b.WriteString(strconv.FormatInt(info.ModTime().UnixNano(), 10))
			b.WriteByte('\n')
		}
	}
	stampDir := func(dir string, inside string) {
		stamp(dir)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			stamp(filepath.Join(dir, e.Name(), inside))
		}
	}
	git := filepath.Join(repo, ".git")
	stampDir(filepath.Join(git, "worktrees"), "HEAD")
	stampDir(filepath.Join(git, "jailwick"), "")
	for _, f := range agentapi.WorktreeFolders {
		stamp(filepath.Join(repo, f))
	}
	return b.String()
}

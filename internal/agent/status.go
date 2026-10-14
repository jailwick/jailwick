package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Sessions finds the agent processes in this container (from /proc; see ownProcess), enriched with Claude's
// own session registry where it exists (best effort; the registry is not a public interface).
func Sessions(c *agentapi.Config) []agentapi.Session {
	boot := bootTime()
	var out []agentapi.Session
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !ownProcess(pid) {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil || !isCommand(cmdline, c.Command) {
			continue
		}
		cwd, _ := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		s := agentapi.Session{PID: pid, Dir: cwd, Worktree: WorktreeOf(c.Dir, cwd), Started: startTime(pid, boot), Notify: notifyOf(pid)}
		readRegistry(c.Home, &s)
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b agentapi.Session) int { return a.Started.Compare(b.Started) })
	return out
}

// isCommand matches a process whose first argument is the agent command (Claude Code sets its
// process title to "claude").
func isCommand(cmdline []byte, command string) bool {
	first, _, _ := bytes.Cut(cmdline, []byte{0})
	first, _, _ = bytes.Cut(first, []byte(" "))
	return len(first) > 0 && filepath.Base(string(first)) == command
}

// WorktreeOf names the worktree folder dir is in, "" for the main checkout of project.
func WorktreeOf(project, dir string) string { return agentapi.WorktreeOf(project, dir) }

func readRegistry(home string, s *agentapi.Session) {
	data, err := os.ReadFile(filepath.Join(claudeDir(home), "sessions", strconv.Itoa(s.PID)+".json"))
	if err != nil {
		return
	}
	var r struct {
		SessionID string `json:"sessionId"`
		Name      string `json:"name"`
		Status    string `json:"status"`
	}
	if json.Unmarshal(data, &r) == nil {
		s.ID, s.Name, s.State = r.SessionID, r.Name, r.Status
	}
}

func bootTime() time.Time {
	data, _ := os.ReadFile("/proc/stat")
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				return time.Unix(n, 0)
			}
		}
	}
	return time.Time{}
}

// startTime reads field 22 of /proc/<pid>/stat (clock ticks since boot; 100 Hz on Linux).
func startTime(pid int, boot time.Time) time.Time {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil || boot.IsZero() {
		return time.Time{}
	}
	// The command name may contain spaces: fields start after the closing parenthesis.
	i := bytes.LastIndexByte(data, ')')
	if i < 0 {
		return time.Time{}
	}
	f := strings.Fields(string(data[i+1:]))
	if len(f) < 20 {
		return time.Time{}
	}
	ticks, err := strconv.ParseInt(f[19], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return boot.Add(time.Duration(ticks) * time.Second / 100)
}

// StopSessions asks every agent session in this container to end (SIGTERM).
func StopSessions(c *agentapi.Config) error {
	ss := Sessions(c)
	for _, s := range ss {
		if p, err := os.FindProcess(s.PID); err == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
	}
	fmt.Printf("asked %d session(s) to end\n", len(ss))
	return nil
}

// EmptyNodeModules lists the sandbox's own node_modules folders with nothing installed yet whose
// package folder has a package.json (relative to the repo, "." for its top).
func EmptyNodeModules(c *agentapi.Config) []string {
	var out []string
	for _, nm := range c.NodeModules {
		pkg := filepath.Dir(nm)
		if _, err := os.Stat(filepath.Join(pkg, "package.json")); err != nil {
			continue
		}
		if entries, err := os.ReadDir(nm); err == nil && len(entries) == 0 {
			rel, _ := filepath.Rel(c.Dir, pkg)
			out = append(out, rel)
		}
	}
	return out
}

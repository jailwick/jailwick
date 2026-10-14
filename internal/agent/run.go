package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// The project's run command (`run:`, the app's Run button) runs in the background in the main
// checkout or a worktree, one per place. Its process id and output are kept in runDir; the
// process lives as long as the sandbox's main container.

// runDir holds each run's pid and log, by place (main, or the worktree's folder name).
var runDir = "/tmp/jailwick-run"

// runLogMax bounds how much of a run's output RunLog returns.
const runLogMax = 64 << 10

func runPaths(place string) (pid, log string) {
	return filepath.Join(runDir, place+".pid"), filepath.Join(runDir, place+".log")
}

func runPlace(worktree string) string {
	if worktree == "" {
		return "main"
	}
	return Folder(worktree)
}

// StartRun starts the run command in the main checkout or a worktree, with that worktree's
// environment. One run per place: a running one is left alone.
func StartRun(c *agentapi.Config, worktree string) error {
	if strings.TrimSpace(c.Run) == "" {
		return errors.New("no run command: set run: in the project (jw project edit), e.g. run: pnpm dev")
	}
	dir, env := c.Dir, map[string]string{}
	if worktree != "" {
		branch, slot, err := Resolve(c, worktree)
		if err != nil {
			return err
		}
		dir, env = worktreeDir(c, branch), WorktreeEnv(c, branch, slot)
	}
	place := runPlace(worktree)
	if r, ok := readRun(place); ok && r.Running {
		return nil
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	pidFile, logFile := runPaths(place)
	log, err := os.Create(logFile)
	if err != nil {
		return err
	}
	defer log.Close()
	fmt.Fprintf(log, "$ %s\n", c.Run)
	cmd := exec.Command("sh", "-c", c.Run)
	cmd.Dir, cmd.Env = dir, mergeEnv(os.Environ(), env)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // its own group: Stop ends all of it
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// StopRun ends the run in a place, with everything it started.
func StopRun(worktree string) error {
	r, ok := readRun(runPlace(worktree))
	if !ok || !r.Running {
		return nil
	}
	if err := syscall.Kill(-r.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	for range 50 { // up to 5 s, then for good
		if !alive(r.PID) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return syscall.Kill(-r.PID, syscall.SIGKILL)
}

// Runs lists the runs there are (running or ended), the main checkout first.
func Runs(c *agentapi.Config) []agentapi.Run {
	entries, _ := os.ReadDir(runDir)
	var out []agentapi.Run
	for _, e := range entries {
		place, ok := strings.CutSuffix(e.Name(), ".pid")
		if !ok {
			continue
		}
		r, ok := readRun(place)
		if !ok {
			continue
		}
		if place != "main" {
			r.Worktree = place
			if _, slot, err := Resolve(c, place); err == nil {
				r.Port = Port(slot)
			}
		}
		out = append(out, r)
	}
	return out
}

// RunLog returns the end of a run's output.
func RunLog(worktree string) agentapi.RunLog {
	_, logFile := runPaths(runPlace(worktree))
	f, err := os.Open(logFile)
	if err != nil {
		return agentapi.RunLog{}
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > runLogMax {
		_, _ = f.Seek(-runLogMax, io.SeekEnd)
	}
	data, _ := io.ReadAll(f)
	return agentapi.RunLog{Text: string(data)}
}

func readRun(place string) (agentapi.Run, bool) {
	pidFile, _ := runPaths(place)
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return agentapi.Run{}, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return agentapi.Run{}, false
	}
	r := agentapi.Run{PID: pid, Running: alive(pid)}
	if st, err := os.Stat(pidFile); err == nil {
		r.Started = st.ModTime()
	}
	return r, true
}

// alive reports whether a process runs (an ended one waiting to be reaped doesn't).
func alive(pid int) bool { return syscall.Kill(pid, 0) == nil && !zombie(pid) }

// zombie reports a process that ended but wasn't reaped yet (its parent, the agent, is gone).
func zombie(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	_, rest, ok := strings.Cut(string(data), ") ")
	return ok && strings.HasPrefix(rest, "Z")
}

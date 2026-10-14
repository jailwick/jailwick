package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

type procSample struct {
	pid   int
	ticks int64 // utime + stime
	rss   int64 // bytes
	where string
	kind  string
}

// Top prints CPU and memory per session (main checkout, each worktree) and for the container,
// sampled over one second. With watch it refreshes every two seconds until interrupted.
func Top(c *agentapi.Config, w io.Writer, watch bool) {
	for {
		before := sample(c)
		cpuBefore := cgroupCPU()
		time.Sleep(time.Second)
		after := sample(c)
		cpuAfter := cgroupCPU()
		if watch {
			fmt.Fprint(w, "\x1b[H\x1b[2J")
		}
		printTop(c, w, before, after, cpuAfter-cpuBefore)
		if !watch {
			return
		}
		time.Sleep(time.Second)
	}
}

func sample(c *agentapi.Config) map[int]procSample {
	out := map[int]procSample{}
	page := int64(os.Getpagesize())
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !ownProcess(pid) {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		i := strings.LastIndexByte(string(stat), ')')
		f := strings.Fields(string(stat[i+1:]))
		if len(f) < 22 {
			continue
		}
		ut, _ := strconv.ParseInt(f[11], 10, 64)
		st, _ := strconv.ParseInt(f[12], 10, 64)
		rss, _ := strconv.ParseInt(f[21], 10, 64)
		cwd, _ := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		where := "(other)"
		if wt := WorktreeOf(c.Dir, cwd); wt != "" {
			where = wt
		} else if strings.HasPrefix(cwd, c.Dir) {
			where = "(main checkout)"
		}
		cmd, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		out[pid] = procSample{pid: pid, ticks: ut + st, rss: rss * page, where: where, kind: classify(string(cmd), c.Command)}
	}
	return out
}

func classify(cmdline, agentCmd string) string {
	if isCommand([]byte(cmdline), agentCmd) {
		return "agent"
	}
	l := strings.ToLower(cmdline)
	for _, k := range []struct{ match, kind string }{
		{"playwright", "browser"}, {"chrom", "browser"}, {"vitest", "tests"}, {"jest", "tests"},
		{"tsc", "typecheck"}, {"vite", "dev server"}, {"next", "dev server"}, {"astro", "dev server"},
		{"pnpm", "pnpm"}, {"npm", "npm"}, {"node", "node"}, {"python", "python"}, {"postgres", "db"},
	} {
		if strings.Contains(l, k.match) {
			return k.kind
		}
	}
	return "other"
}

type group struct {
	cpu   float64
	rss   int64
	procs int
	kinds map[string]int
}

func printTop(c *agentapi.Config, w io.Writer, before, after map[int]procSample, containerUsec int64) {
	groups := map[string]*group{}
	for pid, a := range after {
		g := groups[a.where]
		if g == nil {
			g = &group{kinds: map[string]int{}}
			groups[a.where] = g
		}
		if b, ok := before[pid]; ok {
			g.cpu += float64(a.ticks - b.ticks) // ticks over 1s at 100 Hz = percent of one CPU
		}
		g.rss += a.rss
		g.procs++
		g.kinds[a.kind]++
	}
	memCur, memMax := cgroupMemory()
	fmt.Fprintf(w, "container  cpu %5.1f%%  mem %s / %s\n\n", float64(containerUsec)/1e4, human(memCur), human(memMax))
	fmt.Fprintf(w, "%-28s %6s %9s %5s  %s\n", "WHERE", "CPU%", "MEM", "PROCS", "WHAT")
	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int { return int(groups[b].rss - groups[a].rss) })
	for _, n := range names {
		g := groups[n]
		var kinds []string
		for k, cnt := range g.kinds {
			if k != "other" {
				kinds = append(kinds, fmt.Sprintf("%s×%d", k, cnt))
			}
		}
		slices.Sort(kinds)
		fmt.Fprintf(w, "%-28s %6.1f %9s %5d  %s\n", n, g.cpu, human(g.rss), g.procs, strings.Join(kinds, " "))
	}
}

func cgroupCPU() int64 {
	data, _ := os.ReadFile("/sys/fs/cgroup/cpu.stat")
	for _, l := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(l, "usage_usec "); ok {
			n, _ := strconv.ParseInt(v, 10, 64)
			return n
		}
	}
	return 0
}

func cgroupMemory() (cur, limit int64) {
	read := func(f string) int64 {
		data, _ := os.ReadFile("/sys/fs/cgroup/" + f)
		n, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		return n
	}
	return read("memory.current"), read("memory.max")
}

func human(b int64) string {
	switch {
	case b <= 0:
		return "-"
	case b >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%dM", b>>20)
	}
	return fmt.Sprintf("%dK", b>>10)
}

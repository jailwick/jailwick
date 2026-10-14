package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Ports lists listening TCP ports (from /proc/net/tcp*), with their processes where visible.
func Ports(c *agentapi.Config) []agentapi.Port {
	inodes := map[string]agentapi.Port{}
	for _, file := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		readListeners(file, inodes)
	}
	owners := socketOwners()
	seen := map[int]bool{}
	var out []agentapi.Port
	for inode, p := range inodes {
		if seen[p.Port] {
			continue
		}
		seen[p.Port] = true
		if pid, ok := owners[inode]; ok {
			p.PID = pid
			p.Command, p.Where = describeProcess(c, pid)
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b agentapi.Port) int { return a.Port - b.Port })
	return out
}

// readListeners parses lines like "0: 00000000:0BB8 00000000:0000 0A …" (state 0A = LISTEN).
func readListeners(file string, into map[string]agentapi.Port) {
	f, err := os.Open(file)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Scan() // header
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) < 10 || fs[3] != "0A" {
			continue
		}
		addr, portHex, ok := strings.Cut(fs[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil {
			continue
		}
		address := "0.0.0.0"
		switch addr {
		case "0100007F", "00000000000000000000000001000000":
			address = "127.0.0.1"
		case "00000000000000000000000000000000":
			address = "::"
		}
		into[fs[9]] = agentapi.Port{Port: int(port), Address: address}
	}
}

// socketOwners maps socket inodes to the PIDs holding them (only our own processes are readable).
func socketOwners() map[string]int {
	out := map[string]int{}
	procs, _ := os.ReadDir("/proc")
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue
		}
		fds, _ := os.ReadDir(filepath.Join("/proc", p.Name(), "fd"))
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join("/proc", p.Name(), "fd", fd.Name()))
			if inode, ok := strings.CutPrefix(link, "socket:["); err == nil && ok {
				out[strings.TrimSuffix(inode, "]")] = pid
			}
		}
	}
	return out
}

func describeProcess(c *agentapi.Config, pid int) (cmd, where string) {
	raw, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	parts := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
	for i, p := range parts {
		// Shorten paths of dependencies: …/node_modules/.bin/vite → vite.
		if j := strings.LastIndex(p, "/node_modules/"); j >= 0 {
			parts[i] = filepath.Base(p)
		}
	}
	cmd = strings.Join(parts, " ")
	if len(cmd) > 80 {
		cmd = cmd[:77] + "…"
	}
	if c == nil {
		return cmd, ""
	}
	if pid == 1 || strings.Contains(cmd, "jailwick-agent init") {
		return cmd, "forward"
	}
	cwd, _ := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
	switch {
	case WorktreeOf(c.Dir, cwd) != "":
		return cmd, "wt " + WorktreeOf(c.Dir, cwd)
	case strings.HasPrefix(cwd, c.Dir):
		return cmd, "main checkout"
	}
	return cmd, "other"
}

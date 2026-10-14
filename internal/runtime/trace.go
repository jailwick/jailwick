package runtime

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tracing logs every runtime command jailwick runs (when it started, how long it took, how much it
// printed, its kind and the command) to trace.log in the state folder: to see what the app or a
// command does in the background, and what it costs. It is on with JAILWICK_TRACE=1, or while the
// file trace.on is in the state folder (jw trace on: the desktop app sees no environment, and
// checks the file as it runs). Off, nothing is measured.

// TraceEnv turns tracing on.
const TraceEnv = "JAILWICK_TRACE"

// TraceFlag is the file whose presence turns tracing on.
func TraceFlag(stateDir string) string { return filepath.Join(stateDir, "trace.on") }

// tracingOn reports whether to trace, checking the flag at most every few seconds.
func (d *cli) tracingOn() bool {
	if os.Getenv(TraceEnv) != "" {
		return true
	}
	traces.mu.Lock()
	defer traces.mu.Unlock()
	if time.Since(traces.checked) > traceCheckEvery {
		_, err := os.Stat(TraceFlag(d.stateDir))
		traces.on, traces.checked = err == nil, time.Now()
	}
	return traces.on
}

const traceCheckEvery = 5 * time.Second

type traceLog struct {
	mu      sync.Mutex
	f       *os.File
	on      bool      // the flag file was there
	checked time.Time // when it was looked for
}

var traces traceLog

// TraceFile is where the trace goes.
func TraceFile(stateDir string) string { return filepath.Join(stateDir, "trace.log") }

// do runs cmd, traced when tracing is on. A terminal stays the command's own: only output that
// isn't a terminal is counted.
func (d *cli) do(cmd *exec.Cmd) error {
	if !d.tracingOn() {
		return cmd.Run()
	}
	var count *countWriter
	if _, isFile := cmd.Stdout.(*os.File); !isFile && cmd.Stdout != nil {
		count = &countWriter{w: cmd.Stdout}
		// Output and errors sharing one writer keep sharing it: exec then copies them in one
		// goroutine. Two writers would be written at once, and one buffer would lose lines.
		if sameWriter(cmd.Stderr, cmd.Stdout) {
			cmd.Stderr = count
		}
		cmd.Stdout = count
	}
	start := time.Now()
	err := cmd.Run()
	n := int64(-1)
	if count != nil {
		n = count.n
	}
	traces.write(d.stateDir, start, time.Since(start), n, cmd.Args, err)
	return err
}

func (t *traceLog) write(stateDir string, start time.Time, took time.Duration, out int64, args []string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f == nil {
		f, ferr := os.OpenFile(TraceFile(stateDir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if ferr != nil {
			return
		}
		t.f = f
	}
	size := "-"
	if out >= 0 {
		size = humanBytes(out)
	}
	line := fmt.Sprintf("%s %7dms %8s  %-28s %s", start.Format(traceTime), took.Milliseconds(), size,
		"["+TraceKind(args)+"]", strings.Join(args, " "))
	if err != nil {
		line += "  ERROR " + strings.ReplaceAll(err.Error(), "\n", " ")
	}
	fmt.Fprintln(t.f, line)
}

// traceTime is how a trace line starts.
const traceTime = "2006-01-02T15:04:05.000"

// TraceKind names what a runtime command does, for counting alike ones together: the
// subcommand, and for an exec the program it runs ("exec jailwick-agent status").
func TraceKind(args []string) string {
	if len(args) < 2 {
		return strings.Join(args, " ")
	}
	sub := args[1]
	switch sub {
	case "exec":
		rest := args[2:]
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
			if takesValue(rest[0]) && len(rest) > 1 {
				rest = rest[2:]
			} else {
				rest = rest[1:]
			}
		}
		if len(rest) < 2 { // the container, then the program
			return sub
		}
		kind := "exec " + filepath.Base(rest[1])
		if len(rest) > 2 && !strings.HasPrefix(rest[2], "-") {
			kind += " " + rest[2]
		}
		return kind
	case "compose":
		for i := 2; i < len(args); i++ {
			if strings.HasPrefix(args[i], "-") {
				if takesValue(args[i]) {
					i++
				}
				continue
			}
			return "compose " + args[i]
		}
	}
	return sub
}

// takesValue reports whether a flag of exec or compose is followed by its value.
func takesValue(flag string) bool {
	switch flag {
	case "-u", "--user", "-w", "--workdir", "-e", "--env", "-p", "--project-name", "-f", "--file", "--env-file":
		return true
	}
	return false
}

// sameWriter reports whether two writers are the same one (writers that can't be compared aren't).
func sameWriter(a, b io.Writer) (same bool) {
	defer func() { _ = recover() }()
	return a == b
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

// TraceStat is what one kind of command cost in a trace.
type TraceStat struct {
	Kind  string
	Count int
	Time  time.Duration // all of them together
	Max   time.Duration
	Bytes int64 // printed, where counted
}

// TraceSummary is a trace's commands by kind, the costliest first, and the time they span.
type TraceSummary struct {
	Stats       []TraceStat
	First, Last time.Time
}

// SummarizeTrace reads a trace from since on (zero: all of it).
func SummarizeTrace(r io.Reader, since time.Time) TraceSummary {
	var sum TraceSummary
	by := map[string]*TraceStat{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		f := strings.Fields(line)
		open, end := strings.Index(line, "["), strings.Index(line, "]")
		if len(f) < 4 || open < 0 || end < open {
			continue
		}
		at, err := time.ParseInLocation(traceTime, f[0], time.Local)
		if err != nil || at.Before(since) {
			continue
		}
		ms, _ := strconv.Atoi(strings.TrimSuffix(f[1], "ms"))
		took := time.Duration(ms) * time.Millisecond
		kind := line[open+1 : end]
		st := by[kind]
		if st == nil {
			st = &TraceStat{Kind: kind}
			by[kind] = st
		}
		st.Count++
		st.Time += took
		st.Max = max(st.Max, took)
		st.Bytes += parseBytes(f[2])
		if sum.First.IsZero() {
			sum.First = at
		}
		sum.Last = at
	}
	for _, st := range by {
		sum.Stats = append(sum.Stats, *st)
	}
	slices.SortFunc(sum.Stats, func(a, b TraceStat) int { return cmp.Compare(b.Time, a.Time) })
	return sum
}

func parseBytes(s string) int64 {
	unit := int64(1)
	switch {
	case strings.HasSuffix(s, "MB"):
		unit, s = 1<<20, strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		unit, s = 1<<10, strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	default:
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(v * float64(unit))
}

// HumanBytes writes a size as the trace does.
func HumanBytes(n int64) string { return humanBytes(n) }

package image

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/runtime"
)

// tailLines is how much of a failed build's output its error shows.
const tailLines = 30

// BuildError is a failed build: what the runtime said last, and where the whole output is.
type BuildError struct {
	Image string
	Err   error
	Tail  []string // the last lines of output
	Log   string   // the full output
}

func (e *BuildError) Error() string {
	var s strings.Builder
	fmt.Fprintf(&s, "building %s failed (%v)", e.Image, e.Err)
	if len(e.Tail) > 0 {
		s.WriteString(":\n")
		for _, l := range e.Tail {
			s.WriteString("  " + l + "\n")
		}
	} else {
		s.WriteString("\n")
	}
	s.WriteString("full log: " + paths.Tilde(e.Log))
	return s.String()
}

func (e *BuildError) Unwrap() error { return e.Err }

// LogFile is where the output of an image's last build is kept.
func (b *Builder) LogFile(tag string) string {
	return filepath.Join(b.StateDir, "build", "logs", strings.NewReplacer(":", "-", "/", "-").Replace(tag)+".log")
}

// build runs one build. Its output goes to the log file, to Out (verbose) and line by line to the
// context's watcher; a failure carries the last lines. With files, they are the build context,
// written to a folder of this build's own (builds of different projects can run at once).
func (b *Builder) build(ctx context.Context, o runtime.BuildOptions, files map[string][]byte) error {
	if files != nil {
		dir, err := b.writeContext(files)
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		o.Context = dir
	}
	logPath := b.LogFile(o.Tag)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	log, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer log.Close()
	tail := &lineTail{max: tailLines}
	if fn := watcher(ctx); fn != nil {
		var done, total int
		tail.each = func(line string) {
			if d, t, ok := progressOf(line); ok {
				done, total = d, t
			}
			fn(Event{Line: line, Done: done, Total: total})
		}
	}
	writers := []io.Writer{log, tail}
	if b.Out != nil {
		writers = append(writers, b.Out)
	}
	o.Out = io.MultiWriter(writers...)
	err = b.Driver.Build(ctx, o)
	tail.flush()
	if err != nil {
		return &BuildError{Image: o.Tag, Err: err, Tail: tail.lines(), Log: logPath}
	}
	return nil
}

// stepRE matches a build step's first line in plain progress output: "#9 [stage-0 5/6] RUN …".
var stepRE = regexp.MustCompile(`^#\d+ \[[^\]]*?\b(\d+)/(\d+)\]`)

// progressOf reads how far a build is from a line of its output: step done of total.
func progressOf(line string) (done, total int, ok bool) {
	m := stepRE.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, false
	}
	done, _ = strconv.Atoi(m[1])
	total, _ = strconv.Atoi(m[2])
	return done, total, total > 0
}

// writeContext writes a build context to a new folder; the caller removes it.
func (b *Builder) writeContext(files map[string][]byte) (string, error) {
	root := filepath.Join(b.StateDir, "build", "ctx")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, "")
	if err != nil {
		return "", err
	}
	for n, data := range files {
		p := filepath.Join(dir, n)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// lineTail splits output into lines, keeps the last max and hands each to each (if set).
type lineTail struct {
	max  int
	each func(string)

	mu      sync.Mutex // stdout and stderr may write at once
	partial []byte
	last    []string
}

func (t *lineTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.partial = append(t.partial, p...)
	for {
		i := strings.IndexByte(string(t.partial), '\n')
		if i < 0 {
			break
		}
		t.add(string(t.partial[:i]))
		t.partial = t.partial[i+1:]
	}
	return len(p), nil
}

func (t *lineTail) flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.partial) > 0 {
		t.add(string(t.partial))
		t.partial = nil
	}
}

func (t *lineTail) add(line string) {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return
	}
	if t.each != nil {
		t.each(line)
	}
	t.last = append(t.last, line)
	if len(t.last) > t.max {
		t.last = t.last[len(t.last)-t.max:]
	}
}

func (t *lineTail) lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.last...)
}

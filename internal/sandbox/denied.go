package sandbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
	"github.com/jailwick/jailwick/internal/untrusted"
)

// A sandbox's egress proxy logs the hosts it refused, and each session has a proxy of its own:
// its log says which session asked. Proxy logs go with their containers (a session's when it
// ends), so what they show is kept in the project's data: one file per sandbox, a snapshot of
// its log merged with what the file had (counts only grow), so writing it again is harmless and
// the app and jw never write over each other's sandboxes.

// SessionDenied is what one sandbox was refused: a session's (Session, its sandbox's id), or
// the project's own (Session "": its shells and runs).
type SessionDenied struct {
	Session  string                  `json:"session,omitempty"`
	Worktree string                  `json:"worktree,omitempty"` // "" for the main checkout
	Name     string                  `json:"name,omitempty"`     // the Claude session's name, when known
	Hosts    map[string]DeniedCounts `json:"hosts"`
	Updated  time.Time               `json:"updated"`
}

// DeniedCounts is how often, and when, a sandbox was refused one host.
type DeniedCounts struct {
	Count int       `json:"count"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

// deniedKeep is how long a sandbox's record is kept after it last changed.
const deniedKeep = 30 * 24 * time.Hour

func deniedDir(project string) string { return filepath.Join(paths.ProjectData(project), "blocked") }

// deniedFile is a sandbox's record: a session's by its id; the project's own by its proxy
// container's creation (key "project-<unix time>"): a restart starts a new log, so a new record,
// and the counts of the one before stay as they were.
func deniedFile(project, key string) string { return filepath.Join(deniedDir(project), key+".json") }

// projectKey is the record key of the project's own proxy as it runs now ("project" when it
// can't be told).
func (m *Manager) projectKey(ctx context.Context, project string) string {
	cs, err := m.Driver.Containers(ctx, spec.Prefix+project)
	if err != nil {
		return "project"
	}
	return projectKeyOf(cs)
}

// projectKeyOf is projectKey for the project's containers, already listed.
func projectKeyOf(cs []runtime.Container) string {
	for _, c := range cs {
		if c.Service == spec.ProxyService && c.Labels[spec.LabelSession] == "" && !c.Created.IsZero() {
			return "project-" + strconv.FormatInt(c.Created.Unix(), 10)
		}
	}
	return "project"
}

// Denied reads what a running project's sandboxes were refused, records it by sandbox (see
// SessionDenied) and returns it merged by host, as proxy.ParseDenied does. names are the Claude
// session names by session id, where known, to keep with the record.
func (m *Manager) Denied(ctx context.Context, project string, names map[string]string) (map[string]*proxy.Denied, error) {
	all := map[string]*proxy.Denied{}
	one := func(sandbox, key, session, worktree string) error {
		acc, err := m.deniedIn(ctx, sandbox, key, project)
		if err != nil {
			return err
		}
		_ = recordDenied(project, key, session, worktree, names[session], acc) // a record is a bonus
		proxy.MergeDenied(all, acc)
		return nil
	}
	cs, err := m.Driver.Containers(ctx, spec.Prefix+project) // once, for the project's and its sessions'
	if err != nil {
		return all, err
	}
	var errs []error
	if err := one(spec.Prefix+project, projectKeyOf(cs), "", ""); err != nil {
		errs = append(errs, err)
	}
	boxes := m.boxesOf(ctx, project, cs)
	for _, b := range boxes {
		if err := one(spec.SessionName(project, b.ID), b.ID, b.ID, b.Worktree); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == len(boxes)+1 {
		return all, errors.Join(errs...)
	}
	return all, nil
}

// recordSessionDenied keeps what a session's sandbox was refused before the sandbox goes. Its
// worktree comes from the container's label (not Boxes: that ends finished sessions, here).
func (m *Manager) recordSessionDenied(ctx context.Context, project, id string) {
	worktree := ""
	if cs, err := m.Driver.Containers(ctx, spec.SessionName(project, id)); err == nil {
		for _, c := range cs {
			if w := c.Labels[spec.LabelWorktree]; w != "" {
				worktree = w
			}
		}
	}
	if acc, err := m.deniedIn(ctx, spec.SessionName(project, id), id, project); err == nil {
		_ = recordDenied(project, id, id, worktree, "", acc)
	}
}

// recordProjectDenied keeps what the project's own proxy was refused before a stop, restart or
// recreate removes its container, and its log with it.
func (m *Manager) recordProjectDenied(ctx context.Context, project string) {
	key := m.projectKey(ctx, project)
	if acc, err := m.deniedIn(ctx, spec.Prefix+project, key, project); err == nil {
		_ = recordDenied(project, key, "", "", "", acc)
	}
}

// DeniedRecorded is what a project's sandboxes were refused, from its records (running or not,
// sessions ended or not; see SessionDenied), merged by host as proxy.ParseDenied does.
func DeniedRecorded(project string) map[string]*proxy.Denied {
	all := map[string]*proxy.Denied{}
	recs, _ := DeniedBySession(project)
	for _, rec := range recs {
		one := map[string]*proxy.Denied{}
		for h, c := range rec.Hosts {
			one[h] = &proxy.Denied{Host: h, Count: c.Count, First: c.First, Last: c.Last, Projects: []string{project}}
		}
		proxy.MergeDenied(all, one)
	}
	for _, d := range all {
		d.Projects = slices.Compact(d.Projects)
	}
	return all
}

// deniedIn reads the hosts one sandbox's proxy refused.
func (m *Manager) deniedIn(ctx context.Context, sandbox, key, project string) (map[string]*proxy.Denied, error) {
	l := proxyLogOf(sandbox + "/" + key)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.asked = time.Now()
	if l.following {
		return l.totals(), nil
	}
	var buf bytes.Buffer
	o := runtime.LogOptions{Out: &buf, Timestamps: true}
	if !l.last.IsZero() {
		o.Since = l.last.Add(time.Nanosecond) // the lines after the last one read
	}
	if err := m.Driver.Logs(ctx, sandbox, spec.ProxyService, o); err != nil {
		return nil, err
	}
	lines, last := stripTimestamps(&buf)
	read := map[string]*proxy.Denied{}
	proxy.ParseDenied(lines, project, read)
	switch {
	case !last.IsZero():
		if o.Since.IsZero() { // a whole read starts the totals over
			l.acc = map[string]*proxy.Denied{}
		}
		addDenied(l.acc, read)
		l.last = last
	case o.Since.IsZero(): // a whole log without times: it is the totals, read whole every time
		l.acc = read
	default: // nothing new since the last line read
	}
	if m.feeds != nil && (!l.last.IsZero() || len(l.acc) == 0) { // streaming, and it knows where to go on
		l.follow(m, sandbox, project)
	}
	return l.totals(), nil
}

// totals is a copy of what the log said so far (l.mu held).
func (l *proxyLog) totals() map[string]*proxy.Denied {
	out := make(map[string]*proxy.Denied, len(l.acc))
	for h, d := range l.acc {
		c := *d
		out[h] = &c
	}
	return out
}

// follow keeps the totals current from the proxy's log as it grows (`logs -f` from the line after
// the last one read), until the proxy ends or nobody read the totals for idleFeed (l.mu held).
func (l *proxyLog) follow(m *Manager, sandbox, project string) {
	ctx, cancel := context.WithCancel(m.feeds.ctx)
	l.following = true
	var since time.Time
	if !l.last.IsZero() {
		since = l.last.Add(time.Nanosecond)
	}
	lines := &untrusted.LineWriter{Max: maxProxyLine, Line: func(line []byte) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.add(line, project)
	}}
	go func() {
		_ = m.Driver.Logs(ctx, sandbox, spec.ProxyService, runtime.LogOptions{Follow: true, Since: since, Timestamps: true, Out: lines})
		cancel()
		l.mu.Lock()
		l.following = false
		l.mu.Unlock()
	}()
	go stopWhenIdle(ctx, cancel, func() time.Time {
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.asked
	}, idleFeed)
}

// maxProxyLine bounds a proxy log line kept while it is read.
const maxProxyLine = 64 << 10

// add counts one line of the log into the totals (l.mu held).
func (l *proxyLog) add(line []byte, project string) {
	lines, last := stripTimestamps(bytes.NewReader(line))
	read := map[string]*proxy.Denied{}
	proxy.ParseDenied(lines, project, read)
	addDenied(l.acc, read)
	if last.After(l.last) {
		l.last = last
	}
}

// The app reads its proxies' logs every few seconds, and a proxy logs every request: reading all
// of each log every time cost more as the sandboxes ran. So the process keeps, for each proxy, what
// its log said so far and the time of the last line read, and asks only for the lines after it.
// A project's proxy is keyed by its creation (a restart starts a new log), a session's by its id.

type proxyLog struct {
	mu        sync.Mutex
	acc       map[string]*proxy.Denied // what the whole log said so far
	last      time.Time                // the last line read; zero: read it all next time
	following bool                     // a follower keeps acc current (streaming; see follow)
	asked     time.Time                // the last read, for the follower to stop when idle
	used      time.Time                // guarded by proxyLogs
}

var proxyLogs = struct {
	sync.Mutex
	by map[string]*proxyLog
}{by: map[string]*proxyLog{}}

// proxyLogIdle is how long a proxy's totals are kept without a read (its sandbox is gone).
const proxyLogIdle = 10 * time.Minute

func proxyLogOf(key string) *proxyLog {
	proxyLogs.Lock()
	defer proxyLogs.Unlock()
	now := time.Now()
	for k, l := range proxyLogs.by {
		if now.Sub(l.used) > proxyLogIdle {
			delete(proxyLogs.by, k)
		}
	}
	l := proxyLogs.by[key]
	if l == nil {
		l = &proxyLog{acc: map[string]*proxy.Denied{}}
		proxyLogs.by[key] = l
	}
	l.used = now
	return l
}

// stripTimestamps takes the time off each line of a log read with timestamps, returning the
// lines and the last time (zero when the lines had none).
func stripTimestamps(r io.Reader) (io.Reader, time.Time) {
	var out bytes.Buffer
	var last time.Time
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if ts, rest, ok := strings.Cut(line, " "); ok {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				if t.After(last) {
					last = t
				}
				line = rest
			}
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return &out, last
}

// addDenied adds what a read found to the totals of one log.
func addDenied(acc, read map[string]*proxy.Denied) {
	for h, d := range read {
		have := acc[h]
		if have == nil {
			c := *d
			acc[h] = &c
			continue
		}
		have.Count += d.Count
		if d.Last.After(have.Last) {
			have.Last = d.Last
		}
		if !d.First.IsZero() && (have.First.IsZero() || d.First.Before(have.First)) {
			have.First = d.First
		}
	}
}

// recordDenied merges a sandbox's refusals into its record (written only when it changes) and
// drops the project's records untouched for longer than deniedKeep.
func recordDenied(project, key, session, worktree, name string, acc map[string]*proxy.Denied) error {
	file := deniedFile(project, key)
	rec := SessionDenied{Session: session, Hosts: map[string]DeniedCounts{}}
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &rec)
		if rec.Hosts == nil {
			rec.Hosts = map[string]DeniedCounts{}
		}
	}
	changed := false
	if worktree != "" && rec.Worktree != worktree {
		rec.Worktree, changed = worktree, true
	}
	if name != "" && rec.Name != name {
		rec.Name, changed = name, true
	}
	for h, d := range acc {
		c := rec.Hosts[h]
		next := c
		next.Count = max(c.Count, d.Count)
		if d.Last.After(c.Last) {
			next.Last = d.Last
		}
		if !d.First.IsZero() && (c.First.IsZero() || d.First.Before(c.First)) {
			next.First = d.First
		}
		if next != c {
			rec.Hosts[h], changed = next, true
		}
	}
	if !changed || len(rec.Hosts) == 0 {
		return nil
	}
	rec.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(deniedDir(project), 0o755); err != nil {
		return err
	}
	pruneDenied(project)
	return config.WriteFileAtomic(file, append(data, '\n'), 0o644)
}

func pruneDenied(project string) {
	entries, _ := os.ReadDir(deniedDir(project))
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > deniedKeep {
			_ = os.Remove(filepath.Join(deniedDir(project), e.Name()))
		}
	}
}

// DeniedBySession reads a project's records: what each of its sandboxes was refused, the most
// recently changed first.
func DeniedBySession(project string) ([]SessionDenied, error) {
	entries, err := os.ReadDir(deniedDir(project))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []SessionDenied
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(deniedDir(project), e.Name()))
		if err != nil {
			continue
		}
		var rec SessionDenied
		if json.Unmarshal(data, &rec) == nil && len(rec.Hosts) > 0 {
			out = append(out, rec)
		}
	}
	slices.SortFunc(out, func(a, b SessionDenied) int { return b.Updated.Compare(a.Updated) })
	return out, nil
}

// Discarding clears refused hosts from the app's list until the sandbox asks for them again: a
// host stays discarded while its last refusal is no later than when it was discarded. Kept in
// the project's data (not the browser's storage, which a new app port or browser loses).
func discardFile(project string) string {
	return filepath.Join(paths.ProjectData(project), "blocked-discarded.json")
}

// discard is one discarded host: its last refusal when discarded (Last), and when (At).
type discard struct {
	Last time.Time `json:"last"`
	At   time.Time `json:"at"`
}

func readDiscards(project string) map[string]discard {
	out := map[string]discard{}
	if data, err := os.ReadFile(discardFile(project)); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

// Discarded is a project's discarded hosts: each with its last refusal when discarded.
func Discarded(project string) map[string]time.Time {
	out := map[string]time.Time{}
	for h, d := range readDiscards(project) {
		out[h] = d.Last
	}
	return out
}

// Discard discards hosts (each with its last refusal), or with restore brings them back.
// Discards made longer than deniedKeep ago go, like the records they hide.
func Discard(project string, hosts map[string]time.Time, restore bool) error {
	all := readDiscards(project)
	now := time.Now().UTC()
	for h, last := range hosts {
		if restore {
			delete(all, h)
		} else {
			all[h] = discard{Last: last, At: now}
		}
	}
	for h, d := range all {
		if now.Sub(d.At) > deniedKeep {
			delete(all, h)
		}
	}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.ProjectData(project), 0o755); err != nil {
		return err
	}
	return config.WriteFileAtomic(discardFile(project), append(data, '\n'), 0o644)
}

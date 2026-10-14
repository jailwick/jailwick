package sandbox

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/spec"
)

func TestDeniedRecordsEachSession(t *testing.T) {
	line := func(ts, host string) string {
		return ts + " 0 172.18.0.3 TCP_DENIED/403 3893 CONNECT " + host + ":443 - HIER_NONE/- text/html\n"
	}
	cs := append(projectContainers(), box("sess01", "fix-auth", "running", time.Now())...)
	d := &fakeDriver{containers: cs, logs: map[string]string{
		spec.Prefix + "api":               line("1760435900.000", "a.example"),
		spec.SessionName("api", "sess01"): line("1760435901.000", "b.example") + line("1760435905.000", "b.example"),
	}}
	m := testManager(t, d)

	all, err := m.Denied(context.Background(), "api", map[string]string{"sess01": "stripe-webhooks"})
	if err != nil {
		t.Fatal(err)
	}
	if all["a.example"].Count != 1 || all["b.example"].Count != 2 {
		t.Fatalf("merged: %+v %+v", all["a.example"], all["b.example"])
	}
	// Again: the same log, the same counts.
	if _, err := m.Denied(context.Background(), "api", nil); err != nil {
		t.Fatal(err)
	}
	recs, err := DeniedBySession("api")
	if err != nil || len(recs) != 2 {
		t.Fatalf("%v %+v", err, recs)
	}
	var sess SessionDenied
	for _, r := range recs {
		if r.Session == "sess01" {
			sess = r
		}
	}
	b := sess.Hosts["b.example"]
	if sess.Worktree != "fix-auth" || sess.Name != "stripe-webhooks" || b.Count != 2 ||
		b.First.Unix() != 1760435901 || b.Last.Unix() != 1760435905 || len(sess.Hosts) != 1 {
		t.Fatalf("the session's record: %+v", sess)
	}

	// Ending the session keeps what its proxy saw since.
	d.logs[spec.SessionName("api", "sess01")] += line("1760435990.000", "c.example")
	if err := m.EndSession(context.Background(), "api", "sess01"); err != nil {
		t.Fatal(err)
	}
	recs, _ = DeniedBySession("api")
	for _, r := range recs {
		if r.Session == "sess01" && (r.Hosts["c.example"].Count != 1 || r.Name != "stripe-webhooks") {
			t.Fatalf("after the end: %+v", r)
		}
	}
}

// A stop (or restart) removes the proxies and their logs: what they refused stays listed.
func TestDeniedSurvivesAStop(t *testing.T) {
	line := func(ts, host string) string {
		return ts + " 0 172.18.0.3 TCP_DENIED/403 3893 CONNECT " + host + ":443 - HIER_NONE/- text/html\n"
	}
	cs := append(projectContainers(), box("sess02", "", "running", time.Now())...)
	for i := range cs {
		cs[i].Created = time.Unix(1760000000, 0)
	}
	d := &fakeDriver{containers: cs, logs: map[string]string{
		spec.Prefix + "web":               line("1760435900.000", "x.example"),
		spec.SessionName("web", "sess02"): line("1760435901.000", "y.example"),
	}}
	for i := range d.containers { // the project is web here
		d.containers[i].Labels[spec.LabelProject] = "web"
		if d.containers[i].Labels[spec.LabelSession] != "" {
			d.containers[i].Name = strings.Replace(d.containers[i].Name, "jailwick-api", "jailwick-web", 1)
		}
	}
	m := testManager(t, d)
	if err := m.Down(context.Background(), "web", false); err != nil {
		t.Fatal(err)
	}
	d.logs = map[string]string{} // gone with the containers
	got := DeniedRecorded("web")
	if got["x.example"] == nil || got["x.example"].Count != 1 {
		t.Fatalf("the project's own proxy: %+v", got)
	}

	// Discarding hides a host until it is refused again; restoring brings it back.
	last := got["x.example"].Last
	if err := Discard("web", map[string]time.Time{"x.example": last}, false); err != nil {
		t.Fatal(err)
	}
	if !Discarded("web")["x.example"].Equal(last) {
		t.Fatalf("discarded: %v", Discarded("web"))
	}
	if err := Discard("web", map[string]time.Time{"x.example": {}}, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := Discarded("web")["x.example"]; ok {
		t.Fatal("restored, still discarded")
	}
}

// forgetProxyLogs drops the totals kept from proxy logs (a test's own).
func forgetProxyLogs() {
	proxyLogs.Lock()
	defer proxyLogs.Unlock()
	proxyLogs.by = map[string]*proxyLog{}
}

// After the first read of a proxy's log, a read asks only for the lines after the last one read,
// and adds them: counts are right, never counted twice.
func TestDeniedReadsOnlyNewLines(t *testing.T) {
	line := func(host string) string {
		return "1760435900.000 0 172.18.0.3 TCP_DENIED/403 3893 CONNECT " + host + ":443 - HIER_NONE/- text/html\n"
	}
	d := &fakeDriver{containers: projectContainers(), logs: map[string]string{
		spec.Prefix + "api": line("a.example") + line("a.example"),
	}}
	m := testManager(t, d)
	ctx := context.Background()
	count := func() int {
		t.Helper()
		all, err := m.Denied(ctx, "api", nil)
		if err != nil {
			t.Fatal(err)
		}
		if all["a.example"] == nil {
			return 0
		}
		return all["a.example"].Count
	}
	if n := count(); n != 2 {
		t.Fatalf("first read: %d", n)
	}
	if n := count(); n != 2 {
		t.Fatalf("nothing new: %d", n)
	}
	d.mu.Lock()
	d.logs[spec.Prefix+"api"] += line("a.example") + line("b.example")
	d.mu.Unlock()
	if n := count(); n != 3 {
		t.Fatalf("one more: %d", n)
	}
	d.mu.Lock()
	reads := d.logReads
	d.mu.Unlock()
	if len(reads) != 3 || !reads[0].Since.IsZero() || !reads[0].Timestamps {
		t.Fatalf("the first read is whole, with timestamps: %+v", reads)
	}
	if want := fakeLogStart.Add(time.Second + time.Nanosecond); !reads[1].Since.Equal(want) || !reads[2].Since.Equal(want) {
		t.Fatalf("later reads start after the last line read (%v): %v, %v", want, reads[1].Since, reads[2].Since)
	}
}

// A log read without times is read whole every time, and replaces the totals instead of adding.
func TestStripTimestamps(t *testing.T) {
	r, last := stripTimestamps(strings.NewReader("2026-10-14T09:00:01.5Z x y\n2026-10-14T09:00:00Z z\nno time here\n"))
	text, _ := io.ReadAll(r)
	if string(text) != "x y\nz\nno time here\n" || !last.Equal(time.Date(2026, 10, 14, 9, 0, 1, 5e8, time.UTC)) {
		t.Fatalf("%q %v", text, last)
	}
	if _, last := stripTimestamps(strings.NewReader("a b\n")); !last.IsZero() {
		t.Fatal("no times")
	}
}

// Reading what a project was refused lists its containers once, for its proxy and its sessions'.
func TestDeniedListsOnce(t *testing.T) {
	cs := append(projectContainers(), box("sess01", "", "running", time.Now())...)
	d := &fakeDriver{containers: cs, logs: map[string]string{spec.Prefix + "api": "", spec.SessionName("api", "sess01"): ""}}
	if _, err := testManager(t, d).Denied(context.Background(), "api", nil); err != nil {
		t.Fatal(err)
	}
	if d.listings != 1 {
		t.Fatalf("listed %d times", d.listings)
	}
}

// While streaming, after the first read the proxy's log is followed: new lines are counted as
// they come, and reads come from memory.
func TestDeniedFollowsTheLog(t *testing.T) {
	line := func(host string) string {
		return "1760435900.000 0 172.18.0.3 TCP_DENIED/403 3893 CONNECT " + host + ":443 - HIER_NONE/- text/html\n"
	}
	feed := make(chan string)
	d := &fakeDriver{containers: projectContainers(),
		logs:     map[string]string{spec.Prefix + "api": line("a.example") + line("a.example")},
		logFeeds: map[string]chan string{spec.Prefix + "api": feed}}
	m := testManager(t, d)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Stream(ctx, nil)
	count := func(host string) int {
		t.Helper()
		all, err := m.Denied(ctx, "api", nil)
		if err != nil {
			t.Fatal(err)
		}
		if all[host] == nil {
			return 0
		}
		return all[host].Count
	}
	if n := count("a.example"); n != 2 {
		t.Fatalf("first read: %d", n)
	}
	feed <- line("a.example")
	feed <- line("b.example")
	deadline := time.Now().Add(2 * time.Second)
	for count("a.example") != 3 || count("b.example") != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("followed: a %d, b %d", count("a.example"), count("b.example"))
		}
		time.Sleep(10 * time.Millisecond)
	}
	d.mu.Lock()
	reads := d.logReads
	d.mu.Unlock()
	if len(reads) != 2 || reads[0].Follow || !reads[1].Follow || !reads[1].Since.Equal(fakeLogStart.Add(time.Second+time.Nanosecond)) {
		t.Fatalf("one read, then one follow from after it: %+v", reads)
	}
}

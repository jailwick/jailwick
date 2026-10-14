package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		line           string
		relist, images bool
	}{
		{`{"Type":"container","Action":"start","Actor":{"Attributes":{"dev.jailwick.project":"api"}}}`, true, false},
		{`{"Type":"container","Action":"health_status: healthy","Actor":{"Attributes":{"dev.jailwick.project":"api"}}}`, true, false},
		{`{"Type":"container","status":"die","Actor":{"Attributes":{"dev.jailwick.project":"api"}}}`, true, false}, // older runtimes
		{`{"Type":"container","Action":"exec_start: jailwick-agent status","Actor":{"Attributes":{"dev.jailwick.project":"api"}}}`, false, false},
		{`{"Type":"container","Action":"start","Actor":{"Attributes":{"name":"someone-elses"}}}`, false, false},
		{`{"Type":"image","Action":"tag","Actor":{"Attributes":{"name":"jailwick-sandbox:abc"}}}`, false, true},
		{`{"Type":"network","Action":"connect"}`, false, false},
		{`not json`, false, false},
	} {
		var e event
		_ = json.Unmarshal([]byte(c.line), &e)
		if r, i := classify(e); r != c.relist || i != c.images {
			t.Errorf("%s: relist %v images %v", c.line, r, i)
		}
	}
}

// fakeDocker writes a script standing in for docker: events streams what is put in emit until
// stop exists; ps and inspect answer from files; every call is recorded.
func fakeDocker(t *testing.T) (dir, bin string) {
	t.Helper()
	if goruntime.GOOS == "windows" {
		t.Skip("a shell script stands in for docker")
	}
	dir = t.TempDir()
	bin = filepath.Join(dir, "docker")
	script := `#!/bin/sh
d=` + dir + `
echo "$1" >> $d/calls
case "$1" in
events)
	[ -e $d/noevents ] && { echo "unknown command" >&2; exit 1; }
	while [ ! -e $d/stop ]; do
		if [ -e $d/emit ]; then cat $d/emit; rm -f $d/emit; fi
		sleep 0.05
	done ;;
ps) cat $d/ids ;;
inspect) cat $d/inspect.json ;;
image) echo sha256:abc ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, bin
}

func write(t *testing.T, file, text string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func calls(dir, cmd string) int {
	data, _ := os.ReadFile(filepath.Join(dir, "calls"))
	return strings.Count(string(data), cmd+"\n")
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for range 100 {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("never: %s", what)
}

const oneContainer = `[{"Name":"/jailwick-api-sandbox-1","State":{"Status":"running"},"Config":{"Labels":{"dev.jailwick.project":"api","com.docker.compose.service":"sandbox"}}}]`
const twoContainers = `[{"Name":"/jailwick-api-sandbox-1","State":{"Status":"running"},"Config":{"Labels":{"dev.jailwick.project":"api","com.docker.compose.service":"sandbox"}}},
{"Name":"/jailwick-api--k3-session-1","State":{"Status":"running"},"Config":{"Labels":{"dev.jailwick.project":"api","dev.jailwick.session":"k3","com.docker.compose.service":"session"}}}]`

// While the events stream runs, listings and image lookups come from memory, and a container's
// change lists again; when it stops, the runtime is asked again.
func TestWatchKeepsListingsCurrent(t *testing.T) {
	dir, bin := fakeDocker(t)
	write(t, filepath.Join(dir, "ids"), "c1\n")
	write(t, filepath.Join(dir, "inspect.json"), oneContainer)
	d := newCLI("docker", bin, dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []ContainerEvent
	var evMu sync.Mutex
	go d.Watch(ctx, func(e ContainerEvent) { evMu.Lock(); events = append(events, e); evMu.Unlock() })
	eventually(t, "a first listing", func() bool { _, ok := d.cached(); return ok })

	ps := calls(dir, "ps")
	cs, err := d.AllContainers(ctx)
	if err != nil || len(cs) != 1 || calls(dir, "ps") != ps {
		t.Fatalf("from memory: %v %+v (ps %d → %d)", err, cs, ps, calls(dir, "ps"))
	}

	// A session starts: listed again.
	write(t, filepath.Join(dir, "ids"), "c1\nc2\n")
	write(t, filepath.Join(dir, "inspect.json"), twoContainers)
	write(t, filepath.Join(dir, "emit"), `{"Type":"container","Action":"start","Actor":{"Attributes":{"dev.jailwick.project":"api"}}}`+"\n")
	eventually(t, "the new session", func() bool { cs, _ := d.AllContainers(ctx); return len(cs) == 2 })
	if cs, _ := d.Containers(ctx, "jailwick-api--k3"); len(cs) != 1 || cs[0].Service != "session" {
		t.Fatalf("a session's sandbox: %+v", cs)
	}
	if cs, _ := d.Containers(ctx, "jailwick-api"); len(cs) != 2 {
		t.Fatalf("the project's: %+v", cs)
	}

	// An exec changes nothing: no listing.
	ps = calls(dir, "ps")
	write(t, filepath.Join(dir, "emit"), `{"Type":"container","Action":"exec_start: x","Actor":{"Attributes":{"dev.jailwick.project":"api"}}}`+"\n")
	time.Sleep(400 * time.Millisecond)
	if calls(dir, "ps") != ps {
		t.Fatal("an exec listed again")
	}

	evMu.Lock()
	told := slices.Clone(events)
	evMu.Unlock()
	if len(told) != 2 || told[0].Action != "start" || !strings.HasPrefix(told[1].Action, "exec_start") {
		t.Fatalf("told: %+v", told)
	}

	// Image lookups are kept until an image event.
	d.ImageID(ctx, "jailwick-sandbox:abc")
	d.ImageID(ctx, "jailwick-sandbox:abc")
	if n := calls(dir, "image"); n != 1 {
		t.Fatalf("image asked %d times", n)
	}
	write(t, filepath.Join(dir, "emit"), `{"Type":"image","Action":"tag"}`+"\n")
	eventually(t, "the image forgotten", func() bool { _, ok := d.cachedImage("jailwick-sandbox:abc"); return !ok })

	// The stream ends: the runtime is asked again.
	write(t, filepath.Join(dir, "stop"), "")
	eventually(t, "the listing dropped", func() bool { _, ok := d.cached(); return !ok })
	ps = calls(dir, "ps")
	d.AllContainers(ctx)
	if calls(dir, "ps") != ps+1 {
		t.Fatal("no stream: asked directly")
	}
}

// A runtime without events: Watch gives up and the driver asks directly.
func TestWatchWithoutEvents(t *testing.T) {
	dir, bin := fakeDocker(t)
	write(t, filepath.Join(dir, "noevents"), "")
	d := newCLI("docker", bin, dir)
	if err := d.Watch(context.Background(), nil); !errors.Is(err, ErrNoEvents) {
		t.Fatalf("got %v", err)
	}
	if _, ok := d.cached(); ok {
		t.Fatal("no listing kept")
	}
}

package runtime

import (
	"bytes"
	"os/exec"
	goruntime "runtime"
	"strings"
	"testing"
	"time"
)

func TestTraceKind(t *testing.T) {
	for want, args := range map[string][]string{
		"ps":                         {"docker", "ps", "-a"},
		"exec jailwick-agent status": {"docker", "exec", "-u", "node", "-w", "/code", "jailwick-api-sandbox-1", "/opt/jailwick/bin/jailwick-agent", "status", "-json"},
		"exec sh":                    {"docker", "exec", "jailwick-api--k3-egress-proxy-1", "sh", "-c", "cat /x"},
		"logs":                       {"docker", "logs", "--timestamps", "jailwick-api-egress-proxy-1"},
		"compose up":                 {"docker", "compose", "-p", "jailwick-api", "-f", "/x.yml", "up", "-d"},
	} {
		if got := TraceKind(args); got != want {
			t.Errorf("%v: %q, want %q", args, got, want)
		}
	}
}

func TestSummarizeTrace(t *testing.T) {
	log := strings.Join([]string{
		"2026-10-14T10:00:00.000      40ms    1.0KB  [ps]                         docker ps",
		"2026-10-14T10:00:05.000     300ms    2.0MB  [logs]                       docker logs x",
		"2026-10-14T10:01:00.000     500ms    2.0MB  [logs]                       docker logs x",
		"2026-10-14T10:02:00.000      60ms        -  [exec jailwick-agent status] docker exec …",
		"garbage",
	}, "\n")
	sum := SummarizeTrace(strings.NewReader(log), time.Time{})
	if len(sum.Stats) != 3 || sum.Stats[0].Kind != "logs" || sum.Stats[0].Count != 2 ||
		sum.Stats[0].Time != 800*time.Millisecond || sum.Stats[0].Max != 500*time.Millisecond || sum.Stats[0].Bytes != 4<<20 {
		t.Fatalf("%+v", sum.Stats)
	}
	if sum.Last.Sub(sum.First) != 2*time.Minute {
		t.Errorf("span %v", sum.Last.Sub(sum.First))
	}
	since, _ := time.ParseInLocation(traceTime, "2026-10-14T10:00:30.000", time.Local)
	if s := SummarizeTrace(strings.NewReader(log), since); len(s.Stats) != 2 {
		t.Errorf("since: %+v", s.Stats)
	}
}

// Traced, a command whose output and errors share a writer keeps every line of both.
func TestTraceKeepsSharedOutput(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	t.Setenv(TraceEnv, "1")
	d := newCLI("docker", "sh", t.TempDir())
	var buf bytes.Buffer
	cmd := exec.Command("sh", "-c", `for i in $(seq 1 3000); do echo "out $i"; echo "err $i" >&2; done`)
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := d.do(cmd); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "\n"); n != 6000 {
		t.Fatalf("%d lines of 6000", n)
	}
}

package sandbox

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/spec"
)

func TestContainerChanges(t *testing.T) {
	old := spec.Container{
		Name: "claude", Image: "img:1",
		Env: map[string]string{"A": "1", "GONE": "x", "API_TOKEN": "t1", "DATABASE_URL": "postgres://app:pw1@postgres/app"},
		Mounts: []spec.Mount{
			{Type: spec.Bind, Source: "/s/bin/jailwick-agent-arm64-aaaa", Target: spec.AgentPath, ReadOnly: true},
			{Type: spec.Bind, Source: "/w/api", Target: "/w/api", ReadOnly: true},
			{Type: spec.Bind, Source: "/w/old", Target: "/w/old", ReadOnly: true},
		},
		Secrets: map[string]string{"GH_TOKEN": "g1", "KEPT": "k"},
	}
	want := old
	want.Image = "img:2"
	want.Env = map[string]string{"A": "2", "NEW": "y", "API_TOKEN": "t2", "DATABASE_URL": "postgres://app:pw2@postgres/shop"}
	want.Mounts = []spec.Mount{
		{Type: spec.Bind, Source: "/s/bin/jailwick-agent-arm64-bbbb", Target: spec.AgentPath, ReadOnly: true},
		{Type: spec.Bind, Source: "/w/api", Target: "/w/api"},
		{Type: spec.Bind, Source: "/w/web", Target: "/w/web", ReadOnly: true},
	}
	want.Secrets = map[string]string{"GH_TOKEN": "g2", "KEPT": "k", "ANTHROPIC_API_KEY": "a"}

	infos := map[string]*assets.AgentInfo{
		"/s/bin/jailwick-agent-arm64-aaaa": {Built: "2026-10-13", Changes: []assets.AgentChange{{Commit: "c1", Text: "Ports: one"}}},
		"/s/bin/jailwick-agent-arm64-bbbb": {Release: "0.4.0", Built: "2026-10-15", Changes: []assets.AgentChange{
			{Commit: "c2", Text: "Worktrees: two"}, {Commit: "c1", Text: "Ports: one"}}},
	}
	got := containerChanges(&old, secretDigests(old.Secrets), want, func(f string) *assets.AgentInfo { return infos[f] })
	wantChanges := []Change{
		{Container: "claude", What: "image", Before: "img:1", After: "img:2"},
		{Container: "claude", What: "environment", Item: "A", Before: "1", After: "2"},
		{Container: "claude", What: "environment", Item: "API_TOKEN", Before: "(hidden)", After: "(hidden, changed)"},
		{Container: "claude", What: "environment", Item: "DATABASE_URL", Before: "postgres://app:xxxxx@postgres/app", After: "postgres://app:xxxxx@postgres/shop"},
		{Container: "claude", What: "environment", Item: "GONE", Before: "x"},
		{Container: "claude", What: "environment", Item: "NEW", After: "y"},
		{Container: "claude", What: whatAgent, Before: "jailwick build of 13 Oct 2026", After: "jailwick 0.4.0 (15 Oct 2026)",
			NotesTitle: "What changed in the agent", Notes: []string{"Worktrees: two"}, Detail: "agent aaaa → bbbb"},
		{Container: "claude", What: whatMount, Item: "/w/api", Before: "/w/api (read-only)", After: "/w/api"},
		{Container: "claude", What: whatMount, Item: "/w/web", After: "/w/web (read-only)"},
		{Container: "claude", What: whatMount, Item: "/w/old", Before: "/w/old (read-only)"},
		{Container: "claude", What: whatSecret, Item: "ANTHROPIC_API_KEY", After: "set"},
		{Container: "claude", What: whatSecret, Item: "GH_TOKEN", Before: "set", After: "changed"},
	}
	if !slices.EqualFunc(got, wantChanges, func(a, b Change) bool { return reflect.DeepEqual(a, b) }) {
		t.Fatalf("changes:\n%+v\nwant\n%+v", got, wantChanges)
	}
	if s := summarize(got); !slices.Equal(s, []string{"claude: environment, image, secrets changed; jailwick update: the sandbox helper changed; " +
		"mount /w/api changed; new mount /w/web; removed mount /w/old"}) {
		t.Fatalf("summary: %q", s)
	}
}

func TestChangesWithoutARecord(t *testing.T) {
	c := spec.Container{Name: "postgres", Secrets: map[string]string{"P": "1"}}
	// Secrets differ but weren't recorded (an older applied file): only the hash knows.
	none := func(string) *assets.AgentInfo { return nil }
	if got := summarize(containerChanges(&c, nil, c, none)); !slices.Equal(got, []string{"postgres: secrets changed"}) {
		t.Fatalf("%q", got)
	}
	if got := summarize(containerChanges(nil, nil, c, none)); !slices.Equal(got, []string{"postgres: changed (the running one wasn't recorded)"}) {
		t.Fatalf("%q", got)
	}
}

// The applied record keeps secret digests, never values, and reads back.
func TestAppliedRecord(t *testing.T) {
	sb := &spec.Sandbox{Project: "api", Containers: []spec.Container{{Name: "claude", Secrets: map[string]string{"GH_TOKEN": "s3cret"}}}}
	data, err := json.Marshal(applied{Sandbox: sb, SecretDigests: map[string]map[string]string{"claude": secretDigests(sb.Containers[0].Secrets)}})
	if err != nil || strings.Contains(string(data), "s3cret") {
		t.Fatalf("the record holds a secret: %s %v", data, err)
	}
	var back applied
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if c, d := back.container("claude"); c == nil || d["GH_TOKEN"] == "" {
		t.Fatalf("read back: %+v %v", c, d)
	}
}

// The running agent's build is unknown (written before agents had build info): its latest changes show.
func TestAgentChangeUnknownBefore(t *testing.T) {
	now := &assets.AgentInfo{Built: "2026-10-15", Changes: []assets.AgentChange{{Text: "a"}, {Text: "b"}, {Text: "c"}, {Text: "d"}}}
	c := agentChange("claude", "/x-1", "/x-2", func(f string) *assets.AgentInfo {
		if f == "/x-2" {
			return now
		}
		return nil
	})
	if c.Before != "an earlier jailwick build" || c.NotesTitle != "Latest changes in the agent" || len(c.Notes) != latestShown {
		t.Fatalf("%+v", c)
	}
}

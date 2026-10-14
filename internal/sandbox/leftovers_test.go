package sandbox

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

// The api sandbox runs without its project file (removed, or left by a test); web is a project.
func TestLeftovers(t *testing.T) {
	created := time.Date(2026, 10, 13, 4, 20, 0, 0, time.UTC)
	cs := append(projectContainers(), box("live01", "feat", "running", created)...)
	cs = append(cs, runtime.Container{Name: MainContainer("web"), Service: spec.SandboxService, State: "running",
		Labels: map[string]string{spec.LabelProject: "web"}})
	d := &fakeDriver{containers: cs, status: map[string]string{
		MainContainer("api"): `{"sessions":[{"pid":39,"dir":"/x","started":"2026-10-13T04:20:00Z","name":"refactor-auth-middleware"}]}`,
	}}
	m := &Manager{Store: storeWith(t, "web"), Driver: d, StateDir: t.TempDir()}
	ctx := context.Background()

	ls, err := m.Leftovers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 1 || ls[0].Name != "api" || len(ls[0].Containers) != 4 || !ls[0].Created.Equal(created) {
		t.Fatalf("leftovers: %+v", ls)
	}
	if len(ls[0].Sessions) != 1 || ls[0].Sessions[0].PID != 39 || ls[0].Sessions[0].Name != "refactor-auth-middleware" {
		t.Fatalf("a leftover's sessions, for a look before removing it: %+v", ls[0].Sessions)
	}

	if _, err := m.RemoveLeftover(ctx, "web", RemoveOptions{}); err == nil || !strings.Contains(err.Error(), "is a project") {
		t.Fatalf("a project isn't a leftover: %v", err)
	}
	if _, err := m.RemoveLeftover(ctx, "api", RemoveOptions{}); !errors.Is(err, ErrSessionsRunning) || len(d.downs) != 0 {
		t.Fatalf("its sessions run: removing it must be asked for (%v, downs %v)", err, d.downs)
	}
	if _, err := m.RemoveLeftover(ctx, "api", RemoveOptions{Sessions: true}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.downs, []string{spec.SessionName("api", "live01"), spec.Prefix + "api"}) {
		t.Fatalf("its session sandboxes, then itself: %v", d.downs)
	}
}

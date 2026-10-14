package sandbox

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

var poolsFull = errors.New("failed to create network jailwick-new_sandbox: Error response from daemon: all predefined address pools have been fully subnetted")

func TestNoNetworks(t *testing.T) {
	for msg, want := range map[string]bool{
		poolsFull.Error(): true,
		"could not find an available, non-overlapping IPv4 address pool among the defaults to assign to the network": true,
		"bind source path does not exist": false,
	} {
		if noNetworks(errors.New(msg)) != want {
			t.Errorf("%q", msg)
		}
	}
	if noNetworks(nil) {
		t.Error("nil")
	}
}

// Unused are jailwick's networks whose sandbox has no container left: a stopped project's (its
// containers still there) and other tools' networks are kept.
func TestUnusedNetworks(t *testing.T) {
	cs := projectContainers()
	for i := range cs {
		cs[i].Labels["com.docker.compose.project"] = "jailwick-api" // stopped or running: it has containers
	}
	d := &fakeDriver{containers: cs, networks: []runtime.Network{
		{Name: "jailwick-api_sandbox", Project: "jailwick-api", Addressed: true},
		{Name: "jailwick-gone_sandbox", Project: "jailwick-gone", Addressed: true},
		{Name: "jailwick-api--k3x9q2_proxy", Project: "jailwick-api--k3x9q2", Addressed: true}, // a session ended badly
		{Name: "myapp_default", Project: "myapp", Addressed: true},
		{Name: "bridge", Addressed: true},
	}}
	m := testManager(t, d)
	removed, err := m.PruneNetworks(context.Background())
	if err != nil || !slices.Equal(removed, []string{"jailwick-gone_sandbox", "jailwick-api--k3x9q2_proxy"}) {
		t.Fatalf("removed %v, %v", removed, err)
	}
}

// A start the runtime refuses for want of addresses removes jailwick's unused networks and tries
// once more; with none to remove, it says what to do.
func TestUpRecoversNetworks(t *testing.T) {
	ctx := context.Background()
	d := &fakeDriver{containers: projectContainers(), upErr: poolsFull, upFails: 1,
		networks: []runtime.Network{{Name: "jailwick-gone_sandbox", Project: "jailwick-gone", Addressed: true}}}
	if err := testManager(t, d).up(ctx, spec.Build(prepared().Inputs), runtime.UpOptions{}); err != nil || len(d.ups) != 2 || len(d.removedNets) != 1 {
		t.Fatalf("recovered: %v (ups %v, removed %v)", err, d.ups, d.removedNets)
	}

	d = &fakeDriver{containers: projectContainers(), upErr: poolsFull}
	err := testManager(t, d).up(ctx, spec.Build(prepared().Inputs), runtime.UpOptions{})
	if !errors.Is(err, ErrNoNetworks) || !errors.Is(err, poolsFull) || len(d.ups) != 1 {
		t.Fatalf("nothing to remove: %v (ups %v)", err, d.ups)
	}
}

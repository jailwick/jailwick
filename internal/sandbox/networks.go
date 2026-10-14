package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

// Every network takes an address range from the runtime's pools, and with their defaults Docker
// has room for about 30 networks, shared by everything on the machine. A project uses 3 (its
// services, its proxy's, the way out) and each session 1 (its own proxy's). When they run out, a
// start fails; jailwick then removes its own unused networks and tries once more, and otherwise
// says what to do.

// ErrNoNetworks means the runtime has no network address ranges left.
var ErrNoNetworks = errors.New("the container runtime has no network addresses left")

// PoolsDoc explains giving the runtime more address ranges, provider by provider.
const PoolsDoc = "https://github.com/jailwick/jailwick/blob/main/docs/NETWORK-POOLS.md"

// DefaultNetworks is about how many networks the runtime's default address pools hold.
const DefaultNetworks = 30

// noNetworks reports whether err is the runtime saying its address pools are used up (Docker's
// wordings, old and new).
func noNetworks(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "fully subnetted") ||
		(strings.Contains(msg, "non-overlapping") && strings.Contains(msg, "address pool"))
}

// up runs the runtime's Up; when it has no network addresses left, jailwick's unused networks
// are removed and Up tried once more.
func (m *Manager) up(ctx context.Context, sb *spec.Sandbox, opts runtime.UpOptions) error {
	err := m.Driver.Up(ctx, sb, opts)
	if !noNetworks(err) {
		return err
	}
	if removed, _ := m.PruneNetworks(ctx); len(removed) > 0 {
		if m.Log != nil {
			m.Log("The runtime had no network addresses left: removed %d unused jailwick network(s), trying again", len(removed))
		}
		if err = m.Driver.Up(ctx, sb, opts); !noNetworks(err) {
			return err
		}
	}
	return fmt.Errorf("%w (each project uses 3 networks, each session 1): stop projects you don't use (jw down), "+
		"remove unused networks (docker network prune), or give the runtime more address ranges: %s\n%w", ErrNoNetworks, PoolsDoc, err)
}

// UnusedNetworks are jailwick's networks whose sandbox has no container left (a stopped
// project's keep theirs: its containers need them to start again).
func (m *Manager) UnusedNetworks(ctx context.Context) ([]string, error) {
	nets, err := m.Driver.Networks(ctx)
	if err != nil {
		return nil, err
	}
	cs, err := m.Driver.AllContainers(ctx)
	if err != nil {
		return nil, err
	}
	inUse := map[string]bool{}
	for _, c := range cs {
		inUse[c.Labels["com.docker.compose.project"]] = true
	}
	var out []string
	for _, n := range nets {
		if strings.HasPrefix(n.Project, spec.Prefix) && !inUse[n.Project] {
			out = append(out, n.Name)
		}
	}
	return out, nil
}

// PruneNetworks removes jailwick's unused networks, returning those removed.
func (m *Manager) PruneNetworks(ctx context.Context) ([]string, error) {
	unused, err := m.UnusedNetworks(ctx)
	if err != nil {
		return nil, err
	}
	var removed []string
	var errs []error
	for _, n := range unused {
		if err := m.Driver.RemoveNetwork(ctx, n); err != nil {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, n)
	}
	return removed, errors.Join(errs...)
}

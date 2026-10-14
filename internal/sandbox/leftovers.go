package sandbox

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jailwick/jailwick/internal/hostutil"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

// ErrNoSandbox means no sandbox has that name.
var ErrNoSandbox = errors.New("no sandbox")

// Leftover is a sandbox whose project file is gone: it is no project, so its sessions appear in
// no list. jailwick shows it and removes it when asked, never on its own (it may hold data).
type Leftover struct {
	Name       string              `json:"name"`
	Created    time.Time           `json:"created"`
	Containers []LeftoverContainer `json:"containers"` // its own and its session sandboxes'
	Sessions   []Session           `json:"sessions"`
}

type LeftoverContainer struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// byProject groups containers by the project their label names.
func byProject(cs []runtime.Container) map[string][]runtime.Container {
	by := map[string][]runtime.Container{}
	for _, c := range cs {
		if p := c.Labels[spec.LabelProject]; p != "" {
			by[p] = append(by[p], c)
		}
	}
	return by
}

// isProject reports whether a project file exists for the name.
func (m *Manager) isProject(name string) bool { return hostutil.Exists(m.Store.ProjectFile(name)) }

// Leftovers lists the sandboxes without a project, by name.
func (m *Manager) Leftovers(ctx context.Context) ([]Leftover, error) {
	all, err := m.Driver.AllContainers(ctx)
	if err != nil {
		return nil, err
	}
	var out []Leftover
	for name, cs := range byProject(all) {
		if m.isProject(name) {
			continue
		}
		l := Leftover{Name: name, Containers: []LeftoverContainer{}, Sessions: []Session{}}
		for _, c := range cs {
			l.Containers = append(l.Containers, LeftoverContainer{Name: c.Name, State: c.State})
			if l.Created.IsZero() || c.Created.Before(l.Created) {
				l.Created = c.Created
			}
		}
		slices.SortFunc(l.Containers, func(a, b LeftoverContainer) int { return strings.Compare(a.Name, b.Name) })
		if MainRunning(cs) || slices.ContainsFunc(groupBoxes(name, cs), Box.Running) {
			if ss := m.SessionsOf(ctx, name, cs); ss != nil {
				l.Sessions = ss
			}
		}
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b Leftover) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// RemoveLeftover removes a sandbox without a project (see Remove). A project's name is refused:
// its sandbox is the project's to stop.
func (m *Manager) RemoveLeftover(ctx context.Context, name string, o RemoveOptions) (Removed, error) {
	if m.isProject(name) {
		return Removed{}, fmt.Errorf("%s is a project: stop it with jw down -p %s", name, name)
	}
	cs, err := m.Driver.Containers(ctx, spec.Prefix+name)
	if err != nil {
		return Removed{}, err
	}
	if len(cs) == 0 {
		return Removed{}, fmt.Errorf("%w: %q", ErrNoSandbox, name)
	}
	return m.Remove(ctx, name, o)
}

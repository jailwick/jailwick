package sandbox

import (
	"context"
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

// listingDriver answers a fixed listing of every sandbox.
type listingDriver struct {
	runtime.Driver
	all []runtime.Container
}

func (d listingDriver) AllContainers(context.Context) ([]runtime.Container, error) { return d.all, nil }

// verify is passed the services of the other running sandboxes, never the project's own.
func TestOtherServices(t *testing.T) {
	label := func(project string) map[string]string { return map[string]string{spec.LabelProject: project} }
	d := listingDriver{all: []runtime.Container{
		{Name: "jailwick-api-sandbox-1", Service: "sandbox", State: "running", Labels: label("api")},
		{Name: "jailwick-api-postgres-1", Service: "postgres", State: "running", Labels: label("api")},
		{Name: "jailwick-web-sandbox-1", Service: "sandbox", State: "running", Labels: label("web")},
		{Name: "jailwick-web-redis-1", Service: "redis", State: "running", Labels: label("web")},
		{Name: "jailwick-web-mongodb-1", Service: "mongodb", State: "exited", Labels: label("web")},
	}}
	store := &config.Store{Dir: t.TempDir()}
	for _, p := range []string{"api", "web"} {
		if err := store.SaveProject(config.Project{Name: p, Dir: t.TempDir()}); err != nil {
			t.Fatal(err)
		}
	}
	m := &Manager{Driver: d, Store: store}
	got := m.otherServices(context.Background(), "api")
	if !slices.Equal(got, []string{"jailwick-web-redis-1:6379"}) {
		t.Fatalf("%v", got)
	}
}

// A running sandbox is left alone, except a main container from before the rename, which is
// replaced once (the old one removed with it).
func TestUpOptions(t *testing.T) {
	c := func(service, state string) runtime.Container {
		return runtime.Container{Service: service, State: state}
	}
	for _, tc := range []struct {
		name                      string
		before                    []runtime.Container
		recreate, orphans, legacy bool
	}{
		{"stopped", nil, true, false, false},
		{"running", []runtime.Container{c(spec.SandboxService, "running")}, false, false, false},
		{"from before the rename", []runtime.Container{c(spec.LegacySandboxService, "running")}, true, true, true},
	} {
		opts, legacy := upOptions(tc.before)
		if opts.Recreate != tc.recreate || opts.RemoveOrphans != tc.orphans || legacy != tc.legacy {
			t.Errorf("%s: %+v legacy=%v", tc.name, opts, legacy)
		}
	}
}

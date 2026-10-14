package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/runtime"
)

// imageDriver has images and containers made from some of them; it records removals.
type imageDriver struct {
	runtime.Driver
	imgs       []runtime.Image
	containers []string
	removed    []string
	pulled     []runtime.Image // present, not built by jailwick
}

func (d *imageDriver) ImagesWithLabel(context.Context, string) ([]runtime.Image, error) {
	return d.imgs, nil
}
func (d *imageDriver) ContainerImages(context.Context) ([]string, error) { return d.containers, nil }
func (d *imageDriver) ImageID(context.Context, string) (string, error)   { return "", nil }
func (d *imageDriver) RemoveImage(_ context.Context, ref string) error {
	d.removed = append(d.removed, ref)
	return nil
}

func TestPruneImagesKeepsWhatProjectsUse(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	store := &config.Store{Dir: t.TempDir()}
	if err := store.SaveProject(config.Project{Name: "api", Dir: repo}); err != nil {
		t.Fatal(err)
	}
	d := &imageDriver{}
	m := &Manager{Store: store, Driver: d, StateDir: t.TempDir()}
	raw, _ := store.Project("api")
	r, err := m.images().Ensure(context.Background(), raw.Resolved(m.Global), image.Options{NoBuild: true})
	if err != nil {
		t.Fatal(err)
	}
	d.imgs = []runtime.Image{
		{Ref: r.Proxy, ID: "111111111111"}, {Ref: r.Base, ID: "222222222222"}, {Ref: r.Sandbox, ID: "333333333333"},
		{Ref: "jailwick-sandbox:old", ID: "444444444444"},
		{Ref: "<none>:<none>", ID: "555555555555"},
		{Ref: "<none>:<none>", ID: "666666666666"},
	}
	d.containers = []string{"sha256:666666666666ffff"} // a stopped session still made from it

	removed, err := m.PruneImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.removed, []string{"jailwick-sandbox:old", "555555555555"}) || len(removed) != 2 {
		t.Fatalf("removed %v: only the old sandbox image (by name) and the dangling one (by ID)", d.removed)
	}
}

func (d *imageDriver) ImagesNamed(_ context.Context, refs []string) ([]runtime.Image, error) {
	var out []runtime.Image
	for _, img := range d.pulled {
		if slices.Contains(refs, img.Ref) {
			out = append(out, img)
		}
	}
	return out, nil
}

// A project's own base from a registry is listed, with the project, and never pruned.
func TestImagesListPulledBases(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "shop")
	os.MkdirAll(repo, 0o755)
	store := &config.Store{Dir: t.TempDir()}
	store.SaveProject(config.Project{Name: "shop", Dir: repo, Image: config.Image{ImageSource: config.ImageSource{Base: "ruby"}}})
	d := &imageDriver{pulled: []runtime.Image{{Ref: "ruby:latest", ID: "777777777777", Size: "1GB"}, {Ref: "php:8", ID: "888888888888"}}}
	m := &Manager{Store: store, Driver: d, StateDir: t.TempDir()}
	uses, err := m.Images(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 1 || uses[0].Ref != "ruby:latest" || !uses[0].Pulled || !slices.Equal(uses[0].Projects, []string{"shop"}) {
		t.Fatalf("%+v", uses)
	}
	if removed, _ := m.PruneImages(context.Background()); len(removed) != 0 {
		t.Fatalf("pruned %+v", removed)
	}
}

func TestWithTag(t *testing.T) {
	for in, want := range map[string]string{
		"ruby": "ruby:latest", "ruby:3.3": "ruby:3.3", "localhost:5000/dev": "localhost:5000/dev:latest",
		"ghcr.io/acme/dev:2": "ghcr.io/acme/dev:2", "img@sha256:abc": "img@sha256:abc",
	} {
		if got := withTag(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
}

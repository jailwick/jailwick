package sandbox

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/spec"
)

// removeDriver is fakeDriver with images: the sandbox's containers run on them, and removals are
// recorded. Once removed, no container is made from them.
type removeDriver struct {
	*fakeDriver
	imgs    []runtime.Image
	removed []string
}

func (d *removeDriver) ImagesWithLabel(context.Context, string) ([]runtime.Image, error) {
	return d.imgs, nil
}
func (d *removeDriver) ContainerImages(context.Context) ([]string, error) { return nil, nil }
func (d *removeDriver) ImageID(context.Context, string) (string, error)   { return "", nil }
func (d *removeDriver) RemoveImage(_ context.Context, ref string) error {
	d.removed = append(d.removed, ref)
	return nil
}

func TestRemove(t *testing.T) {
	cs := projectContainers()
	cs[0].ImageID = "sha256:aaaaaaaaaaaa1111" // claude, on api's sandbox image
	cs[1].ImageID = "sha256:bbbbbbbbbbbb2222" // the proxy, shared
	d := &removeDriver{fakeDriver: &fakeDriver{containers: cs, status: map[string]string{
		MainContainer("api"): `{"sessions":[{"pid":39,"dir":"/x","started":"2026-10-13T04:20:00Z"}]}`,
	}}}
	store := storeWith(t, "api", "web")
	m := &Manager{Store: store, Driver: d, StateDir: t.TempDir()}
	ctx := context.Background()
	// Projects with the same image settings share one sandbox image: then destroying api keeps it.
	shared, _ := m.projectImages(ctx, "web")
	webp, _ := store.Project("web")
	webp.Image.Stacks = map[string]string{"go": ""}
	if err := store.SaveProject(webp); err != nil {
		t.Fatal(err)
	}
	api, _ := m.projectImages(ctx, "api")
	web, _ := m.projectImages(ctx, "web")
	d.imgs = []runtime.Image{
		{Ref: api.Sandbox, ID: "aaaaaaaaaaaa"}, {Ref: api.Proxy, ID: "bbbbbbbbbbbb"}, {Ref: api.Base, ID: "cccccccccccc"},
		{Ref: "jailwick-sandbox:webwebwebweb", ID: "dddddddddddd"},
	}
	if api.Sandbox == web.Sandbox || api.Base != web.Base || shared.Sandbox != api.Sandbox {
		t.Fatalf("api and web: different sandbox images on one base: %+v %+v", api, web)
	}

	if _, err := m.Remove(ctx, "api", RemoveOptions{Images: true}); !errors.Is(err, ErrSessionsRunning) || len(d.downs) != 0 {
		t.Fatalf("running sessions: refused unless included (%v)", err)
	}
	got, err := m.Remove(ctx, "api", RemoveOptions{Sessions: true, Images: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Images, []string{api.Sandbox}) || !slices.Equal(d.removed, got.Images) {
		t.Fatalf("only api's own image goes; the base and proxy web needs stay: %v", got.Images)
	}
	if !slices.Equal(d.downs, []string{spec.Prefix + "api"}) || d.volumes[0] {
		t.Fatalf("its sandbox, data kept: %v %v", d.downs, d.volumes)
	}
	if _, err := m.Remove(ctx, "api", RemoveOptions{Sessions: true, Volumes: true}); err != nil || !d.volumes[1] {
		t.Fatalf("with its data: %v %v", err, d.volumes)
	}

	// web back to api's settings: the image is web's too, so it stays.
	webp.Image.Stacks = nil
	if err := store.SaveProject(webp); err != nil {
		t.Fatal(err)
	}
	d.removed = nil
	if got, err := m.Remove(ctx, "api", RemoveOptions{Sessions: true, Images: true}); err != nil || len(got.Images) != 0 {
		t.Fatalf("an image another project uses stays: %v %v", got.Images, err)
	}
}

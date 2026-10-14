package sandbox

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/image"
)

// Images lists the images jailwick built and what uses them: each project's sandbox image, its
// base and the proxy (named without building), and the images of jailwick's containers.
func (m *Manager) Images(ctx context.Context) ([]image.Use, error) { return m.imageUses(ctx, "") }

// imageUses is Images as if the project except didn't exist (being destroyed).
func (m *Manager) imageUses(ctx context.Context, except string) ([]image.Use, error) {
	imgs, err := m.Driver.ImagesWithLabel(ctx, image.LabelBuiltBy)
	if err != nil {
		return nil, err
	}
	wanted := map[string][]string{}
	pulled := map[string][]string{} // projects' own registry images: base or as is, by name
	ps, _ := m.Store.Projects()     // a project that doesn't load has no images to keep
	for _, raw := range ps {
		if raw.Name == except {
			continue
		}
		r, err := m.projectImages(ctx, raw.Name)
		if err != nil {
			continue // its images can't be named, so they can't be told apart; they rebuild
		}
		for _, ref := range []string{r.Proxy, r.Base, r.Sandbox} {
			if ref != "" {
				wanted[ref] = append(wanted[ref], raw.Name)
			}
		}
		img := raw.Resolved(m.Global).Image
		for _, ref := range []string{img.Base, img.Name} {
			if ref != "" {
				pulled[withTag(ref)] = append(pulled[withTag(ref)], raw.Name)
			}
		}
	}
	inUse, err := m.Driver.ContainerImages(ctx)
	if err != nil {
		return nil, err
	}
	uses := image.Usage(imgs, wanted, inUse)
	if len(pulled) > 0 {
		named, _ := m.Driver.ImagesNamed(ctx, slices.Sorted(maps.Keys(pulled))) // a bonus: the list works without
		for _, u := range image.Usage(named, pulled, inUse) {
			u.Pulled = true
			uses = append(uses, u)
		}
	}
	return uses, nil
}

// withTag is an image name with its tag, as the runtime lists it ("ruby" is "ruby:latest").
func withTag(ref string) string {
	last := ref[strings.LastIndex(ref, "/")+1:]
	if strings.Contains(last, ":") || strings.Contains(ref, "@") {
		return ref
	}
	return ref + ":latest"
}

// projectImages names a project's images without building them.
func (m *Manager) projectImages(ctx context.Context, name string) (image.Result, error) {
	raw, err := m.Store.Project(name)
	if err != nil {
		return image.Result{}, err
	}
	return m.images().Ensure(ctx, raw.Resolved(m.Global), image.Options{NoBuild: true})
}

// PruneImages removes the images jailwick built that nothing uses (see Images). Other images and
// the runtime's build cache are left alone. It returns what it removed; a removal that fails is
// reported and the others go on.
func (m *Manager) PruneImages(ctx context.Context) (removed []image.Use, err error) {
	uses, err := m.Images(ctx)
	if err != nil {
		return nil, err
	}
	var failed []error
	for _, u := range uses {
		if !u.Unused() {
			continue
		}
		ref := u.Ref
		if u.Dangling() {
			ref = u.ID
		}
		if err := m.Driver.RemoveImage(ctx, ref); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", ref, err))
			continue
		}
		removed = append(removed, u)
	}
	if len(failed) > 0 {
		return removed, fmt.Errorf("some images were not removed: %v", failed)
	}
	return removed, nil
}

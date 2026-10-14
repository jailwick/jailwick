package sandbox

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/jailwick/jailwick/internal/image"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/spec"
)

// RemoveOptions say what removing a sandbox takes along. Its containers and networks always go
// (its session sandboxes first: they use its networks).
type RemoveOptions struct {
	Sessions bool // end running sessions; without it, a sandbox with sessions is left alone
	Images   bool // also the images built for it that nothing else uses (jw destroy)
	Volumes  bool // also its data: databases, caches, history and its own Claude login (the app login stays)
}

// Removed is what a removal took along besides containers and networks.
type Removed struct {
	Images []string // refs (or IDs) of the images removed
}

// Remove stops and removes a sandbox, a project's or one left without its project. Running
// sessions make it refuse (ErrSessionsRunning) unless o.Sessions.
func (m *Manager) Remove(ctx context.Context, name string, o RemoveOptions) (Removed, error) {
	release, err := lockOp(name, OpStopping)
	if err != nil {
		return Removed{}, err
	}
	defer release()
	if !o.Sessions {
		if ss, err := m.Sessions(ctx, name); err == nil && len(ss) > 0 {
			return Removed{}, fmt.Errorf("%w: %d in %s", ErrSessionsRunning, len(ss), name)
		}
	}
	// The images it runs on, before its containers go.
	var used []string
	if o.Images {
		cs, err := m.Driver.Containers(ctx, spec.Prefix+name)
		if err != nil {
			return Removed{}, err
		}
		for _, c := range cs {
			used = append(used, c.ImageID)
		}
	}
	if err := m.Down(ctx, name, o.Volumes); err != nil {
		return Removed{}, err
	}
	if o.Volumes { // its data goes: generated files and what jailwick kept for it
		_ = os.RemoveAll(paths.ProjectState(name))
		_ = os.RemoveAll(paths.ProjectData(name))
	}
	var out Removed
	if o.Images {
		imgs, err := m.removeImagesOf(ctx, name, used)
		out.Images = imgs
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// removeImagesOf removes the jailwick images a sandbox ran on (used: image IDs) or that were
// built for its project, when no other project or container uses them: the shared base and
// proxy stay while other projects need them.
func (m *Manager) removeImagesOf(ctx context.Context, name string, used []string) ([]string, error) {
	var refs []string
	if r, err := m.projectImages(ctx, name); err == nil && r.Sandbox != "" {
		refs = append(refs, r.Sandbox)
	}
	uses, err := m.imageUses(ctx, name)
	if err != nil {
		return nil, err
	}
	var removed []string
	var failed []error
	for _, u := range uses {
		mine := slices.Contains(refs, u.Ref) || slices.ContainsFunc(used, func(id string) bool { return image.SameImage(id, u.ID) })
		if !mine || !u.Unused() {
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
		removed = append(removed, ref)
	}
	if len(failed) > 0 {
		return removed, fmt.Errorf("some images were not removed: %v", failed)
	}
	return removed, nil
}

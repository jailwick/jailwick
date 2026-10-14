package image

import (
	"strings"

	"github.com/jailwick/jailwick/internal/runtime"
)

// Use is an image jailwick built and what uses it.
type Use struct {
	runtime.Image
	Projects   []string `json:"projects"`   // projects whose images include it (sandbox, base, proxy)
	Containers bool     `json:"containers"` // a container, of any state, is made from it
	// Pulled: a project's own base or image from a registry, not one jailwick built.
	Pulled bool `json:"pulled,omitempty"`
}

// Unused reports whether nothing needs the image: prune may remove it.
func (u Use) Unused() bool { return len(u.Projects) == 0 && !u.Containers }

// Dangling reports whether the image lost its name (a rebuild took it); it is removed by ID.
func (u Use) Dangling() bool { return strings.HasPrefix(u.Ref, "<none>") }

// Usage pairs images with what uses them. wanted maps an image name to the projects that use it;
// containerImages are the image IDs of existing containers.
func Usage(imgs []runtime.Image, wanted map[string][]string, containerImages []string) []Use {
	out := make([]Use, 0, len(imgs))
	for _, img := range imgs {
		u := Use{Image: img, Projects: wanted[img.Ref]}
		if u.Projects == nil {
			u.Projects = []string{}
		}
		for _, id := range containerImages {
			if SameImage(id, img.ID) {
				u.Containers = true
				break
			}
		}
		out = append(out, u)
	}
	return out
}

// SameImage compares image IDs that may be full ("sha256:…") or short (the first 12 digits).
func SameImage(a, b string) bool {
	a, b = strings.TrimPrefix(a, "sha256:"), strings.TrimPrefix(b, "sha256:")
	if a == "" || b == "" {
		return false
	}
	if len(a) < len(b) {
		a, b = b, a
	}
	return strings.HasPrefix(a, b)
}

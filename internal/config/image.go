package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/paths"
)

// Platforms are the image platforms jailwick builds for (image.platform; empty: the machine's).
var Platforms = []string{"linux/amd64", "linux/arm64"}

var (
	// imageRef is what an image reference looks like: [registry[:port]/]path[:tag][@digest].
	imageRef = regexp.MustCompile(`^[a-z0-9]+([._:/-][a-z0-9]+)*(:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(@sha256:[a-f0-9]{64})?$`)
	userName = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
)

// CheckImageRef reports whether s can name an image ("" passes). It doesn't say the image exists.
func CheckImageRef(s string) error {
	if s != "" && !imageRef.MatchString(s) {
		return fmt.Errorf("%q isn't an image name, like eclipse-temurin:21-jdk or ghcr.io/acme/dev:2", s)
	}
	return nil
}

// CheckUserName reports whether s can be a Linux user name ("" passes: the default).
func CheckUserName(s string) error {
	if s != "" && !userName.MatchString(s) {
		return fmt.Errorf("%q isn't a user name (lowercase letters, digits, - and _)", s)
	}
	return nil
}

// CheckHome reports whether s can be a home folder in the image ("" passes: the default).
func CheckHome(s string) error {
	if s != "" && (!strings.HasPrefix(s, "/") || slices.Contains(strings.Split(s, "/"), "..")) {
		return fmt.Errorf("%q must be an absolute path in the image, like /home/node", s)
	}
	return nil
}

// validateImage checks where an image comes from and who runs in it, for a project or the
// global defaults (what names it in problems).
func validateImage(img Image, pr *Problems) {
	n := 0
	for _, v := range []string{img.Base, img.Dockerfile, img.Name} {
		if v != "" {
			n++
		}
	}
	if n > 1 {
		pr.add("image: use only one of base, dockerfile, name")
	}
	if err := CheckImageRef(img.Base); err != nil {
		pr.add("image.base: %v", err)
	}
	if err := CheckImageRef(img.Name); err != nil {
		pr.add("image.name: %v", err)
	}
	if err := CheckUserName(img.User); err != nil {
		pr.add("image.user: %v", err)
	}
	if err := CheckHome(img.Home); err != nil {
		pr.add("image.home: %v", err)
	}
	if img.Platform != "" && !slices.Contains(Platforms, img.Platform) {
		pr.add("image.platform: %q is not one of %s", img.Platform, strings.Join(Platforms, ", "))
	}
}

// inRepo resolves a path the project names: ~ expanded, and relative ones from the repo (not from
// wherever jw runs).
func (p Project) inRepo(file string) string {
	f := paths.Expand(file)
	if f != "" && !filepath.IsAbs(f) && p.Dir != "" {
		return filepath.Join(paths.Expand(p.Dir), f)
	}
	return f
}

// DockerfilePath is the project's image.dockerfile as a path ("" without one).
func (p Project) DockerfilePath() string {
	if p.Image.Dockerfile == "" {
		return ""
	}
	return p.inRepo(p.Image.Dockerfile)
}

// ContextPath is the build context of the project's Dockerfile: image.context, else its folder.
func (p Project) ContextPath() string {
	if p.Image.Context != "" {
		return p.inRepo(p.Image.Context)
	}
	if df := p.DockerfilePath(); df != "" {
		return filepath.Dir(df)
	}
	return ""
}

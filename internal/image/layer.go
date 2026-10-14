package image

import (
	"bytes"
	"fmt"
	"path"
	"strings"
	"text/template"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/stacks"
)

// layer is what the project layer's Dockerfile template is filled with.
type layer struct {
	NodeDefault        string
	Android            bool
	Stacks             []stackLayer
	Browsers           string
	PlaywrightVersions string
	AptPackages        string
}

type stackLayer struct {
	stacks.Choice
	Fragment string // its Dockerfile steps
}

// cacheMounts are BuildKit cache mounts every install step gets: apt's downloads and lists,
// verified downloads, npm's cache. They persist between builds and never end up in the image.
var cacheMounts = []string{"/var/cache/apt", "/var/lib/apt/lists", "/var/cache/jailwick", "/root/.npm"}

var funcs = template.FuncMap{
	// mounts binds lib.sh and the given scripts of the build context to /tmp/jw, and adds the
	// cache mounts: the start of a RUN instruction.
	"mounts": func(scripts ...string) string {
		var b strings.Builder
		for _, s := range append([]string{"lib.sh"}, scripts...) {
			fmt.Fprintf(&b, "--mount=type=bind,source=%s,target=/tmp/jw/%s \\\n    ", s, path.Base(s))
		}
		for _, c := range cacheMounts {
			fmt.Fprintf(&b, "--mount=type=cache,target=%s,sharing=locked \\\n    ", c)
		}
		return b.String()
	},
}

// layerContext returns the build context of a project's layer: its Dockerfile, generated from the
// project's image settings, and only the scripts that Dockerfile runs (editing a stack's script
// doesn't rebuild projects without that stack).
func layerContext(p config.Project) (map[string][]byte, error) {
	node, _ := stacks.Get("node")
	l := layer{
		NodeDefault:        node.Default,
		Android:            p.Image.Android,
		Browsers:           strings.Join(p.Image.Playwright.Browsers, " "),
		PlaywrightVersions: strings.Join(p.Image.Playwright.Versions, " "),
		AptPackages:        strings.Join(p.Image.AptPackages, " "),
	}
	files := map[string][]byte{}
	script := func(name string) { files[name] = assets.MustRead("images/" + projectScript(name)) }
	for _, name := range []string{"lib.sh", "prereqs.sh", "home.sh"} {
		script(name)
	}
	if l.Android {
		script("android.sh")
	}
	if l.Browsers != "" {
		script("playwright.sh")
	}
	for _, c := range stacks.Chosen(p.Image.Stacks) {
		fragment, err := render("images/stacks/"+c.Name+".Dockerfile", c)
		if err != nil {
			return nil, err
		}
		l.Stacks = append(l.Stacks, stackLayer{Choice: c, Fragment: strings.TrimSpace(fragment)})
		script("stacks/" + c.Name + ".sh")
	}
	dockerfile, err := render("images/project/Dockerfile.tmpl", l)
	if err != nil {
		return nil, err
	}
	files["Dockerfile"] = []byte(dockerfile)
	return files, nil
}

// projectScript is where a script of the build context is among the assets.
func projectScript(name string) string {
	if strings.HasPrefix(name, "stacks/") {
		return name
	}
	return "project/" + name
}

func render(asset string, data any) (string, error) {
	t, err := template.New(path.Base(asset)).Funcs(funcs).Parse(string(assets.MustRead(asset)))
	if err != nil {
		return "", fmt.Errorf("%s: %w", asset, err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("%s: %w", asset, err)
	}
	return b.String(), nil
}

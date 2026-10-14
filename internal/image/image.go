// Package image makes sure a project's sandbox and proxy images exist, building them from the
// embedded Dockerfiles (or the user's own) when needed.
package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/runtime"
)

// Labels set on images jailwick builds.
const (
	LabelBuiltBy       = "dev.jailwick.built-by"
	LabelClaudeVersion = "dev.jailwick.claude-code-version"
)

// Result names the images a sandbox runs.
type Result struct {
	Sandbox string
	Base    string // what the sandbox image is built on ("" for image.name)
	Proxy   string
	// JailwickBase: the sandbox image is built on jailwick's base (Node.js and the CLI tools).
	JailwickBase bool
}

// Builder resolves and builds images.
type Builder struct {
	Driver   runtime.Driver
	StateDir string
	Out      io.Writer // a build's whole output (verbose); it always goes to its log file
	// Log reports a step to the user ("Building …"). Progress also goes to the context's
	// watcher (Watch).
	Log func(format string, a ...any)
}

// Options control rebuilding.
type Options struct {
	Rebuild bool // rebuild even when the image exists
	// Fresh rebuilds without the build cache and pulls the images built FROM again, so base
	// packages pick up updates. It implies Rebuild.
	Fresh   bool
	Update  bool // resolve "latest" Claude Code and rebuild if a newer one is out
	NoBuild bool // only compute the image names (status, inspection)
}

// Ensure returns the project's images, building what's missing.
func (b *Builder) Ensure(ctx context.Context, p config.Project, o Options) (Result, error) {
	o.Rebuild = o.Rebuild || o.Fresh
	proxy, err := b.proxy(ctx, o)
	if err != nil {
		return Result{}, err
	}
	r := Result{Proxy: proxy}
	if p.Image.Name != "" {
		r.Sandbox = p.Image.Name
		return r, nil
	}
	base, err := b.base(ctx, p, o)
	if err != nil {
		return r, err
	}
	r.Base = base
	r.JailwickBase = p.Image.Base == "" && p.Image.Dockerfile == ""
	r.Sandbox, err = b.project(ctx, p, base, o)
	return r, err
}

// step reports the start of the build of tag.
func (b *Builder) step(ctx context.Context, tag, format string, a ...any) {
	step, log := fmt.Sprintf(format, a...), paths.Tilde(b.LogFile(tag))
	if b.Log != nil {
		b.Log("%s · log: %s", step, log)
	}
	if fn := watcher(ctx); fn != nil {
		fn(Event{Step: step, Log: log})
	}
}

func (b *Builder) proxy(ctx context.Context, o Options) (string, error) {
	files := assets.Dir("images/proxy")
	tag := "jailwick-proxy:" + digest(files, nil)
	return tag, b.buildIfMissing(ctx, tag, o, "the network filter", func() error {
		return b.build(ctx, runtime.BuildOptions{Tag: tag, Labels: builtBy(), Pull: o.Fresh, NoCache: o.Fresh}, files)
	})
}

// base returns the image the project layer is built FROM.
func (b *Builder) base(ctx context.Context, p config.Project, o Options) (string, error) {
	switch {
	case p.Image.Base != "":
		return p.Image.Base, nil
	case p.Image.Dockerfile != "":
		df, ctxDir := p.DockerfilePath(), p.ContextPath() // relative ones from the repo
		tag, err := userBaseTag(p, df, ctxDir)
		if err != nil {
			return "", err
		}
		return tag, b.buildIfMissing(ctx, tag, o, "your base image", func() error {
			return b.build(ctx, runtime.BuildOptions{Context: ctxDir, Dockerfile: df, Tag: tag, Platform: platform(p),
				Labels: builtBy(), Pull: o.Fresh, NoCache: o.Fresh}, nil)
		})
	}
	files := assets.Dir("images/base")
	tag := "jailwick-base:" + digest(files, map[string]string{"platform": platform(p)})
	return tag, b.buildIfMissing(ctx, tag, o, "the base image (first time: about 10 minutes)", func() error {
		return b.build(ctx, runtime.BuildOptions{Tag: tag, Platform: platform(p), Labels: builtBy(), Pull: o.Fresh, NoCache: o.Fresh}, files)
	})
}

// userBaseTag names the image built from the user's Dockerfile after its content (with the
// .dockerignore and the context folder), so editing it builds it again. Changes to files it copies
// need `jw image build`.
func userBaseTag(p config.Project, dockerfile, ctxDir string) (string, error) {
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		return "", fmt.Errorf("image.dockerfile: %w", err)
	}
	ignore, _ := os.ReadFile(filepath.Join(ctxDir, ".dockerignore"))
	files := map[string][]byte{"Dockerfile": data, ".dockerignore": ignore}
	return "jailwick-userbase-" + p.Name + ":" + digest(files, map[string]string{"context": ctxDir, "platform": platform(p)}), nil
}

// project builds the project layer: extras and Claude Code on top of the base.
func (b *Builder) project(ctx context.Context, p config.Project, base string, o Options) (string, error) {
	baseID, err := b.Driver.ImageID(ctx, base)
	if err != nil {
		return "", err
	}
	if baseID == "" {
		baseID = base // a registry image, pulled by the build
	}
	files, err := layerContext(p)
	if err != nil {
		return "", err
	}
	args := map[string]string{
		"BASE":                base,
		"CLAUDE_CODE_VERSION": p.Image.ClaudeCodeVersion,
		"SANDBOX_USER":        p.Image.User,
		"SANDBOX_HOME":        p.Image.Home,
	}
	tag := "jailwick-sandbox:" + digest(files, mergeMaps(args, map[string]string{"base-id": baseID, "platform": platform(p)}))

	if o.NoBuild {
		return tag, nil
	}
	id, err := b.Driver.ImageID(ctx, tag)
	if err != nil {
		return "", err
	}
	want := p.Image.ClaudeCodeVersion
	if want == "latest" && (id == "" || o.Update || o.Rebuild) {
		if want, err = LatestClaudeCode(ctx); err != nil {
			return "", err
		}
	}
	if id != "" && !o.Rebuild {
		if !o.Update || b.imageLabel(ctx, tag, LabelClaudeVersion) == want {
			return tag, nil
		}
	}
	on := "" // the base it builds on, when it isn't jailwick's
	switch {
	case p.Image.Base != "":
		on = " on " + p.Image.Base
	case p.Image.Dockerfile != "":
		on = " on your Dockerfile's image"
	}
	b.step(ctx, tag, "Setting up the sandbox%s with Claude Code %s (a few minutes)", on, want)
	args["CLAUDE_CODE_VERSION"] = want
	labels := builtBy()
	labels[LabelClaudeVersion] = want
	// Only a registry base can be pulled; jailwick's and the user's bases are local images.
	pull := o.Fresh && p.Image.Base != ""
	return tag, b.build(ctx, runtime.BuildOptions{Tag: tag, Platform: platform(p), Args: args, Labels: labels,
		Pull: pull, NoCache: o.Fresh}, files)
}

func (b *Builder) imageLabel(ctx context.Context, ref, label string) string {
	labels, err := b.Driver.ImageLabels(ctx, ref)
	if err != nil {
		return ""
	}
	return labels[label]
}

func (b *Builder) buildIfMissing(ctx context.Context, tag string, o Options, what string, build func() error) error {
	if o.NoBuild {
		return nil
	}
	if !o.Rebuild {
		id, err := b.Driver.ImageID(ctx, tag)
		if err != nil {
			return err
		}
		if id != "" {
			return nil
		}
	}
	b.step(ctx, tag, "Setting up %s", what)
	return build()
}

func platform(p config.Project) string {
	if p.Image.Platform == "" && p.Image.Android {
		return "linux/amd64" // Android build-tools are x86_64 only
	}
	return p.Image.Platform
}

func builtBy() map[string]string { return map[string]string{LabelBuiltBy: "jailwick"} }

func mergeMaps(a, b map[string]string) map[string]string {
	out := maps.Clone(a)
	maps.Copy(out, b)
	return out
}

// digest hashes files and parameters into a short, stable tag.
func digest(files map[string][]byte, params map[string]string) string {
	h := sha256.New()
	for _, n := range slices.Sorted(maps.Keys(files)) {
		fmt.Fprintf(h, "%s\x00%d\x00", n, len(files[n]))
		h.Write(files[n])
	}
	for _, k := range slices.Sorted(maps.Keys(params)) {
		fmt.Fprintf(h, "%s=%s\x00", k, params[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// LatestClaudeCode asks the npm registry for the current Claude Code version.
func LatestClaudeCode(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://registry.npmjs.org/@anthropic-ai/claude-code/latest", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("looking up the latest Claude Code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("looking up the latest Claude Code: registry answered %s", resp.Status)
	}
	var v struct{ Version string }
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil || v.Version == "" {
		return "", fmt.Errorf("looking up the latest Claude Code: unexpected answer")
	}
	return v.Version, nil
}

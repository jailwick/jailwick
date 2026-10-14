package image

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/runtime"
)

// fakeDriver builds by printing output and, when fail is set, failing. Images it built exist.
type fakeDriver struct {
	runtime.Driver
	output string
	fail   error
	built  []runtime.BuildOptions
	have   map[string]bool
}

func (d *fakeDriver) Build(_ context.Context, o runtime.BuildOptions) error {
	d.built = append(d.built, o)
	if _, err := os.Stat(o.Context); err != nil {
		return fmt.Errorf("no build context: %v", err)
	}
	fmt.Fprint(o.Out, d.output)
	if d.fail != nil {
		return d.fail
	}
	if d.have == nil {
		d.have = map[string]bool{}
	}
	d.have[o.Tag] = true
	return nil
}

func (d *fakeDriver) ImageID(_ context.Context, ref string) (string, error) {
	if d.have[ref] {
		return "sha256:" + ref, nil
	}
	return "", nil
}

func (d *fakeDriver) ImageLabels(context.Context, string) (map[string]string, error) {
	return map[string]string{}, nil
}

func project() config.Project {
	return config.Project{Name: "api", Image: config.Image{ImageSource: config.ImageSource{ClaudeCodeVersion: "2.1.0", User: "node"}}}
}

func TestBuildFailureShowsTheEnd(t *testing.T) {
	var out strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&out, "#5 %d step output\n", i)
	}
	out.WriteString("#5 ERROR: process did not complete") // no newline at the end
	d := &fakeDriver{output: out.String(), fail: errors.New("exit status 1")}
	var steps, lines int
	b := &Builder{Driver: d, StateDir: t.TempDir()}
	ctx := Watch(context.Background(), func(e Event) {
		if e.Step != "" {
			steps++
		}
		if e.Line != "" {
			lines++
		}
	})

	_, err := b.Ensure(ctx, project(), Options{})
	var be *BuildError
	if !errors.As(err, &be) {
		t.Fatalf("want a BuildError, got %v", err)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "building jailwick-proxy:") || !strings.Contains(msg, "#5 ERROR: process did not complete") ||
		strings.Contains(msg, "#5 11 step output\n") || !strings.Contains(msg, "#5 12 step output") || !strings.Contains(msg, "full log: ") {
		t.Fatalf("error should name the image, end with the last %d lines and the log:\n%s", tailLines, msg)
	}
	if steps != 1 || lines != 41 {
		t.Errorf("the watcher got %d steps and %d lines, want 1 and 41", steps, lines)
	}
	if log, _ := os.ReadFile(be.Log); string(log) != out.String() {
		t.Errorf("the log file holds the whole output")
	}
	if _, err := os.Stat(d.built[0].Context); !os.IsNotExist(err) {
		t.Errorf("the build context is removed after the build")
	}
}

func TestEnsureBuildsWhatIsMissing(t *testing.T) {
	d := &fakeDriver{output: "ok\n"}
	b := &Builder{Driver: d, StateDir: t.TempDir()}
	r, err := b.Ensure(context.Background(), project(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.built) != 3 || !r.JailwickBase || !strings.HasPrefix(r.Sandbox, "jailwick-sandbox:") {
		t.Fatalf("first run builds proxy, base and project: %d builds, %+v", len(d.built), r)
	}
	if _, err := b.Ensure(context.Background(), project(), Options{}); err != nil || len(d.built) != 3 {
		t.Fatalf("a second run builds nothing: %d builds, %v", len(d.built), err)
	}
}

func TestUserBaseFollowsItsDockerfile(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	write := func(name, data string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Dockerfile", "FROM debian:12\n")
	p := project()
	p.Image.Dockerfile = df
	tag := func() string {
		t.Helper()
		tag, err := userBaseTag(p, df, dir)
		if err != nil {
			t.Fatal(err)
		}
		return tag
	}
	first := tag()
	if !strings.HasPrefix(first, "jailwick-userbase-api:") || tag() != first {
		t.Fatalf("a stable tag named after the project: %s", first)
	}
	write("Dockerfile", "FROM debian:12\nRUN apt-get update\n")
	second := tag()
	write(".dockerignore", "node_modules\n")
	if second == first || tag() == second {
		t.Fatal("editing the Dockerfile or .dockerignore changes the tag")
	}

	d := &fakeDriver{output: "ok\n"}
	b := &Builder{Driver: d, StateDir: t.TempDir()}
	if _, err := b.Ensure(context.Background(), p, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := d.built[1]; got.Tag != tag() || got.Dockerfile != df || got.Context != dir {
		t.Fatalf("your base is built from your Dockerfile and folder: %+v", got)
	}
}

func TestFreshPullsOnlyRegistryImages(t *testing.T) {
	d := &fakeDriver{output: "ok\n"}
	b := &Builder{Driver: d, StateDir: t.TempDir()}
	if _, err := b.Ensure(context.Background(), project(), Options{Fresh: true}); err != nil {
		t.Fatal(err)
	}
	for _, o := range d.built {
		local := strings.HasPrefix(o.Tag, "jailwick-sandbox:") // FROM jailwick-base, a local image
		if !o.NoCache || o.Pull == local {
			t.Errorf("%s: no cache, and pull only what comes from a registry: %+v", o.Tag, o)
		}
	}
	if len(d.built) != 3 {
		t.Fatalf("fresh rebuilds every image: %d", len(d.built))
	}
}

func TestProgressOf(t *testing.T) {
	for line, want := range map[string][2]int{
		"#9 [stage-0 5/6] RUN --mount=type=cache npm install": {5, 6},
		"#4 [2/3] COPY lib.sh /tmp":                           {2, 3},
		"#9 0.512 added 3 packages":                           {0, 0},
		"#3 [internal] load metadata":                         {0, 0},
	} {
		d, tot, ok := progressOf(line)
		if [2]int{d, tot} != want || ok != (want[1] > 0) {
			t.Errorf("progressOf(%q) = %d, %d, %v; want %v", line, d, tot, ok, want)
		}
	}
}

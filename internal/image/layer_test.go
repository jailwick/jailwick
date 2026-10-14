package image

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

var update = flag.Bool("update", false, "rewrite the golden Dockerfiles in testdata")

func TestLayerDockerfiles(t *testing.T) {
	cases := map[string]config.Image{
		"plain":       {},
		"node-python": {Stacks: map[string]string{"python": "3.12", "node": ""}},
		"java-ruby":   {Stacks: map[string]string{"java": "21", "ruby": ""}},
		"everything": {Stacks: map[string]string{"node": "22", "python": "", "go": "1.25", "rust": "stable", "bun": ""},
			Playwright:  config.Playwright{Browsers: []string{"chromium", "firefox"}, Versions: []string{"1.50.1"}},
			AptPackages: []string{"libvips-dev", "ffmpeg"}, Android: true},
	}
	for name, img := range cases {
		t.Run(name, func(t *testing.T) {
			files, err := layerContext(config.Project{Name: "api", Image: img})
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", name+".Dockerfile")
			if *update {
				if err := os.WriteFile(golden, files["Dockerfile"], 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if string(files["Dockerfile"]) != string(want) {
				t.Errorf("Dockerfile differs from %s (go test ./internal/image -update):\n%s", golden, files["Dockerfile"])
			}
			// Every script the Dockerfile binds is in the context, and nothing else.
			var bound []string
			for _, line := range strings.Split(string(files["Dockerfile"]), "\n") {
				if _, src, ok := strings.Cut(line, "--mount=type=bind,source="); ok {
					src, _, _ = strings.Cut(src, ",")
					if !slices.Contains(bound, src) {
						bound = append(bound, src)
					}
				}
			}
			var context []string
			for f := range files {
				if f != "Dockerfile" {
					context = append(context, f)
				}
			}
			slices.Sort(bound)
			slices.Sort(context)
			if !slices.Equal(bound, context) {
				t.Errorf("bound scripts %v, context %v", bound, context)
			}
		})
	}
}

func TestLayerDigestFollowsWhatIsChosen(t *testing.T) {
	p := config.Project{Name: "api", Image: config.Image{Stacks: map[string]string{"node": ""}}}
	tag := func(p config.Project) string {
		files, err := layerContext(p)
		if err != nil {
			t.Fatal(err)
		}
		return digest(files, nil)
	}
	node := tag(p)
	if _, ok := mustContext(t, p)["stacks/go.sh"]; ok {
		t.Fatal("a node project's context has no go.sh, so editing it doesn't rebuild the project")
	}
	p.Image.Stacks["node"] = "22"
	if tag(p) == node {
		t.Fatal("another version is another image")
	}
}

func mustContext(t *testing.T, p config.Project) map[string][]byte {
	t.Helper()
	files, err := layerContext(p)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

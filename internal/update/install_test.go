package update

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Homebrew updates the running jailwick when it is the formula's binary or the cask's app, and
// updates whichever of the two Homebrew has.
func TestDetect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Homebrew on Windows")
	}
	prefix := t.TempDir()
	home := t.TempDir()
	brew := filepath.Join(prefix, "bin", "brew")
	cli := filepath.Join(prefix, "Cellar", "jailwick", "0.2.8", "bin", "jailwick")
	app := "/Applications/jailwick.app/Contents/MacOS/jailwick-app"
	mkdir := func(p string) {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if in := detect(cli, "", home); in.Homebrew() {
		t.Error("no brew: download")
	}
	if in := detect(cli, brew, home); in.Homebrew() {
		t.Error("brew without the formula: download")
	}
	mkdir(filepath.Dir(cli))
	in := detect(cli, brew, home)
	if !in.Homebrew() || in.Command != "brew upgrade "+Formula {
		t.Errorf("the formula's CLI: %+v", in)
	}
	if in := detect("/Users/me/code/jailwick/bin/jailwick", brew, home); in.Homebrew() {
		t.Error("a build from source isn't Homebrew's")
	}
	if in := detect(app, brew, home); in.Homebrew() {
		t.Error("an app without the cask isn't Homebrew's")
	}
	mkdir(filepath.Join(prefix, "Caskroom", "jailwick-app"))
	want := "brew upgrade " + Formula + " && brew upgrade --cask " + Cask
	for _, exe := range []string{cli, app, filepath.Join(home, "Applications", "jailwick.app", "Contents", "MacOS", "jailwick-app")} {
		if in := detect(exe, brew, home); !in.Homebrew() || in.Command != want {
			t.Errorf("%s: %+v, want both updated", exe, in)
		}
	}
}

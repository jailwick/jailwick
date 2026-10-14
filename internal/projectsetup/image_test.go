package projectsetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

func TestImageSource(t *testing.T) {
	dir := repo(t, "shop", nil)
	os.MkdirAll(filepath.Join(dir, "docker"), 0o755)
	os.WriteFile(filepath.Join(dir, "docker", "dev.Dockerfile"), []byte("FROM ruby:3.3-bookworm\n"), 0o644)
	store := fakeStore{{Name: "shop", Dir: dir}}

	d, err := Edit(store, "shop", map[string]any{"image.source": "base", "image.base": "eclipse-temurin:21-jdk", "image.user": "app", "image.home": "/home/app"})
	if err != nil {
		t.Fatal(err)
	}
	if img := d.Project.Image; img.Base != "eclipse-temurin:21-jdk" || img.User != "app" || img.Home != "/home/app" {
		t.Fatalf("%+v", img)
	}

	// A relative Dockerfile is the repo's, wherever jw runs.
	d, err = Edit(store, "shop", map[string]any{"image.source": "dockerfile", "image.dockerfile": "docker/dev.Dockerfile"})
	if err != nil || d.Project.DockerfilePath() != filepath.Join(dir, "docker", "dev.Dockerfile") {
		t.Fatalf("%v %q", err, d.Project.DockerfilePath())
	}

	// Choosing a source drops the others' values.
	store = fakeStore{{Name: "shop", Dir: dir, Image: config.Image{ImageSource: config.ImageSource{Base: "ruby:3.3"}}}}
	d, err = Edit(store, "shop", map[string]any{"image.source": "default"})
	if err != nil || d.Project.Image.Base != "" {
		t.Fatalf("%v %+v", err, d.Project.Image)
	}
	if got := (&Draft{Project: store[0]}).imageSource(); got != "base" {
		t.Fatalf("read from the project: %q", got)
	}

	if _, err := Edit(fakeStore{{Name: "shop", Dir: dir}}, "shop", map[string]any{"image.source": "base"}); err == nil {
		t.Error("a base image source without an image was accepted")
	}
	for _, bad := range []map[string]any{
		{"image.source": "base", "image.base": "Not An Image"},        // not an image name
		{"image.source": "dockerfile", "image.dockerfile": "nope.Df"}, // not in the repo
		{"image.source": "somewhere"},
		{"image.source": "base", "image.base": "ruby:3.3", "image.user": "Root User"},
		{"image.source": "base", "image.base": "ruby:3.3", "image.home": "home/app"},
		{"image.stacks": []any{"node"}, "image.stacks.node": "34"},
	} {
		if _, err := Edit(store, "shop", bad); err == nil {
			t.Errorf("%v was accepted", bad)
		}
	}
}

func TestLanguageNoteAndSuggestedBase(t *testing.T) {
	dir := repo(t, "shop", map[string]string{"composer.json": "{}"})
	store := fakeStore{{Name: "shop", Dir: dir}}
	d, err := Edit(store, "shop", nil)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := EditForm.Field("image.source")
	note := f.Note(d)
	if !strings.Contains(note, "PHP (composer.json)") || !strings.Contains(note, "php-cli, composer") || !strings.Contains(note, "php:8.3-cli-bookworm") {
		t.Fatalf("note: %q", note)
	}
	// Choosing a base image starts from the one the repo's language needs.
	d, _ = Edit(store, "shop", map[string]any{"image.source": "base"})
	if d.Project.Image.Base != "php:8.3-cli-bookworm" {
		t.Fatalf("prefilled %q", d.Project.Image.Base)
	}
}

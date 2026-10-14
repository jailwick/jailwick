package projectsetup

import (
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

func TestEditFormIsWellFormed(t *testing.T) {
	if err := EditForm.Verify(); err != nil {
		t.Fatal(err)
	}
	for _, f := range EditForm.Fields() {
		if f.Key == "name" || f.Key == "workspace" {
			t.Errorf("%s is changed elsewhere, not in the settings", f.Key)
		}
	}
}

func TestEdit(t *testing.T) {
	p := config.Project{Name: "shop", Dir: repo(t, "shop", nil), Services: []string{"postgres"},
		Postgres: config.Postgres{User: "app", Password: "app", DB: "shop"}, LocalForwards: []string{"5433:postgres:5432"}}
	p.Image.Playwright = config.Playwright{Browsers: []string{"chromium"}, Versions: []string{"1.50.0"}}
	store := fakeStore{p}

	d, err := Edit(store, "shop", map[string]any{
		"image.playwright.browsers": []any{"chromium", "firefox"},
		"run":                       "pnpm dev",
		"resources.memory":          "16g",
		"resources.cpus":            "4",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := d.Project
	if !slices.Equal(got.Image.Playwright.Browsers, []string{"chromium", "firefox"}) || got.Run != "pnpm dev" ||
		got.Resources.Memory != "16g" || got.Resources.CPUs != "4" || got.Postgres.DB != "shop" {
		t.Fatalf("%+v", got)
	}

	// Dropping a service drops its settings and its forwards.
	d, err = Edit(store, "shop", map[string]any{"services": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Project.Postgres.DB != "" || len(d.Project.LocalForwards) != 0 {
		t.Fatalf("postgres left: %+v %v", d.Project.Postgres, d.Project.LocalForwards)
	}

	for _, bad := range []map[string]any{
		{"resources.memory": "lots"}, {"resources.cpus": "0"}, {"name": "renamed"}, {"workspace": "shop"},
	} {
		if _, err := Edit(store, "shop", bad); err == nil {
			t.Errorf("%v was accepted", bad)
		}
	}
	if _, err := Edit(store, "shop", map[string]any{"resources.memory": ""}); err != nil {
		t.Errorf("empty memory takes the default: %v", err)
	}
}

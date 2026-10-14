package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileStore(t *testing.T) {
	s, err := Open("file", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(Global, "GH_TOKEN", "g"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("api", "GH_TOKEN", "p"); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := Lookup(s, "api", "GH_TOKEN"); !ok || v != "p" {
		t.Fatalf("project value should win, got %q", v)
	}
	if v, ok, _ := Lookup(s, "web", "GH_TOKEN"); !ok || v != "g" {
		t.Fatalf("global fallback, got %q", v)
	}
	if err := s.Delete("api", "GH_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("api", "GH_TOKEN"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Set("api", "bad-key", "x"); err == nil {
		t.Fatal("want invalid key error")
	}
	if err := s.Set("api", "K", "a\nb"); err == nil {
		t.Fatal("want newline error")
	}
}

func TestFileStoreRefusesOpenPermissions(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open("file", dir)
	s.Set("api", "K", "v")
	os.Chmod(filepath.Join(dir, "secrets", "api.env"), 0o644)
	if _, err := s.Get("api", "K"); err == nil {
		t.Fatal("want permission error")
	}
}

func TestEnvStore(t *testing.T) {
	t.Setenv("JAILWICK_SECRET_GH_TOKEN", "g")
	t.Setenv("JAILWICK_SECRET_MY_API_GH_TOKEN", "p")
	s, _ := Open("env", "")
	if v, _, _ := Lookup(s, "my-api", "GH_TOKEN"); v != "p" {
		t.Fatalf("got %q", v)
	}
	if v, _, _ := Lookup(s, "web", "GH_TOKEN"); v != "g" {
		t.Fatalf("got %q", v)
	}
	if err := s.Set("api", "K", "v"); err == nil {
		t.Fatal("env store must be read-only")
	}
}

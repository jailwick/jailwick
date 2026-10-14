//go:build !windows

package untrusted

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(secret, []byte("key"), 0o600)
	os.MkdirAll(filepath.Join(dir, "to-web"), 0o755)
	os.WriteFile(filepath.Join(dir, "to-web", "a.md"), []byte("0123456789"), 0o644)
	os.Symlink(secret, filepath.Join(dir, "to-web", "link.md"))
	os.Symlink(filepath.Dir(secret), filepath.Join(dir, "to-api"))
	if err := syscall.Mkfifo(filepath.Join(dir, "to-web", "pipe.md"), 0o644); err != nil {
		t.Fatal(err)
	}

	if data, cut, err := ReadFile(dir, "to-web/a.md", 4); err != nil || string(data) != "0123" || !cut {
		t.Fatalf("a regular file, cut at the limit: %q %v %v", data, cut, err)
	}
	for _, rel := range []string{"to-web/link.md", "to-api/secret", "to-web/pipe.md", "../x", "/etc/passwd"} {
		if _, _, err := ReadFile(dir, rel, 100); err == nil {
			t.Errorf("%s must be refused", rel)
		}
	}
}

func TestFilesAndFolders(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "to-web"), 0o755)
	for _, n := range []string{"b.md", "a.md", "c.md"} {
		os.WriteFile(filepath.Join(dir, "to-web", n), nil, 0o644)
	}
	os.Symlink("/etc/passwd", filepath.Join(dir, "to-web", "x.md"))
	os.Symlink(t.TempDir(), filepath.Join(dir, "to-api"))

	names, more, err := Files(dir, "to-web", 2)
	if err != nil || !slices.Equal(names, []string{"a.md", "b.md"}) || !more {
		t.Fatalf("%v %v %v", names, more, err)
	}
	if got, _ := Folders(dir, "to-"); !slices.Equal(got, []string{"to-web"}) {
		t.Fatalf("a link to a folder isn't one: %v", got)
	}
}

func TestCreateFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), ".zshenv")
	os.Symlink(target, filepath.Join(dir, "README.md")) // dangling: the target doesn't exist

	if created, err := CreateFile(dir, "README.md", []byte("x"), 0o644); err != nil || created {
		t.Fatalf("never through a link: %v %v", created, err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the link's target was created")
	}
	if created, err := CreateFile(dir, "NEW.md", []byte("x"), 0o644); err != nil || !created {
		t.Fatalf("a new file: %v %v", created, err)
	}
}

func TestText(t *testing.T) {
	for in, want := range map[string]string{
		"fix login":                           "fix login",
		"héllo ✓ 日本":                          "héllo ✓ 日本",
		"a\tb\nc":                             "a\tb\nc",
		"\x1b]52;c;Y3VybCBldmlsIHwgc2g=\x07x": "]52;c;Y3VybCBldmlsIHwgc2g=x",
		"\x1b[2J\x1b[Hfake":                   "[2J[Hfake",
		"abc‮gpj.exe":                         "abcgpj.exe",
		"\u009bc":                             "c",
	} {
		if got := Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClean(t *testing.T) {
	type session struct {
		Name string
		Tags []string
		Meta map[string]string
		Sub  *session
	}
	v := []session{{Name: "a\x1b[31m", Tags: []string{"\x07t"}, Meta: map[string]string{"k": "v\x1b"}, Sub: &session{Name: "\x1bb"}}}
	Clean(&v)
	if v[0].Name != "a[31m" || v[0].Tags[0] != "t" || v[0].Meta["k"] != "v" || v[0].Sub.Name != "b" {
		t.Fatalf("%+v %+v", v[0], v[0].Sub)
	}
}

func TestLimitedBuffer(t *testing.T) {
	b := &LimitedBuffer{Max: 4}
	if _, err := b.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("de")); !errors.Is(err, ErrLimit) || string(b.Bytes()) != "abc" {
		t.Fatalf("%v %q", err, b.Bytes())
	}
}

func TestLineWriter(t *testing.T) {
	var got []string
	w := &LineWriter{Max: 5, Line: func(b []byte) { got = append(got, string(b)) }}
	w.Write([]byte("a\nb"))
	w.Write([]byte("c\n\nd"))
	w.Write([]byte("efghij-too-long"))
	w.Write([]byte("-still\nok\n"))
	if strings.Join(got, "|") != "a|bc||ok" {
		t.Fatalf("%q", got)
	}
}

package envfile

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	got, err := Parse(strings.NewReader(`
# a comment
export A=1
B = two words
C="quoted # not a comment"
D='single'
E=plain # a comment
F=
=nokey
noequals
A=last
`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "last", "B": "two words", "C": "quoted # not a comment", "D": "single", "E": "plain", "F": ""}
	if !maps.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReadRefusesFolders(t *testing.T) {
	dir := t.TempDir()
	if _, err := Read(dir); err == nil {
		t.Fatal("a folder is not an env file")
	}
	file := filepath.Join(dir, ".env")
	if err := os.WriteFile(file, []byte("K=v\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if env, err := Read(file); err != nil || env["K"] != "v" {
		t.Fatalf("%v %v", env, err)
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergePath(t *testing.T) {
	dir := t.TempDir()
	extra := filepath.Join(dir, "bin")
	os.Mkdir(extra, 0o755)
	got := mergePath("/usr/bin:/bin:", []string{"/bin", extra, filepath.Join(dir, "missing")})
	if want := "/usr/bin:/bin:" + extra; got != want {
		t.Fatalf("mergePath = %q, want %q", got, want)
	}
}

func TestLoginShellPathIgnoresStartupOutput(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "noisy-sh")
	// A login shell whose startup files print a greeting before the command's output.
	os.WriteFile(shell, []byte("#!/bin/sh\necho 'Welcome!'\nPATH=/from/login:/usr/bin\nexport PATH\nshift\nexec /bin/sh -c \"$2\"\n"), 0o755)
	t.Setenv("SHELL", shell)
	if got := loginShellPath(); got != "/from/login:/usr/bin" {
		t.Fatalf("loginShellPath = %q", got)
	}
}

// The first start asks the login shell and keeps its answer for the next ones.
func TestUseLoginPathKeepsTheAnswer(t *testing.T) {
	dir := t.TempDir()
	shell := filepath.Join(dir, "sh")
	os.WriteFile(shell, []byte("#!/bin/sh\nPATH=/from/login:/usr/bin\nexport PATH\nshift\nexec /bin/sh -c \"$2\"\n"), 0o755)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/usr/bin")
	cache := filepath.Join(dir, "state", "login-path")
	useLoginPath(cache)
	if data, _ := os.ReadFile(cache); string(data) != "/from/login:/usr/bin" {
		t.Fatalf("cache = %q", data)
	}
	if !strings.HasPrefix(os.Getenv("PATH"), "/from/login:/usr/bin") {
		t.Fatalf("PATH = %q", os.Getenv("PATH"))
	}
}

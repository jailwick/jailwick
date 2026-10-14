package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// An app opened from Finder, Launchpad, the Dock or a desktop launcher gets a minimal PATH
// (macOS: /usr/bin:/bin:/usr/sbin:/sbin) without the container runtime's command (docker in
// /usr/local/bin or ~/.orbstack/bin, Homebrew) or jailwick itself. useLoginPath sets PATH as the
// user's login shell has it, as in a terminal, plus the usual install folders.
//
// Starting a login shell can take seconds, so its answer is kept in cache: with one, the app
// starts on it at once and asks the shell again in the background for the next start.
func useLoginPath(cache string) {
	if runtime.GOOS == "windows" {
		return // Windows apps get the user's PATH
	}
	if data, err := os.ReadFile(cache); err == nil && len(data) > 0 {
		setPath(string(data))
		go func() {
			if path := loginShellPath(); path != "" {
				setPath(path)
				savePath(cache, path)
			}
		}()
		return
	}
	path := loginShellPath()
	if path != "" {
		savePath(cache, path)
	} else {
		path = os.Getenv("PATH")
	}
	setPath(path)
}

// setPath sets PATH to path plus the usual install folders.
func setPath(path string) {
	home, _ := os.UserHomeDir()
	_ = os.Setenv("PATH", mergePath(path, []string{
		filepath.Join(home, ".local", "bin"), filepath.Join(home, ".orbstack", "bin"), filepath.Join(home, ".docker", "bin"),
		"/opt/homebrew/bin", "/usr/local/bin", "/Applications/Docker.app/Contents/Resources/bin",
	}))
}

func savePath(cache, path string) {
	if os.MkdirAll(filepath.Dir(cache), 0o755) == nil {
		_ = os.WriteFile(cache, []byte(path), 0o644)
	}
}

// pathMarker separates PATH from whatever the shell's startup files print.
const pathMarker = "__jailwick_path__"

func loginShellPath() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-l", "-c", `printf '`+pathMarker+`%s' "$PATH"`).Output()
	if err != nil {
		return ""
	}
	_, path, ok := strings.Cut(string(out), pathMarker)
	if !ok {
		return ""
	}
	return strings.TrimSpace(path)
}

// mergePath appends the folders that exist and aren't in path yet.
func mergePath(path string, dirs []string) string {
	have := filepath.SplitList(path)
	for _, d := range dirs {
		if slices.Contains(have, d) {
			continue
		}
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			have = append(have, d)
		}
	}
	return strings.Join(slices.DeleteFunc(have, func(s string) bool { return s == "" }), string(os.PathListSeparator))
}

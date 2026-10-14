// Package hostutil holds small helpers about the host machine.
package hostutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Exists reports whether a path exists.
func Exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Timezone returns override, else the host's zone (from /etc/localtime or $TZ), else UTC.
func Timezone(override string) string {
	if override != "" {
		return override
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if _, zone, ok := strings.Cut(link, "zoneinfo/"); ok && zone != "" {
			return zone
		}
	}
	if tz := os.Getenv("TZ"); tz != "" && !strings.HasPrefix(tz, ":") {
		return tz
	}
	return "UTC"
}

// Arch is the host CPU architecture in container terms (arm64 or amd64).
func Arch() string { return runtime.GOARCH }

// Editor returns the user's editor command.
func Editor() string {
	for _, v := range []string{"VISUAL", "EDITOR"} {
		if e := os.Getenv(v); e != "" {
			return e
		}
	}
	return "vi"
}

// Edit opens a file in the user's editor and waits.
func Edit(file string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	parts := strings.Fields(Editor())
	cmd := exec.Command(parts[0], append(parts[1:], file)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// OpenURL opens a URL in the default browser.
func OpenURL(u string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	return exec.Command(name, u).Start()
}

// GitConfig reads a value from the user's git configuration ("" if unset).
func GitConfig(key string) string {
	out, err := exec.Command("git", "config", "--global", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// OpenFile opens a file in its default app; reveal shows it in the file manager instead (on
// Linux: its folder).
func OpenFile(path string, reveal bool) error {
	switch runtime.GOOS {
	case "darwin":
		if reveal {
			return exec.Command("open", "-R", path).Run()
		}
		return exec.Command("open", path).Run()
	case "windows":
		if reveal {
			return exec.Command("explorer", "/select,", path).Start()
		}
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	}
	if reveal {
		path = filepath.Dir(path)
	}
	return exec.Command("xdg-open", path).Start()
}

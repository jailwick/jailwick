// Package assets embeds the files jailwick writes or builds: Dockerfiles, the proxy config,
// allowlist presets, shared Claude instructions and the in-container agent binaries.
package assets

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

//go:embed images claude presets postgres agent shell
var files embed.FS

// MustRead returns an embedded file; the names are fixed at build time.
func MustRead(name string) []byte {
	b, err := files.ReadFile(name)
	if err != nil {
		panic("assets: " + err.Error())
	}
	return b
}

// Dir returns the files of an embedded folder (non-recursive), keyed by base name.
func Dir(dir string) map[string][]byte {
	out := map[string][]byte{}
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		panic("assets: " + err.Error())
	}
	for _, e := range entries {
		if !e.IsDir() {
			out[e.Name()] = MustRead(path.Join(dir, e.Name()))
		}
	}
	return out
}

// ParseDomains reads one domain per line, ignoring blanks and # comments.
func ParseDomains(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line != "" {
			out = append(out, strings.ToLower(line))
		}
	}
	return out
}

// Agent returns the jailwick-agent binary for a Linux architecture (arm64 or amd64).
// $JAILWICK_AGENT_DIR overrides the embedded copies (development builds).
func Agent(arch string) ([]byte, error) {
	name := "jailwick-agent-linux-" + arch
	if dir := os.Getenv("JAILWICK_AGENT_DIR"); dir != "" {
		return os.ReadFile(path.Join(dir, name))
	}
	b, err := files.ReadFile("agent/" + name)
	if err != nil {
		return nil, fmt.Errorf("this jailwick build has no %s; build with `make` (or set JAILWICK_AGENT_DIR)", name)
	}
	return b, nil
}

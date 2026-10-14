// Package envfile reads dotenv files: KEY=value lines, as apps and their tools write them.
package envfile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// MaxSize is the largest file Read accepts; env files are small, anything bigger is not one.
const MaxSize = 1 << 20

// Parse reads KEY=value lines. Blank lines and # comments are skipped, an `export ` prefix is
// dropped, a value in single or double quotes is taken as written, an unquoted value ends at " #".
// The last assignment of a key wins.
func Parse(r io.Reader) (map[string]string, error) {
	env := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if k = strings.TrimSpace(k); !ok || k == "" {
			continue
		}
		env[k] = value(strings.TrimSpace(v))
	}
	return env, sc.Err()
}

func value(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
			return v[1 : end+1]
		}
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

// Read parses an env file. It refuses what isn't a regular file of at most MaxSize.
func Read(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a file", path)
	}
	if st.Size() > MaxSize {
		return nil, fmt.Errorf("%s is too big for an env file", path)
	}
	return Parse(f)
}

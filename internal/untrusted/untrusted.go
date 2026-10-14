// Package untrusted is how the host reads and writes data a sandbox can control: files in
// sandbox-writable folders (handoffs) and what processes in the sandbox print (the agent's
// answers). Such data is input, never instructions, and may be crafted to trick its reader.
// See docs/UNTRUSTED-DATA.md.
//
// The rules: never follow a link; read regular files only, and only up to a limit; create files
// only where nothing exists yet; strip control characters from text before a terminal shows it.
package untrusted

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
)

// ErrLimit is a read that went past its limit.
var ErrLimit = errors.New("too large")

// ReadFile reads up to max bytes of the regular file rel inside dir. No part of rel may be a
// link, and rel must stay inside dir; a larger file is cut at max (truncated is then true).
func ReadFile(dir, rel string, max int64) (data []byte, truncated bool, err error) {
	path, err := inside(dir, rel)
	if err != nil {
		return nil, false, err
	}
	f, err := openNoFollow(path, os.O_RDONLY|nonBlock, 0)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if !st.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%s: not a regular file", path)
	}
	data, err = io.ReadAll(io.LimitReader(f, max+1))
	if int64(len(data)) > max {
		return data[:max], true, err
	}
	return data, false, err
}

// Files lists the regular files of the folder rel inside dir (links and other kinds are left
// out), at most max of them, sorted by name; more is true when there were more.
func Files(dir, rel string, max int) (names []string, more bool, err error) {
	path, err := inside(dir, rel)
	if err != nil {
		return nil, false, err
	}
	if st, err := os.Lstat(path); err != nil || !st.IsDir() {
		return nil, false, fmt.Errorf("%s: not a folder", path)
	}
	entries, err := os.ReadDir(path) // sorted by name; types from Lstat
	if err != nil {
		return nil, false, err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if len(names) == max {
			return names, true, nil
		}
		names = append(names, e.Name())
	}
	return names, false, nil
}

// Folders lists the folders (not links to folders) of dir whose names start with prefix.
func Folders(dir, prefix string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && e.Type()&fs.ModeSymlink == 0 && strings.HasPrefix(e.Name(), prefix) {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// CreateFile writes a new file rel inside dir. It never writes through an existing path: when
// anything is there (a file, a folder, a link, even a broken one), it does nothing and created
// is false.
func CreateFile(dir, rel string, data []byte, perm os.FileMode) (created bool, err error) {
	path, err := inside(dir, rel)
	if err != nil {
		return false, err
	}
	f, err := openNoFollow(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return true, err
	}
	return true, f.Close()
}

// inside joins rel to dir and checks that the result stays in dir with no link on the way.
// dir itself is the host's choice (it may be a link); what is inside it is not.
func inside(dir, rel string) (string, error) {
	rel = filepath.Clean(rel)
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%s: outside %s", rel, dir)
	}
	path := dir
	parts := strings.Split(rel, string(filepath.Separator))
	for i, p := range parts {
		if p == "." {
			continue
		}
		path = filepath.Join(path, p)
		if i == len(parts)-1 {
			break // the last part: the caller's open refuses a link
		}
		if st, err := os.Lstat(path); err == nil && st.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("%s: a link", path)
		}
	}
	return path, nil
}

// Text makes sandbox text safe to show in a terminal: control characters (escape sequences
// included) and invisible direction marks are removed; tabs and new lines stay.
func Text(s string) string {
	if strings.IndexFunc(s, unsafeRune) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return -1
		}
		return r
	}, s)
}

func unsafeRune(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case unicode.IsControl(r): // C0, DEL and C1, ESC included
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r == 0x200E, r == 0x200F: // bidi
		return true
	}
	return false
}

// Clean applies Text to every string in v (a pointer to decoded JSON: structs, slices, maps).
func Clean(v any) {
	clean(reflect.ValueOf(v))
}

func clean(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			clean(v.Elem())
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				clean(v.Field(i))
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			clean(v.Index(i))
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			e := v.MapIndex(k)
			if e.Kind() == reflect.String {
				v.SetMapIndex(k, reflect.ValueOf(Text(e.String())).Convert(e.Type()))
				continue
			}
			c := reflect.New(e.Type()).Elem()
			c.Set(e)
			clean(c)
			v.SetMapIndex(k, c)
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(Text(v.String()))
		}
	}
}

// LimitedBuffer collects at most Max bytes of output; writing more fails with ErrLimit, which
// stops the writer (an exec's output, for instance).
type LimitedBuffer struct {
	Max int
	buf []byte
}

func (b *LimitedBuffer) Write(p []byte) (int, error) {
	if len(b.buf)+len(p) > b.Max {
		return 0, fmt.Errorf("output over %d bytes: %w", b.Max, ErrLimit)
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *LimitedBuffer) Bytes() []byte { return b.buf }

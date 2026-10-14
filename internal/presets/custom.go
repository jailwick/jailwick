package presets

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/jailwick/jailwick/internal/paths"
)

// Custom presets are the user's own groups of hosts, one file each in <config>/presets, in the
// same format as the built-in ones. They join the catalogue under CustomCategory, so a project
// lists them in allow_presets like any other.

// CustomCategory is the category of the user's presets.
const CustomCategory = "custom"

var (
	nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	hostRE = regexp.MustCompile(`^\.?([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,63}$`)
)

// ValidHost reports whether h is an allowlist entry: a domain, or .domain for it and its
// subdomains, in lowercase.
func ValidHost(h string) bool { return hostRE.MatchString(h) }

// CustomDir holds the user's presets.
func CustomDir() string { return filepath.Join(paths.ConfigDir(), "presets") }

func customFile(name string) string { return filepath.Join(CustomDir(), name+".yaml") }

// custom is the stored form: what the user chooses (no detection, never always on).
type custom struct {
	Description string   `yaml:"description"`
	Hosts       []string `yaml:"hosts"`
}

// Custom reads the user's presets, sorted by name. Files that don't make a valid preset are left
// out and reported in the error; the others are returned anyway.
func Custom() ([]Preset, error) {
	files, err := filepath.Glob(filepath.Join(CustomDir(), "*.yaml"))
	if err != nil {
		return nil, err
	}
	var out []Preset
	var errs []error
	for _, f := range files {
		p, err := readCustom(f)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", paths.Tilde(f), err))
			continue
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Preset) int { return strings.Compare(a.Name, b.Name) })
	return out, errors.Join(errs...)
}

func readCustom(file string) (Preset, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Preset{}, err
	}
	var c custom
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Preset{}, err
	}
	p := Preset{
		Name: strings.TrimSuffix(filepath.Base(file), ".yaml"), Description: c.Description,
		Category: CustomCategory, Hosts: c.Hosts,
	}
	return p, checkCustom(p)
}

func checkCustom(p Preset) error {
	if !nameRE.MatchString(p.Name) {
		return fmt.Errorf("invalid preset name %q: use lowercase letters, digits and dashes (max 40)", p.Name)
	}
	if _, ok := builtin(p.Name); ok {
		return fmt.Errorf("%q is a built-in preset; choose another name", p.Name)
	}
	if len(p.Hosts) == 0 {
		return errors.New("no hosts")
	}
	for _, h := range p.Hosts {
		if !ValidHost(h) {
			return fmt.Errorf("%q is not a domain (example.com, or .example.com with its subdomains)", h)
		}
	}
	return nil
}

// SaveCustom creates or replaces one of the user's presets. Hosts must be normalized entries
// (see proxy.Normalize).
func SaveCustom(name, description string, hosts []string) error {
	p := Preset{Name: name, Description: description, Category: CustomCategory, Hosts: hosts}
	if err := checkCustom(p); err != nil {
		return err
	}
	data, err := yaml.Marshal(custom{Description: description, Hosts: hosts})
	if err != nil {
		return err
	}
	return writeAtomic(customFile(name), append([]byte("# A jailwick allowlist preset of your own.\n"), data...))
}

// DeleteCustom removes one of the user's presets.
func DeleteCustom(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid preset name %q", name)
	}
	err := os.Remove(customFile(name))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no preset of yours named %q", name)
	}
	return err
}

// writeAtomic replaces a file through a temporary one, so it is never seen half-written.
func writeAtomic(file string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

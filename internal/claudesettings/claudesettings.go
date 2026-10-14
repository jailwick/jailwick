// Package claudesettings builds the Claude settings a sandbox passes with --settings. They are
// layers, each overriding the ones before it:
//
//	jailwick   the built-in defaults
//	app        <config>/claude/settings.json, every project
//	profiles   <config>/claude/profiles/<name>.json, the projects that list them (claude.profiles)
//	project    <config>/notes/projects/<name>/settings.json, one project
//
// Claude Code's own ~/.claude/settings.json (what /model and /config save) stays under these:
// settings passed with --settings take precedence over it.
package claudesettings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strings"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/paths"
)

// Layer is one source of settings.
type Layer struct {
	Name string // where it comes from: "jailwick" or a file
	Data []byte
}

// Layers returns a project's settings layers in order, skipping the optional files that don't
// exist. A profile the project lists must exist. p is the resolved project.
func Layers(s *config.Store, p config.Project) ([]Layer, error) {
	layers := []Layer{{Name: "jailwick", Data: assets.MustRead("claude/settings.json")}}
	add := func(file string, required func() error) error {
		data, err := os.ReadFile(file)
		switch {
		case errors.Is(err, fs.ErrNotExist) && required != nil:
			return required()
		case errors.Is(err, fs.ErrNotExist):
			return nil
		case err != nil:
			return err
		}
		layers = append(layers, Layer{Name: paths.Tilde(file), Data: data})
		return nil
	}
	if err := add(s.ClaudeSettingsFile(), nil); err != nil {
		return nil, err
	}
	for _, name := range p.Claude.Profiles {
		err := add(s.ProfileFile(name), func() error { return missingProfile(s, name) })
		if err != nil {
			return nil, err
		}
	}
	if err := add(s.ProjectSettingsFile(p.Name), nil); err != nil {
		return nil, err
	}
	return layers, nil
}

func missingProfile(s *config.Store, name string) error {
	have, _ := s.Profiles()
	list := "none yet"
	if len(have) > 0 {
		list = strings.Join(have, ", ")
	}
	return fmt.Errorf("claude.profiles: no profile %q (have: %s); create it with: jw settings edit --profile %s",
		name, list, name)
}

// Build returns the merged settings of a project. p is the resolved project.
func Build(s *config.Store, p config.Project) ([]byte, error) {
	layers, err := Layers(s, p)
	if err != nil {
		return nil, err
	}
	return Merge(layers)
}

// Merge combines layers in order: objects merge key by key, arrays keep every distinct element
// (earlier ones first), and any other value from a later layer replaces the earlier one. A later
// layer can't remove an array element, such as a permission rule, that an earlier one added.
func Merge(layers []Layer) ([]byte, error) {
	merged := map[string]any{}
	for _, l := range layers {
		obj, err := decode(l.Data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", l.Name, err)
		}
		merged = mergeObjects(merged, obj)
	}
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// Check reports whether a settings file is a valid JSON object.
func Check(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if _, err := decode(data); err != nil {
		return fmt.Errorf("%s: %w", paths.Tilde(file), err)
	}
	return nil
}

// decode reads one JSON object, keeping numbers exactly as written.
func decode(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("not a JSON object: %w", err)
	}
	if obj == nil {
		return nil, errors.New("not a JSON object: null")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("not a JSON object: more than one value")
	}
	return obj, nil
}

func mergeObjects(dst, src map[string]any) map[string]any {
	for k, v := range src {
		dst[k] = mergeValues(dst[k], v)
	}
	return dst
}

func mergeValues(old, v any) any {
	switch nv := v.(type) {
	case map[string]any:
		if ov, ok := old.(map[string]any); ok {
			return mergeObjects(ov, nv)
		}
	case []any:
		if ov, ok := old.([]any); ok {
			out := ov
			for _, e := range nv {
				if !containsValue(out, e) {
					out = append(out, e)
				}
			}
			return out
		}
	}
	return v
}

func containsValue(list []any, v any) bool {
	for _, e := range list {
		if reflect.DeepEqual(e, v) {
			return true
		}
	}
	return false
}

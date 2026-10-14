// Package secrets stores tokens outside the YAML files: in the macOS Keychain, a 0600 file, or
// host environment variables. A secret belongs to a scope: a project name, or Global for values
// shared by every project (looked up after the project's own).
package secrets

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"slices"
)

// Global is the scope shared by all projects.
const Global = "_global"

// Well-known secrets passed to every sandbox when set.
var Known = []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "GH_TOKEN"}

// LoginOverrides are the secrets Claude Code uses instead of its login. They limit it: Remote
// Control and Claude in Chrome need the login (the token from claude setup-token is chat only).
var LoginOverrides = []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY"}

// ErrNotFound means the secret isn't set.
var ErrNotFound = errors.New("secret not set")

var keyRE = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// Store is a secret backend.
type Store interface {
	Name() string
	Get(scope, key string) (string, error)
	Set(scope, key, value string) error
	Delete(scope, key string) error
	// Keys lists the names set in a scope (never values).
	Keys(scope string) ([]string, error)
}

// Open returns the backend by name: keychain, file or env. Empty picks keychain on macOS and
// file elsewhere.
func Open(backend, configDir string) (Store, error) {
	if backend == "" {
		backend = "file"
		if runtime.GOOS == "darwin" {
			backend = "keychain"
		}
	}
	switch backend {
	case "keychain":
		if runtime.GOOS != "darwin" {
			return nil, errors.New("secrets: keychain is only available on macOS; use file")
		}
		return newKeychain(configDir), nil
	case "file":
		return &fileStore{dir: configDir}, nil
	case "env":
		return envStore{}, nil
	}
	return nil, fmt.Errorf("secrets: unknown backend %q (use keychain, file or env)", backend)
}

// CheckKey validates a secret name.
func CheckKey(key string) error {
	if !keyRE.MatchString(key) {
		return fmt.Errorf("invalid secret name %q: use UPPER_CASE letters, digits and _", key)
	}
	return nil
}

// Lookup returns a project's secret, falling back to the global scope.
func Lookup(s Store, project, key string) (string, bool, error) {
	for _, scope := range []string{project, Global} {
		v, err := s.Get(scope, key)
		if err == nil {
			return v, true, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", false, err
		}
	}
	return "", false, nil
}

// Resolve returns the values of the given keys for a project, skipping unset ones.
func Resolve(s Store, project string, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, k := range uniq(keys) {
		v, ok, err := Lookup(s, project, k)
		if err != nil {
			return nil, fmt.Errorf("secret %s: %w", k, err)
		}
		if ok {
			out[k] = v
		}
	}
	return out, nil
}

func uniq(keys []string) []string {
	out := slices.Clone(keys)
	slices.Sort(out)
	return slices.Compact(out)
}

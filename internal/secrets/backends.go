package secrets

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// ---- file: <config>/secrets/<scope>.env, mode 0600 ----

type fileStore struct{ dir string }

func (f *fileStore) Name() string { return "file" }

func (f *fileStore) path(scope string) string {
	return filepath.Join(f.dir, "secrets", scope+".env")
}

func (f *fileStore) read(scope string) (map[string]string, error) {
	file := f.path(scope)
	st, err := os.Stat(file)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if st.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is readable by others (mode %v); run: chmod 600 %s", file, st.Mode().Perm(), file)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			m[k] = v
		}
	}
	return m, sc.Err()
}

func (f *fileStore) write(scope string, m map[string]string) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	b.WriteString("# jailwick secrets. Keep this file private (mode 0600).\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, m[k])
	}
	file := f.path(scope)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func (f *fileStore) Get(scope, key string) (string, error) {
	m, err := f.read(scope)
	if err != nil {
		return "", err
	}
	v, ok := m[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *fileStore) Set(scope, key, value string) error {
	if err := CheckKey(key); err != nil {
		return err
	}
	if strings.ContainsAny(value, "\r\n") {
		return errors.New("secret values can't contain line breaks")
	}
	m, err := f.read(scope)
	if err != nil {
		return err
	}
	m[key] = value
	return f.write(scope, m)
}

func (f *fileStore) Delete(scope, key string) error {
	m, err := f.read(scope)
	if err != nil {
		return err
	}
	if _, ok := m[key]; !ok {
		return ErrNotFound
	}
	delete(m, key)
	return f.write(scope, m)
}

func (f *fileStore) Keys(scope string) ([]string, error) {
	m, err := f.read(scope)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys, nil
}

// ---- keychain: macOS `security` generic passwords, service "jailwick", account <scope>/<key> ----
// The Keychain can't list items cheaply, so key names (not values) are kept in an index file.

type keychain struct{ index *fileStore }

func newKeychain(configDir string) *keychain {
	return &keychain{index: &fileStore{dir: filepath.Join(configDir, "keychain-index")}}
}

const keychainService = "jailwick"

func (k *keychain) Name() string { return "keychain" }

func (k *keychain) Get(scope, key string) (string, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", keychainService, "-a", scope+"/"+key, "-w").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 44 {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("keychain: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (k *keychain) Set(scope, key, value string) error {
	if err := CheckKey(key); err != nil {
		return err
	}
	// -U updates an existing item. The value is passed as an argument (visible to local
	// processes for a moment); `security` offers no stdin form for -w with a value.
	cmd := exec.Command("security", "add-generic-password", "-U", "-s", keychainService, "-a", scope+"/"+key, "-l", "jailwick "+scope+" "+key, "-w", value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain: %v: %s", err, bytes.TrimSpace(out))
	}
	return k.index.Set(scope, key, "1")
}

func (k *keychain) Delete(scope, key string) error {
	out, err := exec.Command("security", "delete-generic-password", "-s", keychainService, "-a", scope+"/"+key).CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 44 {
			_ = k.index.Delete(scope, key)
			return ErrNotFound
		}
		return fmt.Errorf("keychain: %v: %s", err, bytes.TrimSpace(out))
	}
	if err := k.index.Delete(scope, key); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

func (k *keychain) Keys(scope string) ([]string, error) { return k.index.Keys(scope) }

// ---- env: read-only, JAILWICK_SECRET_<KEY> or JAILWICK_SECRET_<SCOPE>_<KEY> (for CI) ----

type envStore struct{}

func (envStore) Name() string { return "env" }

func envName(scope, key string) string {
	if scope == Global {
		return "JAILWICK_SECRET_" + key
	}
	return "JAILWICK_SECRET_" + strings.ToUpper(strings.ReplaceAll(scope, "-", "_")) + "_" + key
}

func (envStore) Get(scope, key string) (string, error) {
	if v, ok := os.LookupEnv(envName(scope, key)); ok {
		return v, nil
	}
	return "", ErrNotFound
}

func (envStore) Set(scope, key, _ string) error {
	return fmt.Errorf("secrets backend env is read-only: export %s instead", envName(scope, key))
}

func (envStore) Delete(scope, key string) error {
	return fmt.Errorf("secrets backend env is read-only: unset %s instead", envName(scope, key))
}

func (envStore) Keys(scope string) ([]string, error) {
	prefix := envName(scope, "")
	var keys []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if k, ok := strings.CutPrefix(name, prefix); ok && keyRE.MatchString(k) {
			if scope == Global && strings.Contains(k, "_") && !slices.Contains(Known, k) {
				// JAILWICK_SECRET_<PROJECT>_<KEY> also matches the global prefix; keep known keys only.
				continue
			}
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys, nil
}

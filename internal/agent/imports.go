package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// ImportApprover pre-approves the CLAUDE.md imports of a folder, so Claude doesn't ask about the
// shared instructions (they live outside the repo). It's an interface so the mechanism can be
// replaced without touching its callers.
type ImportApprover interface {
	Approve(dir string) error
}

// importApprover is the approver sessions use.
func importApprover(c *agentapi.Config) ImportApprover {
	cfgDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfgDir == "" {
		cfgDir = filepath.Join(c.Home, ".claude")
	}
	return claudeJSON{file: filepath.Join(cfgDir, ".claude.json")}
}

// claudeJSON approves imports in Claude's own config file, the only place Claude reads it from.
// That file may be shared by several sandboxes (the app login) and is rewritten by Claude
// without a lock jailwick can take, so the write is safe instead: nothing happens once approved,
// the file is replaced atomically (never seen half-written), and the result is read back.
type claudeJSON struct{ file string }

// approvalKeys are the per-folder flags Claude checks before asking about external imports.
var approvalKeys = []string{"hasClaudeMdExternalIncludesApproved", "hasClaudeMdExternalIncludesWarningShown"}

func (a claudeJSON) Approve(dir string) error {
	for range 2 { // a second try if another writer replaced the file in between
		doc, approved, err := a.read(dir)
		if err != nil || approved {
			return err
		}
		if err := a.write(approve(doc, dir)); err != nil {
			return err
		}
		if _, approved, err = a.read(dir); err != nil || approved {
			return err
		}
	}
	return errors.New(a.file + " was rewritten by another process; Claude may ask about the imports once")
}

// read returns the config document (empty when missing) and whether dir is approved.
func (a claudeJSON) read(dir string) (map[string]any, bool, error) {
	doc := map[string]any{}
	data, err := os.ReadFile(a.file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return doc, false, nil
	case err != nil:
		return nil, false, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep Claude's numbers exactly as written
	if err := dec.Decode(&doc); err != nil {
		return nil, false, fmt.Errorf("%s: %w", a.file, err)
	}
	projects, _ := doc["projects"].(map[string]any)
	entry, _ := projects[dir].(map[string]any)
	for _, k := range approvalKeys {
		if entry[k] != true {
			return doc, false, nil
		}
	}
	return doc, true, nil
}

// approve sets dir's approval flags in doc.
func approve(doc map[string]any, dir string) map[string]any {
	projects, _ := doc["projects"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
		doc["projects"] = projects
	}
	entry, _ := projects[dir].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
		projects[dir] = entry
	}
	for _, k := range approvalKeys {
		entry[k] = true
	}
	return doc
}

// write replaces the file atomically through a temporary file of its own (sandboxes sharing the
// folder never use the same one).
func (a claudeJSON) write(doc map[string]any) error {
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(a.file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".claude.json.jailwick-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op once renamed
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), a.file)
}

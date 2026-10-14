// Package history reads Claude Code's saved conversations (<config>/projects/*/*.jsonl), in
// the sandbox and on the host. It only reads.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Options select conversations.
type Options struct {
	Limit   int                     // 0: no limit
	Keep    func(dir string) bool   // nil: all folders
	Where   func(dir string) string // label for a folder; nil: the folder
	Missing bool                    // include conversations whose folder is gone
}

// Root is Claude's projects folder for a config dir (or $CLAUDE_CONFIG_DIR, or ~/.claude).
func Root(configDir string) string {
	if configDir == "" {
		configDir = os.Getenv("CLAUDE_CONFIG_DIR")
	}
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".claude")
	}
	return filepath.Join(configDir, "projects")
}

type entry struct {
	path string
	mod  time.Time
}

// List returns conversations newest first. Conversations without a prompt are skipped.
func List(root string, o Options) []agentapi.Conversation {
	files, _ := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	entries := make([]entry, 0, len(files))
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			entries = append(entries, entry{f, st.ModTime()})
		}
	}
	slices.SortFunc(entries, func(a, b entry) int { return b.mod.Compare(a.mod) })
	out := []agentapi.Conversation{} // none is an empty list, never null
	for _, e := range entries {
		if o.Limit > 0 && len(out) >= o.Limit {
			break
		}
		dir, prompts := scan(e.path, 1)
		if dir == "" || len(prompts) == 0 || (o.Keep != nil && !o.Keep(dir)) {
			continue
		}
		_, err := os.Stat(dir)
		gone := err != nil
		if gone && !o.Missing {
			continue
		}
		where := dir
		if o.Where != nil {
			where = o.Where(dir)
		}
		out = append(out, agentapi.Conversation{
			ID: strings.TrimSuffix(filepath.Base(e.path), ".jsonl"), Dir: dir, Where: where,
			When: e.mod, Prompt: oneLine(prompts[0], 120), Name: oneLine(Name(e.path), 80), Gone: gone, File: e.path,
		})
	}
	return out
}

// nameTail is how much of the end of a conversation file Name reads when Claude Code left no
// title file: a whole file can be tens of megabytes.
const nameTail = 1 << 20

// Name returns the name a conversation was given with /rename, "" for none. Claude Code keeps it
// in <id>/custom-title.json next to the conversation file, and as a "custom-title" line in the
// file itself (the last one wins), which is read from the file's end.
func Name(file string) string {
	var t struct {
		CustomTitle string `json:"customTitle"`
	}
	if data, err := os.ReadFile(filepath.Join(strings.TrimSuffix(file, ".jsonl"), "custom-title.json")); err == nil &&
		json.Unmarshal(data, &t) == nil && t.CustomTitle != "" {
		return t.CustomTitle
	}
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > nameTail {
		_, _ = f.Seek(-nameTail, io.SeekEnd)
	}
	name := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 32*1024*1024)
	for sc.Scan() {
		if !bytes.Contains(sc.Bytes(), []byte(`"custom-title"`)) {
			continue
		}
		var line struct {
			Type        string `json:"type"`
			CustomTitle string `json:"customTitle"`
		}
		if json.Unmarshal(sc.Bytes(), &line) == nil && line.Type == "custom-title" && line.CustomTitle != "" {
			name = line.CustomTitle
		}
	}
	return name
}

// Prompts returns the first user prompts of a conversation file.
func Prompts(file string, max int) []string {
	_, p := scan(file, max)
	return p
}

// scan reads the working folder and up to max user prompts (slash commands, tool output and
// meta entries excluded).
func scan(file string, max int) (dir string, prompts []string) {
	f, err := os.Open(file)
	if err != nil {
		return "", nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 32*1024*1024)
	for sc.Scan() {
		var line struct {
			Type    string `json:"type"`
			Cwd     string `json:"cwd"`
			IsMeta  bool   `json:"isMeta"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if dir == "" && line.Cwd != "" {
			dir = line.Cwd
		}
		if line.Type == "user" && !line.IsMeta && len(prompts) < max {
			if p := text(line.Message.Content); p != "" {
				prompts = append(prompts, p)
			}
		}
		if dir != "" && len(prompts) >= max {
			break
		}
	}
	return dir, prompts
}

// text extracts a prompt: a string, or the text parts of a content array.
func text(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &parts) != nil {
			return ""
		}
		var texts []string
		for _, p := range parts {
			if p.Type == "text" {
				texts = append(texts, p.Text)
			}
		}
		s = strings.Join(texts, " ")
	}
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "<") {
			return l
		}
	}
	return ""
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

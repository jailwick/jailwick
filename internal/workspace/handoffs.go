package workspace

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/untrusted"
)

// Handoff is one request file.
type Handoff struct {
	File    string
	Inbox   string // repo it is for (to-<repo>)
	From    string
	Status  string
	Created string
	Subject string
	Done    bool
	// Version identifies the file as read, for a change to it to be made on what was seen
	// (SetHandoffStatus, DeleteHandoff).
	Version string
}

// Limits on what a handoff folder can make the host read: the sandbox writes it.
const (
	maxHandoffs     = 1000     // requests listed, per inbox
	maxFrontMatter  = 64 << 10 // read of a request to list it
	maxHandoffBytes = 1 << 20  // read of a request to show it
)

// ListHandoffs reads the requests in a handoff folder (open and answered first, then done). The
// sandbox writes the folder, so it is read as untrusted data (package untrusted): links are left
// out, reads are bounded, text is cleaned for a terminal.
func ListHandoffs(dir string) ([]Handoff, error) {
	inboxes, err := untrusted.Folders(dir, "to-")
	if err != nil {
		return nil, err
	}
	var out []Handoff
	for _, box := range append(inboxes, "done") {
		names, _, err := untrusted.Files(dir, box, maxHandoffs)
		if err != nil {
			continue // no done folder yet, or not a folder
		}
		for _, n := range names {
			if filepath.Ext(n) != ".md" {
				continue
			}
			data, _, err := untrusted.ReadFile(dir, filepath.Join(box, n), maxFrontMatter)
			if err != nil {
				continue
			}
			out = append(out, handoffOf(dir, box, n, data))
		}
	}
	slices.SortStableFunc(out, func(a, b Handoff) int {
		if a.Done != b.Done {
			if a.Done {
				return 1
			}
			return -1
		}
		return strings.Compare(b.Created, a.Created)
	})
	return out, nil
}

// handoffOf describes the request name in the folder box (an inbox or done) from its content.
func handoffOf(dir, box, name string, data []byte) Handoff {
	h := frontMatter(data)
	h.File, h.Done = filepath.Join(dir, box, name), box == "done"
	h.Version = version(h.File)
	if !h.Done {
		h.Inbox = untrusted.Text(strings.TrimPrefix(box, "to-"))
	}
	return h
}

// frontMatter reads the fields of a request's front matter, cleaned for a terminal.
func frontMatter(data []byte) Handoff {
	var h Handoff
	sc := bufio.NewScanner(bytes.NewReader(data))
	inside := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "---" {
			if inside {
				break
			}
			inside = true
			continue
		}
		if !inside {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = untrusted.Text(strings.TrimSpace(v))
		if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		switch strings.TrimSpace(k) {
		case "from":
			h.From = v
		case "to":
			if h.Inbox == "" {
				h.Inbox = v
			}
		case "status":
			h.Status = v
		case "created":
			h.Created = v
		case "subject":
			h.Subject = v
		}
	}
	return h
}

// ReadHandoff reads one request of a handoff folder: its front matter and its body. The file must
// be a request of that folder (in an inbox or done), so a path from a page can't read elsewhere,
// and it is read as untrusted data like ListHandoffs does.
func ReadHandoff(dir, file string) (Handoff, string, error) {
	dir = filepath.Clean(dir)
	rel, err := filepath.Rel(dir, filepath.Clean(file))
	box, name, ok := strings.Cut(rel, string(filepath.Separator))
	if err != nil || !ok || strings.Contains(name, string(filepath.Separator)) || filepath.Ext(name) != ".md" ||
		!(box == "done" || strings.HasPrefix(box, "to-")) {
		return Handoff{}, "", fmt.Errorf("%s is not a request of %s: %w", file, dir, fs.ErrNotExist)
	}
	data, cut, err := untrusted.ReadFile(dir, rel, maxHandoffBytes)
	if err != nil {
		return Handoff{}, "", fmt.Errorf("%s: %w", file, fs.ErrNotExist)
	}
	text := untrusted.Text(body(string(data)))
	if cut {
		text += "\n… (only the first MB is shown)"
	}
	return handoffOf(dir, box, name, data), text, nil
}

// body is a request without its front matter.
func body(text string) string {
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return text
	}
	if _, after, ok := strings.Cut(rest, "\n---\n"); ok {
		return strings.TrimLeft(after, "\n")
	}
	return text
}

package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jailwick/jailwick/internal/untrusted"
)

// Keeping the books of a handoff folder from the app: a request's status changes (done moves it
// to done/, reopening moves it back to its inbox) and requests are deleted, as the README lets
// the user do by hand. Sessions write the same files, so every change names the version of the
// file it was decided on (Handoff.Version): a file changed since is left as it is
// (ErrHandoffChanged). The folder is a sandbox's to write: nothing is written through a link,
// files are created only where nothing is (untrusted.CreateFile), and a changed file replaces
// the old one by a rename.

// ErrHandoffChanged is a request changed (or moved) since the page read it.
var ErrHandoffChanged = errors.New("the request changed since it was read: look at it again")

// Statuses a request can have, in order.
var Statuses = []string{"open", "answered", "done"}

// version identifies a request's content as read: its time and size.
func version(path string) string {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		return ""
	}
	return strconv.FormatInt(st.ModTime().UnixNano(), 36) + "-" + strconv.FormatInt(st.Size(), 36)
}

// requestOf checks file is a request of dir (in an inbox or done, no link on the way) at the
// version given, and reads it whole.
func requestOf(dir, file, ver string) (box, name string, data []byte, err error) {
	dir = filepath.Clean(dir)
	rel, err := filepath.Rel(dir, filepath.Clean(file))
	box, name, ok := strings.Cut(rel, string(filepath.Separator))
	if err != nil || !ok || strings.Contains(name, string(filepath.Separator)) || filepath.Ext(name) != ".md" ||
		!(box == "done" || strings.HasPrefix(box, "to-")) {
		return "", "", nil, fmt.Errorf("%s is not a request of %s: %w", file, dir, fs.ErrNotExist)
	}
	if ver == "" || version(filepath.Join(dir, rel)) != ver {
		return "", "", nil, ErrHandoffChanged
	}
	data, cut, err := untrusted.ReadFile(dir, rel, maxHandoffBytes)
	if err != nil {
		return "", "", nil, fmt.Errorf("%s: %w", file, fs.ErrNotExist)
	}
	if cut {
		return "", "", nil, fmt.Errorf("%s is too large to change here: edit it yourself", file)
	}
	return box, name, data, nil
}

var inboxName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// SetHandoffStatus sets a request's status: done moves it to done/, another status takes a done
// request back to its inbox (to-<its "to">). It returns the request's path now.
func SetHandoffStatus(dir, file, ver, status string) (string, error) {
	if !slices.Contains(Statuses, status) {
		return "", fmt.Errorf("status %q: open, answered or done", status)
	}
	box, name, data, err := requestOf(dir, file, ver)
	if err != nil {
		return "", err
	}
	to := box
	switch {
	case status == "done":
		to = "done"
	case box == "done":
		inbox := frontMatter(data).Inbox
		if !inboxName.MatchString(inbox) {
			return "", fmt.Errorf("%s says no repo it is for (to:): move it back yourself", name)
		}
		to = "to-" + inbox
	}
	text := withStatus(string(data), status)
	if to == box {
		return file, replace(dir, box, name, []byte(text))
	}
	if err := os.MkdirAll(filepath.Join(dir, to), 0o755); err != nil {
		return "", err
	}
	created, err := untrusted.CreateFile(dir, filepath.Join(to, name), []byte(text), 0o644)
	if err != nil {
		return "", err
	}
	if !created {
		return "", fmt.Errorf("%s has a request named %s already", to, name)
	}
	if err := os.Remove(filepath.Join(dir, box, name)); err != nil {
		return "", err
	}
	return filepath.Join(dir, to, name), nil
}

// replace writes a request's new content: a new file next to it, renamed over it.
func replace(dir, box, name string, data []byte) error {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	tmp := filepath.Join(box, "."+name+"."+hex.EncodeToString(b)+".tmp")
	created, err := untrusted.CreateFile(dir, tmp, data, 0o644)
	if err != nil || !created {
		return fmt.Errorf("writing %s: %v", name, err)
	}
	if err := os.Rename(filepath.Join(dir, tmp), filepath.Join(dir, box, name)); err != nil {
		_ = os.Remove(filepath.Join(dir, tmp))
		return err
	}
	return nil
}

// withStatus is a request with its front matter's status set (a front matter added when it has
// none).
func withStatus(text, status string) string {
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return "---\nstatus: " + status + "\n---\n\n" + text
	}
	head, after, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return "---\nstatus: " + status + "\n---\n\n" + text
	}
	lines := strings.Split(head, "\n")
	set := false
	for i, l := range lines {
		if k, _, found := strings.Cut(l, ":"); found && strings.TrimSpace(k) == "status" {
			lines[i] = "status: " + status
			set = true
			break
		}
	}
	if !set {
		lines = append(lines, "status: "+status)
	}
	return "---\n" + strings.Join(lines, "\n") + "\n---\n" + after
}

// DeleteHandoff deletes a request (at the version read).
func DeleteHandoff(dir, file, ver string) error {
	box, name, _, err := requestOf(dir, file, ver)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, box, name))
}

// HandoffPath is a request's path when file is one of dir's (for opening it): an error otherwise.
func HandoffPath(dir, file string) (string, error) {
	h, _, err := ReadHandoff(dir, file)
	if err != nil {
		return "", err
	}
	return h.File, nil
}

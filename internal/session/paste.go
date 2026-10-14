package session

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Pasting an image: the agent CLI reads it from the system clipboard when the user presses Ctrl+V
// (or pastes, and the terminal sends an empty paste because the clipboard holds no text). In a
// sandbox there is no clipboard, so `jailwick claude` sits between the terminal and the session:
// on such a key it copies the clipboard's image (nothing else) into the session's clipboard
// folder, then passes the key on, and the agent hands the image over as xclip would (see
// agent.Clipboard). The clipboard is read only on the user's key press, never on the sandbox's
// request.

// pasteKeys are what terminals send for Ctrl+V: the control character, kitty's keyboard protocol
// (with or without an event type) and xterm's modifyOtherKeys; and an empty bracketed paste.
var pasteKeys = [][]byte{
	{0x16},
	[]byte("\x1b[118;5u"),
	[]byte("\x1b[118;5:"),
	[]byte("\x1b[27;5;118~"),
	[]byte("\x1b[200~\x1b[201~"),
}

// isPasteKey reports whether input from the terminal holds a paste key.
func isPasteKey(b []byte) bool {
	for _, k := range pasteKeys {
		if bytes.Contains(b, k) {
			return true
		}
	}
	return false
}

// clipboardReader writes the clipboard's image, as PNG, to a file; an error when it holds none.
type clipboardReader func(ctx context.Context, file string) error

// hostClipboard reads this system's clipboard; nil where jailwick can't.
func hostClipboard() clipboardReader {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("osascript"); err == nil {
			return macClipboard
		}
	case "linux":
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := exec.LookPath("wl-paste"); err == nil {
				return commandClipboard("wl-paste", "--no-newline", "--type", "image/png")
			}
		}
		if os.Getenv("DISPLAY") != "" {
			if _, err := exec.LookPath("xclip"); err == nil {
				return commandClipboard("xclip", "-selection", "clipboard", "-t", "image/png", "-o")
			}
		}
	}
	return nil
}

// macClipboard has AppleScript write the clipboard as PNG (it converts screenshots and copied
// images); it fails when the clipboard holds no image.
func macClipboard(ctx context.Context, file string) error {
	return exec.CommandContext(ctx, "osascript",
		"-e", "on run argv",
		"-e", "set png to (the clipboard as «class PNGf»)",
		"-e", "set f to open for access POSIX file (item 1 of argv) with write permission",
		"-e", "set eof f to 0",
		"-e", "write png to f",
		"-e", "close access f",
		"-e", "end run",
		file).Run()
}

// commandClipboard runs a command that prints the clipboard's image.
func commandClipboard(name string, args ...string) clipboardReader {
	return func(ctx context.Context, file string) error {
		out, err := os.Create(file)
		if err != nil {
			return err
		}
		defer out.Close()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdout = out
		return cmd.Run()
	}
}

// pasteWait is how long a key press waits for the clipboard; pasteKeep is how long the image stays
// in the session's folder (the agent CLI reads it at once).
const (
	pasteWait = 3 * time.Second
	pasteKeep = 30 * time.Second
)

// paster puts the clipboard's image in a session's clipboard folder.
type paster struct {
	dir  string
	read clipboardReader
	mu   sync.Mutex
	gen  int // counts pastes, so an old one's removal leaves a newer image alone
}

// paste replaces the folder's image with the clipboard's (or removes it when the clipboard holds
// none), and removes it again after pasteKeep.
func (p *paster) paste(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gen++
	file := filepath.Join(p.dir, agentapi.ClipboardFile)
	_ = os.Remove(file)
	tmp := file + ".tmp"
	ctx, cancel := context.WithTimeout(ctx, pasteWait)
	defer cancel()
	err := p.read(ctx, tmp)
	if info, statErr := os.Stat(tmp); err != nil || statErr != nil || info.Size() == 0 {
		_ = os.Remove(tmp)
		return
	}
	_ = os.Chmod(tmp, 0o644) // the sandbox's user reads it
	if os.Rename(tmp, file) != nil {
		return
	}
	gen := p.gen
	time.AfterFunc(pasteKeep, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.gen == gen {
			_ = os.Remove(file)
		}
	})
}

package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// The agent CLI reads a pasted image by running xclip (or wl-paste) on Linux. A sandbox has
// neither a display nor the user's clipboard, so the agent answers as both, from the image the
// host put in the session's clipboard folder when the user pressed Ctrl+V. It answers only the
// questions about an image: no text ever comes from the clipboard, and nothing goes to it.

// clipboardType is the only type served.
const clipboardType = "image/png"

// clipboardMaxAge is how long a pasted image is served after the host wrote it (it removes it
// sooner).
const clipboardMaxAge = time.Minute

// IsClipboardTool reports whether the agent was started as one of the clipboard commands.
func IsClipboardTool(arg0 string) bool {
	name := filepath.Base(arg0)
	return name == "xclip" || name == "wl-paste"
}

// Clipboard answers as xclip or wl-paste (by arg0's name) from the image in dir; it returns the
// exit status.
func Clipboard(arg0 string, args []string, dir string, stdout, stderr io.Writer) int {
	var list, read bool
	var typ string
	if filepath.Base(arg0) == "wl-paste" {
		list, read, typ = wlPasteRequest(args)
	} else {
		var ok bool
		if list, read, typ, ok = xclipRequest(args); !ok {
			fmt.Fprintln(stderr, "jailwick: the sandbox can't change your clipboard")
			return 1
		}
	}
	image, err := pastedImage(dir)
	switch {
	case err != nil:
		return 1 // nothing pasted: the agent CLI says there's no image
	case list:
		fmt.Fprintln(stdout, clipboardType)
		return 0
	case read && typ == clipboardType:
		if _, err := stdout.Write(image); err != nil {
			return 1
		}
		return 0
	}
	return 1 // text, or another type: never served
}

// xclipRequest reads xclip's flags: -o reads (ok is false for a write), -t names the type,
// TARGETS lists the types.
func xclipRequest(args []string) (list, read bool, typ string, ok bool) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "-out":
			read = true
		case "-t", "-target":
			if i+1 < len(args) {
				typ = args[i+1]
				i++
			}
		case "-selection", "-sel":
			i++ // clipboard, primary: there's only one here
		}
	}
	if !read {
		return false, false, "", false
	}
	if typ == "TARGETS" {
		return true, false, "", true
	}
	return false, true, typ, true
}

// wlPasteRequest reads wl-paste's flags: -l lists the types, -t/--type names one; without a
// type it asks for text.
func wlPasteRequest(args []string) (list, read bool, typ string) {
	read = true
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-l" || a == "--list-types":
			return true, false, ""
		case a == "-t" || a == "--type":
			if i+1 < len(args) {
				typ = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--type="):
			typ = strings.TrimPrefix(a, "--type=")
		}
	}
	return false, read, typ
}

// pastedImage is the image in dir, if one was pasted within clipboardMaxAge.
func pastedImage(dir string) ([]byte, error) {
	path := filepath.Join(dir, agentapi.ClipboardFile)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if time.Since(info.ModTime()) > clipboardMaxAge || info.Size() == 0 {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(path)
}

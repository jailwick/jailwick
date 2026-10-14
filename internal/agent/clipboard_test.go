package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// The agent answers the agent CLI's clipboard questions about an image, from the pasted file;
// text and writes are refused.
func TestClipboard(t *testing.T) {
	dir := t.TempDir()
	image := []byte("\x89PNG...")
	run := func(arg0 string, args ...string) (int, string) {
		var out, errs bytes.Buffer
		code := Clipboard(arg0, args, dir, &out, &errs)
		return code, out.String()
	}
	cases := []struct {
		arg0 string
		args []string
		code int
		out  string
	}{
		{"/usr/local/bin/xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}, 0, "image/png\n"},
		{"xclip", []string{"-selection", "clipboard", "-t", "image/png", "-o"}, 0, string(image)},
		{"xclip", []string{"-selection", "clipboard", "-t", "image/bmp", "-o"}, 1, ""},
		{"xclip", []string{"-selection", "clipboard", "-t", "text/plain", "-o"}, 1, ""},
		{"xclip", []string{"-selection", "clipboard"}, 1, ""}, // a write
		{"wl-paste", []string{"-l"}, 0, "image/png\n"},
		{"wl-paste", []string{"--type", "image/png"}, 0, string(image)},
		{"wl-paste", []string{"--type=image/png"}, 0, string(image)},
		{"wl-paste", nil, 1, ""}, // text
	}
	check := func(when string, have bool) {
		for _, c := range cases {
			code, out := run(c.arg0, c.args...)
			wantCode, wantOut := c.code, c.out
			if !have {
				wantCode, wantOut = 1, ""
			}
			if code != wantCode || out != wantOut {
				t.Errorf("%s: %s %v: %d %q, want %d %q", when, c.arg0, c.args, code, out, wantCode, wantOut)
			}
		}
	}
	check("nothing pasted", false)
	file := filepath.Join(dir, agentapi.ClipboardFile)
	if err := os.WriteFile(file, image, 0o644); err != nil {
		t.Fatal(err)
	}
	check("pasted", true)
	old := time.Now().Add(-2 * clipboardMaxAge)
	_ = os.Chtimes(file, old, old)
	check("pasted long ago", false)
}

func TestIsClipboardTool(t *testing.T) {
	for arg0, want := range map[string]bool{"/usr/local/bin/xclip": true, "wl-paste": true, "/opt/jailwick/bin/jailwick-agent": false} {
		if IsClipboardTool(arg0) != want {
			t.Errorf("%s", arg0)
		}
	}
}

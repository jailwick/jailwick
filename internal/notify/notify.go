// Package notify shows desktop notifications: macOS through osascript, Linux through notify-send.
// Where neither is available, notifications are dropped (they are a convenience, never needed).
package notify

import (
	"os/exec"
	"runtime"
)

// Send shows a notification. Title and body are passed as arguments, never as script text, so
// text from a sandbox can't change what runs.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("osascript",
			"-e", "on run argv",
			"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
			"-e", "end run", "--", title, body).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err == nil {
			return exec.Command("notify-send", "--app-name=jailwick", "--", title, body).Run()
		}
	}
	return nil
}

// Confirm asks a yes-or-no question in a native dialog: ok is the button that answers yes. It
// waits for the answer; Cancel (or closing the dialog) is no. Where there is no dialog (not macOS)
// it answers yes: a question nobody can answer must not block.
func Confirm(title, message, ok string) bool {
	if runtime.GOOS != "darwin" {
		return true
	}
	err := exec.Command("osascript",
		"-e", "on run argv",
		"-e", `display dialog (item 2 of argv) with title (item 1 of argv) buttons {"Cancel", item 3 of argv} default button "Cancel" cancel button "Cancel" with icon caution`,
		"-e", "end run", "--", title, message, ok).Run()
	return err == nil // Cancel exits with an error (-128)
}

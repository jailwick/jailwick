package hostutil

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrNoFolderPicker means this system has no folder dialog jailwick can open.
var ErrNoFolderPicker = errors.New("no folder dialog on this system: paste the folder's path")

// PickFolder shows the system's folder dialog, starting in start, and returns the chosen folder
// ("" when the user cancels). macOS: the standard Finder panel (osascript); Linux: zenity or
// kdialog; Windows: the folder browser (PowerShell).
func PickFolder(ctx context.Context, title, start string) (string, error) {
	cmd, err := folderDialog(ctx, title, start)
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return "", nil // cancelled (the exit status every one of these uses for it)
	}
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", nil
	}
	return filepath.Clean(path), nil
}

func folderDialog(ctx context.Context, title, start string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		script := `POSIX path of (choose folder with prompt ` + appleString(title)
		if start != "" {
			script += ` default location (POSIX file ` + appleString(start) + `)`
		}
		return exec.CommandContext(ctx, "osascript", "-e", script+`)`), nil
	case "windows":
		ps := `Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; ` +
			`$d.Description = '` + strings.ReplaceAll(title, "'", "''") + `'; $d.SelectedPath = '` + strings.ReplaceAll(start, "'", "''") + `'; ` +
			`if ($d.ShowDialog() -eq 'OK') { $d.SelectedPath } else { exit 1 }`
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", ps), nil
	}
	if p, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--directory", "--title", title}
		if start != "" {
			args = append(args, "--filename", strings.TrimSuffix(start, "/")+"/")
		}
		return exec.CommandContext(ctx, p, args...), nil
	}
	if p, err := exec.LookPath("kdialog"); err == nil {
		return exec.CommandContext(ctx, p, "--getexistingdirectory", start, "--title", title), nil
	}
	return nil, ErrNoFolderPicker
}

// appleString quotes s for AppleScript.
func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

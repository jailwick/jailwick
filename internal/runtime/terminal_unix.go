//go:build unix

package runtime

import (
	"os/exec"
	"syscall"
)

// controlledBy makes the command's standard input its controlling terminal, in a session of its
// own, so it gets the terminal's resize signals.
func controlledBy(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
}

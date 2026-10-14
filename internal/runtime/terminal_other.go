//go:build !unix

package runtime

import "os/exec"

// controlledBy does nothing where terminals have no controlling process.
func controlledBy(*exec.Cmd) {}

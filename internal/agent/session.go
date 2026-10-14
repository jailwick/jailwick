package agent

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Exec replaces this process with the agent command in dir, after pre-approving the shared
// instruction imports (they live outside the repo, which Claude otherwise asks about).
func Exec(c *agentapi.Config, dir string, env []string, args []string) error {
	if err := importApprover(c).Approve(dir); err != nil {
		fmt.Fprintf(os.Stderr, "jailwick: couldn't pre-approve instructions: %v\n", err)
	}
	bin, err := exec.LookPath(c.Command)
	if err != nil {
		return fmt.Errorf("%s is not installed in this image (set agent.command, or use an image that has it)", c.Command)
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	return syscall.Exec(bin, append([]string{c.Command}, args...), env)
}

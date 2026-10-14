// Package agent is jailwick-agent: the helper inside every sandbox. It forwards local ports,
// starts agent sessions (in worktrees), reports status, and runs the isolation checks.
package agent

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// LoadConfig reads the configuration the host generated for this sandbox.
func LoadConfig() (*agentapi.Config, error) {
	file := agentapi.ConfigFile
	if f := os.Getenv("JAILWICK_AGENT_CONFIG"); f != "" {
		file = f
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w (is this a jailwick sandbox?)", file, err)
	}
	var c agentapi.Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	// The configuration is the project's; a session container has its own host name.
	if h := os.Getenv("SANDBOX_HOST"); h != "" {
		c.HostName = h
	}
	return &c, nil
}

// PrintJSON writes v as indented JSON to stdout.
func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

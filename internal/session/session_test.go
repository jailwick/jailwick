package session

import (
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/sandbox"
)

// Chrome is always explicit: Claude's own default would apply to every project on the login.
func TestAgentArgsChrome(t *testing.T) {
	prep := &sandbox.Prepared{Settings: "/s.json", MCPConfig: "/m.json"}
	if args := AgentArgs(prep, "", false); !slices.Contains(args, "--no-chrome") || slices.Contains(args, "--chrome") {
		t.Errorf("off: %v", args)
	}
	if args := AgentArgs(prep, "last", true); !slices.Contains(args, "--chrome") || slices.Contains(args, "--no-chrome") {
		t.Errorf("on: %v", args)
	}
}

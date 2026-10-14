package sandbox

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jailwick/jailwick/internal/config"
)

func TestToolset(t *testing.T) {
	bare := toolsetOf(config.Project{}, false)
	if bare.notes() != nil || bare.mcp() != nil || len(bare.tools()) != 0 || bare.browser() != "" {
		t.Fatal("an image of the user's own, without stacks or browsers: nothing to say, check or start")
	}

	p := config.Project{Image: config.Image{Stacks: map[string]string{"go": "1.25", "node": ""},
		Playwright: config.Playwright{Browsers: []string{"firefox", "chromium"}}}}
	ts := toolsetOf(p, true)
	notes := string(ts.notes())
	for _, want := range []string{"## Preinstalled tools", "## Toolchains", "- Node.js with npm", "(24).", "- Go (1.25).",
		"## Browser testing and screenshots (Playwright)"} {
		if !strings.Contains(notes, want) {
			t.Errorf("tools.md lacks %q:\n%s", want, notes)
		}
	}
	if strings.Index(notes, "Node.js with npm") > strings.Index(notes, "- Go (") {
		t.Error("stacks are listed in layer order")
	}
	tools := ts.tools()
	for _, want := range []string{"psql", "pnpm", "go", "playwright-mcp"} {
		if !slices.Contains(tools, want) {
			t.Errorf("verify expects %s: %v", want, tools)
		}
	}
	var mcp struct {
		MCPServers map[string]struct{ Args []string } `json:"mcpServers"`
	}
	if err := json.Unmarshal(ts.mcp(), &mcp); err != nil {
		t.Fatal(err)
	}
	if args := mcp.MCPServers["playwright"].Args; !slices.Contains(args, "chromium") {
		t.Errorf("chromium when chosen: %v", args)
	}
	if b := toolsetOf(config.Project{Image: config.Image{Playwright: config.Playwright{Browsers: []string{"webkit"}}}}, false).browser(); b != "webkit" {
		t.Errorf("else the first browser: %q", b)
	}
}

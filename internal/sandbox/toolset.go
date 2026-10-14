package sandbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/stacks"
)

// baseTools are commands jailwick's base image has (internal/assets/images/base).
var baseTools = []string{"git", "node", "npm", "gh", "rg", "jq", "psql", "redis-cli"}

// toolset is what a project's sandbox image has beyond the image contract (sh, git, the agent):
// jailwick's base tools, the stacks and Playwright. verify checks it, Claude reads it.
type toolset struct {
	jailwickBase bool
	stacks       []stacks.Choice
	browsers     []string
}

func toolsetOf(p config.Project, jailwickBase bool) toolset {
	return toolset{jailwickBase: jailwickBase, stacks: stacks.Chosen(p.Image.Stacks), browsers: p.Image.Playwright.Browsers}
}

// tools are the commands verify expects.
func (t toolset) tools() []string {
	var out []string
	if t.jailwickBase {
		out = append(out, baseTools...)
	}
	for _, s := range t.stacks {
		out = append(out, s.Tools...)
	}
	if len(t.browsers) > 0 {
		out = append(out, "playwright", "playwright-mcp")
	}
	return out
}

// browser is the browser the Playwright MCP server and verify use: chromium when chosen, else the
// first one; "" without Playwright.
func (t toolset) browser() string {
	switch {
	case len(t.browsers) == 0:
		return ""
	case slices.Contains(t.browsers, "chromium"):
		return "chromium"
	}
	return t.browsers[0]
}

// notes is tools.md, the part of the sandbox's instructions about its tools; nil when there is
// nothing to say.
func (t toolset) notes() []byte {
	var b bytes.Buffer
	if t.jailwickBase {
		b.Write(assets.MustRead("claude/tools/base.md"))
	}
	if len(t.stacks) > 0 {
		b.WriteString("\n## Toolchains\n")
		for _, s := range t.stacks {
			fmt.Fprintf(&b, "- %s (%s). %s\n", s.Description, s.Version, s.Notes)
		}
	}
	if len(t.browsers) > 0 {
		b.Write(assets.MustRead("claude/tools/playwright.md"))
	}
	if b.Len() == 0 {
		return nil
	}
	return bytes.TrimLeft(b.Bytes(), "\n")
}

// mcp is the MCP servers file for the sandbox's agent: the Playwright server when the project
// has browsers, else nil.
func (t toolset) mcp() []byte {
	if t.browser() == "" {
		return nil
	}
	v := map[string]any{"mcpServers": map[string]any{"playwright": map[string]any{
		"command": "playwright-mcp",
		"args":    []string{"--headless", "--browser", t.browser(), "--isolated", "--output-dir", "/tmp/playwright-mcp"},
	}}}
	data, _ := json.MarshalIndent(v, "", "  ")
	return append(data, '\n')
}

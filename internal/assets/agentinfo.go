package assets

import (
	"encoding/json"
	"os"
	"path"
	"strings"
	"time"
)

// AgentInfoFile is where `make agent` writes the AgentInfo of the agents it builds.
const AgentInfoFile = "agent-info.json"

// AgentInfo says where an agent build comes from, in words a person can use: the jailwick
// release (or build date) and the changes to the agent's code, newest first. It sits next to the
// agent binaries, not in them, so the agent's bytes still only change when its code does.
type AgentInfo struct {
	Release string        `json:"release,omitempty"` // jailwick's version (VERSION); "" when unknown
	Built   string        `json:"built"`             // 2006-01-02
	Changes []AgentChange `json:"changes"`
}

// AgentChange is one change to the agent's code: a commit, worded for people.
type AgentChange struct {
	Commit string `json:"commit,omitempty"` // "" for changes not committed yet
	Date   string `json:"date"`
	Text   string `json:"text"`
}

// Label names the build: "jailwick 0.2.5 (15 Oct 2026)"; builds of one version differ by date.
func (i *AgentInfo) Label() string {
	date := ""
	if t, err := time.Parse(time.DateOnly, i.Built); err == nil {
		date = t.Format("2 Jan 2006")
	}
	switch {
	case i.Release != "" && date != "":
		return "jailwick " + i.Release + " (" + date + ")"
	case i.Release != "":
		return "jailwick " + i.Release
	case date != "":
		return "jailwick build of " + date
	}
	return "a jailwick build"
}

// Since returns the changes of i that other doesn't have: what is new in i's agent.
func (i *AgentInfo) Since(other *AgentInfo) []AgentChange {
	known := map[string]bool{}
	for _, c := range other.Changes {
		known[c.Commit+"\x00"+c.Text] = true
	}
	var out []AgentChange
	for _, c := range i.Changes {
		if !known[c.Commit+"\x00"+c.Text] {
			out = append(out, c)
		}
	}
	return out
}

// AgentInfoOf returns the AgentInfo of this build's agents; nil when the build has none (built
// without git, or without `make`). $JAILWICK_AGENT_DIR overrides it, as it does the agents.
func AgentInfoOf() *AgentInfo {
	var data []byte
	var err error
	if dir := os.Getenv("JAILWICK_AGENT_DIR"); dir != "" {
		data, err = os.ReadFile(path.Join(dir, AgentInfoFile))
	} else {
		data, err = files.ReadFile("agent/" + AgentInfoFile)
	}
	if err != nil {
		return nil
	}
	return ParseAgentInfo(data)
}

// ParseAgentInfo reads an AgentInfo; nil when it isn't one.
func ParseAgentInfo(data []byte) *AgentInfo {
	var i AgentInfo
	if json.Unmarshal(data, &i) != nil || i.Built == "" {
		return nil
	}
	return &i
}

// ChangeText words a commit subject for people: "fix(worktrees): copy databases" becomes
// "Worktrees: copy databases"; a subject without a type is kept, capitalised.
func ChangeText(subject string) string {
	s := strings.TrimSpace(subject)
	if head, rest, ok := strings.Cut(s, ": "); ok && !strings.ContainsAny(head, " ") {
		scope := ""
		if i := strings.Index(head, "("); i >= 0 && strings.HasSuffix(strings.TrimSuffix(head, "!"), ")") {
			scope = strings.TrimSuffix(strings.TrimSuffix(head, "!"), ")")[i+1:]
		}
		s = capitalize(rest)
		if scope != "" {
			s = capitalize(scope) + ": " + rest
		}
	}
	return capitalize(s)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

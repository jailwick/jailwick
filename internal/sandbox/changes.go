package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/assets"
	"github.com/jailwick/jailwick/internal/spec"
)

// Change is one difference between a running container and what it would be created from now:
// what changed (and which item of it), with the values before and after. Secret values never
// appear: secrets compare by digest, and secret-looking environment values are masked.
type Change struct {
	Container string `json:"container"`
	What      string `json:"what"`           // "environment", "mount", "jailwick agent", "image build", ...
	Item      string `json:"item,omitempty"` // an environment variable, a mount's target, a secret's name
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	// Notes say more, in words, under NotesTitle: e.g. what changed in jailwick's agent.
	NotesTitle string   `json:"notes_title,omitempty"`
	Notes      []string `json:"notes,omitempty"`
	Detail     string   `json:"detail,omitempty"` // for debugging, e.g. the agents' content hashes
}

// agentInfoFunc finds where the agent at a host path comes from (nil: unknown).
type agentInfoFunc func(file string) *assets.AgentInfo

// latestShown is how many of the agent's latest changes show when the running one is unknown.
const latestShown = 3

// Kinds of change with a fixed wording in the summary.
const (
	whatContainer  = "container"
	whatDefinition = "definition"
	whatImageBuild = "image build"
	whatAgent      = "jailwick agent"
	whatMount      = "mount"
	whatSecret     = "secret"
	whatSecrets    = "secrets"
)

// fieldWords names spec fields for people.
var fieldWords = map[string]string{
	"Image": "image", "Platform": "platform", "Hostname": "host name", "User": "user", "WorkDir": "working folder",
	"Entrypoint": "entrypoint", "Command": "command", "Init": "init", "OneShot": "one-shot", "Ephemeral": "ephemeral",
	"CapDrop": "capabilities", "CapAdd": "capabilities", "NoNewPrivileges": "privileges", "ReadOnlyRoot": "read-only root",
	"Networks": "networks", "NoNetwork": "networks", "Env": "environment", "Mounts": "mounts", "Tmpfs": "tmpfs",
	"Memory": "memory limit", "CPUs": "CPU limit", "Pids": "process limit", "Shm": "shared memory",
	"Health": "health check", "DependsOn": "services", "SharePIDs": "shared processes",
}

// containerChanges lists how want differs from the container as last applied. digests are the
// applied container's secret digests (nil when not recorded).
func containerChanges(old *spec.Container, digests map[string]string, want spec.Container, agentInfo agentInfoFunc) []Change {
	if old == nil {
		return []Change{{Container: want.Name, What: whatDefinition, After: "changed (the running one wasn't recorded)"}}
	}
	var out []Change
	ov, nv := reflect.ValueOf(*old), reflect.ValueOf(want)
	for i := range ov.NumField() {
		name := ov.Type().Field(i).Name
		if name == "Labels" || name == "Secrets" {
			continue
		}
		a, b := ov.Field(i).Interface(), nv.Field(i).Interface()
		if reflect.DeepEqual(a, b) {
			continue
		}
		switch name {
		case "Env":
			out = append(out, envChanges(want.Name, old.Env, want.Env)...)
		case "Mounts":
			out = append(out, mountChanges(want.Name, old.Mounts, want.Mounts, agentInfo)...)
		default:
			out = append(out, Change{Container: want.Name, What: fieldWords[name], Before: show(a), After: show(b)})
		}
	}
	out = append(out, secretChanges(want.Name, digests, want.Secrets)...)
	if len(out) == 0 && digests == nil { // only the hash knows: secrets, which weren't recorded
		out = append(out, Change{Container: want.Name, What: whatSecrets, After: "changed"})
	}
	return out
}

func envChanges(container string, old, cur map[string]string) []Change {
	var out []Change
	for _, k := range sortedUnion(old, cur) {
		a, inOld := old[k]
		b, inCur := cur[k]
		if inOld && inCur && a == b {
			continue
		}
		c := Change{Container: container, What: "environment", Item: k}
		if inOld {
			c.Before = maskEnv(k, a)
		}
		if inCur {
			c.After = maskEnv(k, b)
		}
		if c.Before == c.After { // both masked
			c.Before, c.After = "(hidden)", "(hidden, changed)"
		}
		out = append(out, c)
	}
	return out
}

func mountChanges(container string, old, cur []spec.Mount, agentInfo agentInfoFunc) []Change {
	before := map[string]spec.Mount{}
	for _, m := range old {
		before[m.Target] = m
	}
	var out []Change
	seen := map[string]bool{}
	for _, m := range cur {
		seen[m.Target] = true
		o, ok := before[m.Target]
		switch {
		case ok && o == m:
		case m.Target == spec.AgentPath:
			out = append(out, agentChange(container, o.Source, m.Source, agentInfo))
		default:
			c := Change{Container: container, What: whatMount, Item: m.Target, After: showMount(m)}
			if ok {
				c.Before = showMount(o)
			}
			out = append(out, c)
		}
	}
	for _, m := range old {
		if !seen[m.Target] {
			out = append(out, Change{Container: container, What: whatMount, Item: m.Target, Before: showMount(m)})
		}
	}
	return out
}

// agentChange says which jailwick builds the running and the new agent come from, and what changed
// in the agent between them; the content hashes only go to Detail.
func agentChange(container, before, after string, agentInfo agentInfoFunc) Change {
	c := Change{Container: container, What: whatAgent, Before: "an earlier jailwick build", After: "a development build",
		Detail: "agent " + agentID(before) + " → " + agentID(after)}
	was, now := agentInfo(before), agentInfo(after)
	if was != nil {
		c.Before = was.Label()
	}
	if now == nil {
		return c
	}
	c.After = now.Label()
	news := now.Changes
	if was != nil {
		c.NotesTitle = "What changed in the agent"
		news = now.Since(was)
	} else {
		c.NotesTitle = "Latest changes in the agent"
		news = news[:min(len(news), latestShown)]
	}
	for _, n := range news {
		c.Notes = append(c.Notes, n.Text)
	}
	return c
}

// secretChanges compares secret digests by name. Without recorded digests it says nothing: the
// caller falls back to "secrets changed" when nothing else explains the difference.
func secretChanges(container string, digests map[string]string, cur map[string]string) []Change {
	if digests == nil {
		return nil
	}
	now := secretDigests(cur)
	var out []Change
	for _, k := range sortedUnion(digests, now) {
		a, inOld := digests[k]
		b, inCur := now[k]
		switch {
		case inOld && inCur && a == b:
			continue
		case !inOld:
			out = append(out, Change{Container: container, What: whatSecret, Item: k, After: "set"})
		case !inCur:
			out = append(out, Change{Container: container, What: whatSecret, Item: k, Before: "set"})
		default:
			out = append(out, Change{Container: container, What: whatSecret, Item: k, Before: "set", After: "changed"})
		}
	}
	return out
}

// summarize turns changes into one line per container, for the CLI and the page's summary.
func summarize(changes []Change) []string {
	var lines []string
	for _, group := range byContainer(changes) {
		name := group[0].Container
		var fields, phrases []string
		mounts := 0
		for _, c := range group {
			switch c.What {
			case whatContainer, whatDefinition:
				phrases = append(phrases, c.After)
			case whatImageBuild:
				phrases = append(phrases, "new image")
			case whatAgent:
				phrases = append(phrases, "jailwick update: the sandbox helper changed")
			case whatSecret, whatSecrets:
				fields = append(fields, "secrets")
			case whatMount:
				if mounts++; mounts <= 4 {
					phrases = append(phrases, mountPhrase(c))
				}
			default:
				fields = append(fields, c.What)
			}
		}
		if mounts > 4 {
			phrases = append(phrases, fmt.Sprintf("%d more mounts", mounts-4))
		}
		slices.Sort(fields)
		if fields = slices.Compact(fields); len(fields) > 0 {
			phrases = append([]string{strings.Join(fields, ", ") + " changed"}, phrases...)
		}
		lines = append(lines, name+": "+strings.Join(phrases, "; "))
	}
	return lines
}

func mountPhrase(c Change) string {
	switch {
	case c.Before == "":
		return "new mount " + c.Item
	case c.After == "":
		return "removed mount " + c.Item
	}
	return "mount " + c.Item + " changed"
}

// byContainer groups changes by container, in order of first appearance.
func byContainer(changes []Change) [][]Change {
	var out [][]Change
	at := map[string]int{}
	for _, c := range changes {
		i, ok := at[c.Container]
		if !ok {
			i = len(out)
			at[c.Container] = i
			out = append(out, nil)
		}
		out[i] = append(out[i], c)
	}
	return out
}

// secretDigests are short digests of secret values: enough to tell a change, nothing to read.
func secretDigests(secrets map[string]string) map[string]string {
	out := make(map[string]string, len(secrets))
	for k, v := range secrets {
		sum := sha256.Sum256([]byte(v))
		out[k] = hex.EncodeToString(sum[:6])
	}
	return out
}

var secretKey = regexp.MustCompile(`(?i)pass|secret|token|key|auth|credential|cookie`)

// maskEnv shows an environment value, hidden when its name says it's secret, and with the
// password of a URL hidden.
func maskEnv(key, v string) string {
	if secretKey.MatchString(key) {
		return "(hidden)"
	}
	if u, err := url.Parse(v); err == nil && u.User != nil {
		if _, ok := u.User.Password(); ok {
			u.User = url.UserPassword(u.User.Username(), "xxxxx")
			return u.String()
		}
	}
	return v
}

// agentID is the agent's content hash, from its file name (jailwick-agent-<arch>-<hash>).
func agentID(source string) string {
	base := filepath.Base(source)
	if i := strings.LastIndex(base, "-"); i >= 0 {
		return base[i+1:]
	}
	return base
}

func showMount(m spec.Mount) string {
	s := m.Source
	if m.Subpath != "" {
		s += ":" + m.Subpath
	}
	if m.ReadOnly {
		s += " (read-only)"
	}
	return s
}

// show renders a field's value: text as is, the rest as JSON.
func show(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	data, _ := json.Marshal(v)
	return string(data)
}

func sortedUnion(a, b map[string]string) []string {
	var keys []string
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

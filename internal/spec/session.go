package spec

import (
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// Each agent session runs in its own small sandbox next to the project's: an agent container and
// its own egress proxy, on a proxy network of their own. The services, the project's networks and
// the volumes (login, caches, node_modules) are the project sandbox's, shared.
const (
	SessionService = "session"
	LabelSession   = "dev.jailwick.session"
	LabelWorktree  = "dev.jailwick.worktree"

	sessionSep = "--" // reserved in project names (config.ValidName)
)

// SessionName is the runtime name of a session's sandbox: jailwick-<project>--<id>.
func SessionName(project, id string) string { return Prefix + project + sessionSep + id }

// ParseSessionName splits a session sandbox's name; ok is false for a project's own sandbox.
func ParseSessionName(name string) (project, id string, ok bool) {
	rest, found := strings.CutPrefix(name, Prefix)
	if !found {
		return "", "", false
	}
	return strings.Cut(rest, sessionSep)
}

// SessionOptions are a session's own choices.
type SessionOptions struct {
	Chrome bool // may use Claude in Chrome
	// Private keeps the session out of messages between sessions: it neither sees the project's
	// other sessions nor is seen by them.
	Private bool
	// Terminal is the host terminal it runs in (hostterm.Terminal.Encode); "" when unknown.
	Terminal string
	// Notify is where the session tells it waits (notify.Channel), kept as a label.
	Notify string
}

// BuildSession returns the sandbox of one session. in is the project's, with HostName set to
// the name the host reaches this session's container by and AllowDir to the session's own
// allowlist.
//
// A session shares the project container's PID namespace and its PeersVolume, so Claude Code's
// sessions find and message each other as they would on one machine (it checks a peer is alive
// by its PID). A private session keeps both to itself, as every session did before.
func BuildSession(in Inputs, id, worktree string, o SessionOptions) *Sandbox {
	p := in.Project
	c := claude(in)
	c.Name = SessionService
	c.Entrypoint = []string{AgentPath, "init", "-session"}
	c.Ephemeral = true
	c.Env[agentapi.SessionEnv] = id
	if o.Private {
		c.Mounts = slices.DeleteFunc(c.Mounts, func(m Mount) bool { return m.Source == PeersVolume })
		// Claude Code keeps a file per running process here, named by PID: in memory, the
		// container's own. Its sockets stay in the container's own /tmp.
		c.Tmpfs = []string{sessionList(p.Image.Home) + ":mode=1777"}
		c.Env[agentapi.PrivateEnv] = "1"
	} else {
		c.SharePIDs = SandboxService
		// The init isn't PID 1 there: as a subreaper it still reaps the session's orphans.
		c.Env["TINI_SUBREAPER"] = "1"
	}
	// The services are already up in the project sandbox; the agent waits for the proxy itself.
	// The session prepares the volumes too: the project sandbox may not have been restarted
	// since its login changed.
	c.DependsOn = []Dependency{
		{Service: VolumeInit, Condition: Completed},
		{Service: ProxyService, Condition: Started},
	}
	c.Mounts = append(c.Mounts, clipboardMounts(in)...)
	c.Labels = map[string]string{LabelSession: id, LabelWorktree: worktree}
	if o.Chrome {
		c.Labels[LabelChrome] = "true"
	}
	if o.Private {
		c.Labels[LabelPrivate] = "true"
	}
	if o.Terminal != "" {
		c.Labels[LabelTerminal] = o.Terminal
	}
	if o.Notify != "" {
		c.Labels[LabelNotify] = o.Notify
	}

	vi := volumeInit(in)
	vi.Labels = map[string]string{LabelSession: id}
	px := proxy(in)
	px.Labels = map[string]string{LabelSession: id}

	// The project's volumes, each once (subpaths mount one volume several times); app volumes
	// are used by their own name.
	apps := appVolumes(p)
	var volumes []string
	for _, ct := range []Container{c, vi} {
		for _, m := range ct.Mounts {
			if m.Type == Volume && !slices.Contains(volumes, m.Source) && !slices.Contains(apps, m.Source) {
				volumes = append(volumes, m.Source)
			}
		}
	}
	s := &Sandbox{
		Name:    SessionName(p.Name, id),
		Project: p.Name,
		Parent:  Prefix + p.Name,
		Networks: []Network{
			{Name: SandboxNetwork, External: true},
			{Name: ProxyNetwork, Internal: true},
			{Name: EgressNetwork, External: true},
		},
		Volumes:    volumes,
		AppVolumes: apps,
		Containers: []Container{c, vi, px},
	}
	s.Seal()
	return s
}

// clipboardMounts let the agent CLI paste an image: the session's clipboard folder, read-only,
// and the agent as the clipboard commands it runs (see agent.Clipboard).
func clipboardMounts(in Inputs) []Mount {
	if in.ClipboardDir == "" {
		return nil
	}
	mounts := []Mount{{Type: Bind, Source: in.ClipboardDir, Target: agentapi.ClipboardDir, ReadOnly: true}}
	for _, tool := range agentapi.ClipboardTools {
		mounts = append(mounts, Mount{Type: Bind, Source: in.AgentBinary, Target: tool, ReadOnly: true})
	}
	return mounts
}

package server

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jailwick/jailwick/internal/notify"
	"github.com/jailwick/jailwick/internal/runtime"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/workspace"
)

// notifyEvery is how often the app looks for things worth a notification while it runs.
const notifyEvery = 5 * time.Second

// Notification is one notification: Route is the app's page about it, for a click to open, with
// what the page is to point out after a "?" ("projects/api/network?hosts=a.com"): the place to act
// on it. "" only brings the app forward, where it was (for a notification with nothing to act on).
type Notification struct{ Title, Body, Route string }

// routeTo is the app's route to a page, pointing out what focus names (see the web app's router).
func routeTo(page string, focus url.Values) string {
	if len(focus) == 0 {
		return page
	}
	return page + "?" + focus.Encode()
}

// snapshot is what notifications are about, at one moment.
type snapshot struct {
	sessions map[string]sessionNow // by project/session
	handoffs map[string]handoffNow // open requests, by file
	blocked  map[string][]string   // project -> hosts refused
}

type handoffNow struct {
	workspace string
	file      string // as the workspace lists it
	text      string // "for <to> from <from>: subject"
}

type sessionNow struct {
	project string
	key     string // "<sid>:<pid>", as the app's Sessions tab names it
	where   string // "api · main checkout"
	busy    bool
	// appTells: the app tells when it waits (session_notifications; its terminal does otherwise).
	appTells bool
}

// Watch sends notifications until ctx ends: a session that stopped working and waits for you,
// a new handoff request, hosts a sandbox was refused, a new version of jailwick. The first look
// only records what is there.
// The app (desktop or browser) runs it; notifications off (config.yaml) sends nothing.
func (s *Server) Watch(ctx context.Context) {
	go s.stream(ctx)       // follow the runtime and the sandboxes rather than asking again and again
	go s.watchUpdates(ctx) // a new version of jailwick
	var prev *snapshot
	tick := time.NewTicker(notifyEvery)
	defer tick.Stop()
	for n := 0; ; n++ {
		g, err := s.env.Global()
		if on := err == nil && g.NotificationsOn(); !on {
			prev = nil // nothing to tell: nothing to look at (the next look only records)
		} else {
			cur := s.look(ctx, prev, lookAt(n))
			if prev != nil {
				for _, note := range changes(*prev, cur, g.BlockedNotificationsOn()) {
					_ = s.send(note)
				}
			}
			prev = &cur
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// stream turns on the runtime's events and the sandboxes' status feeds for the app's Manager
// (sandbox.Manager.Stream), once the runtime answers: the notifier, the pages and the menu bar
// then read what they show from memory.
func (s *Server) stream(ctx context.Context) {
	for {
		if m, err := s.env.Manager(ctx, ""); err == nil {
			m.Stream(ctx, func(runtime.ContainerEvent) { s.changed() })
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(notifyEvery):
		}
	}
}

// What a look reads, by how soon it matters: a session waiting for you at every tick; handoffs
// (files) every handoffEvery ticks; hosts a sandbox was refused (its proxies' logs) every
// blockedEvery ticks.
const (
	handoffEvery = 2 // 10 s
	blockedEvery = 6 // 30 s
)

type lookWhat struct{ handoffs, blocked bool }

// lookAt is what the n-th look reads.
func lookAt(n int) lookWhat {
	return lookWhat{handoffs: n%handoffEvery == 0, blocked: n%blockedEvery == 0}
}

// look takes a snapshot; what can't be read now is left empty, and what this look doesn't read
// (what) is the previous snapshot's.
func (s *Server) look(ctx context.Context, prev *snapshot, what lookWhat) snapshot {
	snap := snapshot{sessions: map[string]sessionNow{}, handoffs: map[string]handoffNow{}, blocked: map[string][]string{}}
	if prev != nil && !what.handoffs {
		snap.handoffs = prev.handoffs
	}
	if prev != nil && !what.blocked {
		snap.blocked = prev.blocked
	}
	what.handoffs = what.handoffs || prev == nil
	what.blocked = what.blocked || prev == nil
	ctx, cancel := context.WithTimeout(ctx, notifyEvery)
	defer cancel()
	mode := "auto"
	if g, err := s.env.Global(); err == nil {
		mode = g.SessionNotify()
	}
	if m, err := s.env.Manager(ctx, ""); err == nil {
		// Only sessions the app tells about are asked (their label says where each tells it
		// waits): with notifications in the terminal, none, and their feeds stop while the
		// window is closed.
		announced := func(b sandbox.Box) bool { return notify.AppNotifies(mode, b.Notify) }
		running, _ := m.RunningContainers(ctx)
		for p, cs := range running {
			ss := m.SessionsWhere(ctx, p, cs, announced, mode == "app")
			for _, x := range ss {
				where := "main checkout"
				if x.Worktree != "" {
					where = x.Worktree
				}
				key := fmt.Sprintf("%s/%s/%d", p, x.SID, x.PID)
				snap.sessions[key] = sessionNow{project: p, key: fmt.Sprintf("%s:%d", x.SID, x.PID), where: p + " · " + where, busy: x.State == "busy",
					appTells: notify.AppNotifies(mode, x.Notify)}
			}
			if !what.blocked {
				continue
			}
			names := map[string]string{} // kept with what each session was refused
			for _, x := range ss {
				if x.SID != "" && x.Name != "" {
					names[x.SID] = x.Name
				}
			}
			for h := range s.deniedOf(ctx, m, p, nil, names) { // hosts it keeps blocked aren't news
				snap.blocked[p] = append(snap.blocked[p], h)
			}
		}
	}
	if !what.handoffs {
		return snap
	}
	for _, n := range s.env.Store.Workspaces() {
		w, err := s.env.Store.Workspace(n)
		if err != nil {
			continue
		}
		if dir := workspace.HandoffDir(w); dir != "" {
			hs, _ := workspace.ListHandoffs(dir)
			for _, h := range hs {
				if !h.Done {
					snap.handoffs[n+"/"+h.File] = handoffNow{workspace: n, file: h.File,
						text: fmt.Sprintf("for %s from %s: %s", h.Inbox, h.From, h.Subject)}
				}
			}
		}
	}
	return snap
}

// changes are the notifications from one snapshot to the next; blocked: tell about refused
// hosts (blocked_notifications). They come from the app alone, whatever session_notifications.
func changes(prev, cur snapshot, blocked bool) []Notification {
	var out []Notification
	for _, k := range sortedKeys(cur.sessions) {
		was, ok := prev.sessions[k]
		if now := cur.sessions[k]; ok && was.busy && !now.busy && now.appTells {
			out = append(out, Notification{"Claude is waiting for you", now.where,
				routeTo("projects/"+now.project+"/sessions", url.Values{"session": {now.key}})})
		}
	}
	for _, f := range sortedKeys(cur.handoffs) {
		if _, ok := prev.handoffs[f]; !ok {
			h := cur.handoffs[f]
			out = append(out, Notification{"New handoff request", h.text,
				routeTo("workspaces/"+h.workspace+"/overview", url.Values{"handoff": {h.file}})})
		}
	}
	for _, p := range sortedKeys(cur.blocked) {
		if !blocked {
			break
		}
		var fresh []string
		for _, h := range cur.blocked[p] {
			if !slices.Contains(prev.blocked[p], h) {
				fresh = append(fresh, h)
			}
		}
		if len(fresh) == 0 {
			continue
		}
		slices.Sort(fresh)
		body := strings.Join(fresh[:min(len(fresh), 3)], ", ")
		if len(fresh) > 3 {
			body += fmt.Sprintf(" and %d more", len(fresh)-3)
		}
		out = append(out, Notification{p + ": blocked by the allowlist", body + ". Allow them on the Network tab.",
			routeTo("projects/"+p+"/network", url.Values{"hosts": {strings.Join(fresh, ",")}})})
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// notifyOutcome tells how a start or restart that took a while ended, for whoever stopped watching.
func (s *Server) notifyOutcome(project, op string, took time.Duration, err error) {
	if took < 15*time.Second {
		return // quick: the page showed it
	}
	if g, gerr := s.env.Global(); gerr != nil || !g.NotificationsOn() {
		return
	}
	if errors.Is(err, sandbox.ErrSessionsRunning) || interrupted(err) {
		return // the page asks about the sessions; a cancelled op is the app quitting
	}
	if err != nil {
		msg, _, _ := strings.Cut(err.Error(), "\n")
		_ = s.send(Notification{project + ": " + op + " failed", msg,
			routeTo("projects/"+project+"/overview", url.Values{"show": {"build"}})}) // its output, when a build failed
		return
	}
	_ = s.send(Notification{project + " is ready", "The sandbox is up: open a session.", "projects/" + project + "/sessions"})
}

// send shows a notification: through Notify when set (the desktop app's own), else notify.Send.
func (s *Server) send(n Notification) error {
	if s.Notify != nil {
		return s.Notify(n)
	}
	return notify.Send(n.Title, n.Body)
}

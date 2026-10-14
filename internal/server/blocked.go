package server

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/sandbox"
)

// BlockedView is what a project's sandbox was refused: the hosts to list (Hosts, most often
// first) and those the project keeps blocked, which aren't listed (Kept).
type BlockedView struct {
	Hosts []proxy.Denied `json:"hosts"`
	// Discarded are hosts cleared from the list, not refused since (see discard).
	Discarded []proxy.Denied `json:"discarded"`
	Kept      []string       `json:"kept"`
	// By is which sessions asked for each host listed, the most recent first: kept after a
	// session ends (see sandbox.SessionDenied).
	By map[string][]DeniedBy `json:"by"`
}

// DeniedBy is one session that asked for a refused host: its sandbox (Session; "" for the
// project's own: shells and runs), worktree, Claude session name, and how often and when.
type DeniedBy struct {
	Session  string    `json:"session,omitempty"`
	Worktree string    `json:"worktree,omitempty"`
	Name     string    `json:"name,omitempty"`
	Count    int       `json:"count"`
	First    time.Time `json:"first"`
	Last     time.Time `json:"last"`
}

// deniedOf reads the hosts a running project's sandboxes were refused, leaving out the ones it
// keeps blocked (the project's blocked list) and, with allow, those its allowlist covers by now
// (the proxy's log still has their refusals). Reading records which session asked
// (sandbox.Manager.Denied); names are the Claude session names by session id, where known.
func (s *Server) deniedOf(ctx context.Context, m *sandbox.Manager, project string, allow []string, names map[string]string) map[string]*proxy.Denied {
	acc, err := m.Denied(ctx, project, names)
	if err != nil {
		return map[string]*proxy.Denied{}
	}
	var kept []string
	if p, err := s.env.Store.Project(project); err == nil {
		kept = p.Blocked
	}
	for h := range acc {
		if slices.Contains(kept, h) || slices.ContainsFunc(allow, func(e string) bool { return proxy.Covers(e, h) }) {
			delete(acc, h)
		}
	}
	return acc
}

// blockedOf is what a project's sandboxes were refused, from its records (sandbox.SessionDenied:
// kept through restarts, stops and ended sessions): the hosts to list, and those discarded since
// (gone), leaving out the ones it keeps blocked and those its allowlist covers by now. Running
// proxies are read, and so recorded, first.
func (s *Server) blockedOf(ctx context.Context, project string) (hosts, gone []proxy.Denied) {
	p, err := s.env.Store.Project(project)
	if err != nil {
		return nil, nil
	}
	g, _ := s.env.Global()
	allow, _ := (&sandbox.Manager{Store: s.env.Store, Global: g}).Allowlist(project) // the configuration alone
	if m, err := s.env.Manager(ctx, project); err == nil {
		_, _ = m.Denied(ctx, project, nil) // nothing to read when it doesn't run
	}
	discarded := sandbox.Discarded(project)
	for h, d := range sandbox.DeniedRecorded(project) {
		switch {
		case slices.Contains(p.Blocked, h), slices.ContainsFunc(allow, func(e string) bool { return proxy.Covers(e, h) }):
		case !discarded[h].IsZero() && !d.Last.After(discarded[h]):
			gone = append(gone, *d)
		default:
			hosts = append(hosts, *d)
		}
	}
	return sortDenied(hosts), sortDenied(gone)
}

func sortDenied(list []proxy.Denied) []proxy.Denied {
	acc := make(map[string]*proxy.Denied, len(list))
	for i := range list {
		acc[list[i].Host] = &list[i]
	}
	return proxy.SortDenied(acc)
}

// blocked answers with what a project's sandboxes were refused (BlockedView), running or not.
func (s *Server) blocked(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.env.Store.Project(name)
	if err != nil {
		apiError(w, err)
		return
	}
	hosts, gone := s.blockedOf(r.Context(), name)
	v := BlockedView{Hosts: hosts, Discarded: gone, Kept: nonNil(p.Blocked), By: map[string][]DeniedBy{}}
	recs, _ := sandbox.DeniedBySession(name)
	for _, h := range v.Hosts {
		for _, rec := range recs {
			if c, ok := rec.Hosts[h.Host]; ok {
				v.By[h.Host] = append(v.By[h.Host], DeniedBy{Session: rec.Session, Worktree: rec.Worktree, Name: rec.Name,
					Count: c.Count, First: c.First, Last: c.Last})
			}
		}
		slices.SortFunc(v.By[h.Host], func(a, b DeniedBy) int { return b.Last.Compare(a.Last) })
	}
	writeJSON(w, v)
}

// discard clears hosts from a project's list until its sandbox asks for them again ({hosts:
// [{host, last}]}: each with the last refusal shown), or brings them back ({hosts, restore:
// true}). Kept in the project's data.
func (s *Server) discard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hosts []struct {
			Host string    `json:"host"`
			Last time.Time `json:"last"`
		} `json:"hosts"`
		Restore bool `json:"restore"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	name := r.PathValue("name")
	if _, err := s.env.Store.Project(name); err != nil {
		apiError(w, err)
		return
	}
	hosts := map[string]time.Time{}
	for _, h := range req.Hosts {
		hosts[h.Host] = h.Last
	}
	if err := sandbox.Discard(name, hosts, req.Restore); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// block keeps hosts blocked for a project ({hosts}), or stops keeping them ({hosts, unblock:
// true}): kept ones are refused as before but no longer listed or notified about.
func (s *Server) block(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hosts   []string `json:"hosts"`
		Unblock bool     `json:"unblock"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	p, err := s.env.Store.Project(r.PathValue("name"))
	if err != nil {
		apiError(w, err)
		return
	}
	for _, h := range req.Hosts {
		switch {
		case req.Unblock:
			p.Blocked = slices.DeleteFunc(p.Blocked, func(k string) bool { return k == h })
		case !slices.Contains(p.Blocked, h):
			p.Blocked = append(p.Blocked, h)
		}
	}
	slices.Sort(p.Blocked)
	if len(p.Blocked) == 0 {
		p.Blocked = nil
	}
	if err := p.Validate(); err != nil {
		apiError(w, err)
		return
	}
	if err := s.env.Store.SaveProject(p); err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, map[string]any{"kept": nonNil(p.Blocked)})
}

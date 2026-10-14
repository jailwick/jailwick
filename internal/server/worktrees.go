package server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/config"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/untrusted"
)

// Worktrees made and removed from the app. Making one can take minutes (the database copies, the
// project's setup commands): it runs in the background, and the project's page shows it as it
// goes (ProjectDetail.WorktreeJobs), with its output. A worktree made fine leaves no job behind;
// one that failed, or whose setup failed, keeps its job (and output) until dismissed or made again.

// WorktreeJob is a worktree being made, or made with a problem.
type WorktreeJob struct {
	Name        string `json:"name"`
	Base        string `json:"base"`
	Running     bool   `json:"running"`
	Error       string `json:"error,omitempty"`        // it wasn't made
	SetupFailed bool   `json:"setup_failed,omitempty"` // made, but a setup command failed
}

type wtJob struct {
	WorktreeJob
	log *untrusted.LimitedBuffer
}

// wtJobs are the jobs by project, then worktree.
type wtJobs struct {
	mu sync.Mutex
	by map[string]map[string]*wtJob
}

func (j *wtJobs) of(project string) []WorktreeJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []WorktreeJob{}
	for _, x := range j.by[project] {
		out = append(out, x.WorktreeJob)
	}
	slices.SortFunc(out, func(a, b WorktreeJob) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// start records a new job; false when that worktree is being made already.
func (j *wtJobs) start(project, name, base string) (*wtJob, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.by == nil {
		j.by = map[string]map[string]*wtJob{}
	}
	if j.by[project] == nil {
		j.by[project] = map[string]*wtJob{}
	}
	if x := j.by[project][name]; x != nil && x.Running {
		return nil, false
	}
	x := &wtJob{WorktreeJob: WorktreeJob{Name: name, Base: base, Running: true}, log: &untrusted.LimitedBuffer{Max: 1 << 20}}
	j.by[project][name] = x
	return x, true
}

// end records how a job ended; a worktree made fine leaves none.
func (j *wtJobs) end(project string, x *wtJob, setupFailed bool, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	x.Running, x.SetupFailed = false, setupFailed
	if err != nil {
		x.Error = untrusted.Text(err.Error())
	}
	if err == nil && !setupFailed {
		delete(j.by[project], x.Name)
	}
}

func (j *wtJobs) log(project, name string) (string, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	x := j.by[project][name]
	if x == nil {
		return "", false
	}
	return untrusted.Text(string(x.log.Bytes())), true
}

func (j *wtJobs) dismiss(project, name string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if x := j.by[project][name]; x != nil && !x.Running {
		delete(j.by[project], name)
	}
}

// lockedWriter lets the job's output be read while it is written.
type lockedWriter struct {
	mu *sync.Mutex
	w  *untrusted.LimitedBuffer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(p) // past the limit, the rest is dropped; the job goes on
	return len(p), nil
}

// A worktree's name is its branch's (as jw wt checks it).
var worktreeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func checkWorktreeName(n string) error {
	if !worktreeName.MatchString(n) || strings.Contains(n, "..") || len(n) > 100 || strings.HasSuffix(n, "/") || strings.HasSuffix(n, ".lock") {
		return badInput(fmt.Sprintf("%q can't be a worktree name: letters, digits, . _ - and /, starting with a letter or digit", n))
	}
	return nil
}

// worktreeMakeTime bounds making a worktree: its setup commands install dependencies.
const worktreeMakeTime = 30 * time.Minute

// addWorktree starts making a worktree: {name, base}; the page follows it (WorktreeJobs).
func (s *Server) addWorktree(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
		Base string `json:"base"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, err)
		return
	}
	req.Name, req.Base = strings.TrimSpace(req.Name), strings.TrimSpace(req.Base)
	if req.Base == "" {
		req.Base = "HEAD"
	}
	if err := checkWorktreeName(req.Name); err != nil {
		apiError(w, err)
		return
	}
	if strings.HasPrefix(req.Base, "-") {
		apiError(w, badInput("the branch or commit to start from can't start with -"))
		return
	}
	user := s.sandboxUser(name)
	x, ok := s.wt.start(name, req.Name, req.Base)
	if !ok {
		apiError(w, errBusy{project: req.Name, op: "being made"})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), worktreeMakeTime)
		defer cancel()
		added, err := m.AddWorktree(ctx, name, user, req.Name, req.Base, lockedWriter{&s.wt.mu, x.log})
		s.wt.end(name, x, added.SetupFailed, err)
	}()
	writeJSON(w, map[string]bool{"ok": true}) // made in the background: the page follows it
}

// worktreeLog answers with the output of a worktree being made (or made with a problem): {text}.
func (s *Server) worktreeLog(w http.ResponseWriter, r *http.Request) {
	text, ok := s.wt.log(r.PathValue("name"), r.URL.Query().Get("wt"))
	if !ok {
		apiError(w, fmt.Errorf("%w: no output for worktree %q", config.ErrNotFound, r.URL.Query().Get("wt")))
		return
	}
	writeJSON(w, map[string]string{"text": text})
}

// dismissWorktreeJob forgets a worktree job that ended with a problem.
func (s *Server) dismissWorktreeJob(w http.ResponseWriter, r *http.Request) {
	s.wt.dismiss(r.PathValue("name"), r.URL.Query().Get("wt"))
	writeJSON(w, map[string]bool{"ok": true})
}

// WorktreeCheckView is what removing a worktree would take away, and the sessions running in it.
type WorktreeCheckView struct {
	agentapi.WorktreeCheck
	Sessions int `json:"sessions"`
}

// checkWorktree answers with what removing a worktree (?wt=) would take away.
func (s *Server) checkWorktree(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	wt := r.URL.Query().Get("wt")
	ch, err := m.CheckWorktree(r.Context(), name, wt)
	if err != nil {
		apiError(w, agentError(r.Context(), name, err))
		return
	}
	writeJSON(w, WorktreeCheckView{WorktreeCheck: ch, Sessions: len(s.sessionsIn(r.Context(), m, name, wt))})
}

// removeWorktree removes a worktree (?wt=): ?force=1 with its uncommitted changes, ?branch=merged
// or any to delete its branch too. Sessions running in it refuse it (409).
func (s *Server) removeWorktree(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	wt, branch := q.Get("wt"), q.Get("branch")
	if branch != "" && branch != "merged" && branch != "any" {
		apiError(w, badInput("branch: merged or any"))
		return
	}
	if n := len(s.sessionsIn(r.Context(), m, name, wt)); n > 0 {
		apiError(w, fmt.Errorf("%w: %d session(s) run in %s; end them first", sandbox.ErrSessionsRunning, n, wt))
		return
	}
	if err := m.RemoveWorktree(r.Context(), name, s.sandboxUser(name), wt, q.Get("force") == "1", branch); err != nil {
		apiError(w, agentError(r.Context(), name, err))
		return
	}
	s.wt.dismiss(name, wt)
	writeJSON(w, map[string]bool{"ok": true})
}

// branches answers with the repo's local branches, the main checkout's first.
func (s *Server) branches(w http.ResponseWriter, r *http.Request) {
	m, name, ok := s.manager(w, r)
	if !ok {
		return
	}
	bs, err := m.Branches(r.Context(), name)
	if err != nil {
		apiError(w, agentError(r.Context(), name, err))
		return
	}
	writeJSON(w, bs)
}

// sessionsIn are the sessions running in a worktree (named as its registry does: its branch; a
// session names its folder, the branch with / as -).
func (s *Server) sessionsIn(ctx context.Context, m *sandbox.Manager, project, wt string) []sandbox.Session {
	ss, _ := m.Sessions(ctx, project)
	folder := strings.ReplaceAll(wt, "/", "-")
	return slices.DeleteFunc(ss, func(x sandbox.Session) bool { return x.Worktree != wt && x.Worktree != folder })
}

// sandboxUser is the user a project's sandbox runs as ("": the image's).
func (s *Server) sandboxUser(project string) string {
	raw, err := s.env.Store.Project(project)
	if err != nil {
		return ""
	}
	g, _ := s.env.Global()
	return raw.Resolved(g).Image.User
}

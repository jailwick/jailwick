package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/agentapi"
	"github.com/jailwick/jailwick/internal/history"
	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/session"
	"github.com/jailwick/jailwick/internal/ui"
)

type historyFlags struct {
	host, all, missing, folders bool
	n                           int
}

func (a *app) historyCmd() *cobra.Command {
	var f historyFlags
	cmd := &cobra.Command{
		Use:   "history [folder]",
		Short: "List saved conversations: the sandbox's, or --host (Claude on this machine)",
		Long: `List saved conversations, newest first, with their id, place and first prompt.

The sandbox and the host keep separate histories: a conversation started in one can't be resumed
in the other. --host lists the host's, for a folder (default: here, with subfolders) or --all.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			convs, err := a.conversations(cmd.Context(), f, first(args))
			if err != nil {
				return err
			}
			if f.folders {
				return a.printFolders(convs)
			}
			if len(convs) == 0 {
				a.ui.Println("No conversations.")
				return nil
			}
			running := a.runningIn(cmd.Context(), f)
			var rows [][]string
			for _, c := range convs {
				where := c.Where
				if c.Gone {
					where += " (gone)"
				}
				if s, ok := running[c.ID]; ok {
					where += " (running" + stateOf(s) + ")"
				}
				rows = append(rows, []string{c.When.Format("01-02 15:04"), c.ID, where, c.Name, c.Prompt})
			}
			a.ui.Table([]string{"LAST ACTIVE", "ID", "WHERE", "NAME", "FIRST PROMPT"}, rows)
			if f.host {
				a.ui.Hint("resume one with: jw resume --host")
			} else {
				a.ui.Hint("resume one with: jw resume (or jw claude <id>)")
			}
			return nil
		},
	}
	a.historyFlags(cmd, &f)
	cmd.Flags().BoolVar(&f.folders, "folders", false, "count conversations per folder (--host)")
	return cmd
}

func (a *app) historyFlags(cmd *cobra.Command, f *historyFlags) {
	cmd.Flags().BoolVar(&f.host, "host", false, "conversations of Claude on this machine, not the sandbox")
	cmd.Flags().BoolVar(&f.all, "all", false, "with --host: every folder")
	cmd.Flags().BoolVar(&f.missing, "missing", false, "with --host: include folders that no longer exist")
	cmd.Flags().IntVarP(&f.n, "lines", "n", 20, "how many")
}

func (a *app) conversations(ctx context.Context, f historyFlags, folder string) ([]agentapi.Conversation, error) {
	if !f.host {
		m, p, _, err := a.needProject(ctx)
		if err != nil {
			return nil, err
		}
		var convs []agentapi.Conversation
		if err := a.agentJSON(ctx, m, p, &convs, "history", "-n", fmt.Sprint(f.n)); err != nil {
			return nil, err
		}
		return convs, nil
	}
	o := history.Options{Limit: f.n, Missing: f.missing}
	if f.folders {
		o.Limit = 0
	}
	if !f.all {
		dir := folder
		if dir == "" {
			dir, _ = os.Getwd()
		}
		dir = paths.Real(dir)
		o.Keep = func(d string) bool { return paths.Within(d, dir) }
		o.Where = func(d string) string {
			if d == dir {
				return "."
			}
			if paths.Within(d, dir) {
				return "./" + strings.TrimPrefix(d, dir+"/")
			}
			return paths.Tilde(d)
		}
	} else {
		o.Where = paths.Tilde
	}
	return history.List(history.Root(""), o), nil
}

func (a *app) printFolders(convs []agentapi.Conversation) error {
	type agg struct {
		n    int
		last string
	}
	var order []string
	m := map[string]*agg{}
	for _, c := range convs {
		if m[c.Where] == nil {
			m[c.Where] = &agg{last: c.When.Format("01-02 15:04")}
			order = append(order, c.Where)
		}
		m[c.Where].n++
	}
	var rows [][]string
	for _, w := range order {
		rows = append(rows, []string{m[w].last, fmt.Sprint(m[w].n), w})
	}
	a.ui.Table([]string{"LAST ACTIVE", "COUNT", "FOLDER"}, rows)
	return nil
}

func (a *app) resumeCmd() *cobra.Command {
	var f historyFlags
	var sf sessionFlags
	cmd := &cobra.Command{
		Use:   "resume [folder]",
		Short: "Pick a saved conversation and resume it where it ran (sandbox, or --host)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.n == 20 {
				f.n = 200
			}
			convs, err := a.conversations(cmd.Context(), f, first(args))
			if err != nil {
				return err
			}
			running := a.runningIn(cmd.Context(), f)
			var opts []ui.Option
			held := 0
			for i, c := range convs {
				if c.Gone {
					continue // its folder is gone: can't resume there
				}
				if _, ok := running[c.ID]; ok {
					held++ // a session holds it
					continue
				}
				what := c.Prompt
				if c.Name != "" {
					what = c.Name + ": " + c.Prompt
				}
				label := fmt.Sprintf("%s  %-22s  %s", c.When.Format("01-02 15:04"), trunc(c.Where, 22), what)
				opts = append(opts, ui.Option{Label: label, Value: fmt.Sprint(i)})
			}
			if held > 0 {
				a.ui.Hint("%d running conversation(s) aren't listed: they're open in a session (jw sessions)", held)
			}
			if len(opts) == 0 {
				return errors.New("no conversations to resume")
			}
			pick, err := a.ui.Select("Resume which conversation? (type to filter)", opts, opts[0].Value)
			if err != nil {
				return err
			}
			var i int
			fmt.Sscan(pick, &i)
			c := convs[i]
			if f.host {
				return resumeOnHost(c)
			}
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			wt := ""
			if w, ok := strings.CutPrefix(c.Where, "wt "); ok {
				wt = w
			}
			return a.startSession(cmd.Context(), m, p, sf, session.Options{Worktree: wt, Base: "HEAD", Resume: c.ID})
		},
	}
	a.historyFlags(cmd, &f)
	sf.add(cmd)
	return cmd
}

// resumeOnHost replaces jailwick with `claude --resume <id>` in the conversation's folder.
func resumeOnHost(c agentapi.Conversation) error {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return errors.New("claude isn't installed on this machine")
	}
	if err := os.Chdir(c.Dir); err != nil {
		return err
	}
	return syscall.Exec(bin, []string{"claude", "--resume", c.ID}, os.Environ())
}

func trunc(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// runningIn lists the sandbox's running conversations (nothing for --host, whose conversations
// are this machine's, or when the sessions can't be listed).
func (a *app) runningIn(ctx context.Context, f historyFlags) map[string]sandbox.Session {
	if f.host {
		return nil
	}
	m, p, _, err := a.needProject(ctx)
	if err != nil {
		return nil
	}
	running, _ := m.RunningConversations(ctx, p)
	return running
}

// stateOf is ", busy" or ", idle" when the session's state is known.
func stateOf(s sandbox.Session) string {
	if s.State == "" {
		return ""
	}
	return ", " + s.State
}

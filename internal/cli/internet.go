package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/proxy"
	"github.com/jailwick/jailwick/internal/sandbox"
	"github.com/jailwick/jailwick/internal/ui"
)

const sessionRefHelp = `SESSION is a session id (jw internet lists them), a worktree name, or main for the main
checkout; leave it out when the project has one session.`

func (a *app) internetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "internet",
		Short: "Open the whole internet to one session for a while, or close it (list: no arguments)",
		Long: `Each session has its own egress proxy. Opening it lets that session reach every host and port
for a while (default 15 minutes) or until closed: e.g. so Claude can research. Your machine,
your network and cloud metadata stay blocked, and the other sessions stay on the allowlist.
It closes by itself at the end of the time, when the session ends, or with jw internet close;
connections opened meanwhile end then too.

Without a subcommand: the sessions of this project and whether their internet is open.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, p, _, err := a.needProject(cmd.Context())
			if err != nil {
				return err
			}
			ss, err := m.Sessions(cmd.Context(), p)
			if err != nil {
				return err
			}
			var rows [][]string
			for _, s := range ss {
				if s.SID != "" {
					rows = append(rows, []string{s.SID, where(s), internetLabel(s.Internet, time.Now()), s.Name})
				}
			}
			if len(rows) == 0 {
				a.ui.Println("No sessions with their own proxy. Start one with: jw claude")
				return nil
			}
			a.ui.Table([]string{"SESSION", "WHERE", "INTERNET", "NAME"}, rows)
			return nil
		},
	}
	cmd.AddCommand(a.internetOpenCmd(), a.internetCloseCmd())
	return cmd
}

func (a *app) internetOpenCmd() *cobra.Command {
	var d time.Duration
	cmd := &cobra.Command{
		Use:   "open [SESSION]",
		Short: "Let one session reach the whole internet (default for 15m; --for 0: until closed)",
		Long: `Let one session reach every host and port for --for (default 15m, at most 24h), or until
jw internet close with --for 0. Opening again sets a new end.

` + sessionRefHelp,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := proxy.CheckDuration(d); err != nil {
				return err
			}
			b, m, err := a.pickSession(cmd.Context(), first(args))
			if err != nil {
				return err
			}
			in, err := m.OpenInternet(cmd.Context(), b, d)
			if err != nil {
				return err
			}
			a.ui.OK("session %s (%s): internet %s", b.ID, whereBox(b), internetLabel(&in, time.Now()))
			a.ui.Hint("close it early with: jw internet close %s", b.ID)
			return nil
		},
	}
	cmd.Flags().DurationVar(&d, "for", 15*time.Minute, "how long, e.g. 30m or 2h; 0: until closed")
	return cmd
}

func (a *app) internetCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close [SESSION]",
		Short: "Put a session back on the allowlist (ends what it opened meanwhile)",
		Long:  "Put a session back on the allowlist now. Its connections opened meanwhile end.\n\n" + sessionRefHelp,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, m, err := a.pickSession(cmd.Context(), first(args))
			if err != nil {
				return err
			}
			if err := m.CloseInternet(cmd.Context(), b); err != nil {
				return err
			}
			a.ui.OK("session %s (%s): back on the allowlist", b.ID, whereBox(b))
			return nil
		},
	}
}

// pickSession finds the session a command is about, asking which one when several match.
func (a *app) pickSession(ctx context.Context, ref string) (sandbox.Box, *sandbox.Manager, error) {
	m, p, _, err := a.needProject(ctx)
	if err != nil {
		return sandbox.Box{}, nil, err
	}
	b, err := m.FindSession(ctx, p, ref)
	if !errors.Is(err, sandbox.ErrAmbiguous) || !a.ui.Interactive || a.plain {
		return b, m, err
	}
	ss, err := m.Sessions(ctx, p)
	if err != nil {
		return sandbox.Box{}, nil, err
	}
	var opts []ui.Option
	for _, s := range ss {
		if s.SID == "" || (ref != "" && ref != s.SID && ref != where(s)) {
			continue
		}
		opts = append(opts, ui.Option{Label: fmt.Sprintf("%s  %-14s %s", s.SID, where(s), s.Name), Value: s.SID})
	}
	id, err := a.ui.Select("Which session?", opts, "")
	if err != nil {
		return sandbox.Box{}, nil, err
	}
	b, err = m.FindSession(ctx, p, id)
	return b, m, err
}

// where names a session's checkout the way SESSION references it: main or the worktree.
func where(s sandbox.Session) string {
	if s.Worktree == "" {
		return "main"
	}
	return s.Worktree
}

func whereBox(b sandbox.Box) string {
	if b.Worktree == "" {
		return "main"
	}
	return b.Worktree
}

// internetLabel describes a session's network: allowlist, open, or open with the time left.
func internetLabel(in *proxy.Internet, now time.Time) string {
	switch {
	case in == nil:
		return ""
	case !in.Open:
		return "allowlist"
	case in.Until.IsZero():
		return "open (until closed)"
	}
	left := in.Until.Sub(now).Round(time.Minute)
	if left < time.Minute {
		return "open (closing)"
	}
	h, m := int(left.Hours()), int(left.Minutes())%60
	switch {
	case h == 0:
		return fmt.Sprintf("open (%dm left)", m)
	case m == 0:
		return fmt.Sprintf("open (%dh left)", h)
	}
	return fmt.Sprintf("open (%dh%dm left)", h, m)
}

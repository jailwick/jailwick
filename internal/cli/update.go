package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/buildinfo"
	"github.com/jailwick/jailwick/internal/ui"
	"github.com/jailwick/jailwick/internal/update"
)

// hintTimeout bounds the check jw version and jw doctor make (at most once a day).
const hintTimeout = 3 * time.Second

func (a *app) updateCmd() *cobra.Command {
	var check, yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for a new version of jailwick, and update it with Homebrew",
		Long: `Ask GitHub for the latest release. When it is newer and jailwick came from Homebrew, update
the CLI and the desktop app (those Homebrew has) with brew upgrade; otherwise say where to
download it. Nothing restarts: reopen the app, and running sandboxes take the new helper at
their next restart (jw restart). jailwick also checks once a day by itself (jw version,
jw doctor, the app); updates: off in config.yaml turns that off.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			st := update.New(buildinfo.Version).Check(ctx, 0)
			cancel()
			if st.Latest == nil {
				return fmt.Errorf("checking for a new version: %s", st.Error)
			}
			if st.Error != "" {
				a.ui.Warn("checking for a new version: %s (the last known is %s)", st.Error, st.Latest.Version)
			}
			if !st.Available {
				a.ui.OK("jailwick %s is the latest version", st.Current)
				return nil
			}
			a.ui.OK("jailwick %s is available (you have %s)", st.Latest.Version, st.Current)
			a.ui.Hint("what's new: %s", st.Latest.URL)
			in := update.Detect()
			if !in.Homebrew() {
				a.ui.Hint("this jailwick didn't come from Homebrew: download the new version from the page above")
				return nil
			}
			if check {
				a.ui.Hint("update with: jw update (runs %s)", in.Command)
				return nil
			}
			if !yes {
				ok, err := a.ui.Confirm("Update with Homebrew now?", true)
				if errors.Is(err, ui.ErrNotInteractive) {
					return fmt.Errorf("not a terminal: update with jw update --yes, or %s", in.Command)
				}
				if err != nil || !ok {
					return err
				}
			}
			if err := in.Upgrade(cmd.Context(), a.ui.Out); err != nil {
				return err
			}
			a.ui.OK("updated to %s", st.Latest.Version)
			a.ui.Hint("reopen the app if it's open; running sandboxes take the new helper at their next restart (jw restart)")
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only check; don't update")
	cmd.Flags().BoolVar(&yes, "yes", false, "update without asking")
	return cmd
}

// updateNotice says when a newer jailwick is out, from the daily check (asking GitHub only when the
// last answer is a day old). It stays quiet with updates: off, outside a terminal, or when the
// check fails: it is a courtesy, never in the way.
func (a *app) updateNotice(ctx context.Context) {
	if !a.ui.Interactive {
		return
	}
	if g, err := a.globalConfig(); err != nil || !g.UpdatesOn() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, hintTimeout)
	defer cancel()
	if st := update.New(buildinfo.Version).Check(ctx, update.Daily); st.Available {
		a.ui.Warn("jailwick %s is available (you have %s): jw update", st.Latest.Version, st.Current)
	}
}

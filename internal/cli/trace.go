package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jailwick/jailwick/internal/paths"
	"github.com/jailwick/jailwick/internal/runtime"
)

func (a *app) traceCmd() *cobra.Command {
	var minutes int
	cmd := &cobra.Command{
		Use:   "trace [on|off|clear]",
		Short: "Log what jailwick asks the container runtime, and sum it up (to find what costs energy)",
		Long: `Without an argument, sum up the trace: each kind of runtime command jailwick ran (docker ps,
exec jailwick-agent status, logs…), how often, how long it took and how much it printed, the
costliest first.

  on     start tracing: jailwick, the app included, logs every runtime command it runs
  off    stop tracing (the log stays)
  clear  empty the log

The log is trace.log in the state folder. JAILWICK_TRACE=1 traces one command, without the switch.`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"on", "off", "clear"},
		RunE: func(cmd *cobra.Command, args []string) error {
			state := paths.StateDir()
			flag, log := runtime.TraceFlag(state), runtime.TraceFile(state)
			switch first(args) {
			case "on":
				if err := os.MkdirAll(state, 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(flag, nil, 0o644); err != nil {
					return err
				}
				a.ui.OK("tracing: every runtime command goes to %s", paths.Tilde(log))
				a.ui.Hint("leave the app running a few minutes, then: jw trace (jw trace off stops it)")
				return nil
			case "off":
				if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
					return err
				}
				a.ui.OK("tracing off (the log stays: jw trace sums it up, jw trace clear empties it)")
				return nil
			case "clear":
				if err := os.Truncate(log, 0); err != nil && !os.IsNotExist(err) {
					return err
				}
				a.ui.OK("trace log emptied")
				return nil
			case "":
			default:
				return fmt.Errorf("jw trace [on|off|clear]")
			}
			return a.traceSummary(log, flag, minutes)
		},
	}
	cmd.Flags().IntVar(&minutes, "minutes", 0, "only the last N minutes (default: the whole log)")
	return cmd
}

func (a *app) traceSummary(log, flag string, minutes int) error {
	on := "off"
	if _, err := os.Stat(flag); err == nil {
		on = "on"
	}
	f, err := os.Open(log)
	if os.IsNotExist(err) {
		a.ui.Println("No trace yet (tracing is " + on + "). Start it with: jw trace on")
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	var since time.Time
	if minutes > 0 {
		since = time.Now().Add(-time.Duration(minutes) * time.Minute)
	}
	sum := runtime.SummarizeTrace(f, since)
	if len(sum.Stats) == 0 {
		a.ui.Println("Nothing traced in that time (tracing is " + on + ").")
		return nil
	}
	span := sum.Last.Sub(sum.First)
	perMin := func(n int) string {
		if span < time.Minute {
			return "–"
		}
		return fmt.Sprintf("%.1f", float64(n)/span.Minutes())
	}
	var rows [][]string
	var total time.Duration
	for _, s := range sum.Stats {
		total += s.Time
		rows = append(rows, []string{s.Kind, fmt.Sprint(s.Count), perMin(s.Count),
			s.Time.Round(time.Millisecond).String(), (s.Time / time.Duration(s.Count)).Round(time.Millisecond).String(),
			s.Max.Round(time.Millisecond).String(), runtime.HumanBytes(s.Bytes)})
	}
	a.ui.Table([]string{"KIND", "COUNT", "PER MIN", "TOTAL", "AVG", "MAX", "OUTPUT"}, rows)
	a.ui.Hint("%s of runtime commands over %s (tracing is %s); the log: %s", total.Round(time.Millisecond),
		span.Round(time.Second), on, paths.Tilde(log))
	return nil
}

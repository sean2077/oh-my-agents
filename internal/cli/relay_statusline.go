package cli

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/sean2077/oh-my-agents/internal/relay"
	"github.com/spf13/cobra"
)

func newRelayStatuslineCmd(rootFlag *string) *cobra.Command {
	var pairSlug, preset string
	var asJSON, watch, noColor bool
	var interval int

	cmd := &cobra.Command{
		Use:   "statusline",
		Short: "Compact 'which pair / whose turn' line (pure-read, binding-scoped)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			// A statusLine must never error into the host's status bar:
			// any setup problem (no git root, unresolved identity,
			// uninitialized ledger) degrades to a minimal line, exit 0.
			// `open=false` so a not-yet-initialized ledger is fine.
			l, err := relayLedger(*rootFlag, false)
			if err != nil {
				if asJSON {
					return printJSON(cmd, &relay.StatuslineState{Schema: relay.StatuslineSchema, Text: "relay: —"})
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "relay: —")
				return nil
			}
			if watch {
				return watchStatusline(cmd, l, pairSlug, preset, noColor, time.Duration(interval)*time.Second)
			}
			st := l.Statusline(pairSlug)
			if asJSON {
				return printJSON(cmd, st)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), relay.RenderPreset(st, preset))
			return nil
		}),
	}
	cmd.Flags().StringVar(&pairSlug, "pair", "", "pair slug (default: resolved binding only)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	cmd.Flags().BoolVar(&watch, "watch", false, "repaint a live line until Ctrl-C")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI styling")
	cmd.Flags().IntVar(&interval, "interval", 2, "watch repaint interval in seconds")
	cmd.Flags().StringVar(&preset, "preset", "", "verbosity: minimal|focused|full (default focused)")
	return cmd
}

// watchStatusline repaints in place on a bounded frame interval until the
// user interrupts. Each frame's state computation is itself deadline-bounded
// inside Ledger.Statusline, so a stall degrades the line rather than wedging.
func watchStatusline(cmd *cobra.Command, l *relay.Ledger, pair, preset string, noColor bool, interval time.Duration) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	out := cmd.OutOrStdout()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	paint := func() {
		st := l.Statusline(pair)
		line := relay.RenderPreset(st, preset)
		if !noColor && st.Turn == "you" {
			line = "\x1b[1m" + line + "\x1b[0m"
		}
		_, _ = fmt.Fprintf(out, "\r\x1b[2K%s", line)
	}
	paint()
	for {
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintln(out)
			return nil
		case <-ticker.C:
			paint()
		}
	}
}

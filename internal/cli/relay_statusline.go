package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/sean2077/oh-my-agents/internal/relay"
	"github.com/spf13/cobra"
)

// claudeSettingsPath resolves ~/.claude/settings.json (the statusLine host).
func claudeSettingsPath() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func newRelayStatuslineCmd(rootFlag *string) *cobra.Command {
	var pairSlug string
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
				return watchStatusline(cmd, l, pairSlug, noColor, time.Duration(interval)*time.Second)
			}
			st := l.Statusline(pairSlug)
			if asJSON {
				return printJSON(cmd, st)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), st.Text)
			return nil
		}),
	}
	cmd.Flags().StringVar(&pairSlug, "pair", "", "pair slug (default: resolved binding only)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	cmd.Flags().BoolVar(&watch, "watch", false, "repaint a live line until Ctrl-C")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI styling")
	cmd.Flags().IntVar(&interval, "interval", 2, "watch repaint interval in seconds")

	cmd.AddCommand(
		newStatuslineInstallCmd(),
		newStatuslineUninstallCmd(),
		newStatuslineDoctorCmd(),
	)
	return cmd
}

// watchStatusline repaints in place on a bounded frame interval until the
// user interrupts. Each frame's state computation is itself deadline-bounded
// inside Ledger.Statusline, so a stall degrades the line rather than wedging.
func watchStatusline(cmd *cobra.Command, l *relay.Ledger, pair string, noColor bool, interval time.Duration) error {
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
		line := st.Text
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

func newStatuslineInstallCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Wire `oma relay statusline` into Claude Code settings.json (claude only)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			path, err := claudeSettingsPath()
			if err != nil {
				return err
			}
			if DryRun() {
				state, derr := relay.DoctorStatusline(path)
				if derr != nil {
					return derr
				}
				if state == relay.StatuslineForeign && !force {
					return relay.ErrStatuslineSlotTaken
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would set statusLine in %s (current: %s)\n", path, state)
				return nil
			}
			if err := relay.InstallStatusline(path, force); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "installed relay statusLine in %s\n", path)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing non-relay statusLine")
	return cmd
}

func newStatuslineUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the relay-owned statusLine (leaves a foreign one intact)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			path, err := claudeSettingsPath()
			if err != nil {
				return err
			}
			if DryRun() {
				state, derr := relay.DoctorStatusline(path)
				if derr != nil {
					return derr
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: statusLine is %s in %s\n", state, path)
				return nil
			}
			state, err := relay.UninstallStatusline(path)
			if err != nil {
				return err
			}
			switch state {
			case relay.StatuslineOwned, relay.StatuslineMismatch:
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed relay statusLine from %s\n", path)
			case relay.StatuslineForeign:
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "left a non-relay statusLine intact in %s\n", path)
			default:
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "no statusLine configured in %s\n", path)
			}
			return nil
		}),
	}
}

func newStatuslineDoctorCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report statusLine wiring state (no mutation)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			path, err := claudeSettingsPath()
			if err != nil {
				return err
			}
			state, err := relay.DoctorStatusline(path)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd, map[string]string{"path": path, "state": string(state)})
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", path, state)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

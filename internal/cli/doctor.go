package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sean2077/oh-my-agents/internal/checks"
	"github.com/sean2077/oh-my-agents/internal/state"
	"github.com/spf13/cobra"
)

// commandPaths walks the command tree and returns every full command path
// ("oma asset install", "oma relay pair", …) for refcheck's registered set.
func commandPaths(root *cobra.Command) checks.CommandSet {
	set := checks.CommandSet{root.CommandPath(): root.Runnable()}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			set[sub.CommandPath()] = sub.Runnable()
			walk(sub)
		}
	}
	walk(root)
	return set
}

func newDoctorCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run installation diagnostics (exit 0 clean, 1 warnings, 4 failures)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			home, err := homeDir()
			if err != nil {
				return Errf(ExitState, "cannot resolve home directory: %v", err)
			}
			// the registered surface comes from a fresh root so the set is
			// complete regardless of which command is executing
			set := commandPaths(newRootCmd())
			res := checks.RunAll(checks.InstallChecks(home, findProjectRoot(), set))

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(map[string]any{"schema": "oma-cli/1", "worst": res.Worst, "findings": res.Findings}); err != nil {
					return err
				}
			} else {
				for _, f := range res.Findings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-4s %-22s %s\n", f.Level, f.Check, f.Message)
				}
			}
			switch res.Worst {
			case checks.LevelFail:
				return Errf(ExitGate, "doctor found failures")
			case checks.LevelWarn:
				return Errf(ExitWarn, "doctor found warnings")
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	cmd.AddCommand(newDoctorBudgetCmd(), newDoctorRelayCmd(), newDoctorStateCmd())
	return cmd
}

// newDoctorStateCmd is the workflow-state maintenance surface. The only
// action today is the v0.7.0 -> current session-scope namespace migration.
func newDoctorStateCmd() *cobra.Command {
	var migrateScope, apply bool
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Workflow state maintenance: --migrate-session-scope upgrades v0.7.0 name-session files",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			if !migrateScope {
				return fmt.Errorf("nothing to do: pass --migrate-session-scope")
			}
			root := findProjectRoot()
			if root == "" {
				return Errf(ExitState, "not inside a git project (workflow state lives in <root>/.oma/state)")
			}
			actions, err := state.MigrateSessionScope(root, apply)
			if err != nil {
				return err
			}
			if len(actions) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no v0.7.0 session-scoped state to migrate")
				return nil
			}
			for _, a := range actions {
				verb := "would migrate"
				if a.Applied {
					verb = "migrated"
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s -> %s\n", verb, a.Kind, a.OldName, a.NewName)
			}
			if !apply {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(dry run; pass --apply to perform; originals are backed up under .oma/state/.pre-migration)")
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&migrateScope, "migrate-session-scope", false, "upgrade v0.7.0 name-session state files to the name--s-session form")
	cmd.Flags().BoolVar(&apply, "apply", false, "perform the migration (default: dry-run preview)")
	return cmd
}

// newDoctorRelayCmd is the relay maintenance surface (docs/reference/command-tree.md
// §5): stale-residue cleanup and archive restore stay out of the public
// relay command group.
func newDoctorRelayCmd() *cobra.Command {
	var cleanStale, migrate, apply bool
	var restore, ledgerRoot string
	cmd := &cobra.Command{
		Use:   "relay",
		Short: "Relay ledger maintenance: --clean-stale residue, --restore archived pairs",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			if !cleanStale && restore == "" && !migrate {
				// Plain error → ExitState: ExitUsage is reserved for cobra
				// parse failures (B1 review finding 1).
				return fmt.Errorf("nothing to do: pass --clean-stale, --restore <slug>, and/or --migrate")
			}
			l, err := relayLedger(ledgerRoot, true)
			if err != nil {
				return err
			}
			if restore != "" {
				if err := l.Restore(restore, DryRun()); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "restored %s\n", restore)
			}
			if cleanStale {
				slugs, err := l.AllPairs()
				if err != nil {
					return err
				}
				for _, slug := range slugs {
					actions, err := l.CleanStale(slug, DryRun())
					if err != nil {
						return err
					}
					for _, a := range actions {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", slug, a)
					}
				}
			}
			if migrate {
				actions, err := l.MigrateParticipantSessions(apply)
				if err != nil {
					return err
				}
				if len(actions) == 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no pairs need participant_sessions migration")
				}
				for _, a := range actions {
					verb := "would migrate"
					if a.Applied {
						verb = "migrated"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", verb, a.Pair, a.Note)
				}
				if len(actions) > 0 && !apply {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(dry run; pass --apply to perform)")
				}
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&cleanStale, "clean-stale", false, "remove safe residue (post-publish leftovers, stale abandoned intents)")
	cmd.Flags().StringVar(&restore, "restore", "", "move an archived pair back to the active root (stays terminal)")
	cmd.Flags().StringVar(&ledgerRoot, "ledger-root", "", "override the ledger root")
	cmd.Flags().BoolVar(&migrate, "migrate", false, "repair v0.7.0 pairs missing participant_sessions (both sides must re-join after)")
	cmd.Flags().BoolVar(&apply, "apply", false, "with --migrate: perform the migration (default: dry-run preview)")
	return cmd
}

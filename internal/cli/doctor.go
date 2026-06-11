package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sean2077/oh-my-agents/internal/checks"
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
					fmt.Fprintf(cmd.OutOrStdout(), "%-4s %-22s %s\n", f.Level, f.Check, f.Message)
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
	return cmd
}

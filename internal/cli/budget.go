package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sean2077/oh-my-agents/internal/budget"
	"github.com/sean2077/oh-my-agents/internal/config"
	"github.com/spf13/cobra"
)

func newDoctorBudgetCmd() *cobra.Command {
	var agent, profile string
	var maxTokens int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "budget",
		Short: "Measure the resident prompt surface of installed assets (gate: exit 4 over budget)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("max-resident-tokens") {
				if err := cfg.ApplyFlags(config.FlagOverrides{MaxResidentTokens: &maxTokens}); err != nil {
					return Errf(ExitState, "%v", err)
				}
			}
			eng, err := newEngine()
			if err != nil {
				return err
			}
			rep, err := budget.Measure(eng, agent, profile, cfg.Budget.MaxResidentTokens)
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(map[string]any{"schema": "oma-cli/1", "budget": rep}); err != nil {
					return err
				}
			} else {
				for _, it := range rep.Items {
					fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-12s %5d\n", it.Asset, it.Field, it.Tokens)
				}
				for _, m := range rep.Missing {
					fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-12s     - (not installed)\n", m, "")
				}
				for _, n := range rep.Notes {
					fmt.Fprintf(cmd.OutOrStdout(), "note: %s\n", n)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "total %d / max %d (%s, agent=%s, profile=%s)\n",
					rep.Total, rep.Max, rep.Algo, rep.Agent, rep.Profile)
			}
			if rep.Total > rep.Max {
				return Errf(ExitGate, "resident surface %d tokens exceeds budget %d", rep.Total, rep.Max)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&agent, "agent", "claude", "agent whose resident surface to measure")
	cmd.Flags().StringVar(&profile, "profile", "core4", "asset profile (core4|all)")
	cmd.Flags().IntVar(&maxTokens, "max-resident-tokens", 0, "budget ceiling (default from config budget.max_resident_tokens)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

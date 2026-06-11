package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/config"
	"github.com/spf13/cobra"
)

// findProjectRoot walks up from cwd to the nearest directory containing
// .git (the project anchor for <root>/.oma/config.toml). Empty when none.
func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func loadConfig() (*config.Config, error) {
	home, err := homeDir()
	if err != nil {
		return nil, Errf(ExitState, "cannot resolve home directory: %v", err)
	}
	cfg, err := config.Load(home, findProjectRoot())
	if err != nil {
		return nil, Errf(ExitState, "%v", err)
	}
	return cfg, nil
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect effective configuration (read-only)",
	}
	cmd.AddCommand(newConfigShowCmd(), newConfigPathCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print effective configuration with per-key sources",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			values := map[string]any{
				"relay.ledger_root":          cfg.Relay.LedgerRoot,
				"relay.stale_after":          cfg.Relay.StaleAfter.String(),
				"relay.wait_timeout":         cfg.Relay.WaitTimeout.String(),
				"budget.max_resident_tokens": cfg.Budget.MaxResidentTokens,
				"interview.threshold":        cfg.Interview.Threshold,
				"asset.default_agents":       strings.Join(cfg.Asset.DefaultAgents, ","),
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"schema":           "oma-cli/1",
					"values":           values,
					"sources":          cfg.Sources,
					"threshold_source": cfg.Interview.ThresholdSource,
				})
			}
			keys := make([]string, 0, len(values))
			for k := range values {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24v (%s)\n", k, values[k], cfg.Sources[k])
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-28s %s\n", "interview.threshold_source", cfg.Interview.ThresholdSource)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newConfigPathCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Print resolved config file locations",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"schema":  "oma-cli/1",
					"user":    cfg.UserPath,
					"project": cfg.ProjectPath,
				})
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "user:    %s\nproject: %s\n", cfg.UserPath, orNone(cfg.ProjectPath))
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func orNone(s string) string {
	if s == "" {
		return "(no project root)"
	}
	return s
}

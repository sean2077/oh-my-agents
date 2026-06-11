package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sean2077/oh-my-agents/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, schema registry and pinned algorithms",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"schema":     "oma-cli/1",
					"version":    version.Version,
					"commit":     version.Commit,
					"schemas":    version.Schemas,
					"algorithms": version.Algorithms,
				})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "oma %s (commit %s)\n", version.Version, version.Commit)
			return err
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/sean2077/oh-my-agents/internal/state"
	"github.com/spf13/cobra"
)

func newStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Get/set generic project-level workflow state",
	}
	cmd.AddCommand(newStateGetCmd(), newStateSetCmd())
	return cmd
}

func newStateGetCmd() *cobra.Command {
	var file string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "get <namespace/field>",
		Short: "Read a state value",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			st := state.New(findProjectRoot())
			value, ok, err := st.Get(args[0], file)
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			if !ok {
				return Errf(ExitState, "key %q not set", args[0])
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema": "oma-cli/1", "key": args[0], "value": value})
			}
			fmt.Fprintln(cmd.OutOrStdout(), value)
			return nil
		}),
	}
	cmd.Flags().StringVar(&file, "file", "", "explicit state file path (overrides .oma/state/<namespace>.json)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newStateSetCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "set <namespace/field> <value>",
		Short: "Write a state value (atomic)",
		Args:  cobra.ExactArgs(2),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			st := state.New(findProjectRoot())
			path, err := st.Set(args[0], args[1], file, DryRun())
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			prefix := ""
			if DryRun() {
				prefix = "[dry-run] "
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%swrite %s\n", prefix, path)
			return nil
		}),
	}
	cmd.Flags().StringVar(&file, "file", "", "explicit state file path (overrides .oma/state/<namespace>.json)")
	return cmd
}

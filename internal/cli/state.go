package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/state"
	"github.com/spf13/cobra"
)

func newStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Get/set generic project-level workflow state",
	}
	cmd.AddCommand(newStateGetCmd(), newStateSetCmd(), newStateListCmd())
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
			key, err := scopeStateKey(args[0])
			if err != nil {
				return err
			}
			st := state.New(findProjectRoot())
			value, ok, err := st.Get(key, file)
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			if !ok {
				return Errf(ExitState, "key %q not set", key)
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema": "oma-cli/1", "key": key, "value": value})
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), value)
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
			key, err := scopeStateKey(args[0])
			if err != nil {
				return err
			}
			st := state.New(findProjectRoot())
			path, err := st.Set(key, args[1], file, DryRun())
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			prefix := ""
			if DryRun() {
				prefix = "[dry-run] "
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%swrite %s\n", prefix, path)
			return nil
		}),
	}
	cmd.Flags().StringVar(&file, "file", "", "explicit state file path (overrides .oma/state/<namespace>.json)")
	return cmd
}

func newStateListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list [namespace-prefix]",
		Short: "List state namespaces in the project .oma",
		Args:  cobra.MaximumNArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			prefix := ""
			if len(args) == 1 {
				prefix = args[0]
			}
			st := state.New(findProjectRoot())
			entries, err := st.List(prefix)
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			entries, err = filterSessionEntries(entries)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema": "oma-cli/1", "states": entries})
			}
			for _, ent := range entries {
				keys := make([]string, 0, len(ent.Data))
				for k := range ent.Data {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				parts := make([]string, 0, len(keys))
				for _, k := range keys {
					parts = append(parts, k+"="+ent.Data[k])
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", ent.Namespace, ent.Updated, strings.Join(parts, " "))
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

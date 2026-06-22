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
	cmd.AddCommand(newStateGetCmd(), newStateSetCmd(), newStatePatchCmd(), newStateListCmd(),
		newStateBindWorktreeCmd(), newStateCheckWorktreeCmd())
	return cmd
}

func newStateBindWorktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bind-worktree <namespace>",
		Short: "Bind a state namespace to the current git worktree (mechanical guard)",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			ns, err := workflowScope().ID(args[0])
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			info, err := currentProjectInfo()
			if err != nil {
				return Errf(ExitState, "not inside a git checkout (state lives in <root>/.oma/state)")
			}
			path, err := state.New(info.ProjectRoot).BindWorktree(ns, "", info.ProjectRoot, info.WorktreeRoot, DryRun())
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			prefix := ""
			if DryRun() {
				prefix = "[dry-run] "
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%sbound %s to worktree %s\n", prefix, path, info.WorktreeRoot)
			return nil
		}),
	}
	return cmd
}

func newStateCheckWorktreeCmd() *cobra.Command {
	var allow bool
	cmd := &cobra.Command{
		Use:   "check-worktree <namespace>",
		Short: "Fail unless a state namespace is bound to the current worktree",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			ns, err := workflowScope().ID(args[0])
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			info, err := currentProjectInfo()
			if err != nil {
				return Errf(ExitState, "not inside a git checkout (state lives in <root>/.oma/state)")
			}
			if allow {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "worktree check skipped (--allow-worktree-change)")
				return nil
			}
			if err := state.New(info.ProjectRoot).CheckWorktree(ns, "", info.WorktreeRoot); err != nil {
				return Errf(ExitState, "%v", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ok: %s is on its bound worktree\n", ns)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&allow, "allow-worktree-change", false, "skip the worktree-binding check")
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
			value, ok, revision, err := st.GetWithRevision(key, file)
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			if !ok {
				return Errf(ExitState, "key %q not set", key)
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema": "oma-cli/1", "key": key, "value": value, "revision": revision})
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
	var expectedRevision int64
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
			var expected *int64
			if expectedRevision >= 0 {
				expected = &expectedRevision
			}
			path, err := st.SetExpected(key, args[1], file, DryRun(), expected)
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
	cmd.Flags().Int64Var(&expectedRevision, "expected-revision", -1, "fail unless the state file is at this revision")
	return cmd
}

func newStatePatchCmd() *cobra.Command {
	var file string
	var expectedRevision int64
	var sets []string
	cmd := &cobra.Command{
		Use:   "patch <namespace> --set <field=value> [--set <field=value>...]",
		Short: "Write several state fields atomically",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			ns, err := workflowScope().ID(args[0])
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			values := map[string]string{}
			for _, item := range sets {
				field, value, ok := strings.Cut(item, "=")
				if !ok {
					return Errf(ExitState, "--set %q must be field=value", item)
				}
				values[field] = value
			}
			st := state.New(findProjectRoot())
			var expected *int64
			if expectedRevision >= 0 {
				expected = &expectedRevision
			}
			path, err := st.PatchExpected(ns, values, file, DryRun(), expected)
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
	cmd.Flags().Int64Var(&expectedRevision, "expected-revision", -1, "fail unless the state file is at this revision")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "field=value to write (repeatable)")
	_ = cmd.MarkFlagRequired("set")
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

package cli

import (
	"fmt"
	"path/filepath"

	"github.com/sean2077/oh-my-agents/internal/ralph"
	"github.com/spf13/cobra"
)

func ralphEngine() (*ralph.Engine, error) {
	root := findProjectRoot()
	if root == "" {
		return nil, fmt.Errorf("not inside a git checkout (ralph state lives in <root>/.oma/state)")
	}
	return ralph.NewEngine(filepath.Join(root, ".oma", "state")), nil
}

func newRalphCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ralph", Short: "Solidified persistent loop: counting, stop judgment, history (workflows.md §2)"}
	cmd.AddCommand(newRalphStartCmd(), newRalphNextCmd(), newRalphCheckCmd(), newRalphAbortCmd(), newRalphStatusCmd())
	return cmd
}

func newRalphStartCmd() *cobra.Command {
	var goal, id string
	var maxRounds, stallWindow int
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Initialize a loop (--goal anchors the stop semantics)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := ralphEngine()
			if err != nil {
				return err
			}
			if DryRun() {
				return nil
			}
			s, err := eng.Start(id, goal, maxRounds, stallWindow)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s phase=%s max_rounds=%d stall_window=%d\n", s.ID, s.Phase, s.MaxRounds, s.StallWindow)
			return nil
		}),
	}
	cmd.Flags().StringVar(&goal, "goal", "", "what done means for this loop")
	cmd.Flags().StringVar(&id, "id", "", "instance id (default: timestamp)")
	cmd.Flags().IntVar(&maxRounds, "max-rounds", 10, "exhaustion bound")
	cmd.Flags().IntVar(&stallWindow, "stall-window", 3, "consecutive same-signature failures before stalled")
	_ = cmd.MarkFlagRequired("goal")
	return cmd
}

func newRalphNextCmd() *cobra.Command {
	var id string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Advance one round; stop verdicts (passed/exhausted/stalled) exit 4",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := ralphEngine()
			if err != nil {
				return err
			}
			if DryRun() {
				return nil
			}
			_, v, err := eng.Next(id)
			if err != nil {
				return err
			}
			if asJSON {
				if err := printJSON(cmd, v); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s round=%d %s\n", verdictWord(v.Continue), v.Round, v.Reason)
			}
			if !v.Continue {
				return Errf(ExitGate, "%s", v.Reason)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newRalphCheckCmd() *cobra.Command {
	var id, note string
	var verifierExit int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Record a verifier result the AGENT ran (oma never executes verifiers)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := ralphEngine()
			if err != nil {
				return err
			}
			if DryRun() {
				return nil
			}
			_, v, err := eng.RecordCheck(id, verifierExit, note)
			if err != nil {
				return err
			}
			if asJSON {
				if err := printJSON(cmd, v); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s round=%d %s\n", verdictWord(v.Continue), v.Round, v.Reason)
			}
			if !v.Continue {
				return Errf(ExitGate, "%s", v.Reason)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	cmd.Flags().IntVar(&verifierExit, "verifier-exit", -1, "exit code of the verifier the agent ran")
	cmd.Flags().StringVar(&note, "note", "", "failure signature (stall detection compares these)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	_ = cmd.MarkFlagRequired("verifier-exit")
	return cmd
}

func newRalphAbortCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "abort",
		Short: "Abort a running loop",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := ralphEngine()
			if err != nil {
				return err
			}
			if DryRun() {
				return nil
			}
			s, err := eng.Abort(id)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s phase=%s\n", s.ID, s.Phase)
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	return cmd
}

func newRalphStatusCmd() *cobra.Command {
	var id string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show loop state (read-only)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := ralphEngine()
			if err != nil {
				return err
			}
			s, err := eng.Resolve(id)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd, s)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s phase=%s round=%d/%d checks=%d goal=%q\n",
				s.ID, s.Phase, s.Round, s.MaxRounds, len(s.Checks), s.Goal)
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func verdictWord(cont bool) string {
	if cont {
		return "continue"
	}
	return "stop"
}

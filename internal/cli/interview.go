package cli

import (
	"fmt"
	"path/filepath"

	"github.com/sean2077/oh-my-agents/internal/config"
	"github.com/sean2077/oh-my-agents/internal/interview"
	"github.com/spf13/cobra"
)

func interviewEngine() (*interview.Engine, error) {
	root := findProjectRoot()
	if root == "" {
		return nil, fmt.Errorf("not inside a git checkout (interview state lives in <root>/.oma/state)")
	}
	return interview.NewEngine(filepath.Join(root, ".oma", "state")), nil
}

func newInterviewCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "interview", Short: "Solidified Socratic clarification: scoring math, threshold gate, state (workflows.md §1)"}
	cmd.AddCommand(
		newInterviewStartCmd(),
		newInterviewScoreCmd(),
		newInterviewGateCmd(),
		newInterviewCrystallizeCmd(),
		newInterviewCompleteCmd(),
		newInterviewAbortCmd(),
		newInterviewStatusCmd(),
	)
	return cmd
}

func newInterviewStartCmd() *cobra.Command {
	var threshold float64
	var depth, typ, id, idea string
	var resume bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Initialize an interview (threshold: --threshold > --depth > config > default 0.20)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			var fo config.FlagOverrides
			if cmd.Flags().Changed("threshold") {
				fo.Threshold = &threshold
			}
			if cmd.Flags().Changed("depth") {
				fo.Depth = &depth
			}
			if err := cfg.ApplyFlags(fo); err != nil {
				return err
			}
			eng, err := interviewEngine()
			if err != nil {
				return err
			}
			s, err := eng.Start(id, typ, cfg.Interview.Threshold, cfg.Interview.ThresholdSource, idea, resume, DryRun())
			if err != nil {
				return err
			}
			// Mandatory first line: threshold + provenance (workflows §1.3).
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "threshold: %.2f (source: %s)\n", s.Threshold, s.ThresholdSource)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "interview: %s phase=%s type=%s\n", s.ID, s.Phase, s.Type)
			if DryRun() && !resume {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would create %s\n", eng.StatePath(s.ID))
			}
			return nil
		}),
	}
	cmd.Flags().Float64Var(&threshold, "threshold", 0, "ambiguity gate threshold in [0,1] (beats --depth)")
	cmd.Flags().StringVar(&depth, "depth", "", "quick|standard|deep (0.30/0.20/0.10)")
	cmd.Flags().StringVar(&typ, "type", "greenfield", "greenfield|brownfield (brownfield adds the context dimension)")
	cmd.Flags().StringVar(&id, "id", "", "instance id (default: timestamp)")
	cmd.Flags().StringVar(&idea, "idea", "", "the initial idea (prompt-safe summary)")
	cmd.Flags().BoolVar(&resume, "resume", false, "show an existing interview instead of refusing")
	return cmd
}

func newInterviewScoreCmd() *cobra.Command {
	var input, id string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "score",
		Short: "Apply one agent-evaluated round (round 0 locks the topology)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := interviewEngine()
			if err != nil {
				return err
			}
			in, err := interview.ParseScoresInput(input)
			if err != nil {
				return err
			}
			st, rep, err := eng.Score(id, in, DryRun())
			if err != nil {
				return err
			}
			if DryRun() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would replace %s\n", eng.StatePath(st.ID))
			}
			if asJSON {
				return printJSON(cmd, rep)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "round %d: ambiguity %.3f (threshold %.2f) phase=%s\n", rep.Round, rep.Ambiguity, rep.Threshold, rep.Phase)
			if rep.Weakest != nil {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "weakest: %s × %s (%.2f)\n", rep.Weakest.Component, rep.Weakest.Dimension, rep.Weakest.Score)
			}
			for _, c := range rep.ChallengeSuggestions {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "challenge available: %s\n", c)
			}
			for _, w := range rep.Warnings {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", w)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&input, "input", "", "scores JSON file (oma-interview-scores/1)")
	cmd.Flags().StringVar(&id, "id", "", "instance id (default: the single active interview)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func newInterviewGateCmd() *cobra.Command {
	var id, reason string
	var waive, asJSON bool
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Judge ambiguity ≤ threshold (exit 0 pass, 4 fail); --waive records an early exit",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := interviewEngine()
			if err != nil {
				return err
			}
			st, res, err := eng.Gate(id, waive, reason, DryRun())
			if err != nil {
				return err
			}
			if DryRun() && res.Mutated {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would replace %s\n", eng.StatePath(st.ID))
			}
			if asJSON {
				if err := printJSON(cmd, res); err != nil {
					return err
				}
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ambiguity %.3f vs threshold %.2f (gap %+.3f) after %d rounds\n", res.Ambiguity, res.Threshold, res.Gap, res.Rounds)
				if res.Weakest != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "weakest: %s × %s\n", res.Weakest.Component, res.Weakest.Dimension)
				}
				for _, w := range res.Warnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", w)
				}
			}
			if !res.Pass && !res.Waived {
				return Errf(ExitGate, "gate failed: ambiguity %.3f > threshold %.2f", res.Ambiguity, res.Threshold)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	cmd.Flags().BoolVar(&waive, "waive", false, "record an early exit (interviewing → gate_waived) with a warning")
	cmd.Flags().StringVar(&reason, "reason", "", "waiver reason (required with --waive)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newInterviewCrystallizeCmd() *cobra.Command {
	var id, spec string
	cmd := &cobra.Command{
		Use:   "crystallize",
		Short: "Record the written spec path (gate_passed|gate_waived → crystallized)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := interviewEngine()
			if err != nil {
				return err
			}
			s, err := eng.Crystallize(id, spec, DryRun())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s phase=%s spec=%s\n", s.ID, s.Phase, s.SpecPath)
			if DryRun() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would replace %s\n", eng.StatePath(s.ID))
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	cmd.Flags().StringVar(&spec, "spec", "", "path of the crystallized spec file")
	_ = cmd.MarkFlagRequired("spec")
	return cmd
}

func newInterviewCompleteCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "complete",
		Short: "Close a crystallized interview",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := interviewEngine()
			if err != nil {
				return err
			}
			s, err := eng.Complete(id, DryRun())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s phase=%s\n", s.ID, s.Phase)
			if DryRun() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would replace %s\n", eng.StatePath(s.ID))
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	return cmd
}

func newInterviewAbortCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "abort",
		Short: "Abort a non-terminal interview",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := interviewEngine()
			if err != nil {
				return err
			}
			s, err := eng.Abort(id, DryRun())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s phase=%s\n", s.ID, s.Phase)
			if DryRun() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would replace %s\n", eng.StatePath(s.ID))
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	return cmd
}

func newInterviewStatusCmd() *cobra.Command {
	var id string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show interview state (read-only)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := interviewEngine()
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s phase=%s type=%s rounds=%d ambiguity=%.3f threshold=%.2f\n",
				s.ID, s.Phase, s.Type, len(s.Rounds), s.CurrentAmbiguity, s.Threshold)
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

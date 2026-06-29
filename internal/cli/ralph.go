package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/projectroot"
	"github.com/sean2077/oh-my-agents/internal/ralph"
	"github.com/spf13/cobra"
)

func ralphEngine() (*ralph.Engine, error) {
	info, err := currentProjectInfo()
	if err != nil {
		return nil, failClosed("not inside a git checkout", "cd into a project git checkout — ralph state lives in <root>/.oma/state")
	}
	suffix, err := workflowScope().Suffix()
	if err != nil {
		return nil, err
	}
	eng := ralph.NewEngine(filepath.Join(info.ProjectRoot, ".oma", "state"))
	eng.SessionSuffix = suffix
	eng.ProjectRoot = info.ProjectRoot
	eng.WorktreeRoot = info.WorktreeRoot
	eng.Branch, eng.BaseCommit = gitBranchAndHead(info.WorktreeRoot)
	return eng, nil
}

// gitBranchAndHead reports the current branch and HEAD of root, used to bind a
// ralph loop to its worktree+branch and detect a same-worktree branch switch.
// Empty strings outside a git checkout (the binding check then skips branch).
func gitBranchAndHead(root string) (branch, head string) {
	if root == "" {
		return "", ""
	}
	if out, err := exec.Command("git", "-C", root, "branch", "--show-current").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); err == nil {
		head = strings.TrimSpace(string(out))
	}
	return branch, head
}

func currentProjectInfo() (projectroot.Info, error) {
	dir, err := os.Getwd()
	if err != nil {
		return projectroot.Info{}, err
	}
	return projectroot.Resolve(dir)
}

func newRalphCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ralph", Short: "Solidified persistent loop: counting, stop judgment, history (docs/reference/workflows.md §2)"}
	cmd.AddCommand(newRalphStartCmd(), newRalphNextCmd(), newRalphCheckCmd(), newRalphAbortCmd(), newRalphStatusCmd(), newRalphRebindCmd())
	return cmd
}

func newRalphStartCmd() *cobra.Command {
	var goal, id, keepPolicy string
	var maxRounds, stallWindow, plateauWindow int
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Initialize a loop (--goal anchors the stop semantics)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := ralphEngine()
			if err != nil {
				return err
			}
			s, err := eng.Start(id, ralph.StartOpts{
				Goal: goal, KeepPolicy: keepPolicy,
				MaxRounds: maxRounds, StallWindow: stallWindow, PlateauWindow: plateauWindow,
			}, DryRun())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s phase=%s keep_policy=%s max_rounds=%d stall_window=%d plateau_window=%d\n",
				s.ID, s.Phase, s.KeepPolicy, s.MaxRounds, s.StallWindow, s.PlateauWindow)
			if adv := fuzzyStartAdvisory(goal); adv != "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), adv)
			}
			if DryRun() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would create %s\n", eng.StatePath(s.ID))
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&goal, "goal", "", "what done means for this loop")
	cmd.Flags().StringVar(&id, "id", "", "instance id (default: current session's default ralph loop)")
	cmd.Flags().StringVar(&keepPolicy, "keep-policy", "pass_only", "pass_only | score_improvement")
	cmd.Flags().IntVar(&maxRounds, "max-rounds", 10, "exhaustion bound")
	cmd.Flags().IntVar(&stallWindow, "stall-window", 3, "consecutive same-signature failures before stalled (pass_only)")
	cmd.Flags().IntVar(&plateauWindow, "plateau-window", 3, "consecutive no-improvement rounds before plateaued (score_improvement)")
	_ = cmd.MarkFlagRequired("goal")
	return cmd
}

func newRalphNextCmd() *cobra.Command {
	var id string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Advance one round; stop verdicts (passed/exhausted/stalled/plateaued) exit 4",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := ralphEngine()
			if err != nil {
				return err
			}
			st, v, err := eng.Next(id, DryRun())
			if err != nil {
				return err
			}
			if DryRun() && v.Mutated {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would replace %s\n", eng.StatePath(st.ID))
			}
			if asJSON {
				if err := printJSON(cmd, v); err != nil {
					return err
				}
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s round=%d %s\n", verdictWord(v.Continue), v.Round, v.Reason)
			}
			if !v.Continue {
				return Errf(ExitGate, "%s", v.Reason)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id (default: current session's default ralph loop)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newRalphCheckCmd() *cobra.Command {
	var id, note string
	var verifierExit int
	var score float64
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
			// A nil score means "not provided"; --score 0 is a real value, so
			// distinguish via Changed (RecordCheck enforces policy/score rules).
			var scorePtr *float64
			if cmd.Flags().Changed("score") {
				scorePtr = &score
			}
			st, v, err := eng.RecordCheck(id, verifierExit, scorePtr, note, DryRun())
			if err != nil {
				return err
			}
			if DryRun() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would replace %s\n", eng.StatePath(st.ID))
			}
			if asJSON {
				if err := printJSON(cmd, v); err != nil {
					return err
				}
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s round=%d %s\n", verdictWord(v.Continue), v.Round, v.Reason)
			}
			if !v.Continue {
				return Errf(ExitGate, "%s", v.Reason)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id (default: current session's default ralph loop)")
	cmd.Flags().IntVar(&verifierExit, "verifier-exit", -1, "exit code of the verifier the agent ran")
	cmd.Flags().Float64Var(&score, "score", 0, "evaluator score (required under keep-policy score_improvement)")
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
	cmd.Flags().StringVar(&id, "id", "", "instance id (default: current session's default ralph loop)")
	return cmd
}

func newRalphStatusCmd() *cobra.Command {
	var id string
	var asJSON bool
	var allowWorktreeChange bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show loop state (read-only)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := ralphEngine()
			if err != nil {
				return err
			}
			eng.AllowWorktreeChange = allowWorktreeChange
			s, err := eng.Resolve(id)
			if err != nil {
				return err
			}
			if asJSON {
				// Embed the persisted state and add the stable terminal
				// boolean as a query-only field (never written to disk).
				return printJSON(cmd, struct {
					*ralph.State
					Terminal bool `json:"terminal"`
				}{s, s.Terminal()})
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s phase=%s round=%d/%d checks=%d goal=%q\n",
				s.ID, s.Phase, s.Round, s.MaxRounds, len(s.Checks), s.Goal)
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id (default: current session's default ralph loop)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	cmd.Flags().BoolVar(&allowWorktreeChange, "allow-worktree-change", false, "allow access when the loop was started from another worktree")
	return cmd
}

func newRalphRebindCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "rebind-worktree",
		Short: "Re-point a loop at the current worktree/branch (explicit, mutating)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := ralphEngine()
			if err != nil {
				return err
			}
			s, err := eng.Rebind(id, DryRun())
			if err != nil {
				return err
			}
			prefix := ""
			if DryRun() {
				prefix = "[dry-run] "
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%srebound %s to worktree %s branch %s\n", prefix, s.ID, s.WorktreeRoot, s.Branch)
			return nil
		}),
	}
	cmd.Flags().StringVar(&id, "id", "", "instance id (default: current session's default ralph loop)")
	return cmd
}

func verdictWord(cont bool) string {
	if cont {
		return "continue"
	}
	return "stop"
}

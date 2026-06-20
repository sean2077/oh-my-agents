// Package cli wires the oma command tree (docs/reference/command-tree.md).
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Exit codes per docs/reference/command-tree.md §1.
const (
	ExitOK    = 0 // success
	ExitWarn  = 1 // completed with warnings
	ExitUsage = 2 // usage error (cobra parse/arg failures only)
	ExitState = 3 // environment/state error, fail-closed refusal
	ExitGate  = 4 // gate failed (interview gate, budget, refcheck)
)

// codedError carries an explicit process exit code through cobra's RunE.
type codedError struct {
	code int
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// Errf builds an error that makes oma exit with the given contract code.
func Errf(code int, format string, a ...any) error {
	return &codedError{code: code, err: fmt.Errorf(format, a...)}
}

// run wraps a RunE handler so any uncoded error it returns maps to
// ExitState. Cobra parse/arg/unknown-command failures never enter RunE,
// so ExitUsage stays exclusive to them (B1 review finding 1).
func run(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := fn(cmd, args)
		if err == nil {
			return nil
		}
		var coded *codedError
		if errors.As(err, &coded) {
			return err
		}
		return &codedError{code: ExitState, err: err}
	}
}

const defaultWorkflowSession = "current"

var dryRun bool
var workflowSession = defaultWorkflowSession

// DryRun reports whether --dry-run was passed (global persistent flag:
// mutating commands must report exact paths and write nothing).
func DryRun() bool { return dryRun }

// WorkflowSession returns the workflow-session scope requested by the global
// --session flag.
func WorkflowSession() string { return workflowSession }

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "oma",
		Short:         "oh-my-agents: lightweight dual-agent CLI+Skill toolkit",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "report exact paths that would change, write nothing")
	root.PersistentFlags().StringVar(&workflowSession, "session", defaultWorkflowSession, "scope workflow state to a session slug, or 'current' for the platform session")
	root.AddCommand(newVersionCmd(), newAssetCmd(), newConfigCmd(), newStateCmd(), newDoctorCmd(), newRelayCmd(), newInterviewCmd(), newRalphCmd(), newSelfUpdateCmd())
	return root
}

// Execute runs the command tree and maps errors to contract exit codes.
func Execute() int { return executeWith(newRootCmd(), os.Stderr) }

func executeWith(root *cobra.Command, errOut io.Writer) int {
	err := root.Execute()
	if err == nil {
		return ExitOK
	}
	_, _ = fmt.Fprintln(errOut, "oma:", err)
	var coded *codedError
	if errors.As(err, &coded) {
		return coded.code
	}
	return ExitUsage
}

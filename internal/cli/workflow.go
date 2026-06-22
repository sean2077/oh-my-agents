package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/session"
	"github.com/spf13/cobra"
)

// workflowRow is one workflow instance discovered under .oma/state.
type workflowRow struct {
	Session  string `json:"session"`
	Workflow string `json:"workflow"`
	ID       string `json:"id"`
	Phase    string `json:"phase,omitempty"`
	Worktree string `json:"worktree,omitempty"`
	Revision int64  `json:"revision"`
}

func newWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "workflow", Short: "Inspect workflow state across sessions (read-only)"}
	cmd.AddCommand(newWorkflowListCmd())
	return cmd
}

func newWorkflowListCmd() *cobra.Command {
	var allSessions, asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflow instances (current session by default; --all-sessions for the project view)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			root := findProjectRoot()
			if root == "" {
				return Errf(ExitState, "not inside a git project (workflow state lives in <root>/.oma/state)")
			}
			rows, err := scanWorkflows(root)
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			if !allSessions {
				suffix, err := workflowScope().Suffix()
				if err != nil {
					return Errf(ExitState, "%v", err)
				}
				kept := make([]workflowRow, 0, len(rows))
				for _, r := range rows {
					if r.Session == suffix {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema": "oma-cli/1", "workflows": rows})
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-22s %-10s %-16s %-13s %-8s %s\n", "SESSION", "WORKFLOW", "ID", "PHASE", "REVISION", "WORKTREE")
			for _, r := range rows {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-22s %-10s %-16s %-13s %-8d %s\n", r.Session, r.Workflow, r.ID, r.Phase, r.Revision, r.Worktree)
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&allSessions, "all-sessions", false, "list every session's workflows (default: current session only)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

// scanWorkflows reads every .oma/state/*.json and projects the common fields
// onto a row. It is schema-tolerant (raw decode) so a partial/foreign file is
// skipped rather than failing the listing.
func scanWorkflows(root string) ([]workflowRow, error) {
	dir := filepath.Join(root, ".oma", "state")
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var rows []workflowRow
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var obj struct {
			Phase        string `json:"phase"`
			Revision     int64  `json:"revision"`
			WorktreeRoot string `json:"worktree_root"`
			Data         struct {
				Phase string `json:"phase"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(path), ".json")
		workflow, scoped := "state", base
		switch {
		case strings.HasPrefix(base, "interview-"):
			workflow, scoped = "interview", strings.TrimPrefix(base, "interview-")
		case strings.HasPrefix(base, "ralph-"):
			workflow, scoped = "ralph", strings.TrimPrefix(base, "ralph-")
		}
		id, sess := splitScopedName(scoped)
		if workflow == "state" {
			// A generic namespace's logical name is the workflow (e.g. autopilot);
			// it has no separate instance id.
			workflow, id = id, "-"
			if workflow == "" {
				workflow = "state"
			}
		}
		phase := obj.Phase
		if phase == "" {
			phase = obj.Data.Phase // autopilot keeps phase under data
		}
		rows = append(rows, workflowRow{Session: sess, Workflow: workflow, ID: id, Phase: phase, Worktree: obj.WorktreeRoot, Revision: obj.Revision})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Session != rows[j].Session {
			return rows[i].Session < rows[j].Session
		}
		if rows[i].Workflow != rows[j].Workflow {
			return rows[i].Workflow < rows[j].Workflow
		}
		return rows[i].ID < rows[j].ID
	})
	return rows, nil
}

// splitScopedName splits "name--s-session" into (name, session). A bare suffix
// (the session's default instance) has no separator and reports id "default".
func splitScopedName(scoped string) (id, sess string) {
	if i := strings.Index(scoped, session.ScopeSeparator); i >= 0 {
		return scoped[:i], scoped[i+len(session.ScopeSeparator):]
	}
	return "default", scoped
}

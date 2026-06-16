package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/asset"
	"github.com/sean2077/oh-my-agents/internal/assetaudit"
	"github.com/spf13/cobra"
)

// homeDir resolves the anchor for all asset paths; OMA_HOME overrides for
// tests and sandboxed smoke runs.
func homeDir() (string, error) {
	if h := os.Getenv("OMA_HOME"); h != "" {
		return h, nil
	}
	return os.UserHomeDir()
}

func newEngine() (*asset.Engine, error) {
	home, err := homeDir()
	if err != nil {
		return nil, Errf(ExitState, "cannot resolve home directory: %v", err)
	}
	return asset.NewEngine(home), nil
}

// sourceTypeDirs mirrors the repo assets/ layout (plan §2).
var sourceTypeDirs = asset.TypeDirs

// requireValidNames guards CLI asset-name arguments before they reach any
// path computation (recheck 020 blocker 1).
func requireValidNames(names []string) error {
	for _, n := range names {
		if !asset.ValidName(n) {
			return Errf(ExitUsage, "invalid asset name %q (want lowercase letters, digits, dashes)", n)
		}
	}
	return nil
}

// resolveSource finds <root>/<typedir>/<name>/manifest.json.
func resolveSource(root, name string) (string, error) {
	for _, td := range sourceTypeDirs {
		dir := filepath.Join(root, td, name)
		if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
			return dir, nil
		}
	}
	return "", Errf(ExitState, "asset %q not found under %s/{skills,agents,hooks,prompts}", name, root)
}

func printOps(cmd *cobra.Command, rep *asset.Report) {
	prefix := ""
	if rep.DryRun {
		prefix = "[dry-run] "
	}
	for _, op := range rep.Ops {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s%-7s %s\n", prefix, op.Kind, op.Path)
	}
	for _, sk := range rep.Skips {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%sskip    %s: %s\n", prefix, sk.Agent, sk.Reason)
	}
	for _, w := range rep.Warnings {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%swarn    %s\n", prefix, w)
	}
}

func newAssetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "asset",
		Short: "Manage content assets (skills, subagents, hooks, prompts)",
	}
	cmd.AddCommand(newAssetInstallCmd(), newAssetListCmd(), newAssetCatalogCmd(), newAssetAuditCmd(), newAssetRemoveCmd(), newAssetRollbackCmd())
	return cmd
}

func newAssetInstallCmd() *cobra.Command {
	var from string
	var force bool
	var agents []string
	cmd := &cobra.Command{
		Use:   "install <name>...",
		Short: "Install assets to the canonical root and record them in the registry",
		Args:  cobra.MinimumNArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			if err := requireValidNames(args); err != nil {
				return err
			}
			if from == "" {
				return Errf(ExitState, "no assets source: pass --from <root> (repo assets/ directory)")
			}
			eng, err := newEngine()
			if err != nil {
				return err
			}
			requested := agents
			if len(requested) == 0 {
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				requested = cfg.Asset.DefaultAgents
			}
			for _, name := range args {
				src, err := resolveSource(from, name)
				if err != nil {
					return err
				}
				rep, err := eng.Install(src, asset.Options{DryRun: DryRun(), Force: force, Source: "dir", Agents: requested})
				if err != nil {
					return Errf(ExitState, "install %s: %v", name, err)
				}
				printOps(cmd, rep)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&from, "from", "", "assets source root (e.g. a repo assets/ dir)")
	cmd.Flags().BoolVar(&force, "force", false, "back up and replace unmanaged destinations")
	cmd.Flags().StringSliceVar(&agents, "agent", nil, "narrow projection agents (final = manifest targets ∩ this; default from config asset.default_agents)")
	return cmd
}

func newAssetListCmd() *cobra.Command {
	var asJSON, installed bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List oma-managed assets (--installed verifies canonical + projections on disk)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			eng, err := newEngine()
			if err != nil {
				return err
			}
			entries, err := eng.List()
			if err != nil {
				return Errf(ExitState, "read registry: %v", err)
			}
			type row struct {
				asset.Entry
				Healthy  *bool    `json:"healthy,omitempty"`
				Problems []string `json:"problems,omitempty"`
			}
			rows := make([]row, 0, len(entries))
			for i := range entries {
				r := row{Entry: entries[i]}
				if installed {
					ok, problems := eng.VerifyProjections(&entries[i])
					if sec := eng.VerifyProjectionSecurity(&entries[i]); len(sec) > 0 {
						ok = false
						problems = append(problems, sec...)
					}
					r.Healthy, r.Problems = &ok, problems
				}
				rows = append(rows, r)
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema": "oma-cli/1", "assets": rows})
			}
			for _, r := range rows {
				status := ""
				if installed {
					status = " healthy"
					if !*r.Healthy {
						status = " BROKEN: " + strings.Join(r.Problems, "; ")
					}
				}
				agents := make([]string, 0, len(r.Projections))
				for _, pr := range r.Projections {
					agents = append(agents, pr.Agent)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-9s -> %-16s %s%s\n", r.Name, r.Type, strings.Join(agents, ","), r.CanonicalPath, status)
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	cmd.Flags().BoolVar(&installed, "installed", false, "verify canonical and projections on disk")
	return cmd
}

func newAssetRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>...",
		Short: "Remove managed assets (canonical placement and registry entry)",
		Args:  cobra.MinimumNArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			if err := requireValidNames(args); err != nil {
				return err
			}
			eng, err := newEngine()
			if err != nil {
				return err
			}
			for _, name := range args {
				rep, err := eng.Remove(name, asset.Options{DryRun: DryRun()})
				if err != nil {
					return Errf(ExitState, "remove %s: %v", name, err)
				}
				printOps(cmd, rep)
			}
			return nil
		}),
	}
	return cmd
}

func newAssetRollbackCmd() *cobra.Command {
	var to string
	cmd := &cobra.Command{
		Use:   "rollback <name>",
		Short: "Restore a recorded backup over the canonical placement",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			if err := requireValidNames(args); err != nil {
				return err
			}
			eng, err := newEngine()
			if err != nil {
				return err
			}
			rep, err := eng.Rollback(args[0], to, asset.Options{DryRun: DryRun()})
			if err != nil {
				return Errf(ExitState, "rollback %s: %v", args[0], err)
			}
			printOps(cmd, rep)
			return nil
		}),
	}
	cmd.Flags().StringVar(&to, "to", "", "backup id (default: most recent)")
	return cmd
}

func newAssetCatalogCmd() *cobra.Command {
	var from string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Generated catalog of source assets (derived from manifests; status lifecycle)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			if from == "" {
				from = "assets"
			}
			entries, err := asset.Catalog(from)
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema": "oma-cli/1", "catalog": entries})
			}
			for _, e := range entries {
				line := fmt.Sprintf("%-24s %-9s %-10s %s", e.Name, e.Type, e.Status, strings.Join(e.Targets, ","))
				if e.Canonical != "" {
					line += " -> " + e.Canonical
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&from, "from", "", "assets source root (default: ./assets)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newAssetAuditCmd() *cobra.Command {
	var from string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Advisory bloat audit of source assets (KEEP/ORPHAN/OVERSIZED/RETIRE; never deletes)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			if from == "" {
				from = "assets"
			}
			entries, err := assetaudit.Audit(from)
			if err != nil {
				return Errf(ExitState, "%v", err)
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"schema": "oma-cli/1", "audit": entries})
			}
			for _, e := range entries {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-8s %-10s %-10s loc=%-4d tok=%-4d refs=%-3d  %s\n",
					e.Name, e.Type, e.Status, e.Label, e.LOC, e.ResidentTokens, e.RefCount, e.Reason)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&from, "from", "", "assets source root (default: ./assets)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

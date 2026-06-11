package cli

import (
	"fmt"

	"github.com/sean2077/oh-my-agents/internal/update"
	"github.com/sean2077/oh-my-agents/internal/version"
	"github.com/spf13/cobra"
)

func newSelfUpdateCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Update oma from the pinned GitHub releases (--check is strictly read-only)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			u, err := update.New(version.Version, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			rel, differs, err := u.Check()
			if err != nil {
				return err
			}
			if !differs {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "oma %s is up to date\n", version.Version)
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "update available: %s -> %s\n", version.Version, rel.TagName)
			if check {
				return nil // strictly read-only: report and stop
			}
			return u.Apply(rel, DryRun())
		}),
	}
	cmd.Flags().BoolVar(&check, "check", false, "only query and compare versions; never write")
	return cmd
}

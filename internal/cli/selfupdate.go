package cli

import (
	"fmt"

	"github.com/sean2077/oh-my-agents/internal/update"
	"github.com/sean2077/oh-my-agents/internal/version"
	"github.com/spf13/cobra"
)

func newSelfUpdateCmd() *cobra.Command {
	var check bool
	var allowDowngrade bool
	var channel string
	var pinVersion string
	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Update oma from the pinned GitHub releases (--check is strictly read-only)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			u, err := update.New(version.Version, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			rel, available, err := u.CheckTarget(channel, pinVersion)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if !available {
				// No newer release. Distinguish "up to date" from a running
				// build that is ahead of the target (a downgrade), so e.g. a
				// v1.0.0-rc.1 binary is never silently rolled back to the
				// latest stable release.
				if order, ok := update.CompareVersions(version.Version, rel.TagName); ok && order > 0 {
					_, _ = fmt.Fprintf(out, "oma %s is newer than release %s\n", version.Version, rel.TagName)
					if !allowDowngrade {
						_, _ = fmt.Fprintln(out, "nothing to do (re-run with --allow-downgrade to force a downgrade)")
						return nil
					}
					// --allow-downgrade: fall through and replace with rel.
				} else {
					_, _ = fmt.Fprintf(out, "oma %s is up to date\n", version.Version)
					return nil
				}
			} else {
				_, _ = fmt.Fprintf(out, "update available: %s -> %s\n", version.Version, rel.TagName)
			}
			if check {
				return nil // strictly read-only: report and stop
			}
			return u.Apply(rel, DryRun(), allowDowngrade)
		}),
	}
	cmd.Flags().BoolVar(&check, "check", false, "only query and compare versions; never write")
	cmd.Flags().BoolVar(&allowDowngrade, "allow-downgrade", false, "permit replacing the running binary with an equal or older release")
	cmd.Flags().StringVar(&channel, "channel", "stable", "release channel: stable or prerelease")
	cmd.Flags().StringVar(&pinVersion, "version", "", "update to an exact release tag (overrides --channel)")
	return cmd
}

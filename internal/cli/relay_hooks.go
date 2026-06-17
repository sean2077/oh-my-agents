package cli

import (
	"encoding/json"
	"io"

	"github.com/spf13/cobra"
)

// newRelayHookDispatchCmd is the hidden, machine-invoked auto-continue
// dispatcher (`oma relay hook <event>`). oma no longer ships an installer
// that writes this into a host config — users wire it into their own
// settings.json by hand (see docs/reference/relay-v2-protocol.md §12). The dispatcher
// itself is unchanged: pure-read, safe-fail, always exit 0.
func newRelayHookDispatchCmd(rootFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:    "hook <event>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// HIDDEN machine-invoked dispatcher: reads the host hook payload
			// on stdin, emits decision JSON on stdout, ALWAYS exits 0 so it
			// can never break the host process.
			payload, _ := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
			l, err := relayLedger(*rootFlag, false)
			if err != nil {
				return nil
			}
			if out := l.Hook(args[0], payload); out != nil {
				enc := json.NewEncoder(cmd.OutOrStdout())
				_ = enc.Encode(out)
			}
			return nil
		},
	}
}

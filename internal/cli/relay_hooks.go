package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/sean2077/oh-my-agents/internal/hookcfg"
	"github.com/sean2077/oh-my-agents/internal/relay"
	"github.com/spf13/cobra"
)

// relayHookAsset is the ownership marker value for relay-injected hook
// entries (distinct from content-asset hooks). The dispatcher command the
// host invokes is `oma relay hook <event>`.
const relayHookAsset = "oma-relay"

var relayHookEvents = []string{relay.HookSessionStart, relay.HookPreToolUse, relay.HookStop}

// newRelayHookDispatchCmd is the HIDDEN, machine-invoked dispatcher. It
// reads the host hook payload on stdin, emits the decision JSON on stdout,
// and ALWAYS exits 0 so it can never break the host process.
func newRelayHookDispatchCmd(rootFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:    "hook <event>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Never propagate an error: a broken dispatcher must stay silent.
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

func newRelayHooksCmd(rootFlag *string) *cobra.Command {
	cmd := &cobra.Command{Use: "hooks", Short: "Install the relay auto-continue hooks (SessionStart/PreToolUse/Stop)"}
	cmd.AddCommand(
		newRelayHooksInstallCmd(rootFlag),
		newRelayHooksUninstallCmd(),
		newRelayHooksDoctorCmd(),
	)
	return cmd
}

// hostHookTargets maps --target to (agent, settings path, wrap key).
type hostHookTarget struct {
	agent   string
	path    string
	wrapKey string
}

func resolveHookTargets(target string) ([]hostHookTarget, error) {
	home, err := homeDir()
	if err != nil {
		return nil, err
	}
	all := map[string]hostHookTarget{
		"claude": {"claude", filepath.Join(home, ".claude", "settings.json"), hookcfg.WrapKeySettings},
		"codex":  {"codex", filepath.Join(home, ".codex", "hooks.json"), hookcfg.WrapKeyNone},
	}
	switch target {
	case "claude":
		return []hostHookTarget{all["claude"]}, nil
	case "codex":
		return []hostHookTarget{all["codex"]}, nil
	case "both", "":
		return []hostHookTarget{all["claude"], all["codex"]}, nil
	default:
		return nil, fmt.Errorf("unknown --target %q (want claude|codex|both)", target)
	}
}

// relayHookEntries builds the per-event injection entries for one agent.
// Claude nests command entries under a `hooks` array; codex entries are
// flat command objects (matching internal/hookcfg's two host shapes).
func relayHookEntries(agent string) map[string][]json.RawMessage {
	events := map[string][]json.RawMessage{}
	for _, ev := range relayHookEvents {
		cmd := "oma relay hook " + ev
		var entry json.RawMessage
		if agent == "claude" {
			entry = json.RawMessage(fmt.Sprintf(`{"hooks": [{"type": "command", "command": %q}]}`, cmd))
		} else {
			entry = json.RawMessage(fmt.Sprintf(`{"type": "command", "command": %q}`, cmd))
		}
		events[ev] = []json.RawMessage{entry}
	}
	return events
}

func newRelayHooksInstallCmd(rootFlag *string) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Inject the relay hooks into Claude Code and/or Codex host config",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			_ = rootFlag
			targets, err := resolveHookTargets(target)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, tgt := range targets {
				events := relayHookEntries(tgt.agent)
				if DryRun() {
					_, _ = fmt.Fprintf(out, "dry-run: would inject %d relay hook events into %s\n", len(events), tgt.path)
					continue
				}
				if err := hookcfg.Inject(tgt.path, tgt.wrapKey, relayHookAsset, events); err != nil {
					return fmt.Errorf("install %s hooks: %w", tgt.agent, err)
				}
				_, _ = fmt.Fprintf(out, "installed relay hooks into %s\n", tgt.path)
				if tgt.agent == "codex" {
					_, _ = fmt.Fprintln(out, "  note: run Codex `/hooks` once to trust the new hook entries.")
				}
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&target, "target", "both", "claude|codex|both")
	return cmd
}

func newRelayHooksUninstallCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove only the relay-owned hook entries (foreign hooks untouched)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			targets, err := resolveHookTargets(target)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, tgt := range targets {
				if DryRun() {
					_, _ = fmt.Fprintf(out, "dry-run: would strip relay hooks from %s\n", tgt.path)
					continue
				}
				if err := hookcfg.Remove(tgt.path, tgt.wrapKey, relayHookAsset); err != nil {
					return fmt.Errorf("uninstall %s hooks: %w", tgt.agent, err)
				}
				_, _ = fmt.Fprintf(out, "removed relay hooks from %s\n", tgt.path)
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&target, "target", "both", "claude|codex|both")
	return cmd
}

func newRelayHooksDoctorCmd() *cobra.Command {
	var target string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report relay hook wiring per agent (no mutation)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			targets, err := resolveHookTargets(target)
			if err != nil {
				return err
			}
			report := map[string]any{}
			for _, tgt := range targets {
				cmds, _ := hookcfg.OwnCommands(tgt.path, tgt.wrapKey, relayHookAsset)
				wired := map[string]bool{}
				for _, ev := range relayHookEvents {
					wired[ev] = hookEventWired(cmds, ev)
				}
				report[tgt.agent] = map[string]any{"path": tgt.path, "events": wired}
			}
			if asJSON {
				return printJSON(cmd, report)
			}
			out := cmd.OutOrStdout()
			for _, tgt := range targets {
				info := report[tgt.agent].(map[string]any)
				_, _ = fmt.Fprintf(out, "%s (%s):\n", tgt.agent, info["path"])
				for _, ev := range relayHookEvents {
					_, _ = fmt.Fprintf(out, "  %-14s %v\n", ev, info["events"].(map[string]bool)[ev])
				}
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&target, "target", "both", "claude|codex|both")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func hookEventWired(cmds []string, event string) bool {
	return slices.Contains(cmds, "oma relay hook "+event)
}

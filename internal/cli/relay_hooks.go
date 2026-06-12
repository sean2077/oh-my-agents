package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/hookcfg"
	"github.com/sean2077/oh-my-agents/internal/relay"
	"github.com/spf13/cobra"
)

// relayHookAsset is the ownership marker value for relay-injected hook
// entries (distinct from content-asset hooks). The dispatcher command the
// host invokes is `oma relay hook <event>`.
const relayHookAsset = "oma-relay"

var relayHookEvents = []string{relay.HookSessionStart, relay.HookPreToolUse, relay.HookStop}

// Host matcher scoping (review 099): without a PreToolUse matcher the
// dispatcher would spawn on EVERY tool call. Values mirror the proven v1
// install; codex additionally edits through apply_patch.
const sessionStartMatcher = "startup|resume|clear"

func preToolUseMatcher(agent string) string {
	if agent == "codex" {
		return "^(apply_patch|Edit|Write)$"
	}
	return "^(Edit|Write|MultiEdit)$"
}

// hookTimeouts: host-side wall per event in seconds (the dispatcher
// self-bounds at 3s once running; this caps a wedged spawn).
func hookTimeout(event string) int {
	if event == relay.HookSessionStart {
		return 10
	}
	return 5
}

// omaExecutable resolves the absolute path of the running oma binary for
// embedding into host commands. Installing from a transient build (e.g.
// `go run`) embeds that transient path — doctor surfaces the drift.
func omaExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve oma executable for host command: %w", err)
	}
	return exe, nil
}

// relayHookCommand is the host command for one event: absolute path
// behind an existence guard, POSIX-single-quoted (hookcfg.GuardedCommand).
func relayHookCommand(exe, event string) string {
	return hookcfg.GuardedCommand(exe, "relay hook "+event)
}

// hookCommandEntry / hookMatcherGroup are the host-native entry shapes.
// BOTH hosts take nested matcher groups under a top-level "hooks" key
// (review 099 real-host evidence; codex is NOT a flat root event map).
type hookCommandEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type hookMatcherGroup struct {
	Matcher string             `json:"matcher,omitempty"`
	Hooks   []hookCommandEntry `json:"hooks"`
}

// relayHookEntries builds the per-event injection entries for one agent.
func relayHookEntries(agent, exe string) (map[string][]json.RawMessage, error) {
	events := map[string][]json.RawMessage{}
	for _, ev := range relayHookEvents {
		group := hookMatcherGroup{
			Hooks: []hookCommandEntry{{
				Type:    "command",
				Command: relayHookCommand(exe, ev),
				Timeout: hookTimeout(ev),
			}},
		}
		switch ev {
		case relay.HookSessionStart:
			group.Matcher = sessionStartMatcher
		case relay.HookPreToolUse:
			group.Matcher = preToolUseMatcher(agent)
		} // Stop: matcher-less by design (fires on every stop)
		raw, err := json.Marshal(group)
		if err != nil {
			return nil, err
		}
		events[ev] = []json.RawMessage{raw}
	}
	return events, nil
}

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

func newRelayHooksCmd(rootFlag *string) *cobra.Command {
	cmd := &cobra.Command{Use: "hooks", Short: "Install the relay auto-continue hooks (SessionStart/PreToolUse/Stop)"}
	cmd.AddCommand(
		newRelayHooksInstallCmd(rootFlag),
		newRelayHooksUninstallCmd(),
		newRelayHooksDoctorCmd(),
	)
	return cmd
}

// hostHookTarget maps --target to (agent, settings path, wrap key).
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
	// Both hosts wrap events under a top-level "hooks" key (review 099).
	all := map[string]hostHookTarget{
		"claude": {"claude", filepath.Join(home, ".claude", "settings.json"), hookcfg.WrapKeySettings},
		"codex":  {"codex", filepath.Join(home, ".codex", "hooks.json"), hookcfg.WrapKeySettings},
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
			exe, err := omaExecutable()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, tgt := range targets {
				events, err := relayHookEntries(tgt.agent, exe)
				if err != nil {
					return err
				}
				if DryRun() {
					_, _ = fmt.Fprintf(out, "dry-run: would inject %d relay hook events into %s (binary %s)\n", len(events), tgt.path, exe)
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
			exe, err := omaExecutable()
			if err != nil {
				return err
			}
			report := map[string]any{}
			drifted := false
			for _, tgt := range targets {
				cmds, _ := hookcfg.OwnCommands(tgt.path, tgt.wrapKey, relayHookAsset)
				wired := map[string]bool{}
				drift := false
				for _, ev := range relayHookEvents {
					wired[ev] = hookEventWired(cmds, ev)
					// Drift = wired but not byte-identical to the canonical
					// command for the CURRENT binary. Warn-grade only
					// (review 099): coexisting versions stay wired.
					if wired[ev] && !hookEventCanonical(cmds, exe, ev) {
						drift = true
					}
				}
				drifted = drifted || drift
				report[tgt.agent] = map[string]any{"path": tgt.path, "events": wired, "drift": drift}
			}
			// Emit the report FIRST (json or text), then map drift to the
			// warn exit so both modes honor the contract (review 101
			// blocker: the JSON branch must not bypass exit 1).
			if asJSON {
				if err := printJSON(cmd, report); err != nil {
					return err
				}
			} else {
				out := cmd.OutOrStdout()
				for _, tgt := range targets {
					info := report[tgt.agent].(map[string]any)
					_, _ = fmt.Fprintf(out, "%s (%s):\n", tgt.agent, info["path"])
					for _, ev := range relayHookEvents {
						_, _ = fmt.Fprintf(out, "  %-14s %v\n", ev, info["events"].(map[string]bool)[ev])
					}
					if info["drift"].(bool) {
						_, _ = fmt.Fprintf(out, "  warning: wired to a different oma binary than %s — rerun `oma relay hooks install` to refresh\n", exe)
					}
				}
			}
			if drifted {
				return Errf(ExitWarn, "hook binary drift detected")
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&target, "target", "both", "claude|codex|both")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

// hookEventWired reports whether any relay-owned command still invokes
// the event's dispatcher (looser than exact-string on purpose: an entry
// pointing at an older binary path is wired, just drifted).
func hookEventWired(cmds []string, event string) bool {
	for _, c := range cmds {
		if strings.Contains(c, "relay hook "+event) {
			return true
		}
	}
	return false
}

// hookEventCanonical reports whether the event's owned command matches
// the canonical command for the current executable exactly.
func hookEventCanonical(cmds []string, exe, event string) bool {
	return slices.Contains(cmds, relayHookCommand(exe, event))
}

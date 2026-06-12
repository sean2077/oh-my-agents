package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sean2077/oh-my-agents/internal/hookcfg"
	"github.com/sean2077/oh-my-agents/internal/relay"
)

const testExe = "/opt/o m a's bin/oma"

// realisticCodexHooks mirrors the real ~/.codex/hooks.json shape from
// review 099: a top-level "hooks" wrapper with nested matcher-group
// entries, plus a sibling "state" trust map that MUST survive install
// byte-for-byte.
const realisticCodexHooks = `{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {"type": "command", "command": "existing-foreign-hook", "timeout": 5}
        ]
      }
    ]
  },
  "state": {
    "trusted": {"hooks.json:Stop:0": "sha256:abc123"}
  }
}
`

func writeCodexFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(path, []byte(realisticCodexHooks), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func installRelayHooks(t *testing.T, path, agent, exe string) {
	t.Helper()
	events, err := relayHookEntries(agent, exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := hookcfg.Inject(path, hookcfg.WrapKeySettings, relayHookAsset, events); err != nil {
		t.Fatal(err)
	}
}

// decodeHooks returns the event map from a host file's top-level "hooks"
// key (both hosts use this wrapper — review 099).
func decodeHooks(t *testing.T, path string) map[string][]struct {
	Matcher string `json:"matcher"`
	Asset   string `json:"_oma_asset"`
	Hooks   []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	} `json:"hooks"`
} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Asset   string `json:"_oma_asset"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Hooks
}

// Review 099 must-fix 1: codex install goes under the top-level "hooks"
// wrapper as nested matcher groups, includes apply_patch in PreToolUse,
// and preserves the sibling "state" trust map and foreign entries.
func TestRelayHooksInstallCodexRealShape(t *testing.T) {
	path := writeCodexFixture(t)
	installRelayHooks(t, path, "codex", testExe)

	raw, _ := os.ReadFile(path)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["state"]; !ok {
		t.Fatal("install dropped the sibling state trust map")
	}
	if strings.Contains(string(raw), `"SessionStart"`) && !strings.Contains(string(raw), `"hooks"`) {
		t.Fatal("events must live under the hooks wrapper, not the document root")
	}
	// No event arrays may appear at the document ROOT (the old WrapKeyNone
	// belief): the only top-level keys are hooks and state.
	for key := range doc {
		if key != "hooks" && key != "state" {
			t.Fatalf("unexpected top-level key %q (root must stay hooks+state)", key)
		}
	}

	events := decodeHooks(t, path)
	// Foreign Stop entry preserved alongside ours.
	if len(events["Stop"]) != 2 {
		t.Fatalf("Stop entries = %d, want foreign + relay", len(events["Stop"]))
	}
	for _, ev := range relayHookEvents {
		var found bool
		for _, group := range events[ev] {
			if group.Asset != relayHookAsset {
				continue
			}
			found = true
			if len(group.Hooks) != 1 || group.Hooks[0].Type != "command" {
				t.Fatalf("%s: relay group must hold one command entry: %+v", ev, group)
			}
			cmd := group.Hooks[0].Command
			if !strings.Contains(cmd, "relay hook "+ev) {
				t.Fatalf("%s command = %q", ev, cmd)
			}
			// Guard + hostile-path quoting (review 099 must-fix 2).
			if !strings.Contains(cmd, "|| exit 0") || !strings.Contains(cmd, `'/opt/o m a'\''s bin/oma'`) {
				t.Fatalf("%s command not guarded/quoted: %q", ev, cmd)
			}
			if group.Hooks[0].Timeout == 0 {
				t.Fatalf("%s missing timeout", ev)
			}
		}
		if !found {
			t.Fatalf("event %s not injected", ev)
		}
	}
	// Codex PreToolUse must scope to apply_patch as well.
	for _, group := range events[relay.HookPreToolUse] {
		if group.Asset == relayHookAsset && group.Matcher != "^(apply_patch|Edit|Write)$" {
			t.Fatalf("codex PreToolUse matcher = %q", group.Matcher)
		}
	}
}

// Matchers scope the dispatcher per host (review 099): claude keeps the
// proven v1 edit matcher; SessionStart scopes to startup|resume|clear;
// Stop stays matcher-less by design.
func TestRelayHooksEntriesMatchers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	installRelayHooks(t, path, "claude", testExe)
	events := decodeHooks(t, path)

	want := map[string]string{
		relay.HookSessionStart: "startup|resume|clear",
		relay.HookPreToolUse:   "^(Edit|Write|MultiEdit)$",
		relay.HookStop:         "",
	}
	timeouts := map[string]int{
		relay.HookSessionStart: 10,
		relay.HookPreToolUse:   5,
		relay.HookStop:         5,
	}
	for ev, matcher := range want {
		groups := events[ev]
		if len(groups) != 1 {
			t.Fatalf("%s groups = %d", ev, len(groups))
		}
		if groups[0].Matcher != matcher {
			t.Fatalf("%s matcher = %q, want %q", ev, groups[0].Matcher, matcher)
		}
		if groups[0].Hooks[0].Timeout != timeouts[ev] {
			t.Fatalf("%s timeout = %d, want %d", ev, groups[0].Hooks[0].Timeout, timeouts[ev])
		}
	}
}

// Drift is warn-grade (review 099): an entry pointing at a different
// existing binary stays WIRED; only the canonical check reports drift.
func TestRelayHooksWiredAndDrift(t *testing.T) {
	path := writeCodexFixture(t)
	installRelayHooks(t, path, "codex", testExe)

	cmds, err := hookcfg.OwnCommands(path, hookcfg.WrapKeySettings, relayHookAsset)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range relayHookEvents {
		if !hookEventWired(cmds, ev) {
			t.Fatalf("%s not wired", ev)
		}
		if !hookEventCanonical(cmds, testExe, ev) {
			t.Fatalf("%s should be canonical for the installing binary", ev)
		}
		if hookEventCanonical(cmds, "/elsewhere/oma", ev) {
			t.Fatalf("%s cannot be canonical for a different binary", ev)
		}
	}
	// The foreign entry never counts as wired evidence.
	if hookEventWired([]string{"existing-foreign-hook"}, relay.HookStop) {
		t.Fatal("foreign command must not count as wired")
	}
}

// Uninstall removes only relay-owned entries; foreign hooks and the
// state map survive.
func TestRelayHooksUninstallKeepsForeign(t *testing.T) {
	path := writeCodexFixture(t)
	installRelayHooks(t, path, "codex", testExe)
	if err := hookcfg.Remove(path, hookcfg.WrapKeySettings, relayHookAsset); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "existing-foreign-hook") {
		t.Fatal("uninstall dropped the foreign hook")
	}
	if !strings.Contains(string(raw), `"sha256:abc123"`) {
		t.Fatal("uninstall dropped the state trust map")
	}
	if strings.Contains(string(raw), relayHookAsset) {
		t.Fatal("relay entries not fully removed")
	}
}

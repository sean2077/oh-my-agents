package cli

import (
	"bytes"
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

// runDoctor executes one doctor command tree under a fake OMA_HOME and
// returns (exit code, combined output).
func runDoctor(t *testing.T, home string, args ...string) (int, string) {
	t.Helper()
	t.Setenv("OMA_HOME", home)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	return executeWith(root, &out), out.String()
}

// Review 101 blocker: binary drift/mismatch must exit warn(1) in BOTH
// text and JSON modes, with the report still emitted first.
func TestRelayHooksDoctorDriftExitCodes(t *testing.T) {
	home := t.TempDir()
	for _, agent := range []struct{ name, rel string }{
		{"claude", filepath.Join(".claude", "settings.json")},
		{"codex", filepath.Join(".codex", "hooks.json")},
	} {
		path := filepath.Join(home, agent.rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		// Entries embedded for a DIFFERENT binary → wired but drifted.
		installRelayHooks(t, path, agent.name, "/elsewhere/oma")
	}
	for _, mode := range [][]string{
		{"relay", "hooks", "doctor"},
		{"relay", "hooks", "doctor", "--json"},
	} {
		code, out := runDoctor(t, home, mode...)
		if code != ExitWarn {
			t.Fatalf("%v: exit %d, want %d (warn)\n%s", mode, code, ExitWarn, out)
		}
		if !strings.Contains(out, "Stop") {
			t.Fatalf("%v: report not emitted before warn exit:\n%s", mode, out)
		}
	}

	// Canonical install (this test binary) → exit 0.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []struct{ name, rel string }{
		{"claude", filepath.Join(".claude", "settings.json")},
		{"codex", filepath.Join(".codex", "hooks.json")},
	} {
		installRelayHooks(t, filepath.Join(home, agent.rel), agent.name, exe)
	}
	if code, out := runDoctor(t, home, "relay", "hooks", "doctor", "--json"); code != ExitOK {
		t.Fatalf("canonical doctor exit %d, want 0\n%s", code, out)
	}
}

func TestRelayStatuslineDoctorMismatchExitCodes(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o700); err != nil {
		t.Fatal(err)
	}
	drifted := `{"statusLine": {"type": "command", "command": "oma relay statusline --old", "_oma_relay": "statusline"}}`
	if err := os.WriteFile(settings, []byte(drifted), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range [][]string{
		{"relay", "statusline", "doctor"},
		{"relay", "statusline", "doctor", "--json"},
	} {
		code, out := runDoctor(t, home, mode...)
		if code != ExitWarn {
			t.Fatalf("%v: exit %d, want %d (warn)\n%s", mode, code, ExitWarn, out)
		}
		if !strings.Contains(out, "mismatch") {
			t.Fatalf("%v: report not emitted before warn exit:\n%s", mode, out)
		}
	}
	// Absent slot is informational → exit 0.
	if err := os.WriteFile(settings, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out := runDoctor(t, home, "relay", "statusline", "doctor"); code != ExitOK {
		t.Fatalf("absent doctor exit %d, want 0\n%s", code, out)
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

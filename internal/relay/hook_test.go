package relay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hookPair sets up a bound claude+codex pair where claude has published a
// plan addressed to codex, and returns the codex ledger (the side that
// should auto-continue).
func hookPair(t *testing.T, ck *clock) (codex *Ledger, slug string) {
	t.Helper()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex = testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "body", "review")
	return codex, s.Pair
}

func stopPayload(active bool) []byte {
	b, _ := json.Marshal(HookPayload{Event: HookStop, StopHookActive: active})
	return b
}

func stopPayloadReason(reason string) []byte {
	b, _ := json.Marshal(HookPayload{Event: HookStop, StopReason: reason})
	return b
}

func stopPayloadLastAssistantMessage(message string) []byte {
	payload := map[string]any{
		"hook_event_name":        HookStop,
		"last_assistant_message": message,
	}
	b, _ := json.Marshal(payload)
	return b
}

func TestHookStopAutoContinue(t *testing.T) {
	ck := newClock()
	codex, _ := hookPair(t, ck)

	out := codex.Hook(HookStop, stopPayload(false))
	if out == nil || out.Decision != "block" {
		t.Fatalf("peer published → expect decision:block continuation, got %+v", out)
	}
	if !strings.Contains(out.Reason, "claude published seq=001") {
		t.Fatalf("reason = %q", out.Reason)
	}
	// Never pair decision:block with continue:false — the JSON must carry
	// only the shared subset (check for the KEYS, not substrings of the
	// human-readable reason text which legitimately says "continue").
	raw, _ := json.Marshal(out)
	for _, key := range []string{`"continue":`, `"stopReason":`, `"suppressOutput":`} {
		if strings.Contains(string(raw), key) {
			t.Fatalf("output carries the Codex-unsupported field %s: %s", key, raw)
		}
	}
}

func TestHookStopDedup(t *testing.T) {
	ck := newClock()
	codex, _ := hookPair(t, ck)
	if out := codex.Hook(HookStop, stopPayload(false)); out == nil {
		t.Fatal("first Stop must continue")
	}
	// Same pending artifact → silent (dedup fingerprint).
	if out := codex.Hook(HookStop, stopPayload(false)); out != nil {
		t.Fatalf("second Stop on the same artifact must be silent, got %+v", out)
	}
}

func TestHookStopAntiLoop(t *testing.T) {
	ck := newClock()
	codex, _ := hookPair(t, ck)
	if out := codex.Hook(HookStop, stopPayload(true)); out != nil {
		t.Fatalf("stop_hook_active must be silent (anti-loop), got %+v", out)
	}
}

func TestHookStopEscapeValves(t *testing.T) {
	// A stop reason that classifies as context/rate/auth must stay silent
	// even when a fresh peer artifact would otherwise continue (A3).
	for _, reason := range []string{
		"context window exceeded",
		"hit the token limit",
		"429 rate limit",
		"request was throttled (quota)",
		"401 unauthorized",
		"OAuth token expired",
	} {
		ck := newClock()
		codex, _ := hookPair(t, ck)
		if out := codex.Hook(HookStop, stopPayloadReason(reason)); out != nil {
			t.Fatalf("reason %q must escape (silent), got %+v", reason, out)
		}
	}
	// An ordinary stop reason with a fresh peer artifact still continues.
	ck := newClock()
	codex, _ := hookPair(t, ck)
	if out := codex.Hook(HookStop, stopPayloadReason("end_turn")); out == nil || out.Decision != "block" {
		t.Fatalf("ordinary stop must still continue, got %+v", out)
	}
}

func TestHookStopEscapeValvesCodexLastAssistantMessage(t *testing.T) {
	// Codex Stop payloads expose last_assistant_message instead of
	// stop_reason/reason; escape valves must still stay silent.
	for _, message := range []string{
		"context window exceeded",
		"429 rate limit",
		"OAuth token expired",
	} {
		ck := newClock()
		codex, _ := hookPair(t, ck)
		if out := codex.Hook(HookStop, stopPayloadLastAssistantMessage(message)); out != nil {
			t.Fatalf("last_assistant_message %q must escape (silent), got %+v", message, out)
		}
	}

	ck := newClock()
	codex, _ := hookPair(t, ck)
	if out := codex.Hook(HookStop, stopPayloadLastAssistantMessage("finished the requested review")); out == nil || out.Decision != "block" {
		t.Fatalf("ordinary last_assistant_message must still continue, got %+v", out)
	}
}

func TestHookStopUnboundSilent(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	// A session with no binding (strict binding: silent, never the lone
	// active pair).
	claude := testLedger(t, root, "claude", ck)
	mustPair(t, claude, "topic")
	otherID, _ := makeIdentity("codex", "unbound-window")
	unbound := NewLedger(root, otherID)
	unbound.Now = ck.now
	unbound.Getenv = func(string) string { return "" }
	if out := unbound.Hook(HookStop, stopPayload(false)); out != nil {
		t.Fatalf("unbound session must stay silent, got %+v", out)
	}
}

func TestHookStopAwaitingPeerSilent(t *testing.T) {
	// The side that published last is awaiting the peer → no continuation.
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "body", "review")
	if out := claude.Hook(HookStop, stopPayload(false)); out != nil {
		t.Fatalf("publisher awaiting peer must be silent, got %+v", out)
	}
}

func TestHookStopTerminalSilent(t *testing.T) {
	ck := newClock()
	codex, slug := hookPair(t, ck)
	// A terminal pair has nothing to continue.
	if err := codex.Close(slug, "abandon", "done", false); err != nil {
		t.Fatal(err)
	}
	if out := codex.Hook(HookStop, stopPayload(false)); out != nil {
		t.Fatalf("terminal pair must be silent, got %+v", out)
	}
}

func TestHookPreToolUseDeniesReadyArtifact(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	formal := mustPublish(t, claude, s.Pair, "plan", "body", "next")

	// Claude Code shape: tool_input.file_path.
	payload := map[string]any{
		"hook_event_name": HookPreToolUse,
		"tool_name":       "Edit",
		"tool_input":      map[string]string{"file_path": formal},
	}
	raw, _ := json.Marshal(payload)
	out := claude.Hook(HookPreToolUse, raw)
	if out == nil || out.HookSpecificOutput == nil || out.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("editing a .ready artifact must be denied, got %+v", out)
	}
	if !strings.Contains(out.HookSpecificOutput.PermissionDecisionReason, "correction") {
		t.Fatalf("deny reason should point at the correction flow: %q", out.HookSpecificOutput.PermissionDecisionReason)
	}

	// Codex apply_patch shape: path inside command text.
	cp := map[string]any{
		"hook_event_name": HookPreToolUse,
		"tool_name":       "apply_patch",
		"tool_input":      map[string]string{"command": "apply_patch " + formal},
	}
	craw, _ := json.Marshal(cp)
	if out := claude.Hook(HookPreToolUse, craw); out == nil || out.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("codex apply_patch to a .ready artifact must be denied, got %+v", out)
	}
	for _, sidecar := range []string{formal + ".ready", formal + ".sha256"} {
		sidecarPayload := map[string]any{
			"hook_event_name": HookPreToolUse,
			"tool_name":       "Edit",
			"tool_input":      map[string]string{"file_path": sidecar},
		}
		sraw, _ := json.Marshal(sidecarPayload)
		if out := claude.Hook(HookPreToolUse, sraw); out == nil || out.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("editing published sidecar %s must be denied, got %+v", sidecar, out)
		}
	}

	// A non-artifact edit is allowed (silent).
	other := map[string]any{
		"hook_event_name": HookPreToolUse,
		"tool_name":       "Edit",
		"tool_input":      map[string]string{"file_path": filepath.Join(t.TempDir(), "code.go")},
	}
	oraw, _ := json.Marshal(other)
	if out := claude.Hook(HookPreToolUse, oraw); out != nil {
		t.Fatalf("editing a normal file must be allowed, got %+v", out)
	}
}

func TestHookSessionStartHint(t *testing.T) {
	ck := newClock()
	codex, _ := hookPair(t, ck)
	out := codex.Hook(HookSessionStart, []byte(`{"hook_event_name":"SessionStart"}`))
	if out == nil || !strings.Contains(out.SystemMessage, "relay") {
		t.Fatalf("SessionStart should hint relay state, got %+v", out)
	}
	// SessionStart must NEVER carry a blocking decision.
	if out.Decision != "" {
		t.Fatalf("SessionStart must not block: %+v", out)
	}
}

func TestHookMalformedPayloadSilent(t *testing.T) {
	ck := newClock()
	codex, _ := hookPair(t, ck)
	// Garbage payload must not panic or error; unknown event → silent.
	if out := codex.Hook("BogusEvent", []byte("{not json")); out != nil {
		t.Fatalf("unknown event must be silent, got %+v", out)
	}
}

func TestHookDedupResetsOnNewArtifact(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "body", "review")
	if out := codex.Hook(HookStop, stopPayload(false)); out == nil {
		t.Fatal("first continuation")
	}
	if out := codex.Hook(HookStop, stopPayload(false)); out != nil {
		t.Fatal("dedup silent")
	}
	// codex replies, claude publishes again → a NEW artifact resets dedup.
	mustPublishReview(t, codex, s.Pair, VerdictApprove, 1)
	mustPublish(t, claude, s.Pair, "fix", "fixed", "recheck")
	out := codex.Hook(HookStop, stopPayload(false))
	if out == nil || !strings.Contains(out.Reason, "seq=003") {
		t.Fatalf("new artifact must re-continue, got %+v", out)
	}
}

func TestHookStateNotInLedgerArtifacts(t *testing.T) {
	// The dedup/trail state lives under _hookstate/, never in a pair dir.
	ck := newClock()
	codex, slug := hookPair(t, ck)
	codex.Hook(HookStop, stopPayload(false))
	if _, err := os.Stat(filepath.Join(codex.hookStateDir())); err != nil {
		t.Fatalf("_hookstate dir missing: %v", err)
	}
	// The pair dir must contain only artifacts/sidecars, not hook state.
	entries, _ := os.ReadDir(codex.PairDir(slug))
	for _, e := range entries {
		if strings.Contains(e.Name(), "hookstate") || strings.HasSuffix(e.Name(), ".fp") {
			t.Fatalf("hook state leaked into pair dir: %s", e.Name())
		}
	}
}

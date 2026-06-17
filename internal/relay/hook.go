package relay

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Hook events (docs/reference/relay-v2-protocol.md §12.3). The dispatcher is a
// HIDDEN machine-invoked command; humans never call it.
const (
	HookSessionStart = "SessionStart"
	HookPreToolUse   = "PreToolUse"
	HookStop         = "Stop"
)

// hookDeadline bounds one hook decision so a stalled filesystem can never
// wedge the host process — on timeout the hook stays silent (review 086
// must-fix 3: bounded status, never a waiter).
const hookDeadline = 3 * time.Second

// HookPayload is the tolerant union of the host hook inputs oma reads.
// Absent fields are simply zero — both Claude Code and Codex shapes are
// accepted (platform differences are isolated to file-path extraction).
type HookPayload struct {
	Event          string          `json:"hook_event_name"`
	StopHookActive bool            `json:"stop_hook_active"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	// A3 escape valves: a tolerant union of fields hosts use to report why
	// the turn stopped. Codex Stop payloads expose last_assistant_message.
	// When all reason text is absent, hookStop behaves exactly as before
	// (continue only on a new peer artifact).
	StopReason           string `json:"stop_reason"`
	Reason               string `json:"reason"`
	LastAssistantMessage string `json:"last_assistant_message"`
}

// HookOutput is the decision emitted to the host. Only the shared,
// cross-platform subset is ever produced: `decision:"block"` for a Stop
// continuation (NEVER paired with continue:false), the PreToolUse
// permission-deny object, and `systemMessage` hints. Codex-unsupported
// fields (continue:false / stopReason / suppressOutput) are never set.
type HookOutput struct {
	Decision           string              `json:"decision,omitempty"`
	Reason             string              `json:"reason,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

// Hook dispatches one hook event to its handler within the bounded
// deadline. A nil return means "stay silent" (exit 0, no output). It
// never errors: any internal problem degrades to silence so the host is
// never broken (the one intentional non-silent path is the PreToolUse
// deny). event overrides payload.Event when non-empty.
func (l *Ledger) Hook(event string, raw []byte) *HookOutput {
	var p HookPayload
	_ = json.Unmarshal(raw, &p) // tolerant: a malformed payload → zero value
	if event == "" {
		event = p.Event
	}

	ch := make(chan *HookOutput, 1)
	go func() {
		switch event {
		case HookStop:
			ch <- l.hookStop(p)
		case HookPreToolUse:
			ch <- l.hookPreToolUse(p)
		case HookSessionStart:
			ch <- l.hookSessionStart()
		default:
			ch <- nil
		}
	}()
	select {
	case out := <-ch:
		return out
	case <-time.After(hookDeadline):
		go l.hookTrail(event, "deadline exceeded; silent")
		return nil
	}
}

// hookStop is the auto-continue handler — the experience-defining path.
// Guardrails (review 086 must-fix 3): honor stop_hook_active (anti-loop);
// strict binding (unbound → silent, never the single-active auto-adopt);
// dedup by fingerprint (the same pending artifact never re-continues);
// emit decision:"block" for continuation and never continue:false.
func (l *Ledger) hookStop(p HookPayload) *HookOutput {
	if p.StopHookActive {
		return nil // anti-loop: a hook-induced continuation must not recurse
	}
	// A3 escape valves: never inject a continuation when the host stopped
	// for context exhaustion (would wedge compaction), rate limiting (retry
	// storm) or auth failure (auth loop) — even if a fresh peer artifact is
	// waiting. The peer's turn is still in the ledger; the user reads it via
	// `oma relay status` once the blocking condition clears.
	if valve, escape := stopEscape(p); escape {
		l.hookTrail(HookStop, "escape valve: "+valve)
		return nil
	}
	b, err := l.loadBinding()
	if err != nil {
		return nil // strict binding: an unbound session stays silent
	}
	s, err := l.LoadSession(b.Pair)
	if err != nil || s.Terminal() {
		return nil
	}
	peer, err := s.Peer(l.Identity.Author)
	if err != nil {
		return nil
	}
	path, ok, err := l.newPeerArtifact(l.PairDir(b.Pair), peer)
	if err != nil || !ok {
		return nil
	}
	seq, _, kind, _ := ParseArtifactName(filepath.Base(path))
	fp := fmt.Sprintf("peer:%d", seq)
	if l.hookFingerprintSeen(b.Pair, fp) {
		return nil // dedup: already surfaced this artifact
	}
	l.hookFingerprintMark(b.Pair, fp)
	l.hookTrail(HookStop, fmt.Sprintf("continue on %s seq=%d", peer, seq))
	return &HookOutput{
		Decision: "block",
		Reason: fmt.Sprintf("[relay-action] %s published seq=%03d kind=%s addressed to you — read %s and continue the relay (oma relay status --json).",
			peer, seq, kind, path),
	}
}

// stopEscape classifies a host stop reason into an A3 escape valve. It
// reads the tolerant union of reason fields and matches substrings; an
// empty or unrecognized reason returns ("", false) so the normal
// peer-artifact continuation proceeds unchanged. The bias is deliberately
// toward escaping: a false escape only skips one turn's auto-nudge (the
// peer artifact stays readable), whereas a missed context/rate/auth stop
// is the deadlock these valves exist to prevent.
func stopEscape(p HookPayload) (valve string, escape bool) {
	r := strings.ToLower(strings.TrimSpace(p.StopReason + " " + p.Reason + " " + p.LastAssistantMessage))
	switch {
	case r == "":
		return "", false
	case containsAny(r, "context", "compact", "token limit", "token_limit", "max tokens", "max_tokens"):
		return "context_limit", true
	case containsAny(r, "429", "rate limit", "rate_limit", "ratelimit", "quota", "overloaded", "throttl"):
		return "rate_limit", true
	case containsAny(r, "401", "403", "unauthor", "forbidden", "auth", "expired", "credential", "api key", "api_key"):
		return "auth_error", true
	default:
		return "", false
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// hookPreToolUse denies edits to published .ready artifacts (append-only
// enforcement). The target path is extracted per platform: Claude Code
// exposes tool_input.file_path; Codex apply_patch carries paths in
// tool_input.command. A non-edit or non-artifact path is allowed (nil).
func (l *Ledger) hookPreToolUse(p HookPayload) *HookOutput {
	for _, path := range hookTargetPaths(p) {
		if seq, ok := l.readyArtifactSeq(path); ok {
			return &HookOutput{
				SystemMessage: "[relay-hint] denied edit to a published relay artifact (append-only).",
				HookSpecificOutput: &hookSpecificOutput{
					HookEventName:            HookPreToolUse,
					PermissionDecision:       "deny",
					PermissionDecisionReason: fmt.Sprintf("Published relay artifact is append-only. To correct seq %d: `oma relay draft --kind correction --corrects %d`.", seq, seq),
				},
			}
		}
	}
	return nil
}

// hookSessionStart emits a lightweight bounded status hint — NOT the full
// preflight (review 086 should-fix: FS probes are too costly per session).
func (l *Ledger) hookSessionStart() *HookOutput {
	st := l.statuslineState("")
	if !st.Bound {
		return nil
	}
	msg := "[relay-state] " + st.Text
	if st.Turn == "you" {
		msg += " — your turn"
	}
	return &HookOutput{
		SystemMessage: msg,
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     HookSessionStart,
			AdditionalContext: msg,
		},
	}
}

// readyArtifactSeq reports whether path is a published artifact or one
// of its integrity sidecars under this ledger, and its seq.
func (l *Ledger) readyArtifactSeq(path string) (int, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, false
	}
	root, err := filepath.Abs(l.Root)
	if err != nil {
		return 0, false
	}
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return 0, false
	}
	artifact := abs
	for _, suffix := range []string{".ready", ".sha256"} {
		if strings.HasSuffix(artifact, suffix) {
			artifact = strings.TrimSuffix(artifact, suffix)
			break
		}
	}
	seq, _, _, ok := ParseArtifactName(filepath.Base(artifact))
	if !ok {
		return 0, false
	}
	if _, err := statReady(artifact); err != nil {
		return 0, false
	}
	return seq, true
}

// hookTargetPaths extracts candidate edited paths from a payload across
// platforms.
func hookTargetPaths(p HookPayload) []string {
	if len(p.ToolInput) == 0 {
		return nil
	}
	var ci struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
		Command  string `json:"command"`
	}
	_ = json.Unmarshal(p.ToolInput, &ci)
	var out []string
	for _, s := range []string{ci.FilePath, ci.Path} {
		if s != "" {
			out = append(out, s)
		}
	}
	// Codex apply_patch: paths appear inside the command text; pull any
	// token that looks like an NNN-author-kind.md artifact.
	if ci.Command != "" {
		for tok := range strings.FieldsSeq(ci.Command) {
			if _, _, _, ok := ParseArtifactName(filepath.Base(tok)); ok {
				out = append(out, tok)
			}
		}
	}
	return out
}

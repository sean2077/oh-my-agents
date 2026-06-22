package relay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateParticipantSessions(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	l := testLedger(t, root, "claude", ck)
	s := mustPair(t, l, "topic-a")

	// Simulate a v0.7.0 pair: strip participant_sessions from session.json.
	path := filepath.Join(l.PairDir(s.Pair), "session.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	delete(obj, "participant_sessions")
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	// The current Validate() requires participant_sessions, so the pre-migration
	// pair is unreadable.
	if _, err := l.LoadSession(s.Pair); err == nil {
		t.Fatal("expected LoadSession to reject a pre-migration pair, got nil")
	}

	// Dry-run plans the repair but does not perform it.
	plan, err := l.MigrateParticipantSessions(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || plan[0].Applied {
		t.Fatalf("dry-run plan = %+v, want exactly one unapplied entry", plan)
	}
	if _, err := l.LoadSession(s.Pair); err == nil {
		t.Fatal("dry-run must not repair the pair")
	}

	// Apply repairs it to an empty (unclaimed) participant_sessions map.
	applied, err := l.MigrateParticipantSessions(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 || !applied[0].Applied {
		t.Fatalf("apply = %+v, want exactly one applied entry", applied)
	}
	got, err := l.LoadSession(s.Pair)
	if err != nil {
		t.Fatalf("LoadSession after migration: %v", err)
	}
	if got.ParticipantSessions == nil || len(got.ParticipantSessions) != 0 {
		t.Fatalf("participant_sessions = %v, want empty non-nil map", got.ParticipantSessions)
	}

	again, err := l.MigrateParticipantSessions(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second apply = %d actions, want 0 (idempotent)", len(again))
	}
}

// TestMigrateParticipantSessionsPreservesUnknownFields proves the repair adds
// only participant_sessions and preserves every other field — a v0.7.0
// session.json may carry fields this binary still honors, and the additive
// schema contract forbids dropping them.
func TestMigrateParticipantSessionsPreservesUnknownFields(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	l := testLedger(t, root, "claude", ck)
	s := mustPair(t, l, "topic-keep")

	path := filepath.Join(l.PairDir(s.Pair), "session.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	delete(obj, "participant_sessions")
	obj["future_field"] = json.RawMessage(`"preserve me"`)
	wantPair := string(obj["pair"])
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := l.MigrateParticipantSessions(true); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(after, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["future_field"]) != `"preserve me"` {
		t.Errorf("migration dropped/altered unknown field: %q", got["future_field"])
	}
	if string(got["pair"]) != wantPair {
		t.Errorf("migration altered pair field: got %q want %q", got["pair"], wantPair)
	}
	if _, ok := got["participant_sessions"]; !ok {
		t.Error("migration did not add participant_sessions")
	}

	entries, err := os.ReadDir(l.PairDir(s.Pair))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp residue beside session.json: %s", e.Name())
		}
	}
}

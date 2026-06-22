package relay

import (
	"strings"
	"testing"
)

// TestWaitDeliversOutOfOrderPeerArtifact pins the publication-order fix: when
// the peer's artifact carries a LOWER seq than the reader's own but is
// published AFTER it, wait must still deliver it. Before the consumption
// cursor, the "peer seq > my own latest seq" predicate dropped it silently.
func TestWaitDeliversOutOfOrderPeerArtifact(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "race")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}

	// Concurrent-start interleaving: claude reserves 001, codex reserves 002,
	// codex publishes 002 first, then claude publishes 001 (lower seq, later).
	dClaude, err := claude.CreateDraft(s.Pair, "note", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	dCodex, err := codex.CreateDraft(s.Pair, "note", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codex.Publish(dCodex, PublishInput{Body: "codex first", Prompt: "your turn"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := claude.Publish(dClaude, PublishInput{Body: "claude second", Prompt: "your turn"}, false); err != nil {
		t.Fatal(err)
	}

	// codex's own latest seq is 002; claude's later-published 001 must still
	// be delivered, not skipped.
	res, err := codex.Wait(s.Pair, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != WaitNewArtifact {
		t.Fatalf("codex.Wait code = %d (%s), want WaitNewArtifact delivering claude's seq-001", res.Code, res.Reason)
	}
	if !strings.Contains(res.ArtifactPath, "001-claude") {
		t.Fatalf("codex.Wait delivered %q, want claude's seq-001 artifact", res.ArtifactPath)
	}

	// After codex replies (takes its turn), its cursor advances past claude's
	// 001, so a subsequent wait no longer re-delivers it.
	dReply, err := codex.CreateDraft(s.Pair, "note", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codex.Publish(dReply, PublishInput{Body: "codex reply", Prompt: "your turn"}, false); err != nil {
		t.Fatal(err)
	}
	res2, err := codex.Wait(s.Pair, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Code != WaitTimeout {
		t.Fatalf("after replying, codex.Wait code = %d (%s), want WaitTimeout (claude's 001 already consumed)", res2.Code, res2.Reason)
	}
}

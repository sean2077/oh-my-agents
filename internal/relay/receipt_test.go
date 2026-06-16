package relay

import (
	"strings"
	"testing"
)

// mustPublishReview publishes a kind:review carrying a typed verdict that
// judges `target` (A2).
func mustPublishReview(t *testing.T, l *Ledger, slug, verdict string, target int) string {
	t.Helper()
	draft, err := l.CreateDraft(slug, "review", &target, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := l.Publish(draft, PublishInput{Body: "review body", Prompt: "next please", Verdict: verdict, ReviewTarget: &target}, false)
	if err != nil {
		t.Fatal(err)
	}
	return formal
}

// TestCompletionReceiptAndApproveGate: a lead decision after a non-lead
// approve review carries a receipt, and only then does close --outcome
// approve pass (A1+A2).
func TestCompletionReceiptAndApproveGate(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck) // lead
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "the plan", "review it") // seq 1
	mustPublishReview(t, codex, s.Pair, VerdictApprove, 1)          // seq 2

	// No lead decision yet → approve close is fail-closed (dry-run previews).
	if err := claude.Close(s.Pair, "approve", "premature", true); err == nil {
		t.Fatal("approve close without a decision+receipt must be refused")
	}

	dec := mustPublish(t, claude, s.Pair, "decision", "shipping it", "done") // seq 3
	fm, _, err := ReadArtifact(dec)
	if err != nil {
		t.Fatal(err)
	}
	if fm.ReceiptID == "" || fm.QualityGateSeq == nil || *fm.QualityGateSeq != 2 || fm.PlanRefSeq == nil || *fm.PlanRefSeq != 1 {
		t.Fatalf("decision must carry a receipt over plan 1 + approve review 2: %+v", fm)
	}
	// With the receipt the gate passes — dry-run preview, then for real.
	if err := claude.Close(s.Pair, "approve", "shipped", true); err != nil {
		t.Fatalf("approve close with receipt must pass (dry-run): %v", err)
	}
	if err := claude.Close(s.Pair, "approve", "shipped", false); err != nil {
		t.Fatalf("approve close with receipt must pass: %v", err)
	}
}

// TestApproveGateRejectsApproveWithChanges: approve-with-changes is not a
// close-satisfying verdict, so no receipt forms and approve is refused.
func TestApproveGateRejectsApproveWithChanges(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "the plan", "review it")   // 1
	mustPublishReview(t, codex, s.Pair, VerdictApproveWithChanges, 1) // 2
	mustPublish(t, claude, s.Pair, "decision", "shipping", "done")    // 3: no approve → no receipt

	if err := claude.Close(s.Pair, "approve", "x", true); err == nil || !strings.Contains(err.Error(), "receipt") {
		t.Fatalf("approve-with-changes must not satisfy close: err=%v", err)
	}
	// reject/abandon never need a receipt.
	if err := claude.Close(s.Pair, "abandon", "dropping", false); err != nil {
		t.Fatalf("abandon must always work: %v", err)
	}
}

// TestApproveGateRejectsLeadSelfReview: the approve review must come from
// the non-lead participant; a lead self-approve does not satisfy the gate.
func TestApproveGateRejectsLeadSelfReview(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "the plan", "review it") // 1
	mustPublishReview(t, claude, s.Pair, VerdictApprove, 1)         // 2: LEAD self-approve
	mustPublish(t, claude, s.Pair, "decision", "shipping", "done")  // 3: no non-lead approve → no receipt

	if err := claude.Close(s.Pair, "approve", "x", true); err == nil {
		t.Fatal("a lead self-approve must not satisfy the approve gate")
	}
}

package relay

import (
	"testing"
)

// mustPublishReview publishes a kind:review carrying a typed verdict that
// judges `target` (A2/R3).
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
// approve review of the latest work carries a receipt, and only then does
// close --outcome approve pass (A1+A2).
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
	mustPublishReview(t, codex, s.Pair, VerdictApprove, 1)          // seq 2 approve(plan)

	// No lead decision yet → fail-closed (gate miss).
	if err := claude.Close(s.Pair, "approve", "premature", true); err == nil {
		t.Fatal("approve close without a decision+receipt must be refused")
	}

	dec := mustPublish(t, claude, s.Pair, "decision", "shipping the plan", "done") // seq 3
	fm, _, err := ReadArtifact(dec)
	if err != nil {
		t.Fatal(err)
	}
	if fm.ReceiptID == "" || fm.ReviewedHeadSeq == nil || *fm.ReviewedHeadSeq != 1 || fm.QualityGateSeq == nil || *fm.QualityGateSeq != 2 {
		t.Fatalf("decision must carry a receipt binding reviewed-head 1 + review 2: %+v", fm)
	}
	if err := claude.Close(s.Pair, "approve", "shipped", true); err != nil {
		t.Fatalf("approve close with receipt must pass (dry-run): %v", err)
	}
	if err := claude.Close(s.Pair, "approve", "shipped", false); err != nil {
		t.Fatalf("approve close with receipt must pass: %v", err)
	}
}

// TestApproveGateRejectsUnreviewedFix (R1): a fix published after the approve
// review (which only approved the plan) must block approve close until the
// fix itself is reviewed.
func TestApproveGateRejectsUnreviewedFix(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "the plan", "review it")     // 1
	mustPublishReview(t, codex, s.Pair, VerdictApprove, 1)              // 2 approve(plan)
	mustPublish(t, claude, s.Pair, "fix", "material change", "recheck") // 3 UNREVIEWED
	mustPublish(t, claude, s.Pair, "decision", "shipping", "done")      // 4

	if err := claude.Close(s.Pair, "approve", "x", true); err == nil {
		t.Fatal("approve close must reject: the fix (seq 3) was never reviewed")
	}

	// Once the fix itself is approved, a decision rebuilt over it passes.
	mustPublishReview(t, codex, s.Pair, VerdictApprove, 3)             // 5 approve(fix)
	mustPublish(t, claude, s.Pair, "decision", "shipping fix", "done") // 6 receipt over head 3
	if err := claude.Close(s.Pair, "approve", "shipped", false); err != nil {
		t.Fatalf("approve close after the fix is reviewed must pass: %v", err)
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

	if err := claude.Close(s.Pair, "approve", "x", true); err == nil {
		t.Fatal("approve-with-changes must not satisfy close")
	}
	if err := claude.Close(s.Pair, "abandon", "dropping", false); err != nil {
		t.Fatalf("abandon must always work: %v", err)
	}
}

// TestApproveGateRejectsLeadSelfReview: the approve review must come from the
// non-lead participant; a lead self-approve does not satisfy the gate.
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

// TestReviewRequiresVerdictAndTarget (R3): a ready kind:review without a
// verdict or without a positive target is refused at publish.
func TestReviewRequiresVerdictAndTarget(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "p", "review") // 1

	one := 1
	d1, err := codex.CreateDraft(s.Pair, "review", &one, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codex.Publish(d1, PublishInput{Body: "b", Prompt: "p"}, false); err == nil {
		t.Fatal("a review without a verdict must be refused")
	}

	d2, err := codex.CreateDraft(s.Pair, "review", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codex.Publish(d2, PublishInput{Body: "b", Prompt: "p", Verdict: VerdictApprove}, false); err == nil {
		t.Fatal("a review without a target must be refused")
	}
}

package relay

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestApproveCloseGateFailureRollsBackToActive pins COR-4: an `approve` close
// whose quality gate fails (no valid completion receipt) must roll the pair
// back from `closing` to `active` (session.go:496-512, the wasActive branch),
// returning the gate error — never leaving the pair stuck in `closing`. The pair
// then stays recoverable: an `abandon` close still archives it.
//
// fail-before: if the wasActive rollback (session.go:498-509) were removed, the
// pair would stay `closing` after the failed approve, and the StatusActive
// assertion below fails.
func TestApproveCloseGateFailureRollsBackToActive(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}

	// Approve-close with no lead decision/receipt: the A2 quality gate fails.
	if err := claude.Close(s.Pair, "approve", "premature", false); !errors.Is(err, ErrGate) {
		t.Fatalf("approve close without a receipt: err = %v, want ErrGate", err)
	}

	// The pair must be rolled back to active (recoverable), not stuck closing.
	got, err := claude.LoadSession(s.Pair)
	if err != nil {
		t.Fatalf("load after rolled-back approve close: %v", err)
	}
	if got.Status != StatusActive {
		t.Fatalf("status after failed approve close = %q, want %q (rolled back)", got.Status, StatusActive)
	}
	if got.Closed != nil || got.Outcome != nil || got.Reason != nil {
		t.Fatalf("rollback must clear close fields: closed=%v outcome=%v reason=%v", got.Closed, got.Outcome, got.Reason)
	}

	// Recovery path: the still-active pair can be abandoned, which archives it.
	if err := claude.Close(s.Pair, "abandon", "dropping", false); err != nil {
		t.Fatalf("abandon close after rollback must succeed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "_archive", s.Pair, "CLOSED")); err != nil {
		t.Fatalf("abandon close must archive the pair with a CLOSED marker: %v", err)
	}
	if _, err := os.Stat(claude.PairDir(s.Pair)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("live pair dir must be archived away after abandon: %v", err)
	}
}

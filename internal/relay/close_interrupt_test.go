package relay

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestInterruptedCloseConvergesAtEveryStep mirrors the publish interruption
// matrix (TestInterruptedPublishConvergesAtEveryStep) for Ledger.Close. A kill
// after ANY durable close step must leave a recoverable ledger: re-running the
// identical close converges to an archived, closed pair.
//
// The four step names are the StepHook boundaries instrumented in session.go
// Close: close-closing (status→closing saved) → close-closed (status→closed
// saved) → close-marker (CLOSED file written) → close-archived (pair dir
// renamed into _archive). outcome=abandon exercises the close MUTATION sequence
// without the approve receipt gate (that gate is covered by receipt_test.go).
//
// COR-2 seam: this reuses the existing Ledger.StepHook (relay.go) — the only
// production change is the four l.step("close-…") calls; no public API changes.
func TestInterruptedCloseConvergesAtEveryStep(t *testing.T) {
	steps := []string{"close-closing", "close-closed", "close-marker", "close-archived"}
	for _, killAfter := range steps {
		t.Run(killAfter, func(t *testing.T) {
			ck := newClock()
			root, _ := initRoot(t, ck)
			claude := testLedger(t, root, "claude", ck)
			codex := testLedger(t, root, "codex", ck)
			s := mustPair(t, claude, "topic")
			if _, err := codex.Join(s.Pair, false); err != nil {
				t.Fatal(err)
			}

			boom := errors.New("simulated kill")
			claude.StepHook = func(step string) error {
				if step == killAfter {
					return boom
				}
				return nil
			}
			if err := claude.Close(s.Pair, "abandon", "dropping", false); !errors.Is(err, boom) {
				t.Fatalf("interrupted close at %s: err = %v, want the injected kill", killAfter, err)
			}

			// Re-running the identical close converges, wherever the kill landed.
			claude.StepHook = nil
			if err := claude.Close(s.Pair, "abandon", "dropping", false); err != nil {
				t.Fatalf("converging close after kill at %s: %v", killAfter, err)
			}

			// Convergence == the pair is archived: the live pair dir is gone and
			// the archive holds the pair with its CLOSED marker.
			if _, err := os.Stat(claude.PairDir(s.Pair)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("live pair dir must be archived away after kill at %s: %v", killAfter, err)
			}
			if _, err := os.Stat(filepath.Join(root, "_archive", s.Pair, "CLOSED")); err != nil {
				t.Fatalf("archived CLOSED marker must exist after kill at %s: %v", killAfter, err)
			}
		})
	}
}

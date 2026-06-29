package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestReclaimTimeoutClearsMidAgedAbandonedClaim pins COR-6: an abandoned
// `.reclaim` election claim aged BETWEEN reclaimTimeout and the full lock lease
// must now be cleared. A reclaimer only holds the claim briefly to rewrite the
// owner file, so a claim older than reclaimTimeout is a crashed reclaimer, not a
// live election — it should not block the next reclaimer for the whole lease.
//
// fail-before: with the old `> defaultLockLease` (15m) check, a ~3-minute claim
// is NOT cleared (3m < 15m); with `> reclaimTimeout` (2m) it is.
func TestReclaimTimeoutClearsMidAgedAbandonedClaim(t *testing.T) {
	// Premise guard: a mid-aged claim only distinguishes the two timeouts when
	// reclaimTimeout < (chosen age) < defaultLockLease.
	if reclaimTimeout+time.Minute >= defaultLockLease {
		t.Fatalf("test premise broken: reclaimTimeout+1m (%v) must be < defaultLockLease (%v)", reclaimTimeout+time.Minute, defaultLockLease)
	}

	dir := filepath.Join(t.TempDir(), "resource.lock")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	staleOwner(t, dir) // the lock itself is observed stale (expired owner)

	claim := dir + ".reclaim"
	if err := os.Mkdir(claim, 0o700); err != nil {
		t.Fatal(err)
	}
	// Age the claim to reclaimTimeout < age < defaultLockLease.
	mid := time.Now().Add(-(reclaimTimeout + time.Minute))
	if err := os.Chtimes(claim, mid, mid); err != nil {
		t.Fatal(err)
	}

	token, ok := takeOverStale(dir)
	if ok || token != "" {
		t.Fatalf("takeOverStale must abort while a claim exists, got token=%q ok=%v", token, ok)
	}
	if _, err := os.Stat(claim); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an abandoned .reclaim older than reclaimTimeout must be cleared, stat err = %v", err)
	}
}

package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// backdate sets path's mtime to roughly `defaultLockLease + 5min` ago so it
// reads as older than the lease without depending on wall-clock waiting.
func backdate(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-(defaultLockLease + 5*time.Minute))
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes %q: %v", path, err)
	}
}

// staleOwner writes an owner file whose lease already expired (used to mark a
// lock dir stale by its owner metadata rather than by mtime).
func staleOwner(t *testing.T, dir string) {
	t.Helper()
	past := time.Now().UTC().Add(-time.Hour)
	body := "pid=1\ntoken=stale\ncreated=" + past.Format(time.RFC3339Nano) +
		"\nexpires=" + past.Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStaleLockMtimeFallback(t *testing.T) {
	// staleLock falls back to time.Since(dir mtime) > defaultLockLease whenever
	// the owner file is missing or its `expires` is unparseable
	// (lock.go:117-132). Covers both fallback sub-cases plus a fresh control.
	t.Run("no owner file, old mtime -> stale", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "resource.lock")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		backdate(t, dir)
		stale, err := staleLock(dir)
		if err == nil {
			// readOwner failed with ENOENT, which staleLock returns as the
			// trailing error alongside the mtime verdict.
			t.Fatalf("expected the missing-owner error to be returned, got nil")
		}
		if !stale {
			t.Fatal("a lock dir with no owner and an old mtime must be stale")
		}
	})

	t.Run("corrupt owner expires, old mtime -> stale", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "resource.lock")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		corrupt := "pid=1\ntoken=stale\nexpires=not-a-time\n"
		if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(corrupt), 0o600); err != nil {
			t.Fatal(err)
		}
		backdate(t, dir)
		stale, _ := staleLock(dir)
		if !stale {
			t.Fatal("an unparseable expires with an old mtime must fall back to stale")
		}
	})

	t.Run("no owner file, fresh mtime -> not stale", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "resource.lock")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		// mtime is "now" from the Mkdir; do not backdate.
		stale, _ := staleLock(dir)
		if stale {
			t.Fatal("a freshly created lock dir with no owner must not be stale")
		}
	})
}

func TestTakeOverStaleClearsAbandonedReclaim(t *testing.T) {
	// takeOverStale: when os.Mkdir(claim) fails because dir+".reclaim" already
	// exists AND that claim dir is older than the lease, the abandoned claim is
	// removed and ("", false) is returned (lock.go:149-156).
	dir := filepath.Join(t.TempDir(), "resource.lock")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	staleOwner(t, dir) // dir is observed stale via its owner metadata.

	claim := dir + ".reclaim"
	if err := os.Mkdir(claim, 0o700); err != nil {
		t.Fatal(err)
	}
	backdate(t, claim) // abandoned: older than the lease.

	token, ok := takeOverStale(dir)
	if ok || token != "" {
		t.Fatalf("takeOverStale must abort when a claim exists, got token=%q ok=%v", token, ok)
	}
	if _, err := os.Stat(claim); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an abandoned .reclaim claim must be removed, stat err = %v", err)
	}
}

func TestAcquireLockConcurrentReclaimSingleHolder(t *testing.T) {
	// A stale lock with many concurrent AcquireLock callers must never yield two
	// simultaneous live holders. The .reclaim sibling-Mkdir election serializes
	// reclaimers (lock.go:46-53,147-158), so at most one holds the lock at a
	// time; released holders let waiters acquire in turn.
	dir := filepath.Join(t.TempDir(), "resource.lock")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	staleOwner(t, dir)

	const n = 8
	var (
		held     atomic.Int32 // currently-live holders; must never exceed 1
		overlap  atomic.Bool  // set if two holders are ever live at once
		acquired atomic.Int32 // total successful acquisitions
		wg       sync.WaitGroup
		start    sync.WaitGroup
	)
	start.Add(1)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start.Wait() // release all goroutines together
			lock, err := AcquireLock(dir, 2*time.Second)
			if err != nil {
				if !errors.Is(err, ErrLockHeld) {
					t.Errorf("unexpected AcquireLock error: %v", err)
				}
				return
			}
			acquired.Add(1)
			if held.Add(1) > 1 {
				overlap.Store(true)
			}
			time.Sleep(2 * time.Millisecond) // widen any overlap window
			held.Add(-1)
			if err := lock.Release(); err != nil {
				t.Errorf("release: %v", err)
			}
		}()
	}
	start.Done()
	wg.Wait()

	if overlap.Load() {
		t.Fatal("two holders were live simultaneously; the lock is not exclusive")
	}
	if acquired.Load() < 1 {
		t.Fatal("at least one goroutine must reclaim the stale lock")
	}
	// After all releases the canonical lock path must be free.
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock dir must be free after all holders release, stat err = %v", err)
	}
}

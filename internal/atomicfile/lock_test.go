package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLockReclaimsExpiredOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "resource.lock")
	first, err := AcquireLock(dir, 0)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	owner := "pid=1\ntoken=stale\ncreated=" + past.Format(time.RFC3339Nano) + "\nexpires=" + past.Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(owner), 0o600); err != nil {
		t.Fatal(err)
	}

	second, err := AcquireLock(dir, 0)
	if err != nil {
		t.Fatalf("reclaim expired lock: %v", err)
	}
	if first.token == second.token {
		t.Fatal("reclaimed lock must receive a fresh token")
	}
	if err := first.Release(); !errors.Is(err, ErrLockNotOwned) {
		t.Fatalf("old owner release err = %v, want ErrLockNotOwned", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("second release: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock dir after release: %v", err)
	}
}

func TestReleaseRefusesDifferentOwnerToken(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "resource.lock")
	lock, err := AcquireLock(dir, 0)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	other := &Lock{dir: dir, token: "not-" + lock.token}
	if err := other.Release(); !errors.Is(err, ErrLockNotOwned) {
		t.Fatalf("other release err = %v, want ErrLockNotOwned", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("lock dir should remain: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("owner release: %v", err)
	}
}

func TestReclaimLockPreservesFreshLock(t *testing.T) {
	// Regression: two reclaimers racing the same stale lock must not delete
	// each other's freshly acquired lock. reclaimLock is given the token it
	// observed as stale; if a fresh acquirer slipped in (different token)
	// during the check→reclaim window, the lock must be left intact.
	dir := filepath.Join(t.TempDir(), "resource.lock")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(time.Hour)
	fresh := "pid=2\ntoken=fresh\ncreated=" + future.Format(time.RFC3339Nano) + "\nexpires=" + future.Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(fresh), 0o600); err != nil {
		t.Fatal(err)
	}
	// A reclaimer that observed a now-replaced "stale" token must not nuke the
	// fresh lock now present.
	reclaimLock(dir, "stale")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("fresh lock dir must survive a mismatched reclaim: %v", err)
	}
	tok, err := readOwnerToken(dir)
	if err != nil {
		t.Fatalf("read owner: %v", err)
	}
	if tok != "fresh" {
		t.Fatalf("owner token = %q, want fresh (reclaim must not have replaced it)", tok)
	}
}

func TestReclaimLockRemovesObservedStaleLock(t *testing.T) {
	// reclaimLock removes the lock whose owner still matches the observed token.
	dir := filepath.Join(t.TempDir(), "resource.lock")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	stale := "pid=1\ntoken=stale\ncreated=" + past.Format(time.RFC3339Nano) + "\nexpires=" + past.Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	reclaimLock(dir, "stale")
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("observed-stale lock must be removed, stat err = %v", err)
	}
}

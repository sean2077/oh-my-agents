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

func TestTakeOverStaleReplacesOwnerInPlace(t *testing.T) {
	// A stale lock is reclaimed by transferring ownership IN PLACE: dir is
	// never removed/renamed (so no concurrent Mkdir-acquire window opens), the
	// owner file is overwritten with a fresh token, and the election lock is
	// cleaned up.
	dir := filepath.Join(t.TempDir(), "resource.lock")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	stale := "pid=1\ntoken=stale\ncreated=" + past.Format(time.RFC3339Nano) + "\nexpires=" + past.Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	token, ok := takeOverStale(dir)
	if !ok || token == "" || token == "stale" {
		t.Fatalf("takeOverStale on a stale lock: token=%q ok=%v, want a fresh token", token, ok)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("lock dir must still exist after in-place takeover: %v", err)
	}
	if tok, _ := readOwnerToken(dir); tok != token {
		t.Fatalf("owner token = %q, want the taken-over token %q", tok, token)
	}
	if _, err := os.Stat(dir + ".reclaim"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".reclaim election lock must be cleaned up, stat err = %v", err)
	}
}

func TestTakeOverStaleRefusesFreshLock(t *testing.T) {
	// A lock that is fresh (e.g. re-acquired between the staleness observation
	// and the reclaim) must never be stolen — the owner is left untouched.
	dir := filepath.Join(t.TempDir(), "resource.lock")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(time.Hour)
	fresh := "pid=2\ntoken=fresh\ncreated=" + future.Format(time.RFC3339Nano) + "\nexpires=" + future.Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(fresh), 0o600); err != nil {
		t.Fatal(err)
	}
	if token, ok := takeOverStale(dir); ok {
		t.Fatalf("takeOverStale must refuse a fresh lock, got token=%q ok=true", token)
	}
	if tok, _ := readOwnerToken(dir); tok != "fresh" {
		t.Fatalf("fresh owner must be untouched, got %q", tok)
	}
	if _, err := os.Stat(dir + ".reclaim"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".reclaim election lock must be cleaned up, stat err = %v", err)
	}
}

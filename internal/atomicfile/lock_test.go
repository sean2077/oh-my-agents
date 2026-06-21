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

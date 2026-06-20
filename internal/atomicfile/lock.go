package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrLockHeld means another process currently owns the lock directory.
var ErrLockHeld = errors.New("file lock held")

// Lock is a cross-process directory lock acquired with atomic mkdir.
type Lock struct {
	dir string
}

// AcquireLock obtains dir as an exclusive lock. timeout <= 0 performs one
// attempt. The caller is responsible for ensuring the parent directory exists.
func AcquireLock(dir string, timeout time.Duration) (*Lock, error) {
	deadline := time.Now().Add(timeout)
	for {
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			writeLockMetadata(dir)
			return &Lock{dir: dir}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if timeout <= 0 || !time.Now().Before(deadline) {
			return nil, fmt.Errorf("%w: %s", ErrLockHeld, dir)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Release frees the lock. It removes only the lock directory tree.
func (l *Lock) Release() error {
	if l == nil || l.dir == "" {
		return nil
	}
	return os.RemoveAll(l.dir)
}

// WithLock runs fn while holding dir.
func WithLock(dir string, timeout time.Duration, fn func() error) error {
	l, err := AcquireLock(dir, timeout)
	if err != nil {
		return err
	}
	defer func() { _ = l.Release() }()
	return fn()
}

func writeLockMetadata(dir string) {
	body := []byte(fmt.Sprintf("pid=%d\ncreated=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano)))
	_ = Write(filepath.Join(dir, "owner"), body, 0o600)
}

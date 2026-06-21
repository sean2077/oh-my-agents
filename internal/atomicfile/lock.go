package atomicfile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrLockHeld means another process currently owns the lock directory.
var ErrLockHeld = errors.New("file lock held")

// ErrLockNotOwned means the caller tried to release a lock now owned by
// another token. This protects stale owners from deleting a reclaimed lock.
var ErrLockNotOwned = errors.New("file lock not owned")

const defaultLockLease = 15 * time.Minute

// Lock is a cross-process directory lock acquired with atomic mkdir.
type Lock struct {
	dir   string
	token string
}

// AcquireLock obtains dir as an exclusive lock. timeout <= 0 performs one
// attempt. The caller is responsible for ensuring the parent directory exists.
func AcquireLock(dir string, timeout time.Duration) (*Lock, error) {
	deadline := time.Now().Add(timeout)
	for {
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			token, err := writeLockMetadata(dir)
			if err != nil {
				_ = os.RemoveAll(dir)
				return nil, err
			}
			return &Lock{dir: dir, token: token}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if stale, _ := staleLock(dir); stale {
			_ = reclaimLock(dir)
			continue
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
	token, err := readOwnerToken(l.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if token == "" || token != l.token {
		return fmt.Errorf("%w: %s", ErrLockNotOwned, l.dir)
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

func writeLockMetadata(dir string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	body := []byte(fmt.Sprintf("pid=%d\ntoken=%s\ncreated=%s\nexpires=%s\n",
		os.Getpid(),
		token,
		now.Format(time.RFC3339Nano),
		now.Add(defaultLockLease).Format(time.RFC3339Nano),
	))
	if err := Write(filepath.Join(dir, "owner"), body, 0o600); err != nil {
		return "", err
	}
	return token, nil
}

func randomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func staleLock(dir string) (bool, error) {
	owner, err := readOwner(dir)
	if err == nil {
		if expires := owner["expires"]; expires != "" {
			t, err := time.Parse(time.RFC3339Nano, expires)
			if err == nil {
				return time.Now().UTC().After(t), nil
			}
		}
	}
	info, statErr := os.Stat(dir)
	if statErr != nil {
		return false, statErr
	}
	return time.Since(info.ModTime()) > defaultLockLease, err
}

func reclaimLock(dir string) error {
	parent := filepath.Dir(dir)
	staleName := fmt.Sprintf(".%s.stale.%d.%s", filepath.Base(dir), os.Getpid(), time.Now().UTC().Format("20060102150405.000000000"))
	staleDir := filepath.Join(parent, staleName)
	if err := os.Rename(dir, staleDir); err != nil {
		return err
	}
	return os.RemoveAll(staleDir)
}

func readOwnerToken(dir string) (string, error) {
	owner, err := readOwner(dir)
	if err != nil {
		return "", err
	}
	return owner["token"], nil
}

func readOwner(dir string) (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "owner"))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key == "" {
			continue
		}
		out[key] = value
	}
	return out, nil
}

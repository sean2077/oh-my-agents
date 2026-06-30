package atomicfile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrLockHeld means another process currently owns the lock directory.
var ErrLockHeld = errors.New("file lock held")

// ErrLockNotOwned means the caller tried to release a lock now owned by
// another token. This protects stale owners from deleting a reclaimed lock.
var ErrLockNotOwned = errors.New("file lock not owned")

const defaultLockLease = 15 * time.Minute

// reclaimTimeout bounds how long an abandoned `.reclaim` election claim may
// block the next reclaimer. It is much shorter than the lock lease: a reclaimer
// only holds the claim briefly to rewrite the owner file, so a `.reclaim` older
// than this is a crashed reclaimer and is cleared without waiting the full
// lease (COR-6).
const reclaimTimeout = 2 * time.Minute

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
		if !isLockContended(err) {
			return nil, err
		}
		if stale, _ := staleLock(dir); stale {
			if token, ok := takeOverStale(dir); ok {
				return &Lock{dir: dir, token: token}, nil
			}
			// Takeover aborted (the lock was re-acquired in the meantime, or
			// another reclaimer holds the election) — fall through to the
			// bounded wait and retry.
		}
		if timeout <= 0 || !time.Now().Before(deadline) {
			return nil, fmt.Errorf("%w: %s", ErrLockHeld, dir)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// isLockContended reports whether a Mkdir-acquire error means dir is currently
// contended — held, or racing a concurrent release — rather than a hard
// failure. ErrExist is the portable "already held" signal. On Windows directory
// removal is asynchronous: a lock being released lingers in a pending-delete
// state, and a concurrent Mkdir of the same path fails with ERROR_ACCESS_DENIED
// (surfaced as ErrPermission) instead of ErrExist. Treating that as contention
// lets AcquireLock wait out the transient window and retry rather than surface a
// spurious error — a flaky failure seen only on windows-latest under concurrent
// reclaim. The Windows gate keeps a genuine permission error on Unix (e.g. an
// unwritable parent) failing fast as before.
func isLockContended(err error) bool {
	if errors.Is(err, os.ErrExist) {
		return true
	}
	return runtime.GOOS == "windows" && errors.Is(err, os.ErrPermission)
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

// staleLock reports whether dir is an abandoned lock (lease expired, or — when
// the owner file is missing/corrupt — older than the lease by mtime).
func staleLock(dir string) (bool, error) {
	owner, err := readOwner(dir)
	if err == nil {
		if expires := owner["expires"]; expires != "" {
			t, perr := time.Parse(time.RFC3339Nano, expires)
			if perr == nil {
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

// takeOverStale reclaims a lock observed as stale by transferring ownership IN
// PLACE. It NEVER renames or removes dir, so the canonical lock path is never
// observably free and no concurrent Mkdir-acquire can create a second holder —
// the double-holder window that a rename-away/restore reclaim leaves open.
//
// Reclaimers are serialized by a sibling election lock (`dir+".reclaim"`,
// atomic Mkdir). The single winner re-confirms staleness UNDER the election —
// while it holds the claim no one else can take over, and dir still exists so
// no Mkdir-acquire can occur, so the owner cannot change out from under the
// check — meaning a lock re-acquired since it was observed stale is never
// stolen. It then overwrites the owner file with a fresh token (atomic
// temp+rename; dir untouched). Returns (token, true) only when this caller
// became the owner.
func takeOverStale(dir string) (string, bool) {
	claim := dir + ".reclaim"
	if err := os.Mkdir(claim, 0o700); err != nil {
		// Another reclaimer holds the election, or one crashed mid-reclaim and
		// left an abandoned claim — clear only a clearly-abandoned one (older
		// than the lease) and let the caller retry.
		if info, statErr := os.Stat(claim); statErr == nil && time.Since(info.ModTime()) > reclaimTimeout {
			_ = os.RemoveAll(claim)
		}
		return "", false
	}
	defer func() { _ = os.RemoveAll(claim) }()
	if stale, _ := staleLock(dir); !stale {
		return "", false // re-acquired since we observed it stale — do not steal
	}
	token, err := writeLockMetadata(dir) // atomic owner-file replace; dir is never removed
	if err != nil {
		return "", false
	}
	return token, true
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

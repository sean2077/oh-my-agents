package relay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The hook dedup cache and trail are machine-local diagnostic state under
// the ledger root's `_hookstate/` directory — never inside a pair dir
// (so they are never confused with artifacts) and never synced.

func (l *Ledger) hookStateDir() string { return filepath.Join(l.Root, "_hookstate") }

// fingerprintPath is the per-(session,pair) last-surfaced fingerprint file.
func (l *Ledger) fingerprintPath(pair string) string {
	name := fmt.Sprintf("%s-%s-%s.fp", l.Identity.Author, l.Identity.SessionKey, pair)
	return filepath.Join(l.hookStateDir(), name)
}

// hookFingerprintSeen reports whether fp equals the last fingerprint this
// session surfaced for the pair (dedup: stay silent if unchanged).
func (l *Ledger) hookFingerprintSeen(pair, fp string) bool {
	raw, err := os.ReadFile(l.fingerprintPath(pair))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(raw)) == fp
}

// hookFingerprintMark records fp as surfaced. Best-effort: a write failure
// only risks a duplicate surface, never a broken host.
func (l *Ledger) hookFingerprintMark(pair, fp string) {
	if err := os.MkdirAll(l.hookStateDir(), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(l.fingerprintPath(pair), []byte(fp+"\n"), 0o600)
}

// hookTrail appends one diagnostic line (best-effort; never blocks).
func (l *Ledger) hookTrail(event, msg string) {
	if err := os.MkdirAll(l.hookStateDir(), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(l.hookStateDir(), "trail.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "%s %s: %s\n", l.Now().UTC().Format("2006-01-02T15:04:05Z07:00"), event, msg)
}

// statReady stats the .ready sidecar of an artifact path.
func statReady(artifactPath string) (os.FileInfo, error) {
	return os.Stat(artifactPath + ".ready")
}

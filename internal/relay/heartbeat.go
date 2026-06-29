package relay

import (
	"os"
	"path/filepath"
	"time"
)

// defaultStaleAfter is the heartbeat staleness threshold (protocol §8) used when
// the config layer does not supply relay.stale_after.
const defaultStaleAfter = 15 * time.Minute

// staleAfter resolves the staleness window: the config-injected value
// (relay.stale_after, via Ledger.StaleAfter) when set, else the built-in
// default. The config layer (internal/config) owns duration parsing — "15m"
// and bare seconds both work there — so relay no longer re-parses env itself.
func (l *Ledger) staleAfter() time.Duration {
	if l.StaleAfter > 0 {
		return l.StaleAfter
	}
	return defaultStaleAfter
}

// touchHeartbeat refreshes this author's liveness marker; every relay
// subcommand calls it (heartbeat is an internal mechanism, protocol §8).
// Best-effort by design: a failed touch never blocks the command, and the
// dirent is intentionally NOT fsync'd. Heartbeat is fail-safe liveness state —
// a heartbeat lost to a crash reads as MISSING, and missing is not stale
// (heartbeatStale), so the next relay command simply recreates it. The durable
// ledger writes (sessions, bindings, artifacts, .seq reservations) already go
// through atomicfile's file+directory sync; heartbeat deliberately does not, to
// keep this hot path cheap (COR-5).
func (l *Ledger) touchHeartbeat(slug string) {
	dir := filepath.Join(l.PairDir(slug), ".heartbeat")
	path := filepath.Join(dir, ownerName(l.Identity.Author, l.Identity.SessionKey))
	now := l.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		_ = f.Close()
		_ = os.Chtimes(path, now, now)
	}
}

// heartbeatAge returns the age of an author-session heartbeat, or ok=false
// when none exists yet.
func (l *Ledger) heartbeatAge(slug, author, sessionKey string) (time.Duration, bool) {
	if sessionKey == "" {
		return 0, false
	}
	info, err := os.Stat(filepath.Join(l.PairDir(slug), ".heartbeat", ownerName(author, sessionKey)))
	if err != nil {
		return 0, false
	}
	return l.Now().Sub(info.ModTime()), true
}

// heartbeatStale reports whether an author-session heartbeat exists and is
// older than the staleness window. A missing heartbeat is NOT stale —
// staleness means "was alive, went silent after creating intent".
func (l *Ledger) heartbeatStale(slug, author, sessionKey string) bool {
	age, ok := l.heartbeatAge(slug, author, sessionKey)
	return ok && age > l.staleAfter()
}

func ownerName(author, sessionKey string) string {
	return author + "-" + sessionKey
}

func ownerToken(author, sessionKey string) string {
	return author + ":" + sessionKey
}

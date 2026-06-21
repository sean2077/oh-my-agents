package relay

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// defaultStaleAfter is the heartbeat staleness threshold (protocol §8);
// OMA_RELAY_STALE_AFTER (seconds) overrides.
const defaultStaleAfter = 15 * time.Minute

// staleAfter resolves the configured staleness window.
func (l *Ledger) staleAfter() time.Duration {
	if v := l.Getenv("OMA_RELAY_STALE_AFTER"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultStaleAfter
}

// touchHeartbeat refreshes this author's liveness marker; every relay
// subcommand calls it (heartbeat is an internal mechanism, protocol §8).
// Best-effort: a failed touch never blocks the command.
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

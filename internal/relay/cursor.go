package relay

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The per-reader consumption cursor records the highest peer-artifact seq a
// given (author, session) has consumed in a pair. It lives in a reader-private
// file under .cursor/ so wait/status decide "new peer artifact" by what THIS
// reader has actually moved past — not by the reader's own latest published
// seq. The latter silently skips a peer artifact published out of seq order
// (a lower seq committed AFTER the reader's own), which loses the message on
// the foreground wait, archived-delivery, and Stop-hook auto-continue paths.
// See newPeerArtifact.

// readCursor returns the reader's consumed-peer-seq for pairDir (0 if none).
func (l *Ledger) readCursor(pairDir string) int {
	raw, err := os.ReadFile(filepath.Join(pairDir, ".cursor", ownerName(l.Identity.Author, l.Identity.SessionKey)))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// writeCursor persists the reader's consumed-peer-seq. It is monotonic (never
// lowers an existing value) and best-effort: the cursor is an optimization
// over the always-on-disk artifacts, so a write failure must never fail the
// publish that triggered it.
func (l *Ledger) writeCursor(pairDir string, seq int) {
	if seq <= 0 || seq <= l.readCursor(pairDir) {
		return
	}
	dir := filepath.Join(pairDir, ".cursor")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	_ = writeFileAtomic(filepath.Join(dir, ownerName(l.Identity.Author, l.Identity.SessionKey)), []byte(strconv.Itoa(seq)+"\n"), 0o600)
}

// advanceCursorToLatestPeer moves the reader's cursor past every peer artifact
// currently ready. It is called under the pair lock when the reader takes its
// own turn (publishes) — the durable "I have read the peer up to here" signal
// that keeps wait idempotent (re-running wait re-delivers the same unconsumed
// artifact) without re-surfacing already-answered ones.
func (l *Ledger) advanceCursorToLatestPeer(pairDir string) {
	names, err := publishedArtifacts(pairDir, true)
	if err != nil {
		return
	}
	maxPeer := 0
	for _, name := range names {
		seq, author, _, ok := ParseArtifactName(name)
		if !ok || author == l.Identity.Author {
			continue
		}
		if seq > maxPeer {
			maxPeer = seq
		}
	}
	l.writeCursor(pairDir, maxPeer)
}

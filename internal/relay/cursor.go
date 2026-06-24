package relay

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The per-reader consumption cursor records the highest peer-artifact seq a
// given (author, session) has CONSUMED in a pair. It lives in a reader-private
// file under .cursor/ so wait/status decide "new peer artifact" by what THIS
// reader has actually moved past — not by the reader's own latest published
// seq, which silently skips a peer artifact published out of seq order (a
// lower seq committed AFTER the reader's own). See newPeerArtifact.
//
// Consumption is two-phase. An artifact is DELIVERED the moment wait/hook
// surfaces it (recorded in the ".seen" sidecar); it is only CONSUMED once the
// reader takes its own turn and publishes, at which point the cursor advances
// to the delivered mark. Advancing the cursor straight to the latest peer seq
// on publish would skip an out-of-order peer artifact the reader never saw;
// advancing on delivery instead would break wait idempotency (a crash between
// delivery and action must still re-deliver). Keying the two apart gives both.

// readSeqFile returns the non-negative int stored at path (0 if absent/invalid).
func readSeqFile(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (l *Ledger) cursorPath(pairDir, suffix string) string {
	return filepath.Join(pairDir, ".cursor", ownerName(l.Identity.Author, l.Identity.SessionKey)+suffix)
}

// writeSeqFile persists seq to a reader-private file under .cursor/. It is
// monotonic (never lowers an existing value) and best-effort: the marks are an
// optimization over the always-on-disk artifacts, so a write failure must
// never fail the publish/wait that triggered it.
func (l *Ledger) writeSeqFile(pairDir, suffix string, seq int) {
	if seq <= 0 || seq <= readSeqFile(l.cursorPath(pairDir, suffix)) {
		return
	}
	dir := filepath.Join(pairDir, ".cursor")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	_ = writeFileAtomic(l.cursorPath(pairDir, suffix), []byte(strconv.Itoa(seq)+"\n"), 0o600)
}

// readCursor returns the reader's consumed-peer-seq for pairDir (0 if none).
func (l *Ledger) readCursor(pairDir string) int { return readSeqFile(l.cursorPath(pairDir, "")) }

// readDelivered returns the highest peer-artifact seq surfaced to this reader
// by wait/hook (0 if none) — delivered but not necessarily consumed.
func (l *Ledger) readDelivered(pairDir string) int {
	return readSeqFile(l.cursorPath(pairDir, ".seen"))
}

// markDelivered records that the peer artifact at seq has been surfaced to this
// reader. Called when wait/hook returns an artifact; monotonic, best-effort.
func (l *Ledger) markDelivered(pairDir string, seq int) { l.writeSeqFile(pairDir, ".seen", seq) }

// advanceCursorToConsumed moves the reader's consumption cursor up to the
// highest peer artifact actually DELIVERED to this reader. It is called under
// the pair lock when the reader takes its own turn (publishes): publishing
// acknowledges the peer artifacts the reader has been shown, and ONLY those —
// never a lower-seq peer artifact published out of order that this reader
// never consumed (delivering those is the whole point of the cursor; see
// newPeerArtifact). writeSeqFile is monotonic, so this never lowers the cursor.
func (l *Ledger) advanceCursorToConsumed(pairDir string) {
	l.writeSeqFile(pairDir, "", l.readDelivered(pairDir))
}

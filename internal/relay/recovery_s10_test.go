package relay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// claudeLedger builds a claude ledger with an explicit session so a test can
// model two distinct same-author sessions (testLedger pins one per author).
func claudeLedger(t *testing.T, root string, ck *clock, session string) *Ledger {
	t.Helper()
	id, err := makeIdentity("claude", session)
	if err != nil {
		t.Fatal(err)
	}
	l := NewLedger(root, id)
	l.Now = ck.now
	l.PollInterval = time.Millisecond
	return l
}

func seqOf(t *testing.T, draftPath string) int {
	t.Helper()
	seq, _, _, ok := ParseArtifactName(filepath.Base(draftPath))
	if !ok {
		t.Fatalf("cannot parse seq from %q", draftPath)
	}
	return seq
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func globHit(t *testing.T, pattern string) bool {
	t.Helper()
	m, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %q: %v", pattern, err)
	}
	return len(m) > 0
}

func seqPath(pairDir string, seq int) string {
	return filepath.Join(pairDir, ".seq", fmt.Sprintf("%03d", seq))
}

// --- DRIFT-5: oma relay status refreshes the caller heartbeat (protocol §8) ---

func TestStatusTouchesCallerHeartbeat(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}

	// Age claude's heartbeat by jumping the clock an hour past its last touch.
	ck.advance(time.Hour)
	before, ok := claude.heartbeatAge(s.Pair, "claude", claude.Identity.SessionKey)
	if !ok {
		t.Fatal("claude heartbeat should exist after pair creation")
	}

	if _, err := claude.Status(s.Pair, 0); err != nil {
		t.Fatalf("status: %v", err)
	}

	after, ok := claude.heartbeatAge(s.Pair, "claude", claude.Identity.SessionKey)
	if !ok {
		t.Fatal("heartbeat must still exist after status")
	}
	// fail-before: drop the touchHeartbeat in Status and `after` stays ~before.
	if after >= before {
		t.Fatalf("Status must refresh the caller heartbeat: age before=%v after=%v", before, after)
	}
}

// --- DRIFT-5 guard: oma statusline stays pure-read (protocol §140) ---

func TestStatuslineStaysPureRead(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}

	hb := filepath.Join(claude.PairDir(s.Pair), ".heartbeat", ownerName("claude", claude.Identity.SessionKey))
	info, err := os.Stat(hb)
	if err != nil {
		t.Fatalf("stat heartbeat: %v", err)
	}
	mtimeBefore := info.ModTime()

	ck.advance(time.Hour)
	_ = claude.Statusline(s.Pair) // pure read: must NOT touch the heartbeat

	info, err = os.Stat(hb)
	if err != nil {
		t.Fatalf("stat heartbeat after statusline: %v", err)
	}
	if !info.ModTime().Equal(mtimeBefore) {
		t.Fatalf("Statusline must be pure-read: heartbeat mtime changed %v -> %v", mtimeBefore, info.ModTime())
	}
}

// --- COR-1/COR-3: pair join --rebind cleans the replaced session's orphans ---

func TestRebindCleansReplacedSessionReservations(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claudeA := claudeLedger(t, root, ck, "session-A")
	claudeB := claudeLedger(t, root, ck, "session-B")
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claudeA, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	pairDir := claudeA.PairDir(s.Pair)

	// (i) orphan reservation: a reserved draft, never published.
	orphan, err := claudeA.CreateDraft(s.Pair, "note", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	orphanSeq := seqOf(t, orphan)

	// (ii) post-publish residue: a fully published seq whose reservation marker
	// lingered (kill between .ready and cleanup). Publish cleans the .seq, so
	// re-create the leftover marker to model the residue the hasReady branch
	// must remove while preserving the ready formal.
	readyFormal := mustPublish(t, claudeA, s.Pair, "plan", "body", "next")
	readySeq := seqOf(t, readyFormal)
	writeRawReservation(t, claudeA, pairDir, readySeq)

	// (iii) interrupted publish: formal + .sha256 written, no .ready, draft and
	// reservation survive (kill at sha256-written).
	interrupted, err := claudeA.CreateDraft(s.Pair, "fix", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	interruptedSeq := seqOf(t, interrupted)
	claudeA.StepHook = func(step string) error {
		if step == "sha256-written" {
			return errors.New("kill before .ready")
		}
		return nil
	}
	if _, err := claudeA.Publish(interrupted, PublishInput{Body: "b", Prompt: "p"}, false); err == nil {
		t.Fatal("interrupted publish must fail")
	}
	claudeA.StepHook = nil

	// (peer) codex reserves a seq that must NEVER be touched (cross-author).
	codexDraft, err := codex.CreateDraft(s.Pair, "note", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	codexSeq := seqOf(t, codexDraft)

	// Pre-rebind sanity: every reservation exists.
	for _, seq := range []int{orphanSeq, readySeq, interruptedSeq, codexSeq} {
		if !exists(seqPath(pairDir, seq)) {
			t.Fatalf(".seq/%03d should exist before rebind", seq)
		}
	}

	// Rebind: claudeB reclaims claude's seat from claudeA.
	if _, err := claudeB.Rejoin(s.Pair, false); err != nil {
		t.Fatalf("rejoin: %v", err)
	}

	// (i) orphan: reservation + draft gone.
	if exists(seqPath(pairDir, orphanSeq)) {
		t.Errorf("orphan .seq/%03d must be removed", orphanSeq)
	}
	if globHit(t, filepath.Join(pairDir, ".draft", fmt.Sprintf("%03d-claude-*.md", orphanSeq))) {
		t.Errorf("orphan draft at %03d must be removed", orphanSeq)
	}
	// (ii) ready residue: reservation gone, the ready formal PRESERVED.
	if exists(seqPath(pairDir, readySeq)) {
		t.Errorf("post-publish residue .seq/%03d must be removed", readySeq)
	}
	if !globHit(t, filepath.Join(pairDir, fmt.Sprintf("%03d-claude-*.md", readySeq))) {
		t.Errorf("ready formal at %03d must be preserved", readySeq)
	}
	// (iii) interrupted: reservation + draft gone, incomplete formal QUARANTINED.
	if exists(seqPath(pairDir, interruptedSeq)) {
		t.Errorf("interrupted .seq/%03d must be removed", interruptedSeq)
	}
	if globHit(t, filepath.Join(pairDir, ".draft", fmt.Sprintf("%03d-claude-*.md", interruptedSeq))) {
		t.Errorf("interrupted draft at %03d must be removed", interruptedSeq)
	}
	if globHit(t, filepath.Join(pairDir, fmt.Sprintf("%03d-claude-*.md", interruptedSeq))) {
		t.Errorf("interrupted formal at %03d must be quarantined away (no live .md)", interruptedSeq)
	}
	if !globHit(t, filepath.Join(pairDir, fmt.Sprintf("%03d-claude-*.md.stale", interruptedSeq))) {
		t.Errorf("interrupted formal at %03d must be quarantined to *.stale", interruptedSeq)
	}
	// (peer) codex reservation preserved — cross-author safety.
	if !exists(seqPath(pairDir, codexSeq)) {
		t.Errorf("peer codex .seq/%03d must be preserved across rebind", codexSeq)
	}

	// The new session can still operate (reserve a fresh seq).
	if _, err := claudeB.CreateDraft(s.Pair, "note", nil, nil, false); err != nil {
		t.Fatalf("new session must work after rebind: %v", err)
	}
}

// writeRawReservation re-creates a bare .seq marker owned by l's identity, used
// to model a post-publish residue reservation that publish cleanup missed.
func writeRawReservation(t *testing.T, l *Ledger, pairDir string, seq int) {
	t.Helper()
	body := fmt.Sprintf("%s %s\n", ownerToken(l.Identity.Author, l.Identity.SessionKey), l.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(seqPath(pairDir, seq), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

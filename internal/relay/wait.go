package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Wait result codes (protocol §8); the CLI maps them to process exits.
const (
	WaitNewArtifact = 0  // new peer artifact published; path returned
	WaitTimeout     = 10 // nothing happened within the window
	WaitStaleIntent = 11 // peer created a publish intent then went silent
	WaitTerminal    = 12 // pair reached a terminal status
)

// WaitResult reports why Wait returned.
type WaitResult struct {
	Code         int    `json:"code"`
	ArtifactPath string `json:"artifact_path,omitempty"`
	Reason       string `json:"reason"`
}

// Wait blocks until the peer publishes a new artifact, the pair goes
// terminal, the peer's publish intent goes stale, or the timeout
// elapses. Check order is READY FIRST (protocol §8 rev A.2): a new
// ready artifact always wins — over terminal status (an unconsumed
// closing decision must still be delivered), over stale-intent
// (publish-then-kill leaves draft residue that is a cleanup warning,
// never exit 11), and over the deadline. Ready-priority survives
// close+archive (review 054 blocker 2): when the pair directory has
// been archived, the archived tree is checked for an undelivered peer
// artifact before reporting terminal.
func (l *Ledger) Wait(slug string, timeout time.Duration) (*WaitResult, error) {
	s, dir, archived, err := l.waitTarget(slug)
	if err != nil {
		return nil, err
	}
	peer, err := s.Peer(l.Identity.Author)
	if err != nil {
		return nil, err
	}
	if archived {
		return l.archivedResult(dir, peer)
	}
	interval := l.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	deadline := l.Now().Add(timeout)
	for {
		l.touchHeartbeat(s.Pair)

		if path, ok, err := l.newPeerArtifact(l.PairDir(s.Pair), peer); err != nil {
			return nil, err
		} else if ok {
			return &WaitResult{Code: WaitNewArtifact, ArtifactPath: path, Reason: "new artifact from " + peer}, nil
		}
		cur, err := l.LoadSession(s.Pair)
		if err != nil {
			// The pair directory disappears when the peer closes+archives
			// mid-wait: an undelivered artifact in the archive still wins.
			if errors.Is(err, ErrPairNotFound) {
				return l.archivedResult(filepath.Join(l.Root, "_archive", s.Pair), peer)
			}
			return nil, err
		}
		if cur.Terminal() {
			return &WaitResult{Code: WaitTerminal, Reason: "pair is " + cur.Status}, nil
		}
		if stale, seq := l.staleIntent(s.Pair, peer); stale {
			return &WaitResult{Code: WaitStaleIntent, Reason: fmt.Sprintf("%s reserved seq %03d then went silent (heartbeat stale)", peer, seq)}, nil
		}
		if !l.Now().Before(deadline) {
			return &WaitResult{Code: WaitTimeout, Reason: "timeout"}, nil
		}
		time.Sleep(interval)
	}
}

// waitTarget resolves the pair for Wait: explicit slug or binding first,
// each falling back to the archive so an archived pair's undelivered
// artifacts stay reachable; the active-only auto-adopt path runs last.
func (l *Ledger) waitTarget(slug string) (*Session, string, bool, error) {
	if slug == "" {
		if b, err := l.loadBinding(); err == nil {
			slug = b.Pair
		}
	}
	if slug != "" {
		if s, err := l.LoadSession(slug); err == nil {
			return s, l.PairDir(slug), false, nil
		}
		s, dir, archErr := l.loadArchivedSession(slug)
		if archErr == nil {
			return s, dir, true, nil
		}
		// A present-but-invalid archive entry surfaces its specific
		// fail-closed reason; only a genuinely absent one is "not found".
		if !errors.Is(archErr, os.ErrNotExist) {
			return nil, "", false, archErr
		}
		return nil, "", false, relayError(ErrPairNotFound, "pair %q not found (active or archived) under %s", slug, l.Root)
	}
	s, err := l.ResolvePair("", true)
	if err != nil {
		return nil, "", false, err
	}
	return s, l.PairDir(s.Pair), false, nil
}

// loadArchivedSession reads a pair archived under _archive/<slug> with
// the SAME consistency bar as the active path (review 056 blocker 1):
// the embedded pair name must match the directory, and an archive entry
// must be terminal — an active-status session inside _archive is corrupt
// or tampered, never trusted.
func (l *Ledger) loadArchivedSession(slug string) (*Session, string, error) {
	dir := filepath.Join(l.Root, "_archive", slug)
	raw, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w: no archived pair %q (%w)", ErrRelay, ErrPairNotFound, slug, os.ErrNotExist)
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, "", fmt.Errorf("%w: archived session.json of %s not valid JSON: %v", ErrRelay, slug, err)
	}
	if err := s.Validate(); err != nil {
		return nil, "", err
	}
	if s.Pair != slug {
		return nil, "", fmt.Errorf("%w: archived session.json pair %q does not match directory %q (corrupt or tampered archive)", ErrRelay, s.Pair, slug)
	}
	if !s.Terminal() {
		return nil, "", fmt.Errorf("%w: archived pair %s has non-terminal status %q (corrupt or tampered archive)", ErrRelay, slug, s.Status)
	}
	return &s, dir, nil
}

// archivedResult delivers an undelivered peer artifact from an archived
// pair directory, else reports terminal.
func (l *Ledger) archivedResult(dir, peer string) (*WaitResult, error) {
	if path, ok, err := l.newPeerArtifact(dir, peer); err == nil && ok {
		return &WaitResult{Code: WaitNewArtifact, ArtifactPath: path, Reason: "undelivered artifact from " + peer + " (pair archived)"}, nil
	}
	return &WaitResult{Code: WaitTerminal, Reason: "pair archived"}, nil
}

// newPeerArtifact returns the newest READY artifact authored by peer
// with seq greater than our own latest published seq, within pairDir.
func (l *Ledger) newPeerArtifact(pairDir, peer string) (string, bool, error) {
	names, err := publishedArtifacts(pairDir, true)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil // pair dir vanished: terminal handling follows
		}
		return "", false, err
	}
	myLatest, best := 0, ""
	bestSeq := 0
	for _, name := range names {
		seq, author, _, _ := ParseArtifactName(name)
		switch author {
		case l.Identity.Author:
			if seq > myLatest {
				myLatest = seq
			}
		case peer:
			if seq > bestSeq {
				bestSeq, best = seq, name
			}
		}
	}
	if best != "" && bestSeq > myLatest {
		return filepath.Join(pairDir, best), true, nil
	}
	return "", false, nil
}

// staleIntent reports a peer .seq reservation that has no matching ready
// formal artifact while the peer heartbeat is stale. A reservation WITH
// a matching ready artifact is post-publish residue: a cleanup warning
// surfaced by status/doctor, never a stale-intent signal.
func (l *Ledger) staleIntent(slug, peer string) (bool, int) {
	if !l.heartbeatStale(slug, peer) {
		return false, 0
	}
	for _, seq := range l.reservations(slug, peer) {
		if !l.hasReadyAt(slug, seq, peer) {
			return true, seq
		}
	}
	return false, 0
}

// reservations lists .seq reservation numbers owned by one author; the
// owner lives in the marker CONTENT (first token), the filename is the
// bare seq so O_EXCL is exclusive across authors.
func (l *Ledger) reservations(slug, author string) []int {
	dir := filepath.Join(l.PairDir(slug), ".seq")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var seqs []int
	for _, ent := range entries {
		n, err := strconv.Atoi(ent.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			continue
		}
		owner, _, _ := strings.Cut(strings.TrimSpace(string(raw)), " ")
		if owner == author {
			seqs = append(seqs, n)
		}
	}
	return seqs
}

// hasReadyAt reports a ready formal artifact at seq by author.
func (l *Ledger) hasReadyAt(slug string, seq int, author string) bool {
	return l.hasArtifactAt(slug, seq, author, true)
}

// hasFormalAt reports ANY formal artifact file at seq by author, ready
// or not — an unready formal plus a surviving draft is a resumable
// interrupted publish.
func (l *Ledger) hasFormalAt(slug string, seq int, author string) bool {
	return l.hasArtifactAt(slug, seq, author, false)
}

func (l *Ledger) hasArtifactAt(slug string, seq int, author string, readyOnly bool) bool {
	names, err := publishedArtifacts(l.PairDir(slug), readyOnly)
	if err != nil {
		return false
	}
	for _, name := range names {
		if s, a, _, _ := ParseArtifactName(name); s == seq && a == author {
			return true
		}
	}
	return false
}

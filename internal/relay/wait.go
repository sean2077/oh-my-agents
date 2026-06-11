package relay

import (
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
// never exit 11), and over the deadline.
func (l *Ledger) Wait(slug string, timeout time.Duration) (*WaitResult, error) {
	s, err := l.ResolvePair(slug, true)
	if err != nil {
		return nil, err
	}
	peer, err := s.Peer(l.Identity.Author)
	if err != nil {
		return nil, err
	}
	interval := l.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	deadline := l.Now().Add(timeout)
	for {
		l.touchHeartbeat(s.Pair)

		if path, ok, err := l.newPeerArtifact(s.Pair, peer); err != nil {
			return nil, err
		} else if ok {
			return &WaitResult{Code: WaitNewArtifact, ArtifactPath: path, Reason: "new artifact from " + peer}, nil
		}
		cur, err := l.LoadSession(s.Pair)
		if err != nil {
			// The pair directory disappears when the peer closes+archives:
			// that IS the terminal signal, not an error.
			if strings.Contains(err.Error(), "not found") {
				return &WaitResult{Code: WaitTerminal, Reason: "pair archived"}, nil
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

// newPeerArtifact returns the newest READY artifact authored by peer
// with seq greater than our own latest published seq.
func (l *Ledger) newPeerArtifact(slug, peer string) (string, bool, error) {
	pairDir := l.PairDir(slug)
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
	names, err := publishedArtifacts(l.PairDir(slug), true)
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

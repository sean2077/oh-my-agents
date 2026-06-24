package relay

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ArtifactSummary is one published artifact in a status report.
type ArtifactSummary struct {
	Seq    int    `json:"seq"`
	Author string `json:"author"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Name   string `json:"name"`
	Path   string `json:"path"` // absolute path — cold-orient reads this (review 068)
}

// HeartbeatInfo reports one participant's liveness.
type HeartbeatInfo struct {
	LastBeat   *time.Time `json:"last_heartbeat"`
	AgeSeconds int        `json:"age_seconds"`
	Stale      bool       `json:"stale"`
}

// SeatDriftInfo reports that this author's participant seat is held by a
// different platform session than the caller's — the orphaned-seat state a
// session resume under a new id leaves behind. The read-only status REPORTS it
// (rather than refusing) so the drift is diagnosable; `pair join <slug>
// --rebind` reclaims the seat.
type SeatDriftInfo struct {
	Author      string `json:"author"`
	HeldBy      string `json:"held_by"`
	ThisSession string `json:"this_session"`
	Hint        string `json:"hint"`
}

// PairStatus is the `oma relay status --json` document.
type PairStatus struct {
	Pair             string                   `json:"pair"`
	Session          *Session                 `json:"session"`
	Terminal         bool                     `json:"terminal"` // stable end-state signal; consumers treat session.status as opaque
	NextSeq          int                      `json:"next_seq"`
	Latest           *ArtifactSummary         `json:"latest,omitempty"`
	Artifacts        []ArtifactSummary        `json:"artifacts,omitempty"`
	Heartbeats       map[string]HeartbeatInfo `json:"heartbeats"`
	MyDrafts         []string                 `json:"my_drafts,omitempty"`
	PeerReservations []int                    `json:"peer_reservations,omitempty"` // from .seq only; peer .draft/ is never read
	Residue          []string                 `json:"residue,omitempty"`           // cleanup warnings for doctor
	LegacyV1         string                   `json:"legacy_v1,omitempty"`         // v1 tree present (archival; oma never touches it)
	SeatDrift        *SeatDriftInfo           `json:"seat_drift,omitempty"`        // this author's seat held by a different same-author session
}

// Status assembles the diagnostic view of one pair (read-only).
func (l *Ledger) Status(slug string, last int) (*PairStatus, error) {
	s, err := l.ResolvePair(slug, false)
	if err != nil {
		return nil, err
	}
	pairDir := l.PairDir(s.Pair)
	st := &PairStatus{Pair: s.Pair, Session: s, Terminal: s.Terminal(), Heartbeats: map[string]HeartbeatInfo{}}
	// A read-only diagnostic must be able to DIAGNOSE a drifted seat, not
	// refuse it: when this author's seat is held by a different same-author
	// session (a resumed window under a new id), report the drift instead of
	// failing closed, so the user sees the held-by id and the recovery command.
	if err := s.requireParticipantSession(l.Identity); err != nil {
		held := s.participantSession(l.Identity.Author)
		if held == "" || held == l.Identity.SessionKey {
			return nil, err // genuinely not a participant / unjoined — keep refusing
		}
		st.SeatDrift = &SeatDriftInfo{
			Author:      l.Identity.Author,
			HeldBy:      held,
			ThisSession: l.Identity.SessionKey,
			Hint:        fmt.Sprintf("reclaim with `oma relay pair join %s --rebind` if that session is gone", s.Pair),
		}
	}

	next, err := l.nextSeq(s.Pair)
	if err != nil {
		return nil, err
	}
	st.NextSeq = next

	names, err := publishedArtifacts(pairDir, false)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		seq, author, kind, _ := ParseArtifactName(name)
		fm, _, readErr := ReadArtifact(filepath.Join(pairDir, name))
		if readErr != nil {
			st.Residue = append(st.Residue, fmt.Sprintf("%s: %v", name, readErr))
			continue
		}
		st.Artifacts = append(st.Artifacts, ArtifactSummary{Seq: seq, Author: author, Kind: kind, Status: fm.Status, Name: name, Path: filepath.Join(pairDir, name)})
	}
	if n := len(st.Artifacts); n > 0 {
		latest := st.Artifacts[n-1]
		st.Latest = &latest
		if last > 0 && n > last {
			st.Artifacts = st.Artifacts[n-last:]
		}
	}

	for _, p := range s.Participants {
		info := HeartbeatInfo{}
		sessionKey := s.participantSession(p)
		if age, ok := l.heartbeatAge(s.Pair, p, sessionKey); ok {
			t := l.Now().Add(-age)
			info.LastBeat = &t
			info.AgeSeconds = int(age.Seconds())
			info.Stale = age > l.staleAfter()
		}
		st.Heartbeats[p] = info
	}

	if drafts, err := os.ReadDir(filepath.Join(pairDir, ".draft")); err == nil {
		for _, ent := range drafts {
			if seq, author, _, ok := ParseArtifactName(ent.Name()); ok && author == l.Identity.Author {
				st.MyDrafts = append(st.MyDrafts, ent.Name())
				if l.hasReadyAt(s.Pair, seq, author) {
					st.Residue = append(st.Residue, fmt.Sprintf("draft %s has a published counterpart (post-publish leftover; doctor cleans)", ent.Name()))
				}
			}
		}
	}
	peer, _ := s.Peer(l.Identity.Author)
	peerSession := s.participantSession(peer)
	for _, seq := range l.reservations(s.Pair, peer, peerSession) {
		st.PeerReservations = append(st.PeerReservations, seq)
		if l.hasReadyAt(s.Pair, seq, peer) {
			st.Residue = append(st.Residue, fmt.Sprintf(".seq/%03d (reserved by %s) has a published counterpart (post-publish leftover; doctor cleans)", seq, peer))
		}
	}
	for _, seq := range l.reservations(s.Pair, l.Identity.Author, l.Identity.SessionKey) {
		if l.hasReadyAt(s.Pair, seq, l.Identity.Author) {
			st.Residue = append(st.Residue, fmt.Sprintf(".seq/%03d (reserved by %s) has a published counterpart (post-publish leftover; doctor cleans)", seq, l.Identity.Author))
		}
	}

	// Legacy v1 report (protocol §1): archival only, oma never touches it.
	if top, err := gitTopFromRoot(l.Root); err == nil {
		shared := filepath.Join(top, ".shared")
		if _, err := os.Stat(filepath.Join(shared, "_relay")); err == nil {
			st.LegacyV1 = shared
		}
	}
	return st, nil
}

// gitTopFromRoot recovers <top> from <top>/.oma/relay.
func gitTopFromRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if filepath.Base(abs) == "relay" && filepath.Base(filepath.Dir(abs)) == ".oma" {
		return filepath.Dir(filepath.Dir(abs)), nil
	}
	return "", fmt.Errorf("non-default root")
}

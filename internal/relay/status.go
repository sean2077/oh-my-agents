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
}

// HeartbeatInfo reports one participant's liveness.
type HeartbeatInfo struct {
	LastBeat   *time.Time `json:"last_heartbeat"`
	AgeSeconds int        `json:"age_seconds"`
	Stale      bool       `json:"stale"`
}

// PairStatus is the `oma relay status --json` document.
type PairStatus struct {
	Pair             string                   `json:"pair"`
	Session          *Session                 `json:"session"`
	NextSeq          int                      `json:"next_seq"`
	Latest           *ArtifactSummary         `json:"latest,omitempty"`
	Artifacts        []ArtifactSummary        `json:"artifacts,omitempty"`
	Heartbeats       map[string]HeartbeatInfo `json:"heartbeats"`
	MyDrafts         []string                 `json:"my_drafts,omitempty"`
	PeerReservations []int                    `json:"peer_reservations,omitempty"` // from .seq only; peer .draft/ is never read
	Residue          []string                 `json:"residue,omitempty"`           // cleanup warnings for doctor
	LegacyV1         string                   `json:"legacy_v1,omitempty"`         // v1 tree present (archival; oma never touches it)
}

// Status assembles the diagnostic view of one pair (read-only).
func (l *Ledger) Status(slug string, last int) (*PairStatus, error) {
	s, err := l.ResolvePair(slug, false)
	if err != nil {
		return nil, err
	}
	pairDir := l.PairDir(s.Pair)
	st := &PairStatus{Pair: s.Pair, Session: s, Heartbeats: map[string]HeartbeatInfo{}}

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
		st.Artifacts = append(st.Artifacts, ArtifactSummary{Seq: seq, Author: author, Kind: kind, Status: fm.Status, Name: name})
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
		if age, ok := l.heartbeatAge(s.Pair, p); ok {
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
	for _, seq := range l.reservations(s.Pair, peer) {
		st.PeerReservations = append(st.PeerReservations, seq)
		if l.hasReadyAt(s.Pair, seq, peer) {
			st.Residue = append(st.Residue, fmt.Sprintf(".seq/%03d.%s has a published counterpart (post-publish leftover; doctor cleans)", seq, peer))
		}
	}
	for _, seq := range l.reservations(s.Pair, l.Identity.Author) {
		if l.hasReadyAt(s.Pair, seq, l.Identity.Author) {
			st.Residue = append(st.Residue, fmt.Sprintf(".seq/%03d.%s has a published counterpart (post-publish leftover; doctor cleans)", seq, l.Identity.Author))
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

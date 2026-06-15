package relay

import (
	"fmt"
	"time"
)

// StatuslineSchema versions the --json shape.
const StatuslineSchema = "oma-relay-statusline/1"

// StatuslineState is the compact "which pair / whose turn" snapshot
// (docs/relay-v2-protocol.md §12.2). It is binding-scoped and pure-read:
// an unbound window resolves to Bound=false and shows nothing about any
// lone active pair (review 086 must-fix 4). It never exposes private
// draft contents.
type StatuslineState struct {
	Schema       string `json:"schema"`
	Bound        bool   `json:"bound"`
	Pair         string `json:"pair,omitempty"`
	Status       string `json:"status,omitempty"` // active | closed | cancelled | failed
	Turn         string `json:"turn,omitempty"`   // you | <peer> | idle | done
	LatestSeq    int    `json:"latest_seq,omitempty"`
	LatestKind   string `json:"latest_kind,omitempty"`
	LatestAuthor string `json:"latest_author,omitempty"`
	PeerStale    bool   `json:"peer_stale,omitempty"`
	Text         string `json:"text"`
}

// statuslineDeadline bounds one state computation so a stalled mount or
// filesystem cannot wedge the host's statusLine (which calls this often).
const statuslineDeadline = 2 * time.Second

// Statusline computes the snapshot for the bound pair, binding-scoped and
// pure-read, within a hard deadline. explicit overrides the binding.
// Resolution order is explicit > binding ONLY — never the single-active
// auto-adopt, so an unbound window stays empty.
func (l *Ledger) Statusline(explicit string) *StatuslineState {
	type result struct{ st *StatuslineState }
	ch := make(chan result, 1)
	go func() { ch <- result{l.statuslineState(explicit)} }()
	select {
	case r := <-ch:
		return r.st
	case <-time.After(statuslineDeadline):
		return &StatuslineState{Schema: StatuslineSchema, Text: "relay: …(slow)"}
	}
}

// statuslineState does the actual pure-read computation.
func (l *Ledger) statuslineState(explicit string) *StatuslineState {
	st := &StatuslineState{Schema: StatuslineSchema}

	slug := explicit
	if slug == "" {
		b, err := l.loadBinding()
		if err != nil {
			st.Text = "relay: no pair" // unbound window: show nothing
			return st
		}
		slug = b.Pair
	}
	s, err := l.LoadSession(slug)
	pairDir := l.PairDir(slug)
	if err != nil {
		// A bound pair that was closed is now under _archive: show it as
		// done rather than unreadable.
		if as, dir, aerr := l.loadArchivedSession(slug); aerr == nil {
			s, pairDir = as, dir
		} else {
			st.Text = "relay: " + slug + " (unreadable)"
			return st
		}
	}
	st.Bound = true
	st.Pair = slug
	st.Status = s.Status

	peer, _ := s.Peer(l.Identity.Author)

	// Latest published artifact (pure read; peer .draft/ is never read).
	if names, nerr := publishedArtifacts(pairDir, true); nerr == nil && len(names) > 0 {
		latest := names[len(names)-1]
		seq, author, kind, _ := ParseArtifactName(latest)
		st.LatestSeq, st.LatestAuthor, st.LatestKind = seq, author, kind
	}

	switch {
	case s.Terminal():
		st.Turn = "done"
	case st.LatestAuthor == "":
		st.Turn = "you" // freshly bootstrapped: write the first artifact
	case st.LatestAuthor == l.Identity.Author:
		st.Turn = peer // waiting on the peer
		st.PeerStale = peer != "" && l.heartbeatStale(slug, peer)
	default:
		st.Turn = "you" // peer addressed me
	}

	st.Text = renderStatusline(st)
	return st
}

// renderStatusline builds the compact one-line text.
func renderStatusline(st *StatuslineState) string {
	var turn string
	switch st.Turn {
	case "you":
		turn = "your turn"
	case "done":
		turn = "done"
	case "idle", "":
		turn = "idle"
	default:
		turn = "awaiting " + st.Turn
		if st.PeerStale {
			turn += " (stale)"
		}
	}
	line := fmt.Sprintf("relay %s · %s", st.Pair, turn)
	if st.LatestSeq > 0 {
		line += fmt.Sprintf(" · %03d %s %s", st.LatestSeq, st.LatestKind, st.LatestAuthor)
	}
	return line
}

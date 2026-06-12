package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sean2077/oh-my-agents/internal/hookcfg"
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

// --- install / uninstall / doctor (Claude Code statusLine, §12.2) ---

// statuslineCommand is the relay-owned statusLine command; statuslineMarker
// stamps ownership so a foreign statusLine is never mistaken for ours.
const (
	statuslineCommand = "oma relay statusline"
	statuslineMarker  = "_oma_relay"
	statuslineMarkVal = "statusline"
)

// statuslineValue is the canonical relay statusLine object.
var statuslineValue = json.RawMessage(fmt.Sprintf(
	`{"type": "command", "command": %q, %q: %q}`,
	statuslineCommand, statuslineMarker, statuslineMarkVal))

// StatuslineDoctorState reports the wiring of a host statusLine without
// mutating anything.
type StatuslineDoctorState string

const (
	StatuslineAbsent   StatuslineDoctorState = "absent"   // no statusLine configured
	StatuslineOwned    StatuslineDoctorState = "owned"    // relay-owned and current
	StatuslineForeign  StatuslineDoctorState = "foreign"  // a non-relay statusLine occupies the slot
	StatuslineMismatch StatuslineDoctorState = "mismatch" // relay-owned but the command drifted
)

// ErrStatuslineSlotTaken marks a foreign statusLine refused without --force.
var ErrStatuslineSlotTaken = errors.New("a non-relay statusLine is already configured (use --force to replace)")

// DoctorStatusline classifies the statusLine slot in a host settings file
// (pure read, no mutation).
func DoctorStatusline(settingsPath string) (StatuslineDoctorState, error) {
	raw, found, err := hookcfg.GetTopLevel(settingsPath, "statusLine")
	if err != nil {
		return "", err
	}
	if !found {
		return StatuslineAbsent, nil
	}
	owned, cmd := statuslineOwnership(raw)
	switch {
	case !owned:
		return StatuslineForeign, nil
	case cmd == statuslineCommand:
		return StatuslineOwned, nil
	default:
		return StatuslineMismatch, nil
	}
}

// InstallStatusline wires the relay statusLine into a Claude settings.json.
// A foreign slot is refused unless force; an owned/mismatched/absent slot is
// set to the canonical value (idempotent).
func InstallStatusline(settingsPath string, force bool) error {
	state, err := DoctorStatusline(settingsPath)
	if err != nil {
		return err
	}
	if state == StatuslineForeign && !force {
		return ErrStatuslineSlotTaken
	}
	return hookcfg.SetTopLevel(settingsPath, "statusLine", statuslineValue)
}

// UninstallStatusline removes only a relay-owned statusLine; a foreign one is
// left intact (reported via the returned state).
func UninstallStatusline(settingsPath string) (StatuslineDoctorState, error) {
	state, err := DoctorStatusline(settingsPath)
	if err != nil {
		return "", err
	}
	switch state {
	case StatuslineOwned, StatuslineMismatch:
		if err := hookcfg.DeleteTopLevel(settingsPath, "statusLine"); err != nil {
			return "", err
		}
	}
	return state, nil
}

// statuslineOwnership reports whether a statusLine value carries the relay
// marker, and its command string.
func statuslineOwnership(raw json.RawMessage) (owned bool, command string) {
	var probe struct {
		Marker  *string `json:"_oma_relay"`
		Command string  `json:"command"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false, ""
	}
	return probe.Marker != nil && *probe.Marker == statuslineMarkVal, probe.Command
}

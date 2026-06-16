package relay

import (
	"fmt"
	"path/filepath"
	"strings"
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
	LatestStatus string `json:"latest_status,omitempty"`
	Peer         string `json:"peer,omitempty"`
	State        string `json:"state,omitempty"` // your_move|waiting|peer_stale|paused|decision|fresh|closed|cancelled|failed|neutral
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
	st.Peer = peer

	// Latest published artifact (pure read; peer .draft/ is never read). The
	// latest artifact's status (e.g. timed_out → an @user pause) needs its
	// frontmatter, so the latest one is parsed; failure degrades to no status.
	if names, nerr := publishedArtifacts(pairDir, true); nerr == nil && len(names) > 0 {
		latest := names[len(names)-1]
		seq, author, kind, _ := ParseArtifactName(latest)
		st.LatestSeq, st.LatestAuthor, st.LatestKind = seq, author, kind
		if fm, _, rerr := ReadArtifact(filepath.Join(pairDir, latest)); rerr == nil {
			st.LatestStatus = fm.Status
		}
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

	st.State = statuslineLabel(st, s.Status, s.Terminal())
	st.Text = renderStatusline(st, false)
	return st
}

// statuslineLabel maps the snapshot to an agent-ledger-style lifecycle state
// (the glyph/format driver). Terminal session status wins; then a timed_out
// latest artifact (an @user pause); then a decision; then fresh / waiting /
// peer_stale / your_move. Turn is kept untouched for backward compatibility.
func statuslineLabel(st *StatuslineState, sessStatus string, terminal bool) string {
	switch {
	case terminal:
		return sessStatus // closed | cancelled | failed
	case st.LatestStatus == "timed_out":
		return "paused"
	case st.LatestKind == "decision":
		return "decision"
	case st.LatestAuthor == "":
		return "fresh"
	case st.Turn != "you" && st.Turn != "done": // Turn holds the peer name → waiting on peer
		if st.PeerStale {
			return "peer_stale"
		}
		return "waiting"
	default:
		return "your_move" // peer addressed me
	}
}

// Statusline presets (A5): rendering verbosity. focused (default) is the
// agent-ledger-style glyph line embedded in StatuslineState.Text; minimal and
// full are opt-in via the CLI --preset flag. All are pure formatting over the
// already-computed state — they read nothing extra.
const (
	PresetMinimal = "minimal"
	PresetFocused = "focused"
	PresetFull    = "full"
)

// slStyle maps a lifecycle state to its glyph + SGR color (the agent-ledger
// relay look). Glyphs are single-width BMP symbols (no emoji), so a host
// footer's column math stays correct.
var slStyle = map[string][2]string{
	"your_move":  {"▸", "1;32"}, // bold green — peer published; act
	"waiting":    {"○", "2"},    // dim — you published; waiting
	"peer_stale": {"!", "1;31"}, // bold red — peer heartbeat stale
	"paused":     {"‖", "1;35"}, // bold magenta — timed_out / @user pause
	"decision":   {"◆", "1;36"}, // bold cyan — a decision break
	"closed":     {"✓", "2"},    // dim
	"cancelled":  {"✗", "2"},    // dim
	"failed":     {"✗", "1;31"}, // bold red
	"fresh":      {"·", "2"},    // dim — no artifacts yet
	"neutral":    {"○", "2"},    // dim
}

// slLoud states render the whole body in color (high signal), not just glyph.
var slLoud = map[string]bool{"your_move": true, "paused": true, "peer_stale": true, "failed": true}

func slPaint(text, code string, color bool) string {
	if color && code != "" {
		return "\x1b[" + code + "m" + text + "\x1b[0m"
	}
	return text
}

// statuslineBody builds the uncolored body for a state at the given preset.
func statuslineBody(st *StatuslineState, preset string) string {
	pair, peer, state := st.Pair, st.Peer, st.State
	if peer == "" {
		peer = "?"
	}
	tail := ""
	if st.LatestSeq > 0 {
		tail = strings.TrimRight(fmt.Sprintf(" · #%d %s", st.LatestSeq, st.LatestKind), " ")
	}
	if preset == PresetMinimal {
		return strings.TrimRight(pair+"  "+minimalLabel(state), " ")
	}
	var body string
	switch state {
	case "your_move":
		body = fmt.Sprintf("%s ← %s  YOUR MOVE%s", pair, peer, tail)
	case "waiting":
		body = fmt.Sprintf("%s → %s  waiting%s", pair, peer, tail)
	case "peer_stale":
		body = fmt.Sprintf("%s → %s  peer stale?%s", pair, peer, tail)
	case "paused":
		body = fmt.Sprintf("%s  @user%s", pair, tail)
	case "decision":
		body = pair + "  decision"
		if st.LatestSeq > 0 {
			body += fmt.Sprintf(" · #%d", st.LatestSeq)
		}
	case "closed", "cancelled", "failed":
		body = pair + "  " + state
	case "fresh":
		if peer != "?" {
			body = fmt.Sprintf("%s → %s  new pair", pair, peer)
		} else {
			body = pair + "  new pair"
		}
	default: // neutral
		body = pair
		if st.LatestSeq > 0 {
			body += fmt.Sprintf("  #%d %s %s", st.LatestSeq, st.LatestAuthor, st.LatestKind)
		}
	}
	if preset == PresetFull && st.Status != "" {
		body += " [" + st.Status + "]"
	}
	return body
}

func minimalLabel(state string) string {
	switch state {
	case "your_move":
		return "YOUR MOVE"
	case "peer_stale":
		return "peer stale?"
	case "paused":
		return "@user"
	case "fresh":
		return "new pair"
	case "waiting", "decision", "closed", "cancelled", "failed":
		return state
	default:
		return "idle"
	}
}

// renderStatusline renders the focused agent-ledger-style line (used for the
// plain Text field; color=false).
func renderStatusline(st *StatuslineState, color bool) string {
	return renderPresetLine(st, PresetFocused, color)
}

func renderPresetLine(st *StatuslineState, preset string, color bool) string {
	if !st.Bound || st.State == "" {
		return st.Text
	}
	style, ok := slStyle[st.State]
	if !ok {
		style = slStyle["neutral"]
	}
	glyph := slPaint(style[0], style[1], color)
	body := statuslineBody(st, preset)
	if slLoud[st.State] {
		body = slPaint(body, style[1], color)
	}
	return strings.TrimRight(glyph+" "+body, " ")
}

// RenderPreset renders a computed state at the requested verbosity, optionally
// with ANSI color (the CLI footer); the plain Text field stays color-free so
// the SessionStart hook hint never embeds escape codes.
func RenderPreset(st *StatuslineState, preset string, color bool) string {
	if !st.Bound {
		return st.Text
	}
	return renderPresetLine(st, preset, color)
}

package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sean2077/oh-my-agents/internal/schemaver"
)

// Binding pins one author-session to one pair (protocol §4a).
type Binding struct {
	Schema      string    `json:"schema"`
	Author      string    `json:"author"`
	SessionHash string    `json:"session_hash"`
	Pair        string    `json:"pair"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
}

func (l *Ledger) bindingPath() string {
	return filepath.Join(l.Root, "_bindings", l.Identity.Author+"-"+l.Identity.SessionKey+".json")
}

func (l *Ledger) writeBinding(pair string) error {
	now := l.Now().UTC()
	b := Binding{Schema: BindingSchema, Author: l.Identity.Author, SessionHash: l.Identity.SessionKey, Pair: pair, Created: now, Updated: now}
	if prev, err := l.loadBinding(); err == nil {
		b.Created = prev.Created
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(l.bindingPath(), append(raw, '\n'), 0o600)
}

func (l *Ledger) loadBinding() (*Binding, error) {
	raw, err := os.ReadFile(l.bindingPath())
	if err != nil {
		return nil, err
	}
	var b Binding
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("%w: binding not valid JSON: %v", ErrRelay, err)
	}
	if major, ok := schemaver.Major(b.Schema, "oma-relay-binding"); !ok || major != 1 {
		return nil, fmt.Errorf("%w: binding schema %q, want %s", ErrRelay, b.Schema, BindingSchema)
	}
	return &b, nil
}

// ResolvePair applies the §4a order: explicit --pair beats the binding
// file beats single-active auto-adopt (which persists a binding); any
// remaining ambiguity is a zero-write refusal listing candidates.
// autoBind=false (read-only commands under --dry-run discipline) skips
// persisting the auto-adopted binding but resolves identically.
func (l *Ledger) ResolvePair(explicit string, autoBind bool) (*Session, error) {
	if explicit != "" {
		return l.LoadSession(explicit)
	}
	if b, err := l.loadBinding(); err == nil {
		s, loadErr := l.LoadSession(b.Pair)
		if loadErr != nil {
			return nil, fmt.Errorf("%w: binding points at %q which is missing or invalid (rebind with `oma relay pair join <slug>`): %v", ErrRelay, b.Pair, loadErr)
		}
		if s.Terminal() {
			return nil, fmt.Errorf("%w: binding points at %s which is %s (rebind with `oma relay pair join <slug>`)", ErrRelay, b.Pair, s.Status)
		}
		if err := s.requireParticipantSession(l.Identity); err != nil {
			return nil, err
		}
		return s, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// Auto-adopt iterates ALL pairs and fails closed on a corrupt
	// candidate (same rule as workflow --id resolution, review 060
	// blocker 2): silently skipping one could bind the wrong pair.
	// Listing (pair list) stays lenient — it REPORTS corrupt rows.
	all, err := l.AllPairs()
	if err != nil {
		return nil, err
	}
	var mine, drifted []string
	for _, slug := range all {
		s, err := l.LoadSession(slug)
		if err != nil {
			return nil, fmt.Errorf("%w: cannot auto-bind: pair %s is unreadable: %v", ErrRelay, slug, err)
		}
		if s.Terminal() {
			continue
		}
		if s.Status != StatusActive {
			continue
		}
		if _, perr := s.Peer(l.Identity.Author); perr != nil {
			continue
		}
		if existing := s.ParticipantSessions[l.Identity.Author]; existing != "" && existing != l.Identity.SessionKey {
			drifted = append(drifted, slug) // seat held by another same-author session
			continue
		}
		mine = append(mine, slug)
	}
	switch len(mine) {
	case 1:
		if autoBind {
			var claimed *Session
			err := l.withPairLock(mine[0], func() error {
				var err error
				claimed, err = l.LoadSession(mine[0])
				if err != nil {
					return err
				}
				if err := claimed.mutationError(); err != nil {
					return err
				}
				changed, err := claimed.claimParticipant(l.Identity)
				if err != nil {
					return err
				}
				if changed {
					if err := l.saveSession(claimed); err != nil {
						return err
					}
				}
				if err := l.writeBinding(mine[0]); err != nil {
					return err
				}
				l.touchHeartbeat(mine[0])
				return nil
			})
			return claimed, err
		}
		s, err := l.LoadSession(mine[0])
		if err != nil {
			return nil, err
		}
		return s, nil
	case 0:
		if len(drifted) > 0 {
			return nil, fmt.Errorf("%w: no active pair for %s with this session, but %d hold your seat under another session — reclaim with `oma relay pair join <slug> --rebind`: %s",
				ErrRelay, l.Identity.Author, len(drifted), strings.Join(drifted, ", "))
		}
		return nil, fmt.Errorf("%w: no active pair for %s (create one with `oma relay pair new <topic>`)", ErrRelay, l.Identity.Author)
	default:
		return nil, fmt.Errorf("%w: %d active pairs for %s, cannot disambiguate — pass --pair <slug> or `oma relay pair join <slug>`; candidates: %s",
			ErrRelay, len(mine), l.Identity.Author, strings.Join(mine, ", "))
	}
}

// Join binds this author-session to an existing active pair.
func (l *Ledger) Join(slug string, dryRun bool) (*Session, error) {
	if dryRun {
		s, err := l.LoadSession(slug)
		if err != nil {
			return nil, err
		}
		if err := s.mutationError(); err != nil {
			return nil, err
		}
		if _, err := s.Peer(l.Identity.Author); err != nil {
			return nil, err
		}
		if existing := s.ParticipantSessions[l.Identity.Author]; existing != "" && existing != l.Identity.SessionKey {
			return nil, fmt.Errorf("%w: participant %s of %s is claimed by another session", ErrRelay, l.Identity.Author, slug)
		}
		return s, nil
	}
	var s *Session
	err := l.withPairLock(slug, func() error {
		var err error
		s, err = l.LoadSession(slug)
		if err != nil {
			return err
		}
		if err := s.mutationError(); err != nil {
			return err
		}
		if _, err := s.Peer(l.Identity.Author); err != nil {
			return err
		}
		changed, err := s.claimParticipant(l.Identity)
		if err != nil {
			return err
		}
		if changed {
			if err := l.saveSession(s); err != nil {
				return err
			}
		}
		if err := l.writeBinding(slug); err != nil {
			return err
		}
		l.touchHeartbeat(slug)
		return nil
	})
	return s, err
}

// Rejoin reclaims this author's seat in an active pair when it is held by a
// DIFFERENT same-author session — the orphaned-seat state a platform session
// resume under a new id leaves behind. It is the explicit operator override
// behind `oma relay pair join <slug> --rebind`: unlike Join (which refuses a
// non-matching seat, fail-closed), Rejoin reassigns it. The seat is a single
// field, not a destructive overwrite, so no backup is taken; the explicit flag
// is the safety boundary. Liveness is intentionally NOT gated on the prior
// heartbeat: a just-resumed session and a live concurrent one both look recent,
// so the heartbeat cannot distinguish them — the operator asserts the prior
// session is gone by passing --rebind.
func (l *Ledger) Rejoin(slug string, dryRun bool) (*Session, error) {
	if dryRun {
		s, err := l.LoadSession(slug)
		if err != nil {
			return nil, err
		}
		if err := s.mutationError(); err != nil {
			return nil, err
		}
		if _, err := s.Peer(l.Identity.Author); err != nil {
			return nil, err
		}
		return s, nil
	}
	var s *Session
	err := l.withPairLock(slug, func() error {
		var err error
		s, err = l.LoadSession(slug)
		if err != nil {
			return err
		}
		if err := s.mutationError(); err != nil {
			return err
		}
		if _, err := s.Peer(l.Identity.Author); err != nil {
			return err
		}
		if s.ParticipantSessions == nil {
			s.ParticipantSessions = map[string]string{}
		}
		oldSession := s.ParticipantSessions[l.Identity.Author]
		s.ParticipantSessions[l.Identity.Author] = l.Identity.SessionKey
		if err := l.saveSession(s); err != nil {
			return err
		}
		if err := l.writeBinding(slug); err != nil {
			return err
		}
		// COR-1/COR-3: the replaced session's sequence reservations (and their
		// paired drafts / incomplete formals) become invisible to the normal
		// session-scoped doctor/status scans once the seat moves, so clean them
		// here under the same pair lock. Only (this author, oldSession) markers
		// are touched — never the peer's, never the new session's.
		if oldSession != "" && oldSession != l.Identity.SessionKey {
			if err := l.cleanReplacedReservations(slug, l.Identity.Author, oldSession); err != nil {
				return err
			}
			// COR-6: the consumption cursor (and its .seen delivered-mark) is
			// named by sessionKey too, so it must follow the seat — otherwise the
			// reclaiming session reads cursor 0 and wait re-delivers the peer's
			// entire already-consumed history. Best-effort, monotonic merge.
			l.migrateCursorFrom(l.PairDir(slug), oldSession)
		}
		l.touchHeartbeat(slug)
		return nil
	})
	return s, err
}

// cleanReplacedReservations removes the sequence reservations a now-replaced
// author-session left behind in slug, plus their paired drafts. After `pair
// join --rebind` reassigns the author seat (Rejoin), these orphans are
// invisible to the session-scoped doctor/status scans, and the operator has
// asserted the old session is gone — so they are cleaned here under the pair
// lock. Formal-aware, mirroring CleanStale: a seq with a ready same-author
// formal (post-publish residue) or no formal at all is removed outright; a
// formal WITHOUT .ready is an interrupted publish from the dead session that
// nothing can still resume, so its incomplete formal is quarantined (renamed
// *.stale, like doctor) BEFORE the draft and reservation are removed — never
// leaving a hidden unready formal. Only (author, oldSession) markers match, so
// the peer's and the new session's reservations are never touched.
func (l *Ledger) cleanReplacedReservations(slug, author, oldSession string) error {
	pairDir := l.PairDir(slug)
	for _, seq := range l.reservations(slug, author, oldSession) {
		if l.hasFormalAt(slug, seq, author) && !l.hasReadyAt(slug, seq, author) {
			formals, _ := filepath.Glob(filepath.Join(pairDir, fmt.Sprintf("%03d-%s-*.md", seq, author)))
			for _, f := range formals {
				if err := os.Rename(f, f+".stale"); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				_ = os.Remove(f + ".sha256")
			}
		}
		seqPath := filepath.Join(pairDir, ".seq", fmt.Sprintf("%03d", seq))
		if err := os.Remove(seqPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		drafts, _ := filepath.Glob(filepath.Join(pairDir, ".draft", fmt.Sprintf("%03d-%s-*.md", seq, author)))
		for _, d := range drafts {
			if err := os.Remove(d); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	if major, ok := schemaMajor(b.Schema, "oma-relay-binding"); !ok || major != 1 {
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
	var mine []string
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
		if _, perr := s.Peer(l.Identity.Author); perr == nil {
			mine = append(mine, slug)
		}
	}
	switch len(mine) {
	case 1:
		s, err := l.LoadSession(mine[0])
		if err != nil {
			return nil, err
		}
		if autoBind {
			if err := l.writeBinding(mine[0]); err != nil {
				return nil, err
			}
		}
		return s, nil
	case 0:
		return nil, fmt.Errorf("%w: no active pair for %s (create one with `oma relay pair new <topic>`)", ErrRelay, l.Identity.Author)
	default:
		return nil, fmt.Errorf("%w: %d active pairs for %s, cannot disambiguate — pass --pair <slug> or `oma relay pair join <slug>`; candidates: %s",
			ErrRelay, len(mine), l.Identity.Author, strings.Join(mine, ", "))
	}
}

// Join binds this author-session to an existing active pair.
func (l *Ledger) Join(slug string, dryRun bool) (*Session, error) {
	s, err := l.LoadSession(slug)
	if err != nil {
		return nil, err
	}
	if s.Terminal() {
		return nil, fmt.Errorf("%w: pair %s is %s", ErrRelay, slug, s.Status)
	}
	if err := s.mutationError(); err != nil {
		return nil, err
	}
	if _, err := s.Peer(l.Identity.Author); err != nil {
		return nil, err
	}
	if dryRun {
		return s, nil
	}
	if err := l.writeBinding(slug); err != nil {
		return nil, err
	}
	l.touchHeartbeat(slug)
	return s, nil
}

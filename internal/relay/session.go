package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Session is the pair metadata document (docs/reference/schemas.md §4).
type Session struct {
	Schema       string            `json:"schema"`
	Pair         string            `json:"pair"`
	Project      string            `json:"project"`
	Participants []string          `json:"participants"`
	Roles        map[string]string `json:"roles"`
	Status       string            `json:"status"`
	Created      time.Time         `json:"created"`
	Closed       *time.Time        `json:"closed"`
	Outcome      *string           `json:"outcome"`
	Reason       *string           `json:"reason"`
}

var (
	pairSlugRe = regexp.MustCompile(`^\d{8}-[a-z0-9][a-z0-9-]{0,47}$`)
	topicRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
)

// knownRoles are the role keys oma validates; unknown keys are tolerated
// on read (minor-additive policy) but never written by this build.
var knownRoles = map[string]bool{"lead": true, "planner": true, "implementer": true, "reviewer": true}

// Terminal reports whether the session reached an end state.
func (s *Session) Terminal() bool {
	return s.Status == "closed" || s.Status == "cancelled" || s.Status == "failed"
}

// Validate enforces the schema contract including the roles.lead
// semantics (review 046 guidance): lead is required, names exactly one
// participant; every known role value must be a participant; exactly two
// distinct participants.
func (s *Session) Validate() error {
	if major, ok := schemaMajor(s.Schema, "oma-relay"); !ok || major != 2 {
		return fmt.Errorf("%w: session schema %q, want %s", ErrRelay, s.Schema, Schema)
	}
	if !pairSlugRe.MatchString(s.Pair) {
		return fmt.Errorf("%w: pair slug %q (want YYYYMMDD-<topic>)", ErrRelay, s.Pair)
	}
	if len(s.Participants) != 2 {
		return fmt.Errorf("%w: participants must be exactly 2, got %v", ErrRelay, s.Participants)
	}
	if s.Participants[0] == s.Participants[1] {
		return fmt.Errorf("%w: participants must be distinct (%s)", ErrRelay, s.Participants[0])
	}
	isParticipant := map[string]bool{}
	for _, p := range s.Participants {
		if !authorRe.MatchString(p) {
			return fmt.Errorf("%w: participant %q invalid", ErrRelay, p)
		}
		isParticipant[p] = true
	}
	lead, ok := s.Roles["lead"]
	if !ok || lead == "" {
		return fmt.Errorf("%w: roles.lead is required (primary decision-maker, docs/reference/workflows.md §4.1)", ErrRelay)
	}
	for role, holder := range s.Roles {
		if !knownRoles[role] {
			continue // minor-additive tolerance
		}
		if !isParticipant[holder] {
			return fmt.Errorf("%w: role %s names %q which is not a participant", ErrRelay, role, holder)
		}
	}
	switch s.Status {
	case "active", "closed", "cancelled", "failed":
	default:
		return fmt.Errorf("%w: session status %q", ErrRelay, s.Status)
	}
	return nil
}

// Peer returns the other participant for the given author.
func (s *Session) Peer(author string) (string, error) {
	switch author {
	case s.Participants[0]:
		return s.Participants[1], nil
	case s.Participants[1]:
		return s.Participants[0], nil
	default:
		return "", fmt.Errorf("%w: %s is not a participant of %s (%v)", ErrRelay, author, s.Pair, s.Participants)
	}
}

// PairDir is the directory of one pair.
func (l *Ledger) PairDir(slug string) string { return filepath.Join(l.Root, slug) }

// LoadSession reads and validates one pair's session.json.
func (l *Ledger) LoadSession(slug string) (*Session, error) {
	raw, err := os.ReadFile(filepath.Join(l.PairDir(slug), "session.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: pair %q not found under %s", ErrRelay, slug, l.Root)
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%w: session.json of %s not valid JSON: %v", ErrRelay, slug, err)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Pair != slug {
		return nil, fmt.Errorf("%w: session.json pair %q does not match directory %q", ErrRelay, s.Pair, slug)
	}
	return &s, nil
}

// saveSession writes session.json atomically.
func (l *Ledger) saveSession(s *Session) error {
	if err := s.Validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(l.PairDir(s.Pair), "session.json"), append(raw, '\n'), 0o600)
}

// ActivePairs lists non-terminal pairs (sorted).
func (l *Ledger) ActivePairs() ([]string, error) {
	all, err := l.AllPairs()
	if err != nil {
		return nil, err
	}
	var active []string
	for _, slug := range all {
		s, err := l.LoadSession(slug)
		if err != nil {
			continue // corrupt pairs surface via status/doctor, not listing
		}
		if !s.Terminal() {
			active = append(active, slug)
		}
	}
	return active, nil
}

// AllPairs lists every pair directory (active and terminal, not archived).
func (l *Ledger) AllPairs() ([]string, error) {
	entries, err := os.ReadDir(l.Root)
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, ent := range entries {
		if ent.IsDir() && pairSlugRe.MatchString(ent.Name()) {
			slugs = append(slugs, ent.Name())
		}
	}
	sort.Strings(slugs)
	return slugs, nil
}

// NewPair creates a pair: slug = YYYYMMDD-<topic>, creator = lead by
// default (protocol §4), peer defaulting to the claude↔codex counterpart.
func (l *Ledger) NewPair(topic, peer, project string, dryRun bool) (*Session, error) {
	if !topicRe.MatchString(topic) {
		return nil, fmt.Errorf("%w: topic %q (want lowercase slug ≤48 chars)", ErrRelay, topic)
	}
	author := l.Identity.Author
	if peer == "" {
		switch author {
		case "claude":
			peer = "codex"
		case "codex":
			peer = "claude"
		default:
			return nil, fmt.Errorf("%w: cannot infer peer for author %q; pass --peer", ErrRelay, author)
		}
	}
	slug := l.Now().UTC().Format("20060102") + "-" + topic
	s := &Session{
		Schema:       Schema,
		Pair:         slug,
		Project:      project,
		Participants: []string{author, peer},
		Roles:        map[string]string{"lead": author, "planner": author, "implementer": author, "reviewer": peer},
		Status:       "active",
		Created:      l.Now().UTC(),
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(l.PairDir(slug)); err == nil {
		return nil, fmt.Errorf("%w: pair %s already exists", ErrRelay, slug)
	}
	if dryRun {
		return s, nil
	}
	for _, sub := range []string{".draft", ".seq", ".heartbeat"} {
		if err := os.MkdirAll(filepath.Join(l.PairDir(slug), sub), 0o700); err != nil {
			return nil, err
		}
	}
	if err := l.saveSession(s); err != nil {
		return nil, err
	}
	if err := l.writeBinding(slug); err != nil {
		return nil, err
	}
	l.touchHeartbeat(slug)
	return s, nil
}

// SetLead persists a user-confirmed lead swap (docs/reference/workflows.md §4.1: the
// swap is recorded as kind: decision AND session.json.roles.lead is
// updated so later turns resolve the new authority).
func (l *Ledger) SetLead(slug, name string, dryRun bool) (*Session, error) {
	s, err := l.LoadSession(slug)
	if err != nil {
		return nil, err
	}
	if s.Terminal() {
		return nil, fmt.Errorf("%w: pair %s is %s", ErrRelay, slug, s.Status)
	}
	if _, err := s.Peer(name); err != nil {
		return nil, fmt.Errorf("%w: lead %q is not a participant of %s (%v)", ErrRelay, name, slug, s.Participants)
	}
	s.Roles["lead"] = name
	if dryRun {
		return s, nil
	}
	if err := l.saveSession(s); err != nil {
		return nil, err
	}
	l.touchHeartbeat(slug)
	return s, nil
}

// Close ends a pair and archives it (protocol §9).
func (l *Ledger) Close(slug, outcome, reason string, dryRun bool) error {
	switch outcome {
	case "approve", "reject", "abandon":
	default:
		return fmt.Errorf("%w: outcome %q not in approve|reject|abandon", ErrRelay, outcome)
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: --reason is required", ErrRelay)
	}
	s, err := l.LoadSession(slug)
	if err != nil {
		return err
	}
	if s.Terminal() {
		return fmt.Errorf("%w: pair %s is already %s", ErrRelay, slug, s.Status)
	}
	// A2 quality gate: `approve` is fail-closed unless the lead's latest
	// kind:decision carries a valid completion receipt over a non-lead
	// approve review (the referenced plan + review must still hash-match).
	// reject and abandon need no receipt. Runs under --dry-run too so it
	// previews the refusal.
	if outcome == "approve" {
		if err := l.verifyApproveClose(slug); err != nil {
			return err
		}
	}
	if dryRun {
		return nil
	}
	now := l.Now().UTC()
	s.Status = "closed"
	s.Closed = &now
	s.Outcome = &outcome
	s.Reason = &reason
	if err := l.saveSession(s); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(l.PairDir(slug), "CLOSED"), []byte(now.Format(time.RFC3339)+"\n"), 0o600); err != nil {
		return err
	}
	archive := filepath.Join(l.Root, "_archive")
	if err := os.MkdirAll(archive, 0o700); err != nil {
		return err
	}
	dest := filepath.Join(archive, slug)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%w: archive already holds %s; resolve manually", ErrRelay, slug)
	}
	return os.Rename(l.PairDir(slug), dest)
}

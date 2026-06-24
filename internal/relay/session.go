package relay

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sean2077/oh-my-agents/internal/jsonmerge"
)

const (
	StatusActive    = "active"
	StatusClosing   = "closing"
	StatusClosed    = "closed"
	StatusCancelled = "cancelled"
	StatusFailed    = "failed"
)

// Session is the pair metadata document (docs/reference/schemas.md §4).
type Session struct {
	Schema       string   `json:"schema"`
	Pair         string   `json:"pair"`
	Project      string   `json:"project"`
	Participants []string `json:"participants"`
	// ParticipantSessions claims each author slot for one concrete platform
	// session hash. A peer may be unclaimed until `pair join`, but a claimed
	// author cannot be reused by another same-author session.
	ParticipantSessions map[string]string `json:"participant_sessions"`
	Roles               map[string]string `json:"roles"`
	Status              string            `json:"status"`
	// WorktreeRoot/Branch/BaseCommit bind the pair to the creator's checkout
	// (minor-additive, optional). They let close refuse an approval driven
	// from a different worktree than the one the work was done in.
	WorktreeRoot string     `json:"worktree_root,omitempty"`
	Branch       string     `json:"branch,omitempty"`
	BaseCommit   string     `json:"base_commit,omitempty"`
	Created      time.Time  `json:"created"`
	Closed       *time.Time `json:"closed"`
	Outcome      *string    `json:"outcome"`
	Reason       *string    `json:"reason"`

	// extra preserves unknown top-level fields across load/save (schemas.md
	// minor-additive contract). Unexported, so it is never serialized itself.
	extra map[string]json.RawMessage
}

// checkWorktreeBinding refuses a mutation when the pair is bound to one
// worktree and the caller is in another. It is a no-op when the binding or the
// caller's git context is absent, or when the caller passed --allow-worktree-change.
func (s *Session) checkWorktreeBinding(git GitContext) error {
	if git.AllowWorktreeChange || s.WorktreeRoot == "" || git.WorktreeRoot == "" {
		return nil
	}
	if s.WorktreeRoot != git.WorktreeRoot {
		return fmt.Errorf("%w: pair %s is bound to worktree %s but you are in %s (pass --allow-worktree-change to act from here)", ErrRelay, s.Pair, s.WorktreeRoot, git.WorktreeRoot)
	}
	return nil
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
	return s.Status == StatusClosed || s.Status == StatusCancelled || s.Status == StatusFailed
}

func (s *Session) mutationError() error {
	switch {
	case s.Status == StatusClosing:
		return relayError(ErrPairClosing, "pair %s is closing", s.Pair)
	case s.Terminal():
		return fmt.Errorf("%w: pair %s is %s", ErrRelay, s.Pair, s.Status)
	case s.Status != StatusActive:
		return fmt.Errorf("%w: pair %s has unsupported status %q", ErrRelay, s.Pair, s.Status)
	default:
		return nil
	}
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
	if s.ParticipantSessions == nil {
		return fmt.Errorf("%w: participant_sessions is required", ErrRelay)
	}
	for author, sessionKey := range s.ParticipantSessions {
		if !isParticipant[author] {
			return fmt.Errorf("%w: participant_sessions names non-participant %q", ErrRelay, author)
		}
		if !sessionKeyRe.MatchString(sessionKey) {
			return fmt.Errorf("%w: participant_sessions[%s] %q invalid", ErrRelay, author, sessionKey)
		}
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
	case StatusActive, StatusClosing, StatusClosed, StatusCancelled, StatusFailed:
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

func (s *Session) participantSession(author string) string {
	if s.ParticipantSessions == nil {
		return ""
	}
	return s.ParticipantSessions[author]
}

func (s *Session) claimParticipant(id Identity) (bool, error) {
	if _, err := s.Peer(id.Author); err != nil {
		return false, err
	}
	if s.ParticipantSessions == nil {
		s.ParticipantSessions = map[string]string{}
	}
	if existing := s.ParticipantSessions[id.Author]; existing != "" {
		if existing != id.SessionKey {
			return false, fmt.Errorf("%w: participant %s seat in %s is held by session %s, not this one — reclaim it with `oma relay pair join %s --rebind` if that session is gone", ErrRelay, id.Author, s.Pair, existing, s.Pair)
		}
		return false, nil
	}
	s.ParticipantSessions[id.Author] = id.SessionKey
	return true, nil
}

func (s *Session) requireParticipantSession(id Identity) error {
	if _, err := s.Peer(id.Author); err != nil {
		return err
	}
	if existing := s.ParticipantSessions[id.Author]; existing != id.SessionKey {
		if existing == "" {
			return fmt.Errorf("%w: participant %s has not joined %s with this session; run `oma relay pair join %s`", ErrRelay, id.Author, s.Pair, s.Pair)
		}
		return fmt.Errorf("%w: participant %s seat in %s is held by session %s, not this one — if that session is gone (e.g. a resumed window under a new id), reclaim it with `oma relay pair join %s --rebind`", ErrRelay, id.Author, s.Pair, existing, s.Pair)
	}
	return nil
}

// PairDir is the directory of one pair.
func (l *Ledger) PairDir(slug string) string { return filepath.Join(l.Root, slug) }

// LoadSession reads and validates one pair's session.json.
func (l *Ledger) LoadSession(slug string) (*Session, error) {
	raw, err := os.ReadFile(filepath.Join(l.PairDir(slug), "session.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, relayError(ErrPairNotFound, "pair %q not found under %s", slug, l.Root)
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
	if s.extra, err = jsonmerge.Extra(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// saveSession writes session.json atomically.
func (l *Ledger) saveSession(s *Session) error {
	if err := s.Validate(); err != nil {
		return err
	}
	raw, err := jsonmerge.Marshal(s, s.extra)
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
		if s.Status == StatusActive {
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
	now := l.Now().UTC()
	slug := pairSlug(now, topic, "")
	s := &Session{
		Schema:       Schema,
		Pair:         slug,
		Project:      project,
		Participants: []string{author, peer},
		ParticipantSessions: map[string]string{
			author: l.Identity.SessionKey,
		},
		Roles:        map[string]string{"lead": author, "planner": author, "implementer": author, "reviewer": peer},
		Status:       StatusActive,
		WorktreeRoot: l.GitContext.WorktreeRoot,
		Branch:       l.GitContext.Branch,
		BaseCommit:   l.GitContext.HeadCommit,
		Created:      now,
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if dryRun {
		return s, nil
	}
	if err := os.MkdirAll(l.Root, 0o700); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			suffix, err := randomPairSuffix()
			if err != nil {
				return nil, err
			}
			s.Pair = pairSlug(now, topic, suffix)
			if err := s.Validate(); err != nil {
				return nil, err
			}
		}
		pairDir := l.PairDir(s.Pair)
		if err := os.Mkdir(pairDir, 0o700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return nil, err
		}
		if err := l.initPairDir(s); err != nil {
			_ = os.RemoveAll(pairDir)
			return nil, err
		}
		return s, nil
	}
	return nil, relayError(ErrConflict, "could not create a unique pair id for topic %q", topic)
}

func (l *Ledger) initPairDir(s *Session) error {
	for _, sub := range []string{".draft", ".seq", ".heartbeat", ".cursor"} {
		if err := os.Mkdir(filepath.Join(l.PairDir(s.Pair), sub), 0o700); err != nil {
			return err
		}
	}
	if err := l.saveSession(s); err != nil {
		return err
	}
	if err := l.writeBinding(s.Pair); err != nil {
		return err
	}
	l.touchHeartbeat(s.Pair)
	return nil
}

func pairSlug(now time.Time, topic, suffix string) string {
	label := topic
	if suffix != "" {
		maxTopic := 48 - 1 - len(suffix)
		if len(label) > maxTopic {
			label = strings.TrimRight(label[:maxTopic], "-")
		}
		label += "-" + suffix
	}
	return now.Format("20060102") + "-" + label
}

func randomPairSuffix() (string, error) {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// SetLead persists a user-confirmed lead swap (docs/reference/workflows.md §4.1: the
// swap is recorded as kind: decision AND session.json.roles.lead is
// updated so later turns resolve the new authority).
func (l *Ledger) SetLead(slug, name string, dryRun bool) (*Session, error) {
	if dryRun {
		s, err := l.LoadSession(slug)
		if err != nil {
			return nil, err
		}
		if err := s.mutationError(); err != nil {
			return nil, err
		}
		if err := s.requireParticipantSession(l.Identity); err != nil {
			return nil, err
		}
		if _, err := s.Peer(name); err != nil {
			return nil, fmt.Errorf("%w: lead %q is not a participant of %s (%v)", ErrRelay, name, slug, s.Participants)
		}
		s.Roles["lead"] = name
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
		if err := s.requireParticipantSession(l.Identity); err != nil {
			return err
		}
		if _, err := s.Peer(name); err != nil {
			return fmt.Errorf("%w: lead %q is not a participant of %s (%v)", ErrRelay, name, slug, s.Participants)
		}
		s.Roles["lead"] = name
		if err := l.saveSession(s); err != nil {
			return err
		}
		l.touchHeartbeat(slug)
		return nil
	})
	return s, err
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
	if dryRun {
		s, err := l.LoadSession(slug)
		if err != nil {
			if errors.Is(err, ErrPairNotFound) {
				return l.closeArchivedResult(slug, outcome, reason)
			}
			return err
		}
		switch s.Status {
		case StatusActive, StatusClosing:
			if err := s.requireParticipantSession(l.Identity); err != nil {
				return err
			}
			if err := s.checkWorktreeBinding(l.GitContext); err != nil {
				return err
			}
			if outcome == "approve" {
				return l.verifyApproveClose(slug)
			}
			return nil
		case StatusClosed:
			return closeParamsMatch(s, outcome, reason)
		default:
			if err := s.mutationError(); err != nil {
				return err
			}
			return fmt.Errorf("%w: pair %s has unsupported close state %q", ErrRelay, s.Pair, s.Status)
		}
	}
	return l.withPairLock(slug, func() error {
		archive := filepath.Join(l.Root, "_archive")
		dest := filepath.Join(archive, slug)
		s, err := l.LoadSession(slug)
		if err != nil {
			if errors.Is(err, ErrPairNotFound) {
				return l.closeArchivedResult(slug, outcome, reason)
			}
			return err
		}
		switch s.Status {
		case StatusActive, StatusClosing, StatusClosed:
			if err := s.requireParticipantSession(l.Identity); err != nil {
				return err
			}
			if err := s.checkWorktreeBinding(l.GitContext); err != nil {
				return err
			}
		default:
			if err := s.mutationError(); err != nil {
				return err
			}
			return fmt.Errorf("%w: pair %s has unsupported close state %q", ErrRelay, s.Pair, s.Status)
		}
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("%w: archive already holds %s; resolve manually", ErrRelay, slug)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(archive, 0o700); err != nil {
			return err
		}

		wasActive := s.Status == StatusActive
		closedPath := filepath.Join(l.PairDir(slug), "CLOSED")
		if s.Status == StatusActive {
			s.Status = StatusClosing
			if err := l.saveSession(s); err != nil {
				return err
			}
		}

		// A2 quality gate: `approve` is fail-closed unless the lead's
		// latest kind:decision carries a valid completion receipt. This
		// re-check runs while holding the pair mutation lock.
		if s.Status != StatusClosed && outcome == "approve" {
			if err := l.verifyApproveClose(slug); err != nil {
				if wasActive {
					s.Status = StatusActive
					s.Closed, s.Outcome, s.Reason = nil, nil, nil
					// Surface a failed rollback rather than swallowing it: if this
					// save fails the pair is left `closing` (which refuses
					// mutations). It is still recoverable — re-run close once the
					// gate passes, or `close --outcome abandon` — so name that path
					// instead of leaving an opaque frozen pair.
					if rbErr := l.saveSession(s); rbErr != nil {
						return fmt.Errorf("%w: approve gate failed (%v); rolling back to active also failed: %v — pair %s is left %q; re-run close after the gate passes, or `oma relay close --outcome abandon --reason …`", ErrRelay, err, rbErr, slug, StatusClosing)
					}
				}
				return err
			}
		}

		if s.Status == StatusClosed {
			if err := closeParamsMatch(s, outcome, reason); err != nil {
				return err
			}
		} else {
			now := l.Now().UTC()
			s.Status = StatusClosed
			s.Closed = &now
			s.Outcome = &outcome
			s.Reason = &reason
			if err := l.saveSession(s); err != nil {
				return err
			}
		}
		closedAt := l.Now().UTC()
		if s.Closed != nil {
			closedAt = *s.Closed
		}
		if err := writeFileAtomic(closedPath, []byte(closedAt.Format(time.RFC3339)+"\n"), 0o600); err != nil {
			return err
		}
		if err := os.Rename(l.PairDir(slug), dest); err != nil {
			return err
		}
		return nil
	})
}

func (l *Ledger) closeArchivedResult(slug, outcome, reason string) error {
	s, _, err := l.loadArchivedSession(slug)
	if err != nil {
		return err
	}
	if s.Status != StatusClosed {
		return fmt.Errorf("%w: archived pair %s is %s, not closed", ErrRelay, slug, s.Status)
	}
	if err := s.requireParticipantSession(l.Identity); err != nil {
		return err
	}
	return closeParamsMatch(s, outcome, reason)
}

func closeParamsMatch(s *Session, outcome, reason string) error {
	if s.Outcome == nil || *s.Outcome != outcome {
		got := "<nil>"
		if s.Outcome != nil {
			got = *s.Outcome
		}
		return fmt.Errorf("%w: pair %s already closed with outcome %s, not %s", ErrRelay, s.Pair, got, outcome)
	}
	if s.Reason == nil || *s.Reason != reason {
		return fmt.Errorf("%w: pair %s already closed with a different reason", ErrRelay, s.Pair)
	}
	return nil
}

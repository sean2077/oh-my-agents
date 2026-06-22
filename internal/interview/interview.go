// Package interview implements the solidified surface of the Socratic
// clarification workflow (docs/reference/workflows.md §1): deterministic scoring
// math, threshold gating and state persistence live HERE; question
// generation, dimension assessment and ontology extraction stay with the
// agent, which feeds results in via `oma interview score --input`.
package interview

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sean2077/oh-my-agents/internal/atomicfile"
	"github.com/sean2077/oh-my-agents/internal/jsonmerge"
	"github.com/sean2077/oh-my-agents/internal/session"
)

// Schema constants (docs/reference/schemas.md §5).
const (
	Schema       = "oma-interview/1"
	ScoresSchema = "oma-interview-scores/1"
)

// Interview phases (docs/reference/workflows.md §1.1).
const (
	PhaseTopologyPending = "topology_pending"
	PhaseInterviewing    = "interviewing"
	PhaseGatePassed      = "gate_passed"
	PhaseGateWaived      = "gate_waived"
	PhaseCrystallized    = "crystallized"
	PhaseCompleted       = "completed"
	PhaseAborted         = "aborted"
)

// ErrInterview marks fail-closed interview-state refusals.
var ErrInterview = errors.New("interview refused (fail-closed)")

// Component is one locked top-level outcome (Round 0 topology gate).
type Component struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	Status        string             `json:"status"` // active | deferred
	Evidence      []string           `json:"evidence"`
	ClarityScores map[string]float64 `json:"clarity_scores"`
}

// Deferral records a user-confirmed component deferral.
type Deferral struct {
	ComponentID string `json:"component_id"`
	Reason      string `json:"reason"`
}

// Topology is the locked component shape.
type Topology struct {
	Status                  string      `json:"status"` // pending | confirmed
	Components              []Component `json:"components"`
	Deferrals               []Deferral  `json:"deferrals"`
	LastTargetedComponentID string      `json:"last_targeted_component_id"`
}

// Round is one Q&A round with its scores.
type Round struct {
	Round     int                           `json:"round"`
	Component string                        `json:"component"`
	Dimension string                        `json:"dimension"`
	Question  string                        `json:"question"`
	Answer    string                        `json:"answer"`
	Scores    map[string]map[string]float64 `json:"scores"`
	Ambiguity float64                       `json:"ambiguity"`
}

// Entity is one ontology entity reported by the agent.
type Entity struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Fields        []string `json:"fields"`
	Relationships []string `json:"relationships"`
}

// OntologySnapshot tracks entity convergence across rounds.
type OntologySnapshot struct {
	Round             int      `json:"round"`
	Entities          []Entity `json:"entities"`
	StabilityRatio    *float64 `json:"stability_ratio"` // nil on the first snapshot (N/A)
	MatchingReasoning string   `json:"matching_reasoning,omitempty"`
}

// State is the oma-interview/1 document (docs/reference/workflows.md §1.2).
type State struct {
	Schema             string             `json:"schema"`
	ID                 string             `json:"id"`
	Revision           int64              `json:"revision"`
	Phase              string             `json:"phase"`
	Type               string             `json:"type"` // greenfield | brownfield
	Threshold          float64            `json:"threshold"`
	ThresholdSource    string             `json:"threshold_source"`
	InitialIdea        string             `json:"initial_idea"`
	Topology           Topology           `json:"topology"`
	Rounds             []Round            `json:"rounds"`
	OntologySnapshots  []OntologySnapshot `json:"ontology_snapshots"`
	ChallengeModesUsed []string           `json:"challenge_modes_used"`
	CurrentAmbiguity   float64            `json:"current_ambiguity"`
	GateWaiver         string             `json:"gate_waiver,omitempty"` // early-exit warning record
	SpecPath           string             `json:"spec_path,omitempty"`
	Created            time.Time          `json:"created"`
	Updated            time.Time          `json:"updated"`

	// extra preserves unknown top-level fields across load/save (schemas.md
	// minor-additive contract). Unexported, so it is never serialized itself.
	extra map[string]json.RawMessage
}

// Terminal reports an end state.
func (s *State) Terminal() bool { return s.Phase == PhaseCompleted || s.Phase == PhaseAborted }

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Engine anchors interview state under dir (.oma/state).
type Engine struct {
	Dir           string
	Now           func() time.Time
	SessionSuffix string
}

// NewEngine builds an Engine for the given state directory.
func NewEngine(dir string) *Engine { return &Engine{Dir: dir, Now: time.Now} }

func (e *Engine) path(id string) string { return filepath.Join(e.Dir, "interview-"+id+".json") }

func (e *Engine) scopedID(id string) (string, error) {
	if e.SessionSuffix == "" {
		return strings.TrimSpace(id), nil
	}
	return session.ScopeName(strings.TrimSpace(id), e.SessionSuffix)
}

func (e *Engine) matchesSession(id string) bool {
	return e.SessionSuffix == "" || session.MatchesScope(id, e.SessionSuffix)
}

// Start initializes a new interview (phase topology_pending). An existing
// id is refused unless resume=true, which loads and returns it untouched.
func (e *Engine) Start(id, typ string, threshold float64, source, idea string, resume, dryRun bool) (*State, error) {
	id = strings.TrimSpace(id)
	if id == "" && e.SessionSuffix == "" {
		id = e.Now().UTC().Format("20060102-150405")
	}
	var err error
	id, err = e.scopedID(id)
	if err != nil {
		return nil, err
	}
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("%w: id %q (want %s)", ErrInterview, id, idRe)
	}
	if typ != "greenfield" && typ != "brownfield" {
		return nil, fmt.Errorf("%w: type %q not in greenfield|brownfield", ErrInterview, typ)
	}
	if threshold < 0 || threshold > 1 {
		return nil, fmt.Errorf("%w: threshold %.3f outside [0,1]", ErrInterview, threshold)
	}
	if _, err := os.Stat(e.path(id)); err == nil {
		if resume {
			return e.Load(id)
		}
		return nil, fmt.Errorf("%w: interview %q already exists (use --resume to inspect/continue)", ErrInterview, id)
	}
	now := e.Now().UTC()
	s := &State{
		Schema: Schema, ID: id, Phase: PhaseTopologyPending, Type: typ,
		Threshold: threshold, ThresholdSource: source, InitialIdea: idea,
		Topology:         Topology{Status: "pending", Components: []Component{}, Deferrals: []Deferral{}},
		Rounds:           []Round{},
		CurrentAmbiguity: 1.0,
		Created:          now, Updated: now,
	}
	if dryRun {
		return s, nil
	}
	err = e.withInstanceLock(id, func() error {
		if _, statErr := os.Stat(e.path(id)); statErr == nil {
			if resume {
				s, statErr = e.Load(id)
				return statErr
			}
			return fmt.Errorf("%w: interview %q already exists (use --resume to inspect/continue)", ErrInterview, id)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		return e.save(s)
	})
	return s, err
}

// StatePath is the absolute state file for one id (dry-run reporting).
func (e *Engine) StatePath(id string) string { return e.path(id) }

// Load reads and validates one interview state (fail-closed on schema).
func (e *Engine) Load(id string) (*State, error) {
	raw, err := os.ReadFile(e.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: interview %q not found under %s", ErrInterview, id, e.Dir)
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%w: state not valid JSON (backup at %s.bak): %v", ErrInterview, e.path(id), err)
	}
	if major, ok := schemaMajor(s.Schema, "oma-interview"); !ok || major != 1 {
		return nil, fmt.Errorf("%w: state schema %q, want %s", ErrInterview, s.Schema, Schema)
	}
	if s.ID != id {
		return nil, fmt.Errorf("%w: state id %q does not match file %q", ErrInterview, s.ID, id)
	}
	if s.extra, err = jsonmerge.Extra(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Resolve picks the target instance. With a session suffix, omitted --id means
// the session's default interview. Without a session suffix, the legacy engine
// fallback still requires exactly one non-terminal interview.
func (e *Engine) Resolve(id string) (*State, error) {
	id = strings.TrimSpace(id)
	if id != "" || e.SessionSuffix != "" {
		scoped, err := e.scopedID(id)
		if err != nil {
			return nil, err
		}
		return e.Load(scoped)
	}
	matches, err := filepath.Glob(filepath.Join(e.Dir, "interview-*.json"))
	if err != nil {
		return nil, err
	}
	var active []*State
	for _, m := range matches {
		base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "interview-"), ".json")
		if !e.matchesSession(base) {
			continue
		}
		s, err := e.Load(base)
		if err != nil {
			// A corrupt or foreign-major candidate must fail the omitted-id
			// resolution closed (review 060 blocker 2): silently skipping it
			// could route a mutating command at the wrong instance.
			return nil, fmt.Errorf("%w: cannot resolve --id: candidate %s is unreadable: %v", ErrInterview, filepath.Base(m), err)
		}
		if !s.Terminal() {
			active = append(active, s)
		}
	}
	switch len(active) {
	case 1:
		return active[0], nil
	case 0:
		return nil, fmt.Errorf("%w: no active interview (start one with `oma interview start`)", ErrInterview)
	default:
		ids := make([]string, len(active))
		for i, s := range active {
			ids[i] = s.ID
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("%w: %d active interviews, pass --id; candidates: %s", ErrInterview, len(active), strings.Join(ids, ", "))
	}
}

// transition validates a phase edge against the §1.1 state machine.
func (s *State) transition(to string) error {
	legal := map[string][]string{
		PhaseTopologyPending: {PhaseInterviewing, PhaseAborted},
		PhaseInterviewing:    {PhaseGatePassed, PhaseGateWaived, PhaseAborted},
		PhaseGatePassed:      {PhaseCrystallized, PhaseAborted},
		PhaseGateWaived:      {PhaseCrystallized, PhaseAborted},
		PhaseCrystallized:    {PhaseCompleted, PhaseAborted},
	}
	for _, ok := range legal[s.Phase] {
		if ok == to {
			s.Phase = to
			return nil
		}
	}
	return fmt.Errorf("%w: illegal transition %s → %s", ErrInterview, s.Phase, to)
}

// Crystallize records the spec path (gate_passed|gate_waived → crystallized).
func (e *Engine) Crystallize(id, specPath string, dryRun bool) (*State, error) {
	if specPath == "" {
		return nil, fmt.Errorf("%w: --spec is required", ErrInterview)
	}
	if _, err := os.Stat(specPath); err != nil {
		return nil, fmt.Errorf("%w: spec file %s not found (write it before crystallizing)", ErrInterview, specPath)
	}
	return e.withResolved(id, dryRun, func(s *State) error {
		if err := s.transition(PhaseCrystallized); err != nil {
			return err
		}
		s.SpecPath = specPath
		if dryRun {
			return nil
		}
		return e.save(s)
	})
}

// Complete closes a crystallized interview.
func (e *Engine) Complete(id string, dryRun bool) (*State, error) {
	return e.withResolved(id, dryRun, func(s *State) error {
		if err := s.transition(PhaseCompleted); err != nil {
			return err
		}
		if dryRun {
			return nil
		}
		return e.save(s)
	})
}

// Abort ends any non-terminal interview.
func (e *Engine) Abort(id string, dryRun bool) (*State, error) {
	return e.withResolved(id, dryRun, func(s *State) error {
		if s.Terminal() {
			return fmt.Errorf("%w: interview %s is already %s", ErrInterview, s.ID, s.Phase)
		}
		s.Phase = PhaseAborted
		if dryRun {
			return nil
		}
		return e.save(s)
	})
}

// save persists atomically with a single-generation .bak.
func (e *Engine) save(s *State) error {
	s.Updated = e.Now().UTC()
	s.Revision++
	raw, err := jsonmerge.Marshal(s, s.extra)
	if err != nil {
		return err
	}
	path := e.path(s.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := atomicfile.WriteWithBackup(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write interview state: %w", err)
	}
	return nil
}

func (e *Engine) withResolved(id string, dryRun bool, fn func(*State) error) (*State, error) {
	s, err := e.Resolve(id)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return s, fn(s)
	}
	err = e.withInstanceLock(s.ID, func() error {
		var err error
		s, err = e.Load(s.ID)
		if err != nil {
			return err
		}
		return fn(s)
	})
	return s, err
}

func (e *Engine) withInstanceLock(id string, fn func() error) error {
	lockRoot := filepath.Join(e.Dir, ".locks")
	if err := os.MkdirAll(lockRoot, 0o700); err != nil {
		return err
	}
	lock, err := atomicfile.AcquireLock(filepath.Join(lockRoot, "interview-"+id+".lock"), 30*time.Second)
	if err != nil {
		if errors.Is(err, atomicfile.ErrLockHeld) {
			return fmt.Errorf("%w: interview %s is being mutated by another process", ErrInterview, id)
		}
		return err
	}
	defer func() { _ = lock.Release() }()
	return fn()
}

// schemaMajor: strict digit-only parse (per-package copy by convention).
func schemaMajor(schema, wantDomain string) (int, bool) {
	domain, ver, found := strings.Cut(schema, "/")
	if !found || domain != wantDomain || ver == "" || ver[0] == '0' {
		return 0, false
	}
	for i := 0; i < len(ver); i++ {
		if ver[i] < '0' || ver[i] > '9' {
			return 0, false
		}
	}
	major, err := strconv.Atoi(ver)
	if err != nil || major < 1 {
		return 0, false
	}
	return major, true
}

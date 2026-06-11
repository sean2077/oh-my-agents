// Package ralph implements the solidified surface of the persistent
// improvement loop (docs/workflows.md §2): counting, stop-judgment and
// history live HERE; doing the work and RUNNING the verifier stay with
// the agent — oma never executes verifier commands (security contract),
// the agent reports the exit code via `oma ralph check`.
package ralph

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
)

// Schema is the persisted document schema (docs/schemas.md §6).
const Schema = "oma-ralph/1"

// Ralph phases (workflows.md §2.1).
const (
	PhaseRunning   = "running"
	PhasePassed    = "passed"
	PhaseExhausted = "exhausted"
	PhaseStalled   = "stalled"
	PhaseAborted   = "aborted"
)

// ErrRalph marks fail-closed ralph-state refusals.
var ErrRalph = errors.New("ralph refused (fail-closed)")

// Check is one verifier result reported by the agent.
type Check struct {
	Round        int       `json:"round"`
	VerifierExit int       `json:"verifier_exit"`
	Note         string    `json:"note,omitempty"`
	At           time.Time `json:"at"`
}

// State is the oma-ralph/1 document.
type State struct {
	Schema      string    `json:"schema"`
	ID          string    `json:"id"`
	Phase       string    `json:"phase"`
	Goal        string    `json:"goal"`
	MaxRounds   int       `json:"max_rounds"`
	Round       int       `json:"round"`
	Checks      []Check   `json:"checks"`
	StallWindow int       `json:"stall_window"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
}

// Terminal reports an end state.
func (s *State) Terminal() bool { return s.Phase != PhaseRunning }

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Engine anchors ralph state under dir (.oma/state).
type Engine struct {
	Dir string
	Now func() time.Time
}

// NewEngine builds an Engine for the given state directory.
func NewEngine(dir string) *Engine { return &Engine{Dir: dir, Now: time.Now} }

func (e *Engine) path(id string) string { return filepath.Join(e.Dir, "ralph-"+id+".json") }

// Start initializes a loop. goal is required: it anchors the stop
// semantics the agent reasons against.
func (e *Engine) Start(id, goal string, maxRounds, stallWindow int) (*State, error) {
	if strings.TrimSpace(goal) == "" {
		return nil, fmt.Errorf("%w: --goal is required (it anchors the stop judgment)", ErrRalph)
	}
	if id == "" {
		id = e.Now().UTC().Format("20060102-150405")
	}
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("%w: id %q (want %s)", ErrRalph, id, idRe)
	}
	if maxRounds <= 0 {
		maxRounds = 10
	}
	if stallWindow <= 0 {
		stallWindow = 3
	}
	if _, err := os.Stat(e.path(id)); err == nil {
		return nil, fmt.Errorf("%w: ralph %q already exists", ErrRalph, id)
	}
	now := e.Now().UTC()
	s := &State{
		Schema: Schema, ID: id, Phase: PhaseRunning, Goal: goal,
		MaxRounds: maxRounds, StallWindow: stallWindow,
		Checks:  []Check{},
		Created: now, Updated: now,
	}
	return s, e.save(s)
}

// Load reads and validates one ralph state.
func (e *Engine) Load(id string) (*State, error) {
	raw, err := os.ReadFile(e.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: ralph %q not found under %s", ErrRalph, id, e.Dir)
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%w: state not valid JSON (backup at %s.bak): %v", ErrRalph, e.path(id), err)
	}
	if major, ok := schemaMajor(s.Schema, "oma-ralph"); !ok || major != 1 {
		return nil, fmt.Errorf("%w: state schema %q, want %s", ErrRalph, s.Schema, Schema)
	}
	if s.ID != id {
		return nil, fmt.Errorf("%w: state id %q does not match file %q", ErrRalph, s.ID, id)
	}
	return &s, nil
}

// Resolve picks the instance for an omitted --id: exactly one running
// loop must exist.
func (e *Engine) Resolve(id string) (*State, error) {
	if id != "" {
		return e.Load(id)
	}
	matches, err := filepath.Glob(filepath.Join(e.Dir, "ralph-*.json"))
	if err != nil {
		return nil, err
	}
	var active []*State
	for _, m := range matches {
		base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "ralph-"), ".json")
		s, err := e.Load(base)
		if err != nil {
			continue
		}
		if !s.Terminal() {
			active = append(active, s)
		}
	}
	switch len(active) {
	case 1:
		return active[0], nil
	case 0:
		return nil, fmt.Errorf("%w: no running ralph loop (start one with `oma ralph start --goal …`)", ErrRalph)
	default:
		ids := make([]string, len(active))
		for i, s := range active {
			ids[i] = s.ID
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("%w: %d running loops, pass --id; candidates: %s", ErrRalph, len(active), strings.Join(ids, ", "))
	}
}

// Verdict is the machine-readable next/check outcome.
type Verdict struct {
	Phase    string `json:"phase"`
	Round    int    `json:"round"`
	Continue bool   `json:"continue"`
	Reason   string `json:"reason"`
}

// Next advances the loop one round. Terminal states report stop with
// their reason and stay unchanged (idempotent — review 058 guardrail);
// crossing max_rounds flips to exhausted.
func (e *Engine) Next(id string) (*State, *Verdict, error) {
	s, err := e.Resolve(id)
	if err != nil {
		return nil, nil, err
	}
	if s.Terminal() {
		return s, &Verdict{Phase: s.Phase, Round: s.Round, Continue: false, Reason: "loop is " + s.Phase}, nil
	}
	s.Round++
	if s.Round > s.MaxRounds {
		s.Phase = PhaseExhausted
		if err := e.save(s); err != nil {
			return nil, nil, err
		}
		return s, &Verdict{Phase: s.Phase, Round: s.Round, Continue: false,
			Reason: fmt.Sprintf("exhausted: round %d exceeds max_rounds %d", s.Round, s.MaxRounds)}, nil
	}
	if err := e.save(s); err != nil {
		return nil, nil, err
	}
	return s, &Verdict{Phase: s.Phase, Round: s.Round, Continue: true,
		Reason: fmt.Sprintf("round %d of %d", s.Round, s.MaxRounds)}, nil
}

// RecordCheck stores one verifier result. Exit 0 → passed. A run of
// stall_window consecutive failures sharing the same non-empty note is a
// stall (the note is the failure signature).
func (e *Engine) RecordCheck(id string, verifierExit int, note string) (*State, *Verdict, error) {
	s, err := e.Resolve(id)
	if err != nil {
		return nil, nil, err
	}
	if s.Terminal() {
		return nil, nil, fmt.Errorf("%w: loop %s is %s; check is not legal on a terminal loop", ErrRalph, s.ID, s.Phase)
	}
	s.Checks = append(s.Checks, Check{Round: s.Round, VerifierExit: verifierExit, Note: note, At: e.Now().UTC()})
	switch {
	case verifierExit == 0:
		s.Phase = PhasePassed
	case e.stalled(s):
		s.Phase = PhaseStalled
	}
	if err := e.save(s); err != nil {
		return nil, nil, err
	}
	v := &Verdict{Phase: s.Phase, Round: s.Round, Continue: s.Phase == PhaseRunning}
	switch s.Phase {
	case PhasePassed:
		v.Reason = "verifier passed"
	case PhaseStalled:
		v.Reason = fmt.Sprintf("stalled: %d consecutive failures with signature %q — change strategy", s.StallWindow, note)
	default:
		v.Reason = fmt.Sprintf("verifier exit %d recorded", verifierExit)
	}
	return s, v, nil
}

// stalled reports stall_window consecutive same-signature failures.
func (e *Engine) stalled(s *State) bool {
	n := s.StallWindow
	if len(s.Checks) < n {
		return false
	}
	last := s.Checks[len(s.Checks)-1]
	if last.VerifierExit == 0 || strings.TrimSpace(last.Note) == "" {
		return false
	}
	for i := len(s.Checks) - n; i < len(s.Checks); i++ {
		c := s.Checks[i]
		if c.VerifierExit == 0 || c.Note != last.Note {
			return false
		}
	}
	return true
}

// Abort ends a running loop.
func (e *Engine) Abort(id string) (*State, error) {
	s, err := e.Resolve(id)
	if err != nil {
		return nil, err
	}
	if s.Terminal() {
		return nil, fmt.Errorf("%w: loop %s is already %s", ErrRalph, s.ID, s.Phase)
	}
	s.Phase = PhaseAborted
	return s, e.save(s)
}

// save persists atomically with a single-generation .bak.
func (e *Engine) save(s *State) error {
	s.Updated = e.Now().UTC()
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := e.path(s.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if prev, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", prev, 0o600); err != nil {
			return fmt.Errorf("write state backup: %w", err)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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

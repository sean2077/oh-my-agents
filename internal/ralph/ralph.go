// Package ralph implements the solidified surface of the persistent
// improvement loop (docs/reference/workflows.md §2): counting, stop-judgment and
// history live HERE; doing the work and RUNNING the verifier stay with
// the agent — oma never executes verifier commands (security contract),
// the agent reports the exit code (and, under score_improvement, the
// evaluator score) via `oma ralph check`.
package ralph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

// Schema is the persisted document schema (docs/reference/schemas.md §6). The /2 bump
// (R1) adds the keep-policy state contract (KeepPolicy/PlateauWindow/
// BestRound/BestScore + the PhasePlateaued terminal). Load rejects any other
// major fail-closed; there is no /1→/2 migration layer (terminal design).
const Schema = "oma-ralph/2"

// Ralph phases (docs/reference/workflows.md §2.1).
const (
	PhaseRunning   = "running"
	PhasePassed    = "passed"
	PhaseExhausted = "exhausted"
	PhaseStalled   = "stalled"
	PhasePlateaued = "plateaued" // score_improvement: no score gain for plateau_window rounds
	PhaseAborted   = "aborted"
)

// Keep-policy values (R1). pass_only is the historical behavior (verifier
// exit 0 passes; same-signature failures stall). score_improvement keeps the
// best score and stops when it plateaus.
const (
	KeepPassOnly         = "pass_only"
	KeepScoreImprovement = "score_improvement"
)

// ErrRalph marks fail-closed ralph-state refusals.
var ErrRalph = errors.New("ralph refused (fail-closed)")

// Check is one verifier result reported by the agent. Score is set only
// under keep-policy score_improvement (the evaluator's numeric result); it
// stays nil for pass_only, so a pass_only Check marshals identically to the
// pre-/2 shape (receipt hashes stay stable).
type Check struct {
	Round        int       `json:"round"`
	VerifierExit int       `json:"verifier_exit"`
	Score        *float64  `json:"score,omitempty"`
	Note         string    `json:"note,omitempty"`
	At           time.Time `json:"at"`
}

// State is the oma-ralph/2 document.
type State struct {
	Schema       string  `json:"schema"`
	ID           string  `json:"id"`
	Revision     int64   `json:"revision"`
	Session      string  `json:"session,omitempty"`
	ProjectRoot  string  `json:"project_root,omitempty"`
	WorktreeRoot string  `json:"worktree_root,omitempty"`
	Phase        string  `json:"phase"`
	Goal         string  `json:"goal"`
	KeepPolicy   string  `json:"keep_policy"`
	MaxRounds    int     `json:"max_rounds"`
	Round        int     `json:"round"`
	Checks       []Check `json:"checks"`
	StallWindow  int     `json:"stall_window"`
	// PlateauWindow/BestRound/BestScore drive score_improvement keep-best and
	// the plateau stop. BestRound is 0 until the first scored check.
	PlateauWindow int       `json:"plateau_window"`
	BestRound     int       `json:"best_round,omitempty"`
	BestScore     *float64  `json:"best_score,omitempty"`
	Created       time.Time `json:"created"`
	Updated       time.Time `json:"updated"`
	// Receipt (A1): sha256 over {goal, checks, terminal_check}, set when the
	// loop reaches a meaningful terminal. For pass_only that is PhasePassed;
	// for score_improvement it is also plateau/exhaustion (the kept best is
	// the deliverable), and terminal_check is the best-score check. It proves
	// the recorded results, not that the agent truly ran the command.
	Receipt string `json:"receipt,omitempty"`

	// extra preserves unknown top-level fields across load/save (schemas.md
	// minor-additive contract). Unexported, so it is never serialized itself.
	extra map[string]json.RawMessage
}

// Terminal reports an end state.
func (s *State) Terminal() bool { return s.Phase != PhaseRunning }

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Engine anchors ralph state under dir (.oma/state).
type Engine struct {
	Dir                 string
	Now                 func() time.Time
	SessionSuffix       string
	ProjectRoot         string
	WorktreeRoot        string
	AllowWorktreeChange bool
}

// NewEngine builds an Engine for the given state directory.
func NewEngine(dir string) *Engine { return &Engine{Dir: dir, Now: time.Now} }

func (e *Engine) path(id string) string { return filepath.Join(e.Dir, "ralph-"+id+".json") }

func (e *Engine) scopedID(id string) (string, error) {
	if e.SessionSuffix == "" {
		return strings.TrimSpace(id), nil
	}
	return session.ScopeName(strings.TrimSpace(id), e.SessionSuffix)
}

func (e *Engine) matchesSession(id string) bool {
	return e.SessionSuffix == "" || session.MatchesScope(id, e.SessionSuffix)
}

// StartOpts configures a new loop. Zero values take documented defaults
// (KeepPolicy→pass_only, MaxRounds→10, StallWindow→3, PlateauWindow→3).
type StartOpts struct {
	Goal          string
	KeepPolicy    string
	MaxRounds     int
	StallWindow   int
	PlateauWindow int
}

// Start initializes a loop. Goal is required: it anchors the stop semantics
// the agent reasons against.
func (e *Engine) Start(id string, opts StartOpts, dryRun bool) (*State, error) {
	if strings.TrimSpace(opts.Goal) == "" {
		return nil, fmt.Errorf("%w: --goal is required (it anchors the stop judgment)", ErrRalph)
	}
	keep := opts.KeepPolicy
	if keep == "" {
		keep = KeepPassOnly
	}
	if keep != KeepPassOnly && keep != KeepScoreImprovement {
		return nil, fmt.Errorf("%w: keep-policy %q must be %s or %s", ErrRalph, keep, KeepPassOnly, KeepScoreImprovement)
	}
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
		return nil, fmt.Errorf("%w: id %q (want %s)", ErrRalph, id, idRe)
	}
	maxRounds := opts.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}
	stallWindow := opts.StallWindow
	if stallWindow <= 0 {
		stallWindow = 3
	}
	plateauWindow := opts.PlateauWindow
	if plateauWindow <= 0 {
		plateauWindow = 3
	}
	if _, err := os.Stat(e.path(id)); err == nil {
		return nil, fmt.Errorf("%w: ralph %q already exists", ErrRalph, id)
	}
	now := e.Now().UTC()
	s := &State{
		Schema: Schema, ID: id, Phase: PhaseRunning, Goal: opts.Goal,
		Session:      e.SessionSuffix,
		ProjectRoot:  e.ProjectRoot,
		WorktreeRoot: e.WorktreeRoot,
		KeepPolicy:   keep,
		MaxRounds:    maxRounds, StallWindow: stallWindow, PlateauWindow: plateauWindow,
		Checks:  []Check{},
		Created: now, Updated: now,
	}
	if dryRun {
		return s, nil
	}
	err = e.withInstanceLock(id, func() error {
		if _, statErr := os.Stat(e.path(id)); statErr == nil {
			return fmt.Errorf("%w: ralph %q already exists", ErrRalph, id)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		return e.save(s)
	})
	return s, err
}

// StatePath is the absolute state file for one id (dry-run reporting).
func (e *Engine) StatePath(id string) string { return e.path(id) }

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
	if major, ok := schemaMajor(s.Schema, "oma-ralph"); !ok || major != 2 {
		return nil, fmt.Errorf("%w: state schema %q, want %s", ErrRalph, s.Schema, Schema)
	}
	if s.ID != id {
		return nil, fmt.Errorf("%w: state id %q does not match file %q", ErrRalph, s.ID, id)
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	if s.extra, err = jsonmerge.Extra(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (e *Engine) loadResolved(id string) (*State, error) {
	s, err := e.Load(id)
	if err != nil {
		return nil, err
	}
	if err := e.checkWorktreeBinding(s); err != nil {
		return nil, err
	}
	return s, nil
}

func (e *Engine) checkWorktreeBinding(s *State) error {
	if e.AllowWorktreeChange || e.WorktreeRoot == "" || s.WorktreeRoot == "" {
		return nil
	}
	if s.ProjectRoot != "" && e.ProjectRoot != "" && filepath.Clean(s.ProjectRoot) != filepath.Clean(e.ProjectRoot) {
		return fmt.Errorf("%w: loop %s belongs to project %s; current project is %s", ErrRalph, s.ID, s.ProjectRoot, e.ProjectRoot)
	}
	if filepath.Clean(s.WorktreeRoot) != filepath.Clean(e.WorktreeRoot) {
		return fmt.Errorf("%w: loop %s is bound to worktree %s; current worktree is %s (pass --allow-worktree-change to inspect or intentionally continue)",
			ErrRalph, s.ID, s.WorktreeRoot, e.WorktreeRoot)
	}
	return nil
}

// validate enforces the /2 keep-policy contract on LOADED persisted state, so a
// hand-written or corrupt file cannot bypass the fail-closed rules Start applies
// to new loops (codex gate-2 must-fix): an unknown keep_policy must not fall
// through to pass_only, and score_improvement must keep a live plateau stop.
func (s *State) validate() error {
	if s.KeepPolicy != KeepPassOnly && s.KeepPolicy != KeepScoreImprovement {
		return fmt.Errorf("%w: persisted keep_policy %q must be %s or %s", ErrRalph, s.KeepPolicy, KeepPassOnly, KeepScoreImprovement)
	}
	if s.KeepPolicy == KeepScoreImprovement && s.PlateauWindow <= 0 {
		return fmt.Errorf("%w: score_improvement requires plateau_window > 0 (got %d)", ErrRalph, s.PlateauWindow)
	}
	return nil
}

// Resolve picks the target loop. With a session suffix, omitted --id means the
// session's default loop. Without a session suffix, the legacy engine fallback
// still requires exactly one running loop.
func (e *Engine) Resolve(id string) (*State, error) {
	id = strings.TrimSpace(id)
	if id != "" || e.SessionSuffix != "" {
		scoped, err := e.scopedID(id)
		if err != nil {
			return nil, err
		}
		return e.loadResolved(scoped)
	}
	matches, err := filepath.Glob(filepath.Join(e.Dir, "ralph-*.json"))
	if err != nil {
		return nil, err
	}
	var active []*State
	for _, m := range matches {
		base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "ralph-"), ".json")
		if !e.matchesSession(base) {
			continue
		}
		s, err := e.loadResolved(base)
		if err != nil {
			// A corrupt or foreign-major candidate must fail the omitted-id
			// resolution closed (review 060 blocker 2).
			return nil, fmt.Errorf("%w: cannot resolve --id: candidate %s is unreadable: %v", ErrRalph, filepath.Base(m), err)
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
	Mutated  bool   `json:"-"` // a write happened (or would have, under --dry-run)
}

// Next advances the loop one round. Terminal states report stop with
// their reason and stay unchanged (idempotent — review 058 guardrail);
// crossing max_rounds flips to exhausted. Under score_improvement an
// exhausted loop still earns a receipt over the kept best.
func (e *Engine) Next(id string, dryRun bool) (*State, *Verdict, error) {
	var v *Verdict
	s, err := e.withResolved(id, dryRun, func(s *State) error {
		if s.Terminal() {
			v = &Verdict{Phase: s.Phase, Round: s.Round, Continue: false, Reason: "loop is " + s.Phase}
			return nil
		}
		s.Round++
		if s.Round > s.MaxRounds {
			s.Phase = PhaseExhausted
			if s.KeepPolicy == KeepScoreImprovement && len(s.Checks) > 0 {
				s.Receipt = ralphReceipt(s)
			}
			if err := e.saveUnless(s, dryRun); err != nil {
				return err
			}
			v = &Verdict{Phase: s.Phase, Round: s.Round, Continue: false, Mutated: true,
				Reason: fmt.Sprintf("exhausted: round %d exceeds max_rounds %d", s.Round, s.MaxRounds)}
			return nil
		}
		if err := e.saveUnless(s, dryRun); err != nil {
			return err
		}
		v = &Verdict{Phase: s.Phase, Round: s.Round, Continue: true, Mutated: true,
			Reason: fmt.Sprintf("round %d of %d", s.Round, s.MaxRounds)}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return s, v, nil
}

// RecordCheck stores one verifier result. Exit 0 → passed. Under pass_only a
// run of stall_window consecutive failures sharing the same non-empty note is
// a stall (the note is the failure signature). Under score_improvement every
// check must carry a finite --score; the loop keeps the strict-best score and
// plateaus when no improvement lands within plateau_window rounds.
func (e *Engine) RecordCheck(id string, verifierExit int, score *float64, note string, dryRun bool) (*State, *Verdict, error) {
	var v *Verdict
	s, err := e.withResolved(id, dryRun, func(s *State) error {
		if s.Terminal() {
			return fmt.Errorf("%w: loop %s is %s; check is not legal on a terminal loop", ErrRalph, s.ID, s.Phase)
		}
		// Score/policy validation is fail-closed: score_improvement demands a
		// finite score on every check; pass_only forbids one (it would be inert).
		switch s.KeepPolicy {
		case KeepScoreImprovement:
			if score == nil {
				return fmt.Errorf("%w: keep-policy score_improvement requires --score on every check", ErrRalph)
			}
			if math.IsNaN(*score) || math.IsInf(*score, 0) {
				return fmt.Errorf("%w: --score must be a finite number", ErrRalph)
			}
		default: // pass_only (and any legacy empty policy already normalized at Start)
			if score != nil {
				return fmt.Errorf("%w: --score is only valid under keep-policy score_improvement", ErrRalph)
			}
		}
		s.Checks = append(s.Checks, Check{Round: s.Round, VerifierExit: verifierExit, Score: score, Note: note, At: e.Now().UTC()})
		// keep-best: strict improvement only (no epsilon).
		if s.KeepPolicy == KeepScoreImprovement && score != nil {
			if s.BestScore == nil || *score > *s.BestScore {
				best := *score
				s.BestScore = &best
				s.BestRound = s.Round
			}
		}
		switch {
		case verifierExit == 0:
			s.Phase = PhasePassed
			s.Receipt = ralphReceipt(s)
		case s.KeepPolicy == KeepScoreImprovement:
			if plateaued(s) {
				s.Phase = PhasePlateaued
				s.Receipt = ralphReceipt(s)
			}
		default: // pass_only
			if e.stalled(s) {
				s.Phase = PhaseStalled
			}
		}
		if err := e.saveUnless(s, dryRun); err != nil {
			return err
		}
		v = &Verdict{Phase: s.Phase, Round: s.Round, Continue: s.Phase == PhaseRunning, Mutated: true}
		switch s.Phase {
		case PhasePassed:
			v.Reason = "verifier passed"
		case PhasePlateaued:
			v.Reason = fmt.Sprintf("plateaued: no score improvement over %d rounds (best %s at round %d) — change strategy", s.PlateauWindow, fmtScore(s.BestScore), s.BestRound)
		case PhaseStalled:
			v.Reason = fmt.Sprintf("stalled: %d consecutive failures with signature %q — change strategy", s.StallWindow, note)
		default:
			if s.KeepPolicy == KeepScoreImprovement {
				v.Reason = fmt.Sprintf("score %s recorded (best %s at round %d)", fmtScore(score), fmtScore(s.BestScore), s.BestRound)
			} else {
				v.Reason = fmt.Sprintf("verifier exit %d recorded", verifierExit)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return s, v, nil
}

// stalled reports stall_window consecutive same-signature failures (pass_only).
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

// plateaued reports that no strict score improvement has landed in the last
// plateau_window rounds. BestRound is the round of the last strict best, so
// round-current minus BestRound is the run of non-improving rounds. A tie
// (== best) is not an improvement and counts toward the plateau. Round-based
// to honor the "N rounds without improvement" contract (one improvement
// judgment per round).
func plateaued(s *State) bool {
	if s.PlateauWindow <= 0 || s.BestRound == 0 {
		return false
	}
	return s.Round-s.BestRound >= s.PlateauWindow
}

// bestScoreCheck returns the strict-best scored check (the kept result), or
// nil if no check carried a score.
func bestScoreCheck(s *State) *Check {
	var best *Check
	for i := range s.Checks {
		c := &s.Checks[i]
		if c.Score == nil {
			continue
		}
		if best == nil || *c.Score > *best.Score {
			best = c
		}
	}
	return best
}

// ralphReceipt hashes the loop's terminal evidence (A1). Caller guarantees at
// least one recorded check. For score_improvement the terminal_check is the
// best-score check (the kept deliverable); otherwise it is the last check.
func ralphReceipt(s *State) string {
	terminal := s.Checks[len(s.Checks)-1]
	if s.KeepPolicy == KeepScoreImprovement {
		if bc := bestScoreCheck(s); bc != nil {
			terminal = *bc
		}
	}
	payload := struct {
		Schema        string  `json:"schema"`
		Goal          string  `json:"goal"`
		KeepPolicy    string  `json:"keep_policy"`
		Checks        []Check `json:"checks"`
		TerminalCheck Check   `json:"terminal_check"`
	}{
		Schema:        "oma-ralph-receipt/1",
		Goal:          s.Goal,
		KeepPolicy:    s.KeepPolicy,
		Checks:        s.Checks,
		TerminalCheck: terminal,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// fmtScore renders a *float64 for human verdict reasons (never used in the
// receipt hash, which marshals the raw value).
func fmtScore(p *float64) string {
	if p == nil {
		return "n/a"
	}
	return strconv.FormatFloat(*p, 'g', -1, 64)
}

// Abort ends a running loop.
func (e *Engine) Abort(id string, dryRun bool) (*State, error) {
	return e.withResolved(id, dryRun, func(s *State) error {
		if s.Terminal() {
			return fmt.Errorf("%w: loop %s is already %s", ErrRalph, s.ID, s.Phase)
		}
		s.Phase = PhaseAborted
		if dryRun {
			return nil
		}
		return e.save(s)
	})
}

// saveUnless persists except under --dry-run (the full validation and
// state computation above still ran — security-contract §1).
func (e *Engine) saveUnless(s *State, dryRun bool) error {
	if dryRun {
		return nil
	}
	return e.save(s)
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
		return fmt.Errorf("write ralph state: %w", err)
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
		if err := e.checkWorktreeBinding(s); err != nil {
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
	lock, err := atomicfile.AcquireLock(filepath.Join(lockRoot, "ralph-"+id+".lock"), 30*time.Second)
	if err != nil {
		if errors.Is(err, atomicfile.ErrLockHeld) {
			return fmt.Errorf("%w: ralph %s is being mutated by another process", ErrRalph, id)
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

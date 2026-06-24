package interview

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Dimension weight tables (docs/reference/workflows.md §1.3): ambiguity =
// 1 - Σ(weight × dimension_total), dimension_total = min across active
// components.
var (
	greenfieldWeights = map[string]float64{"goal": 0.40, "constraints": 0.30, "criteria": 0.30}
	brownfieldWeights = map[string]float64{"goal": 0.35, "constraints": 0.25, "criteria": 0.25, "context": 0.15}
)

func (s *State) weights() map[string]float64 {
	if s.Type == "brownfield" {
		return brownfieldWeights
	}
	return greenfieldWeights
}

// ScoresInput is the agent→CLI envelope (oma-interview-scores/1,
// docs/reference/schemas.md §5). Round 0 carries the locked topology instead of
// component scores (the Round 0 topology gate; minor-additive field).
type ScoresInput struct {
	Schema            string                        `json:"schema"`
	Round             int                           `json:"round"`
	Topology          *TopologyInput                `json:"topology,omitempty"`
	ComponentScores   map[string]map[string]float64 `json:"component_scores,omitempty"`
	Question          string                        `json:"question,omitempty"`
	Answer            string                        `json:"answer,omitempty"`
	Ontology          *OntologyInput                `json:"ontology,omitempty"`
	ChallengeModeUsed string                        `json:"challenge_mode_used,omitempty"`
}

// TopologyInput locks the Round 0 component shape.
type TopologyInput struct {
	Components []Component `json:"components"`
	Deferrals  []Deferral  `json:"deferrals,omitempty"`
}

// OntologyInput is the per-round entity extraction.
type OntologyInput struct {
	Entities          []Entity `json:"entities"`
	MatchingReasoning string   `json:"matching_reasoning,omitempty"`
}

// Report is the deterministic outcome of one score call (--json).
type Report struct {
	Phase                string             `json:"phase"`
	Round                int                `json:"round"`
	Ambiguity            float64            `json:"ambiguity"`
	Threshold            float64            `json:"threshold"`
	DimensionTotals      map[string]float64 `json:"dimension_totals,omitempty"`
	Weakest              *Target            `json:"weakest,omitempty"`
	RotationApplied      bool               `json:"rotation_applied,omitempty"`
	OntologyStability    *float64           `json:"ontology_stability,omitempty"`
	ChallengeSuggestions []string           `json:"challenge_suggestions,omitempty"`
	Warnings             []string           `json:"warnings,omitempty"`
}

// Target names the weakest (component, dimension) pair.
type Target struct {
	Component string  `json:"component"`
	Dimension string  `json:"dimension"`
	Score     float64 `json:"score"`
}

// ParseScoresInput decodes and schema-checks one scores file.
func ParseScoresInput(path string) (*ScoresInput, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInterview, err)
	}
	var in ScoresInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("%w: scores input not valid JSON: %v", ErrInterview, err)
	}
	if major, ok := schemaMajor(in.Schema, "oma-interview-scores"); !ok || major != 1 {
		return nil, fmt.Errorf("%w: scores schema %q, want %s", ErrInterview, in.Schema, ScoresSchema)
	}
	return &in, nil
}

// Score applies one agent-evaluated round: Round 0 locks the topology;
// later rounds run the deterministic math (dimension minima, weighted
// ambiguity, ontology stability, weakest-target rotation, challenge
// suggestions) and append the round record.
func (e *Engine) Score(id string, in *ScoresInput, dryRun bool) (*State, *Report, error) {
	var rep *Report
	s, err := e.withResolved(id, dryRun, func(s *State) error {
		// Round numbering: topology lock is round 0 and lives outside Rounds;
		// scored rounds are 1-based and append to Rounds.
		expected := len(s.Rounds) + 1
		if s.Phase == PhaseTopologyPending {
			expected = 0
		}
		if in.Round != expected {
			return fmt.Errorf("%w: input round %d, expected %d (replays and skips are refused)", ErrInterview, in.Round, expected)
		}
		var err error
		switch s.Phase {
		case PhaseTopologyPending:
			rep, err = e.lockTopology(s, in, dryRun)
		case PhaseInterviewing:
			rep, err = e.scoreRound(s, in, dryRun)
		default:
			err = fmt.Errorf("%w: score is not legal in phase %s", ErrInterview, s.Phase)
		}
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return s, rep, nil
}

func (e *Engine) lockTopology(s *State, in *ScoresInput, dryRun bool) (*Report, error) {
	if in.Round != 0 || in.Topology == nil {
		return nil, fmt.Errorf("%w: phase topology_pending requires a round-0 input carrying topology", ErrInterview)
	}
	if len(in.Topology.Components) == 0 {
		return nil, fmt.Errorf("%w: topology must lock at least one component", ErrInterview)
	}
	seen := map[string]bool{}
	hasActive := false
	for i := range in.Topology.Components {
		c := &in.Topology.Components[i]
		if !idRe.MatchString(c.ID) {
			return nil, fmt.Errorf("%w: component id %q (want %s)", ErrInterview, c.ID, idRe)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("%w: duplicate component id %q", ErrInterview, c.ID)
		}
		seen[c.ID] = true
		switch c.Status {
		case "active":
			hasActive = true
		case "deferred":
		default:
			return nil, fmt.Errorf("%w: component %s status %q not in active|deferred", ErrInterview, c.ID, c.Status)
		}
		if c.ClarityScores == nil {
			c.ClarityScores = map[string]float64{}
		}
	}
	if !hasActive {
		return nil, fmt.Errorf("%w: topology needs at least one active component", ErrInterview)
	}
	for _, d := range in.Topology.Deferrals {
		if !seen[d.ComponentID] {
			return nil, fmt.Errorf("%w: deferral names unknown component %q", ErrInterview, d.ComponentID)
		}
	}
	s.Topology.Components = in.Topology.Components
	s.Topology.Deferrals = in.Topology.Deferrals
	if s.Topology.Deferrals == nil {
		s.Topology.Deferrals = []Deferral{}
	}
	s.Topology.Status = "confirmed"
	if err := s.transition(PhaseInterviewing); err != nil {
		return nil, err
	}
	if !dryRun {
		if err := e.save(s); err != nil {
			return nil, err
		}
	}
	return &Report{Phase: s.Phase, Round: 0, Ambiguity: s.CurrentAmbiguity, Threshold: s.Threshold}, nil
}

func (e *Engine) scoreRound(s *State, in *ScoresInput, dryRun bool) (*Report, error) {
	if len(in.ComponentScores) == 0 {
		return nil, fmt.Errorf("%w: component_scores required in phase interviewing", ErrInterview)
	}
	weights := s.weights()
	active := s.activeComponents()

	// Every ACTIVE component must be scored on every weighted dimension —
	// a silently missing score would undercount ambiguity (loud-gate rule).
	byID := map[string]*Component{}
	for i := range s.Topology.Components {
		byID[s.Topology.Components[i].ID] = &s.Topology.Components[i]
	}
	for id := range in.ComponentScores {
		c, known := byID[id]
		if !known {
			return nil, fmt.Errorf("%w: scores name unknown component %q", ErrInterview, id)
		}
		if c.Status != "active" {
			return nil, fmt.Errorf("%w: component %q is deferred; deferred components are excluded from scoring", ErrInterview, id)
		}
	}
	for _, c := range active {
		scores, ok := in.ComponentScores[c.ID]
		if !ok {
			return nil, fmt.Errorf("%w: active component %q missing from component_scores (score every active component)", ErrInterview, c.ID)
		}
		for dim := range weights {
			v, ok := scores[dim]
			if !ok {
				return nil, fmt.Errorf("%w: component %q missing dimension %q", ErrInterview, c.ID, dim)
			}
			if v < 0 || v > 1 {
				return nil, fmt.Errorf("%w: component %q dimension %q score %.3f outside [0,1]", ErrInterview, c.ID, dim, v)
			}
		}
		for dim := range scores {
			if _, ok := weights[dim]; !ok {
				return nil, fmt.Errorf("%w: component %q has unknown dimension %q for type %s", ErrInterview, c.ID, dim, s.Type)
			}
		}
	}

	// Deterministic math: per-dimension minimum across active components.
	totals := map[string]float64{}
	for dim := range weights {
		minV := 1.0
		for _, c := range active {
			if v := in.ComponentScores[c.ID][dim]; v < minV {
				minV = v
			}
		}
		totals[dim] = minV
	}
	clarity := 0.0
	for dim, w := range weights {
		clarity += w * totals[dim]
	}
	ambiguity := 1.0 - clarity

	// Persist per-component clarity onto the topology.
	for _, c := range active {
		byID[c.ID].ClarityScores = in.ComponentScores[c.ID]
	}

	// Ontology stability vs the previous snapshot (N/A on the first).
	var stability *float64
	if in.Ontology != nil {
		if len(s.OntologySnapshots) > 0 {
			prev := s.OntologySnapshots[len(s.OntologySnapshots)-1].Entities
			r := stabilityRatio(prev, in.Ontology.Entities)
			stability = &r
		}
		s.OntologySnapshots = append(s.OntologySnapshots, OntologySnapshot{
			Round: in.Round, Entities: in.Ontology.Entities,
			StabilityRatio: stability, MatchingReasoning: in.Ontology.MatchingReasoning,
		})
	}

	// Weakest (component, dimension) with rotation away from the last
	// targeted component when equally-weak alternatives exist.
	weakest, rotated := s.pickWeakest(in.ComponentScores, active, weights)
	s.Topology.LastTargetedComponentID = weakest.Component

	if in.ChallengeModeUsed != "" {
		if !contains(s.ChallengeModesUsed, in.ChallengeModeUsed) {
			s.ChallengeModesUsed = append(s.ChallengeModesUsed, in.ChallengeModeUsed)
		}
	}
	s.Rounds = append(s.Rounds, Round{
		Round: in.Round, Component: weakest.Component, Dimension: weakest.Dimension,
		Question: in.Question, Answer: in.Answer,
		Scores: in.ComponentScores, Ambiguity: ambiguity,
	})
	s.CurrentAmbiguity = ambiguity
	if !dryRun {
		if err := e.save(s); err != nil {
			return nil, err
		}
	}

	rep := &Report{
		Phase: s.Phase, Round: in.Round, Ambiguity: ambiguity, Threshold: s.Threshold,
		DimensionTotals: totals, Weakest: &weakest, RotationApplied: rotated,
		OntologyStability:    stability,
		ChallengeSuggestions: s.challengeSuggestions(len(s.Rounds), ambiguity),
		Warnings:             roundWarnings(len(s.Rounds)),
	}
	return rep, nil
}

func (s *State) activeComponents() []Component {
	var out []Component
	for _, c := range s.Topology.Components {
		if c.Status == "active" {
			out = append(out, c)
		}
	}
	return out
}

// pickWeakest scans active components × weighted dimensions for the
// minimum score; among tied minima it rotates away from the last
// targeted component when an alternative exists (docs/reference/workflows.md §1.3).
// Iteration is deterministic: sorted component ids, sorted dimensions.
func (s *State) pickWeakest(scores map[string]map[string]float64, active []Component, weights map[string]float64) (Target, bool) {
	ids := make([]string, len(active))
	for i, c := range active {
		ids[i] = c.ID
	}
	sort.Strings(ids)
	dims := make([]string, 0, len(weights))
	for d := range weights {
		dims = append(dims, d)
	}
	sort.Strings(dims)

	minV := 2.0
	var candidates []Target
	for _, id := range ids {
		for _, dim := range dims {
			v := scores[id][dim]
			switch {
			case v < minV:
				minV = v
				candidates = []Target{{Component: id, Dimension: dim, Score: v}}
			case v == minV:
				candidates = append(candidates, Target{Component: id, Dimension: dim, Score: v})
			}
		}
	}
	for _, cand := range candidates {
		if cand.Component != s.Topology.LastTargetedComponentID {
			rotated := len(candidates) > 1 && candidates[0].Component == s.Topology.LastTargetedComponentID
			return cand, rotated
		}
	}
	return candidates[0], false // every tied candidate is the last target
}

// challengeSuggestions lists challenge modes unlocked at the round
// thresholds (contrarian ≥4, simplifier ≥6, ontologist ≥8 while
// ambiguity > 0.3), each suggested only while unused.
func (s *State) challengeSuggestions(roundCount int, ambiguity float64) []string {
	var out []string
	if roundCount >= 4 && !contains(s.ChallengeModesUsed, "contrarian") {
		out = append(out, "contrarian")
	}
	if roundCount >= 6 && !contains(s.ChallengeModesUsed, "simplifier") {
		out = append(out, "simplifier")
	}
	if roundCount >= 8 && ambiguity > 0.3 && !contains(s.ChallengeModesUsed, "ontologist") {
		out = append(out, "ontologist")
	}
	return out
}

// roundWarnings: soft at ≥10 rounds, hard-cap notice at ≥20 — the gate
// still judges numerically; overriding is the user's call.
func roundWarnings(roundCount int) []string {
	var out []string
	if roundCount >= 20 {
		out = append(out, fmt.Sprintf("hard cap: %d rounds reached — consider gate --waive or abort (user decision)", roundCount))
	} else if roundCount >= 10 {
		out = append(out, fmt.Sprintf("soft warning: %d rounds without passing the gate", roundCount))
	}
	return out
}

// stabilityRatio = (stable + changed) / total current entities; stable =
// same name, changed = same type with >50% field overlap (a rename).
func stabilityRatio(prev, cur []Entity) float64 {
	if len(cur) == 0 {
		return 0
	}
	prevByName := map[string]Entity{}
	for _, p := range prev {
		prevByName[p.Name] = p
	}
	matched := map[string]bool{} // prev names already consumed by a rename
	count := 0
	for _, c := range cur {
		if _, ok := prevByName[c.Name]; ok {
			matched[c.Name] = true // consume the prev entity so it cannot ALSO be claimed as a rename source
			count++                // stable
			continue
		}
		for _, p := range prev {
			if matched[p.Name] || p.Type != c.Type {
				continue
			}
			if _, sameName := prevByName[c.Name]; sameName {
				continue
			}
			if fieldOverlap(p.Fields, c.Fields) > 0.5 {
				matched[p.Name] = true
				count++ // changed (rename)
				break
			}
		}
	}
	return float64(count) / float64(len(cur))
}

// fieldOverlap = |intersection| / |union|; both-empty yields 0 (no
// evidence of identity).
func fieldOverlap(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	setA := map[string]bool{}
	for _, f := range a {
		setA[f] = true
	}
	inter := 0
	union := len(setA)
	for _, f := range b {
		if setA[f] {
			inter++
		} else {
			union++
		}
	}
	return float64(inter) / float64(union)
}

// GateResult is the machine-readable gate outcome (review 058 guardrail).
type GateResult struct {
	Pass      bool     `json:"pass"`
	Ambiguity float64  `json:"ambiguity"`
	Threshold float64  `json:"threshold"`
	Gap       float64  `json:"gap"` // ambiguity - threshold (≤0 when passing)
	Weakest   *Target  `json:"weakest,omitempty"`
	Rounds    int      `json:"rounds"`
	Warnings  []string `json:"warnings,omitempty"`
	Phase     string   `json:"phase"`
	Waived    bool     `json:"waived,omitempty"`
	Mutated   bool     `json:"-"` // a write happened (or would have, under --dry-run)
}

// Gate judges current_ambiguity ≤ threshold (exact equality passes).
// Passing moves interviewing → gate_passed (idempotent when already
// passed). waive records an early exit: interviewing → gate_waived with
// the warning persisted.
func (e *Engine) Gate(id string, waive bool, waiveReason string, dryRun bool) (*State, *GateResult, error) {
	var res *GateResult
	s, err := e.withResolved(id, dryRun, func(s *State) error {
		res = &GateResult{
			Ambiguity: s.CurrentAmbiguity, Threshold: s.Threshold,
			Gap: s.CurrentAmbiguity - s.Threshold, Rounds: len(s.Rounds),
			Warnings: roundWarnings(len(s.Rounds)),
		}
		if waive {
			if waiveReason == "" {
				return fmt.Errorf("%w: --waive requires --reason (the warning record)", ErrInterview)
			}
			if err := s.transition(PhaseGateWaived); err != nil {
				return err
			}
			s.GateWaiver = fmt.Sprintf("waived at round %d with ambiguity %.3f > threshold %.3f: %s",
				len(s.Rounds), s.CurrentAmbiguity, s.Threshold, waiveReason)
			if !dryRun {
				if err := e.save(s); err != nil {
					return err
				}
			}
			res.Phase, res.Waived, res.Mutated = s.Phase, true, true
			return nil
		}
		res.Pass = s.CurrentAmbiguity <= s.Threshold
		if res.Pass {
			if s.Phase == PhaseInterviewing {
				if err := s.transition(PhaseGatePassed); err != nil {
					return err
				}
				res.Mutated = true
				if !dryRun {
					if err := e.save(s); err != nil {
						return err
					}
				}
			} else if s.Phase != PhaseGatePassed {
				return fmt.Errorf("%w: gate is not legal in phase %s", ErrInterview, s.Phase)
			}
		} else {
			if s.Phase != PhaseInterviewing {
				return fmt.Errorf("%w: gate is not legal in phase %s", ErrInterview, s.Phase)
			}
			if last := s.lastRound(); last != nil {
				weakest := Target{Component: last.Component, Dimension: last.Dimension}
				if cs, ok := last.Scores[last.Component]; ok {
					weakest.Score = cs[last.Dimension]
				}
				res.Weakest = &weakest
			}
		}
		res.Phase = s.Phase
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return s, res, nil
}

func (s *State) lastRound() *Round {
	if len(s.Rounds) == 0 {
		return nil
	}
	return &s.Rounds[len(s.Rounds)-1]
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

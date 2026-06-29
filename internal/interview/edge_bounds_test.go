package interview

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// S13a edge/boundary pins (TEST-ONLY). These lock two numeric guards that the
// engine's correctness rests on, against off-by-one / sign-flip regressions:
//
//   - score.go scoreRound: `if v < 0 || v > 1` (the component-score range
//     guard) — out-of-range scores must fail closed before they reach the
//     ambiguity math.
//   - score.go Gate: `res.Pass = s.CurrentAmbiguity <= s.Threshold` (the gate
//     comparison) — exact equality passes; anything strictly above fails.

// TestComponentScoreBoundsAreValidated pins the range guard in
// (*Engine).scoreRound — the `if v < 0 || v > 1` check (score.go, ~line 213).
//
// Component scores are agent-supplied and feed straight into the per-dimension
// minimum / weighted-clarity math; a value outside [0,1] would silently corrupt
// ambiguity (e.g. a negative score can push clarity > 1 and ambiguity < 0, or a
// >1 score can mask a genuinely weak dimension). The guard must therefore
// fail-closed.
//
// fail-before rationale: delete `if v < 0 || v > 1 { ... }` at score.go ~213 and
// the out-of-range sub-tests below would be ACCEPTED (no error), letting bad
// scores into the ambiguity computation. Both inclusive endpoints (0.0 and 1.0)
// must stay accepted, so the boundary cannot be tightened to `<= 0 || >= 1`.
func TestComponentScoreBoundsAreValidated(t *testing.T) {
	// One active greenfield component → its own scores are the dimension minima,
	// so each (goal, constraints, criteria) triple is validated directly.
	score := func(t *testing.T, scores map[string]float64) error {
		t.Helper()
		e := testEngine(t)
		mustStart(t, e, "greenfield", 0.20)
		mustScore(t, e, topologyInput("a"))
		_, _, err := e.Score("t1", scoresInput(1, map[string]map[string]float64{"a": scores}), false)
		return err
	}

	t.Run("inclusive endpoints 0.0 and 1.0 are accepted", func(t *testing.T) {
		// Both extremes present in one round: criteria=0.0 (floor) and
		// goal=constraints=1.0 (ceiling) must all pass the [0,1] guard.
		if err := score(t, map[string]float64{"goal": 1.0, "constraints": 1.0, "criteria": 0.0}); err != nil {
			t.Fatalf("boundary values 0.0/1.0 rejected: %v", err)
		}
	})

	t.Run("below zero is rejected", func(t *testing.T) {
		for _, bad := range []float64{-0.0001, -0.2, -1.0} {
			err := score(t, map[string]float64{"goal": bad, "constraints": 0.5, "criteria": 0.5})
			if !errors.Is(err, ErrInterview) {
				t.Fatalf("score %.4f below zero: err = %v, want ErrInterview", bad, err)
			}
			if !strings.Contains(err.Error(), "outside [0,1]") {
				t.Fatalf("score %.4f: err = %q, want it to name the [0,1] range", bad, err)
			}
		}
	})

	t.Run("above one is rejected", func(t *testing.T) {
		for _, bad := range []float64{1.0001, 1.5, 2.0} {
			err := score(t, map[string]float64{"goal": 0.5, "constraints": 0.5, "criteria": bad})
			if !errors.Is(err, ErrInterview) {
				t.Fatalf("score %.4f above one: err = %v, want ErrInterview", bad, err)
			}
			if !strings.Contains(err.Error(), "outside [0,1]") {
				t.Fatalf("score %.4f: err = %q, want it to name the [0,1] range", bad, err)
			}
		}
	})
}

// TestGateThresholdBoundaryIsInclusive pins the gate comparison in
// (*Engine).Gate — `res.Pass = s.CurrentAmbiguity <= s.Threshold` (score.go,
// ~line 501): ambiguity EXACTLY at threshold passes, and ambiguity just above
// it fails.
//
// The scoring math (ambiguity = 1 - Σ weight·dim) cannot land on the threshold
// literal byte-for-byte (e.g. all dims 0.90 yields 0.0999…9866, not 0.10), so
// the existing TestGateExactEqualityPasses does not strictly distinguish `<`
// from `<=`. To pin the boundary deterministically this test drives the engine
// to PhaseInterviewing and then sets State.CurrentAmbiguity to the SAME float
// value as Threshold (byte-identical), exercising Gate's comparison directly —
// exactly the construction the slice brief calls for.
//
// fail-before rationale: change `<=` to `<` at score.go ~501 and the
// equal-to-threshold sub-test below would wrongly FAIL the gate (Pass==false,
// no transition to gate_passed). The just-above sub-test guards the other
// direction so the comparison cannot be loosened to `<=`+slack either.
func TestGateThresholdBoundaryIsInclusive(t *testing.T) {
	const threshold = 0.10 // deep-interview depth threshold

	// armInterview returns an engine whose interview sits in PhaseInterviewing
	// with one scored round persisted, then overwrites CurrentAmbiguity to amb
	// and Threshold to the fixed literal so Gate's comparison is the only
	// variable under test. Re-saving in-package via the unexported save keeps
	// the on-disk state valid for Resolve/Load.
	armInterview := func(t *testing.T, amb float64) *Engine {
		t.Helper()
		e := testEngine(t)
		mustStart(t, e, "greenfield", threshold)
		mustScore(t, e, topologyInput("a"))
		// Any legal round to populate Rounds (so the fail path has a weakest
		// target); its computed ambiguity is irrelevant — we overwrite below.
		mustScore(t, e, scoresInput(1, map[string]map[string]float64{
			"a": {"goal": 0.5, "constraints": 0.5, "criteria": 0.5},
		}))
		s, err := e.Load("t1")
		if err != nil {
			t.Fatal(err)
		}
		s.Threshold = threshold
		s.CurrentAmbiguity = amb
		if err := e.save(s); err != nil {
			t.Fatal(err)
		}
		return e
	}

	t.Run("ambiguity exactly at threshold passes", func(t *testing.T) {
		// Byte-identical to Threshold: `<=` holds, `<` would not.
		e := armInterview(t, threshold)
		_, res, err := e.Gate("t1", false, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Pass {
			t.Fatalf("ambiguity == threshold (%.17g) must pass: res = %+v", threshold, res)
		}
		if res.Phase != PhaseGatePassed {
			t.Fatalf("passing gate must transition to gate_passed, got %s", res.Phase)
		}
		if math.Abs(res.Gap) > 0 {
			t.Fatalf("gap at exact equality = %.17g, want 0", res.Gap)
		}
	})

	t.Run("ambiguity just above threshold fails", func(t *testing.T) {
		// Next representable double above the threshold: strictly > threshold,
		// so `<=` is false and the gate must refuse to pass.
		above := math.Nextafter(threshold, math.Inf(1))
		if !(above > threshold) {
			t.Fatalf("test setup: nextafter %.17g not above threshold %.17g", above, threshold)
		}
		e := armInterview(t, above)
		_, res, err := e.Gate("t1", false, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Pass {
			t.Fatalf("ambiguity just above threshold (%.17g > %.17g) must fail: res = %+v", above, threshold, res)
		}
		if res.Phase != PhaseInterviewing {
			t.Fatalf("failing gate must stay in interviewing, got %s", res.Phase)
		}
		if res.Gap <= 0 {
			t.Fatalf("gap just above threshold = %.17g, want > 0", res.Gap)
		}
		if res.Weakest == nil {
			t.Fatal("failing gate must report a weakest target")
		}
	})
}

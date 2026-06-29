package interview

import "testing"

// S4 / PHIL-1: the stall-escalation threshold (ambiguity flat across the last 3
// rounds) is now a binary verdict, not skill-prompt arithmetic.

func TestStalledDetection(t *testing.T) {
	mk := func(ambs ...float64) *State {
		s := &State{}
		for _, a := range ambs {
			s.Rounds = append(s.Rounds, Round{Ambiguity: a})
		}
		return s
	}
	cases := []struct {
		name string
		ambs []float64
		want bool
	}{
		{"no rounds (insufficient data)", nil, false},
		{"one round (insufficient data)", []float64{0.5}, false},
		{"two rounds (insufficient data)", []float64{0.5, 0.5}, false},
		{"plateaued within 0.05", []float64{0.62, 0.60, 0.59}, true},
		{"moved more than 0.05", []float64{0.62, 0.50, 0.48}, false},
		{"only the last 3 count", []float64{0.95, 0.10, 0.60, 0.59, 0.61}, true},
		{"recent jump breaks the plateau", []float64{0.60, 0.60, 0.60, 0.40}, false},
	}
	for _, c := range cases {
		if got := mk(c.ambs...).stalled(); got != c.want {
			t.Errorf("%s: stalled() = %v, want %v (ambs=%v)", c.name, got, c.want, c.ambs)
		}
	}
}

func TestStallEscalationInReport(t *testing.T) {
	// Identical scores → unchanged ambiguity → plateau once 3 rounds exist.
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.01) // unreachable threshold keeps interviewing
	mustScore(t, e, topologyInput("a"))
	same := map[string]map[string]float64{"a": {"goal": 0.5, "constraints": 0.5, "criteria": 0.5}}
	for r := 1; r <= 3; r++ {
		rep := mustScore(t, e, scoresInput(r, same))
		want := r >= 3 // needs 3 flat rounds before it counts as stalled
		if rep.StallEscalation != want {
			t.Fatalf("round %d StallEscalation = %v, want %v (ambiguity %.3f)", r, rep.StallEscalation, want, rep.Ambiguity)
		}
	}

	// Moving ambiguity (0.5 → 0.3 → 0.1) must not escalate.
	e2 := testEngine(t)
	mustStart(t, e2, "greenfield", 0.01)
	mustScore(t, e2, topologyInput("a"))
	var rep *Report
	for r, v := range []float64{0.5, 0.7, 0.9} {
		rep = mustScore(t, e2, scoresInput(r+1, map[string]map[string]float64{
			"a": {"goal": v, "constraints": v, "criteria": v},
		}))
	}
	if rep.StallEscalation {
		t.Fatalf("moving ambiguity must not escalate: StallEscalation=%v (ambiguity %.3f)", rep.StallEscalation, rep.Ambiguity)
	}
}

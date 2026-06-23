package interview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	e := NewEngine(filepath.Join(t.TempDir(), "state"))
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	n := 0
	e.Now = func() time.Time { n++; return base.Add(time.Duration(n) * time.Second) }
	return e
}

func mustStart(t *testing.T, e *Engine, typ string, threshold float64) *State {
	t.Helper()
	s, err := e.Start("t1", typ, threshold, "test", "idea", false, false)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func topologyInput(ids ...string) *ScoresInput {
	comps := make([]Component, len(ids))
	for i, id := range ids {
		comps[i] = Component{ID: id, Name: id, Description: id, Status: "active"}
	}
	return &ScoresInput{Schema: ScoresSchema, Round: 0, Topology: &TopologyInput{Components: comps}}
}

func scoresInput(round int, scores map[string]map[string]float64) *ScoresInput {
	return &ScoresInput{Schema: ScoresSchema, Round: round, ComponentScores: scores, Question: "q", Answer: "a"}
}

func mustScore(t *testing.T, e *Engine, in *ScoresInput) *Report {
	t.Helper()
	_, rep, err := e.Score("t1", in, false)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestSessionScopedDefaultIDAndOmittedResolve(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	e1 := NewEngine(dir)
	e1.SessionSuffix = "sess-a"
	e1.Now = func() time.Time { return base }
	e2 := NewEngine(dir)
	e2.SessionSuffix = "sess-b"
	e2.Now = func() time.Time { return base.Add(time.Second) }

	first, err := e1.Start("", "greenfield", 0.20, "test", "idea", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "sess-a" {
		t.Fatalf("default scoped id = %q", first.ID)
	}
	if _, err := e2.Start("feature", "greenfield", 0.20, "test", "idea", false, false); err != nil {
		t.Fatal(err)
	}
	got, err := e1.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != first.ID {
		t.Fatalf("omitted resolve = %q, want %q", got.ID, first.ID)
	}
	if _, err := e1.Resolve("feature"); err == nil {
		t.Fatal("explicit logical id must stay inside the current session")
	}
}

func TestConcurrentProcessScoresDoNotLoseRounds(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	e := NewEngine(dir)
	e.SessionSuffix = "same"
	e.Now = func() time.Time { return time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC) }
	if _, err := e.Start("case", "greenfield", 0.20, "test", "idea", false, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Score("case", topologyInput("a"), false); err != nil {
		t.Fatal(err)
	}

	start := filepath.Join(t.TempDir(), "start")
	workers := 8
	helperDeadline := 2 * time.Minute
	if runtime.GOOS == "windows" {
		// Windows hosted runners can starve helper processes behind directory-lock
		// polling under -race. Keep real cross-process coverage with lower fanout.
		workers = 2
		helperDeadline = 4 * time.Minute
	}
	type child struct {
		cmd *exec.Cmd
		out *bytes.Buffer
	}
	children := make([]child, 0, workers)
	for round := 1; round <= workers; round++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestInterviewScoreHelperProcess$")
		cmd.Env = append(os.Environ(),
			"OMA_INTERVIEW_HELPER=1",
			"OMA_INTERVIEW_DIR="+dir,
			"OMA_INTERVIEW_START="+start,
			"OMA_INTERVIEW_ROUND="+strconv.Itoa(round),
			"OMA_INTERVIEW_HELPER_DEADLINE_MS="+strconv.FormatInt(helperDeadline.Milliseconds(), 10),
		)
		out := &bytes.Buffer{}
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Start(); err != nil {
			t.Fatalf("start helper round %02d: %v", round, err)
		}
		children = append(children, child{cmd: cmd, out: out})
	}
	if err := os.WriteFile(start, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i, child := range children {
		if err := child.cmd.Wait(); err != nil {
			t.Fatalf("helper %02d: %v\n%s", i+1, err, child.out.String())
		}
	}

	got, err := e.Load("case--s-same")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rounds) != workers {
		t.Fatalf("rounds = %d, want %d: %+v", len(got.Rounds), workers, got.Rounds)
	}
	if got.Revision != int64(2+workers) {
		t.Fatalf("revision = %d, want %d", got.Revision, 2+workers)
	}
	for i, round := range got.Rounds {
		if round.Round != i+1 {
			t.Fatalf("round[%d] = %d, want %d", i, round.Round, i+1)
		}
	}
}

func TestInterviewScoreHelperProcess(t *testing.T) {
	if os.Getenv("OMA_INTERVIEW_HELPER") != "1" {
		return
	}
	waitForFile := func(path string) {
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(path); err == nil {
				return
			} else if !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			if time.Now().After(deadline) {
				fmt.Fprintln(os.Stderr, "timed out waiting for start file")
				os.Exit(2)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	round, err := strconv.Atoi(os.Getenv("OMA_INTERVIEW_ROUND"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	helperDeadline := 2 * time.Minute
	if raw := os.Getenv("OMA_INTERVIEW_HELPER_DEADLINE_MS"); raw != "" {
		ms, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ms <= 0 {
			fmt.Fprintf(os.Stderr, "invalid OMA_INTERVIEW_HELPER_DEADLINE_MS=%q\n", raw)
			os.Exit(2)
		}
		helperDeadline = time.Duration(ms) * time.Millisecond
	}
	waitForFile(os.Getenv("OMA_INTERVIEW_START"))
	e := NewEngine(os.Getenv("OMA_INTERVIEW_DIR"))
	e.SessionSuffix = "same"
	in := scoresInput(round, map[string]map[string]float64{
		"a": {"goal": 0.5, "constraints": 0.5, "criteria": 0.5},
	})
	deadline := time.Now().Add(helperDeadline)
	for {
		if _, _, err := e.Score("case", in, false); err == nil {
			os.Exit(0)
		} else if roundPersisted(e, round) {
			os.Exit(0)
		} else if (!strings.Contains(err.Error(), "replays and skips are refused") && !strings.Contains(err.Error(), "being mutated by another process")) || time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		time.Sleep(time.Duration(20+round*5+os.Getpid()%7) * time.Millisecond)
	}
}

func roundPersisted(e *Engine, round int) bool {
	s, err := e.Load("case--s-same")
	if err != nil || len(s.Rounds) < round {
		return false
	}
	return s.Rounds[round-1].Round == round
}

func TestGreenfieldAmbiguityFormula(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	mustScore(t, e, topologyInput("a", "b"))

	// dimension totals = min across components:
	// goal min(0.8,0.6)=0.6  constraints min(0.5,0.9)=0.5  criteria min(0.7,0.4)=0.4
	// clarity = .40*.6 + .30*.5 + .30*.4 = .24+.15+.12 = .51 → ambiguity .49
	rep := mustScore(t, e, scoresInput(1, map[string]map[string]float64{
		"a": {"goal": 0.8, "constraints": 0.5, "criteria": 0.7},
		"b": {"goal": 0.6, "constraints": 0.9, "criteria": 0.4},
	}))
	if math.Abs(rep.Ambiguity-0.49) > 1e-9 {
		t.Fatalf("ambiguity = %.6f, want 0.49", rep.Ambiguity)
	}
	if rep.Weakest.Component != "b" || rep.Weakest.Dimension != "criteria" {
		t.Fatalf("weakest = %+v, want b×criteria", rep.Weakest)
	}
}

func TestBrownfieldAddsContextDimension(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "brownfield", 0.20)
	mustScore(t, e, topologyInput("a"))

	// missing context must fail closed
	_, _, err := e.Score("t1", scoresInput(1, map[string]map[string]float64{
		"a": {"goal": 1, "constraints": 1, "criteria": 1},
	}), false)
	if err == nil || !strings.Contains(err.Error(), `"context"`) {
		t.Fatalf("missing context: err = %v", err)
	}
	// clarity = .35*1 + .25*1 + .25*1 + .15*0.2 = .88 → ambiguity .12
	rep := mustScore(t, e, scoresInput(1, map[string]map[string]float64{
		"a": {"goal": 1, "constraints": 1, "criteria": 1, "context": 0.2},
	}))
	if math.Abs(rep.Ambiguity-0.12) > 1e-9 {
		t.Fatalf("ambiguity = %.6f, want 0.12", rep.Ambiguity)
	}
}

func TestGateExactEqualityPasses(t *testing.T) {
	// review 058 guardrail: pin the boundary — ambiguity == threshold is ≤.
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	mustScore(t, e, topologyInput("a"))
	// all dims 0.8 → clarity 0.8 → ambiguity exactly 0.2
	mustScore(t, e, scoresInput(1, map[string]map[string]float64{
		"a": {"goal": 0.8, "constraints": 0.8, "criteria": 0.8},
	}))
	_, res, err := e.Gate("t1", false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pass || res.Phase != PhaseGatePassed || math.Abs(res.Gap) > 1e-9 {
		t.Fatalf("exact-equality gate = %+v, want pass", res)
	}
	// Gate is idempotent once passed.
	if _, res, err = e.Gate("t1", false, "", false); err != nil || !res.Pass {
		t.Fatalf("repeat gate = %+v err=%v", res, err)
	}
}

func TestGateFailsWithMachineReadableReason(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.10)
	mustScore(t, e, topologyInput("a"))
	mustScore(t, e, scoresInput(1, map[string]map[string]float64{
		"a": {"goal": 0.5, "constraints": 0.5, "criteria": 0.5},
	}))
	_, res, err := e.Gate("t1", false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pass || res.Weakest == nil || res.Gap <= 0 {
		t.Fatalf("failing gate = %+v, want weakest+positive gap", res)
	}
	raw, _ := json.Marshal(res)
	for _, key := range []string{`"pass"`, `"gap"`, `"weakest"`, `"threshold"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("gate JSON missing %s: %s", key, raw)
		}
	}
}

func TestGateWaiveRecordsWarning(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.10)
	mustScore(t, e, topologyInput("a"))
	mustScore(t, e, scoresInput(1, map[string]map[string]float64{
		"a": {"goal": 0.5, "constraints": 0.5, "criteria": 0.5},
	}))
	if _, _, err := e.Gate("t1", true, "", false); err == nil {
		t.Fatal("waive without reason must refuse")
	}
	s, res, err := e.Gate("t1", true, "user accepts the risk", false)
	if err != nil || !res.Waived || s.Phase != PhaseGateWaived {
		t.Fatalf("waive = %+v err=%v", res, err)
	}
	if !strings.Contains(s.GateWaiver, "user accepts the risk") || !strings.Contains(s.GateWaiver, "0.500") {
		t.Fatalf("waiver record = %q", s.GateWaiver)
	}
}

func TestIllegalTransitionsTable(t *testing.T) {
	// review 058 guardrail: every illegal edge refuses, table-driven.
	type op func(e *Engine) error
	score := func(e *Engine) error {
		_, _, err := e.Score("t1", scoresInput(99, map[string]map[string]float64{"a": {"goal": 1, "constraints": 1, "criteria": 1}}), false)
		return err
	}
	topo := func(e *Engine) error { _, _, err := e.Score("t1", topologyInput("z"), false); return err }
	gate := func(e *Engine) error { _, _, err := e.Gate("t1", false, "", false); return err }
	crystallize := func(e *Engine) error {
		spec := filepath.Join(os.TempDir(), "spec.md")
		_ = os.WriteFile(spec, []byte("s"), 0o600)
		_, err := e.Crystallize("t1", spec, false)
		return err
	}
	complete := func(e *Engine) error { _, err := e.Complete("t1", false); return err }
	abort := func(e *Engine) error { _, err := e.Abort("t1", false); return err }

	// drive an interview to a given phase
	advanceTo := func(t *testing.T, e *Engine, phase string) {
		t.Helper()
		mustStart(t, e, "greenfield", 0.20)
		if phase == PhaseTopologyPending {
			return
		}
		mustScore(t, e, topologyInput("a"))
		if phase == PhaseInterviewing {
			return
		}
		mustScore(t, e, scoresInput(1, map[string]map[string]float64{
			"a": {"goal": 0.9, "constraints": 0.9, "criteria": 0.9},
		}))
		if _, _, err := e.Gate("t1", false, "", false); err != nil {
			t.Fatal(err)
		}
		if phase == PhaseGatePassed {
			return
		}
		spec := filepath.Join(t.TempDir(), "spec.md")
		if err := os.WriteFile(spec, []byte("s"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Crystallize("t1", spec, false); err != nil {
			t.Fatal(err)
		}
		if phase == PhaseCrystallized {
			return
		}
		if _, err := e.Complete("t1", false); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		phase string
		op    string
		fn    op
	}{
		{PhaseTopologyPending, "gate-pass-attempt", gate}, // ambiguity 1.0 > threshold but phase is wrong too
		{PhaseTopologyPending, "crystallize", crystallize},
		{PhaseTopologyPending, "complete", complete},
		{PhaseInterviewing, "topology-again", topo},
		{PhaseInterviewing, "crystallize", crystallize},
		{PhaseInterviewing, "complete", complete},
		{PhaseGatePassed, "score", score},
		{PhaseGatePassed, "complete", complete},
		{PhaseCrystallized, "score", score},
		{PhaseCrystallized, "crystallize", crystallize},
		{PhaseCompleted, "score", score},
		{PhaseCompleted, "gate", gate},
		{PhaseCompleted, "crystallize", crystallize},
		{PhaseCompleted, "complete", complete},
		{PhaseCompleted, "abort", abort},
	}
	for _, tc := range cases {
		t.Run(tc.phase+"/"+tc.op, func(t *testing.T) {
			e := testEngine(t)
			advanceTo(t, e, tc.phase)
			if err := tc.fn(e); !errors.Is(err, ErrInterview) {
				t.Fatalf("phase %s op %s: err = %v, want ErrInterview", tc.phase, tc.op, err)
			}
		})
	}
}

func TestRoundReplayAndSkipRefused(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	mustScore(t, e, topologyInput("a"))
	in := scoresInput(1, map[string]map[string]float64{"a": {"goal": 0.5, "constraints": 0.5, "criteria": 0.5}})
	mustScore(t, e, in)
	if _, _, err := e.Score("t1", in, false); err == nil || !strings.Contains(err.Error(), "expected 2") {
		t.Fatalf("replay round 1: err = %v", err)
	}
	if _, _, err := e.Score("t1", scoresInput(5, in.ComponentScores), false); err == nil {
		t.Fatal("skipping rounds must refuse")
	}
}

func TestRotationAvoidsLastTargeted(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	mustScore(t, e, topologyInput("a", "b"))
	// equal weakness on both components' goal: first round targets a.
	equal := map[string]map[string]float64{
		"a": {"goal": 0.2, "constraints": 0.9, "criteria": 0.9},
		"b": {"goal": 0.2, "constraints": 0.9, "criteria": 0.9},
	}
	rep := mustScore(t, e, scoresInput(1, equal))
	if rep.Weakest.Component != "a" {
		t.Fatalf("round 1 weakest = %+v", rep.Weakest)
	}
	// same tie next round: rotation must move off a.
	rep = mustScore(t, e, scoresInput(2, equal))
	if rep.Weakest.Component != "b" || !rep.RotationApplied {
		t.Fatalf("round 2 weakest = %+v rotation=%v, want b with rotation", rep.Weakest, rep.RotationApplied)
	}
}

func TestOntologyStability(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	mustScore(t, e, topologyInput("a"))
	scores := map[string]map[string]float64{"a": {"goal": 0.5, "constraints": 0.5, "criteria": 0.5}}

	in1 := scoresInput(1, scores)
	in1.Ontology = &OntologyInput{Entities: []Entity{
		{Name: "Task", Type: "entity", Fields: []string{"id", "title", "due"}},
		{Name: "Project", Type: "entity", Fields: []string{"id", "name"}},
	}}
	rep := mustScore(t, e, in1)
	if rep.OntologyStability != nil {
		t.Fatalf("first snapshot must be N/A, got %v", *rep.OntologyStability)
	}
	// Round 2: Task stable; Project renamed to Workspace (same type,
	// fields {id,name} overlap 2/2 > 50%); Tag is new.
	in2 := scoresInput(2, scores)
	in2.Ontology = &OntologyInput{Entities: []Entity{
		{Name: "Task", Type: "entity", Fields: []string{"id", "title", "due"}},
		{Name: "Workspace", Type: "entity", Fields: []string{"id", "name"}},
		{Name: "Tag", Type: "entity", Fields: []string{"label"}},
	}}
	rep = mustScore(t, e, in2)
	if rep.OntologyStability == nil || math.Abs(*rep.OntologyStability-2.0/3.0) > 1e-9 {
		t.Fatalf("stability = %v, want 2/3", rep.OntologyStability)
	}
}

func TestChallengeSuggestionTriggers(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.01) // unreachable threshold keeps interviewing
	mustScore(t, e, topologyInput("a"))
	low := map[string]map[string]float64{"a": {"goal": 0.5, "constraints": 0.5, "criteria": 0.5}}
	var rep *Report
	for r := 1; r <= 8; r++ {
		in := scoresInput(r, low)
		if r == 5 {
			in.ChallengeModeUsed = "contrarian"
		}
		rep = mustScore(t, e, in)
		switch r {
		case 3:
			if len(rep.ChallengeSuggestions) != 0 {
				t.Fatalf("round 3 suggestions = %v", rep.ChallengeSuggestions)
			}
		case 4:
			if !contains(rep.ChallengeSuggestions, "contrarian") {
				t.Fatalf("round 4 must suggest contrarian: %v", rep.ChallengeSuggestions)
			}
		case 5:
			if contains(rep.ChallengeSuggestions, "contrarian") {
				t.Fatalf("used contrarian must not re-suggest: %v", rep.ChallengeSuggestions)
			}
		case 6:
			if !contains(rep.ChallengeSuggestions, "simplifier") {
				t.Fatalf("round 6 must suggest simplifier: %v", rep.ChallengeSuggestions)
			}
		case 8:
			if !contains(rep.ChallengeSuggestions, "ontologist") { // ambiguity 0.5 > 0.3
				t.Fatalf("round 8 must suggest ontologist: %v", rep.ChallengeSuggestions)
			}
		}
	}
	// ontologist requires ambiguity > 0.3: clear rounds suppress it.
	e2 := testEngine(t)
	mustStart(t, e2, "greenfield", 0.01)
	mustScore(t, e2, topologyInput("a"))
	clear := map[string]map[string]float64{"a": {"goal": 0.9, "constraints": 0.9, "criteria": 0.9}}
	for r := 1; r <= 8; r++ {
		rep = mustScore(t, e2, scoresInput(r, clear))
	}
	if contains(rep.ChallengeSuggestions, "ontologist") {
		t.Fatalf("ambiguity 0.1 must suppress ontologist: %v", rep.ChallengeSuggestions)
	}
}

func TestRoundGuardWarnings(t *testing.T) {
	if w := roundWarnings(9); len(w) != 0 {
		t.Fatalf("9 rounds: %v", w)
	}
	if w := roundWarnings(10); len(w) != 1 || !strings.Contains(w[0], "soft") {
		t.Fatalf("10 rounds: %v", w)
	}
	if w := roundWarnings(20); len(w) != 1 || !strings.Contains(w[0], "hard cap") {
		t.Fatalf("20 rounds: %v", w)
	}
}

func TestStartRefusesExistingUnlessResume(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	if _, err := e.Start("t1", "greenfield", 0.20, "test", "idea", false, false); !errors.Is(err, ErrInterview) {
		t.Fatalf("duplicate start: err = %v", err)
	}
	s, err := e.Start("t1", "greenfield", 0.99, "other", "ignored", true, false)
	if err != nil || s.Threshold != 0.20 {
		t.Fatalf("resume must load untouched state: %+v err=%v", s, err)
	}
}

func TestCorruptStateFailsClosedWithBak(t *testing.T) {
	e := testEngine(t)
	s := mustStart(t, e, "greenfield", 0.20)
	mustScore(t, e, topologyInput("a")) // second save → .bak exists
	path := filepath.Join(e.Dir, "interview-"+s.ID+".json")
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatal(".bak missing after overwrite")
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Load("t1"); err == nil || !strings.Contains(err.Error(), ".bak") {
		t.Fatalf("corrupt state: err = %v, want backup hint", err)
	}
	if err := os.WriteFile(path, []byte(`{"schema":"oma-interview/2","id":"t1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Load("t1"); err == nil || !strings.Contains(err.Error(), "oma-interview/2") {
		t.Fatalf("unknown major: err = %v", err)
	}
}

func TestResolveAmbiguityRefused(t *testing.T) {
	e := testEngine(t)
	if _, err := e.Start("a1", "greenfield", 0.2, "t", "i", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Start("a2", "greenfield", 0.2, "t", "i", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resolve(""); err == nil || !strings.Contains(err.Error(), "a1, a2") {
		t.Fatalf("ambiguous resolve: err = %v", err)
	}
}

func TestDeferredComponentsExcludedFromMath(t *testing.T) {
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	in := topologyInput("a")
	in.Topology.Components = append(in.Topology.Components, Component{ID: "later", Name: "later", Description: "d", Status: "deferred"})
	in.Topology.Deferrals = []Deferral{{ComponentID: "later", Reason: "user deferred"}}
	mustScore(t, e, in)

	// scoring the deferred component is refused; omitting it is fine.
	if _, _, err := e.Score("t1", scoresInput(1, map[string]map[string]float64{
		"a":     {"goal": 1, "constraints": 1, "criteria": 1},
		"later": {"goal": 0, "constraints": 0, "criteria": 0},
	}), false); err == nil || !strings.Contains(err.Error(), "deferred") {
		t.Fatalf("scoring deferred: err = %v", err)
	}
	rep := mustScore(t, e, scoresInput(1, map[string]map[string]float64{
		"a": {"goal": 1, "constraints": 1, "criteria": 1},
	}))
	if rep.Ambiguity != 0 {
		t.Fatalf("deferred component leaked into math: %.3f", rep.Ambiguity)
	}
}

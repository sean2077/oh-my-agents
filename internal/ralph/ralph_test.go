package ralph

import (
	"errors"
	"os"
	"path/filepath"
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

func mustStartLoop(t *testing.T, e *Engine, maxRounds, stallWindow int) *State {
	t.Helper()
	s, err := e.Start("r1", "make tests pass", maxRounds, stallWindow)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStartRequiresGoal(t *testing.T) {
	e := testEngine(t)
	if _, err := e.Start("r1", "  ", 10, 3); !errors.Is(err, ErrRalph) {
		t.Fatalf("blank goal: err = %v", err)
	}
}

func TestExhaustionBoundaryExact(t *testing.T) {
	// review 058 guardrail: pin the boundary — round == max continues,
	// max+1 exhausts.
	e := testEngine(t)
	mustStartLoop(t, e, 3, 3)
	for i := 1; i <= 3; i++ {
		_, v, err := e.Next("r1")
		if err != nil || !v.Continue || v.Round != i {
			t.Fatalf("round %d: %+v err=%v", i, v, err)
		}
	}
	_, v, err := e.Next("r1")
	if err != nil || v.Continue || v.Phase != PhaseExhausted {
		t.Fatalf("round 4 on max 3: %+v err=%v", v, err)
	}
	// Idempotent: next on a terminal loop reports stop without advancing.
	s, v2, err := e.Next("r1")
	if err != nil || v2.Continue || s.Round != 4 || s.Phase != PhaseExhausted {
		t.Fatalf("repeat next on terminal: %+v state=%+v err=%v", v2, s, err)
	}
}

func TestCheckPassStallAndIllegal(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	if _, _, err := e.Next("r1"); err != nil {
		t.Fatal(err)
	}

	// two same-signature failures: still running
	for i := 0; i < 2; i++ {
		_, v, err := e.RecordCheck("r1", 1, "TestFoo fails")
		if err != nil || !v.Continue {
			t.Fatalf("failure %d: %+v err=%v", i+1, v, err)
		}
	}
	// a different signature resets the window
	if _, v, _ := e.RecordCheck("r1", 1, "TestBar fails"); v.Phase != PhaseRunning {
		t.Fatalf("signature change: %+v", v)
	}
	// three consecutive same signatures → stalled
	for i := 0; i < 2; i++ {
		if _, v, err := e.RecordCheck("r1", 1, "TestBar fails"); i < 1 && (err != nil || v.Phase != PhaseRunning) {
			t.Fatalf("stall buildup: %+v err=%v", v, err)
		}
	}
	s, _ := e.Load("r1")
	if s.Phase != PhaseStalled {
		t.Fatalf("phase = %s, want stalled", s.Phase)
	}
	// check on a terminal loop is an illegal transition
	if _, _, err := e.RecordCheck("r1", 1, "x"); !errors.Is(err, ErrRalph) {
		t.Fatalf("check on terminal: err = %v", err)
	}
	// next on terminal: stop verdict, unchanged
	if _, v, err := e.Next("r1"); err != nil || v.Continue {
		t.Fatalf("next on stalled: %+v err=%v", v, err)
	}
}

func TestVerifierExitZeroPasses(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	_, _, _ = e.Next("r1")
	_, v, err := e.RecordCheck("r1", 0, "")
	if err != nil || v.Phase != PhasePassed || v.Continue {
		t.Fatalf("pass: %+v err=%v", v, err)
	}
}

func TestEmptyNoteFailuresNeverStall(t *testing.T) {
	// stall detection keys on the failure signature; without one the
	// loop must keep running (no false stall on heterogeneous failures).
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	_, _, _ = e.Next("r1")
	for i := 0; i < 5; i++ {
		_, v, err := e.RecordCheck("r1", 1, "")
		if err != nil || v.Phase != PhaseRunning {
			t.Fatalf("noteless failure %d: %+v err=%v", i, v, err)
		}
	}
}

func TestAbortAndIllegalAbort(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	s, err := e.Abort("r1")
	if err != nil || s.Phase != PhaseAborted {
		t.Fatalf("abort: %+v err=%v", s, err)
	}
	if _, err := e.Abort("r1"); !errors.Is(err, ErrRalph) {
		t.Fatalf("double abort: err = %v", err)
	}
}

func TestCorruptStateFailsClosed(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	_, _, _ = e.Next("r1") // second save → .bak
	path := filepath.Join(e.Dir, "ralph-r1.json")
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatal(".bak missing")
	}
	if err := os.WriteFile(path, []byte(`{"schema":"oma-ralph/9","id":"r1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Load("r1"); err == nil || !strings.Contains(err.Error(), "oma-ralph/9") {
		t.Fatalf("unknown major: err = %v", err)
	}
}

func TestResolveSingleActive(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	s, err := e.Resolve("")
	if err != nil || s.ID != "r1" {
		t.Fatalf("resolve: %+v err=%v", s, err)
	}
	if _, err := e.Start("r2", "second goal", 10, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resolve(""); err == nil || !strings.Contains(err.Error(), "r1, r2") {
		t.Fatalf("ambiguous: err = %v", err)
	}
}

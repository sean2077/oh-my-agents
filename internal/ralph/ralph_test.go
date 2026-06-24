package ralph

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	s, err := e.Start("r1", StartOpts{Goal: "make tests pass", MaxRounds: maxRounds, StallWindow: stallWindow}, false)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustStartScore(t *testing.T, e *Engine, maxRounds, plateauWindow int) *State {
	t.Helper()
	s, err := e.Start("r1", StartOpts{Goal: "maximize score", KeepPolicy: KeepScoreImprovement, MaxRounds: maxRounds, PlateauWindow: plateauWindow}, false)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func fptr(f float64) *float64 { return &f }

func TestSessionScopedDefaultIDAndOmittedResolve(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	e1 := NewEngine(dir)
	e1.SessionSuffix = "sess-a"
	e1.Now = func() time.Time { return base }
	e2 := NewEngine(dir)
	e2.SessionSuffix = "sess-b"
	e2.Now = func() time.Time { return base.Add(time.Second) }

	first, err := e1.Start("", StartOpts{Goal: "g"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "sess-a" {
		t.Fatalf("default scoped id = %q", first.ID)
	}
	if _, err := e2.Start("feature", StartOpts{Goal: "other"}, false); err != nil {
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

func TestConcurrentProcessChecksDoNotLoseRecords(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	e := NewEngine(dir)
	e.SessionSuffix = "same"
	e.Now = func() time.Time { return time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC) }
	if _, err := e.Start("loop", StartOpts{Goal: "record every check", MaxRounds: 64}, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Next("loop", false); err != nil {
		t.Fatal(err)
	}

	start := filepath.Join(t.TempDir(), "start")
	workers := 16
	if runtime.GOOS == "windows" {
		workers = 4
	}
	type child struct {
		cmd *exec.Cmd
		out *bytes.Buffer
	}
	children := make([]child, 0, workers)
	for i := 0; i < workers; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRalphCheckHelperProcess$")
		cmd.Env = append(os.Environ(),
			"OMA_RALPH_HELPER=1",
			"OMA_RALPH_DIR="+dir,
			"OMA_RALPH_START="+start,
			"OMA_RALPH_NOTE="+fmt.Sprintf("sig-%02d", i),
		)
		out := &bytes.Buffer{}
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Start(); err != nil {
			t.Fatalf("start helper %02d: %v", i, err)
		}
		children = append(children, child{cmd: cmd, out: out})
	}
	if err := os.WriteFile(start, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i, child := range children {
		if err := child.cmd.Wait(); err != nil {
			t.Fatalf("helper %02d: %v\n%s", i, err, child.out.String())
		}
	}

	got, err := e.Load("loop--s-same")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Checks) != workers {
		t.Fatalf("checks = %d, want %d: %+v", len(got.Checks), workers, got.Checks)
	}
	if got.Revision != int64(2+workers) {
		t.Fatalf("revision = %d, want %d", got.Revision, 2+workers)
	}
	seen := map[string]bool{}
	for _, c := range got.Checks {
		seen[c.Note] = true
	}
	for i := 0; i < workers; i++ {
		note := fmt.Sprintf("sig-%02d", i)
		if !seen[note] {
			t.Fatalf("missing check note %s in %+v", note, got.Checks)
		}
	}
}

func TestRalphCheckHelperProcess(t *testing.T) {
	if os.Getenv("OMA_RALPH_HELPER") != "1" {
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
	waitForFile(os.Getenv("OMA_RALPH_START"))
	e := NewEngine(os.Getenv("OMA_RALPH_DIR"))
	e.SessionSuffix = "same"
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if _, _, err := e.RecordCheck("loop", 1, nil, os.Getenv("OMA_RALPH_NOTE"), false); err == nil {
			os.Exit(0)
		} else if !strings.Contains(err.Error(), "being mutated by another process") || time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartRequiresGoal(t *testing.T) {
	e := testEngine(t)
	if _, err := e.Start("r1", StartOpts{Goal: "  "}, false); !errors.Is(err, ErrRalph) {
		t.Fatalf("blank goal: err = %v", err)
	}
}

func TestKeepPolicyValidation(t *testing.T) {
	e := testEngine(t)
	if _, err := e.Start("r1", StartOpts{Goal: "x", KeepPolicy: "bogus"}, false); !errors.Is(err, ErrRalph) {
		t.Fatalf("bad keep-policy: err = %v", err)
	}
	// empty keep-policy normalizes to pass_only.
	s, err := e.Start("r2", StartOpts{Goal: "x"}, false)
	if err != nil || s.KeepPolicy != KeepPassOnly {
		t.Fatalf("default keep-policy: %+v err=%v", s, err)
	}
}

func TestExhaustionBoundaryExact(t *testing.T) {
	// review 058 guardrail: pin the boundary — round == max continues,
	// max+1 exhausts.
	e := testEngine(t)
	mustStartLoop(t, e, 3, 3)
	for i := 1; i <= 3; i++ {
		_, v, err := e.Next("r1", false)
		if err != nil || !v.Continue || v.Round != i {
			t.Fatalf("round %d: %+v err=%v", i, v, err)
		}
	}
	_, v, err := e.Next("r1", false)
	if err != nil || v.Continue || v.Phase != PhaseExhausted {
		t.Fatalf("round 4 on max 3: %+v err=%v", v, err)
	}
	// Idempotent: next on a terminal loop reports stop without advancing.
	s, v2, err := e.Next("r1", false)
	if err != nil || v2.Continue || s.Round != 4 || s.Phase != PhaseExhausted {
		t.Fatalf("repeat next on terminal: %+v state=%+v err=%v", v2, s, err)
	}
}

func TestCheckPassStallAndIllegal(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	if _, _, err := e.Next("r1", false); err != nil {
		t.Fatal(err)
	}

	// two same-signature failures: still running
	for i := 0; i < 2; i++ {
		_, v, err := e.RecordCheck("r1", 1, nil, "TestFoo fails", false)
		if err != nil || !v.Continue {
			t.Fatalf("failure %d: %+v err=%v", i+1, v, err)
		}
	}
	// a different signature resets the window
	if _, v, _ := e.RecordCheck("r1", 1, nil, "TestBar fails", false); v.Phase != PhaseRunning {
		t.Fatalf("signature change: %+v", v)
	}
	// three consecutive same signatures → stalled
	for i := 0; i < 2; i++ {
		if _, v, err := e.RecordCheck("r1", 1, nil, "TestBar fails", false); i < 1 && (err != nil || v.Phase != PhaseRunning) {
			t.Fatalf("stall buildup: %+v err=%v", v, err)
		}
	}
	s, _ := e.Load("r1")
	if s.Phase != PhaseStalled {
		t.Fatalf("phase = %s, want stalled", s.Phase)
	}
	// check on a terminal loop is an illegal transition
	if _, _, err := e.RecordCheck("r1", 1, nil, "x", false); !errors.Is(err, ErrRalph) {
		t.Fatalf("check on terminal: err = %v", err)
	}
	// next on terminal: stop verdict, unchanged
	if _, v, err := e.Next("r1", false); err != nil || v.Continue {
		t.Fatalf("next on stalled: %+v err=%v", v, err)
	}
}

func TestVerifierExitZeroPasses(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	_, _, _ = e.Next("r1", false)
	s, v, err := e.RecordCheck("r1", 0, nil, "", false)
	if err != nil || v.Phase != PhasePassed || v.Continue {
		t.Fatalf("pass: %+v err=%v", v, err)
	}
	// pass_only earns a reproducible receipt (Score stays nil → stable bytes).
	if s.Receipt == "" || ralphReceipt(s) != s.Receipt {
		t.Fatalf("pass_only receipt: %q", s.Receipt)
	}
}

func TestEmptyNoteFailuresNeverStall(t *testing.T) {
	// stall detection keys on the failure signature; without one the
	// loop must keep running (no false stall on heterogeneous failures).
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	_, _, _ = e.Next("r1", false)
	for i := 0; i < 5; i++ {
		_, v, err := e.RecordCheck("r1", 1, nil, "", false)
		if err != nil || v.Phase != PhaseRunning {
			t.Fatalf("noteless failure %d: %+v err=%v", i, v, err)
		}
	}
}

func TestScoreImprovementStrictAndPlateau(t *testing.T) {
	// score_improvement: strict-best tracking, ties do not improve, and a
	// plateau_window run of non-improving rounds is terminal with a receipt
	// whose terminal_check is the best-score check (not the last one).
	e := testEngine(t)
	mustStartScore(t, e, 10, 2)
	// best (7) lands at round 2, then declines/ties into a plateau at round 4.
	want := []struct {
		score float64
		phase string
	}{
		{5, PhaseRunning},   // r1: best 5@1
		{7, PhaseRunning},   // r2: best 7@2 (strict improvement)
		{7, PhaseRunning},   // r3: tie, no improvement (1 non-improving round)
		{5, PhasePlateaued}, // r4: 2 non-improving rounds → plateau
	}
	var last *State
	for i, w := range want {
		if _, _, err := e.Next("r1", false); err != nil {
			t.Fatal(err)
		}
		st, v, err := e.RecordCheck("r1", 1, fptr(w.score), "", false)
		if err != nil {
			t.Fatalf("r%d: %v", i+1, err)
		}
		if v.Phase != w.phase {
			t.Fatalf("r%d phase = %s, want %s", i+1, v.Phase, w.phase)
		}
		last = st
	}
	if last.BestRound != 2 || last.BestScore == nil || *last.BestScore != 7 {
		t.Fatalf("best = round %d score %v (tie must not update best)", last.BestRound, last.BestScore)
	}
	if last.Receipt == "" || ralphReceipt(last) != last.Receipt {
		t.Fatalf("plateau receipt missing/irreproducible: %q vs %q", last.Receipt, ralphReceipt(last))
	}
	if bc := bestScoreCheck(last); bc == nil || bc.Score == nil || *bc.Score != 7 {
		t.Fatalf("receipt terminal must be best-score check, got %+v", bc)
	}
}

func TestCheckRefusedBeforeFirstNext(t *testing.T) {
	// A check measures a round's work; recording one before the first `next`
	// (round 0) is refused. This keeps round-based plateau accounting honest —
	// a best score can never anchor at round 0 and defeat the plateau stop.
	e := testEngine(t)
	mustStartScore(t, e, 10, 2)
	if _, _, err := e.RecordCheck("r1", 1, fptr(5), "", false); !errors.Is(err, ErrRalph) {
		t.Fatalf("check before first next must be refused, got err = %v", err)
	}
	// pass_only loops too: no round-0 check.
	e2 := testEngine(t)
	mustStartLoop(t, e2, 10, 3)
	if _, _, err := e2.RecordCheck("r1", 1, nil, "boom", false); !errors.Is(err, ErrRalph) {
		t.Fatalf("pass_only check before first next must be refused, got err = %v", err)
	}
}

func TestScoreValidationFailClosed(t *testing.T) {
	e := testEngine(t)
	mustStartScore(t, e, 10, 3)
	_, _, _ = e.Next("r1", false)
	// missing score under score_improvement is refused
	if _, _, err := e.RecordCheck("r1", 1, nil, "", false); !errors.Is(err, ErrRalph) {
		t.Fatalf("missing score: err = %v", err)
	}
	// non-finite scores are refused
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, _, err := e.RecordCheck("r1", 1, fptr(bad), "", false); !errors.Is(err, ErrRalph) {
			t.Fatalf("non-finite %v: err = %v", bad, err)
		}
	}
}

func TestScoreRejectedUnderPassOnly(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	_, _, _ = e.Next("r1", false)
	if _, _, err := e.RecordCheck("r1", 1, fptr(5), "", false); !errors.Is(err, ErrRalph) {
		t.Fatalf("score under pass_only must be refused: err = %v", err)
	}
}

func TestScoreExhaustionEarnsReceipt(t *testing.T) {
	// score_improvement: exhaustion is a meaningful terminal (the kept best is
	// the deliverable), so it earns a receipt even without a pass or plateau.
	e := testEngine(t)
	mustStartScore(t, e, 2, 100) // plateau window huge → exhaust first
	for i := 1; i <= 2; i++ {
		_, _, _ = e.Next("r1", false)
		if _, _, err := e.RecordCheck("r1", 1, fptr(float64(i)), "", false); err != nil {
			t.Fatal(err)
		}
	}
	s, v, err := e.Next("r1", false) // round 3 > max 2
	if err != nil || v.Continue || s.Phase != PhaseExhausted {
		t.Fatalf("exhaust: %+v err=%v", v, err)
	}
	if s.Receipt == "" || ralphReceipt(s) != s.Receipt {
		t.Fatalf("exhaustion receipt missing/irreproducible: %q", s.Receipt)
	}
}

func TestAbortAndIllegalAbort(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	s, err := e.Abort("r1", false)
	if err != nil || s.Phase != PhaseAborted {
		t.Fatalf("abort: %+v err=%v", s, err)
	}
	if _, err := e.Abort("r1", false); !errors.Is(err, ErrRalph) {
		t.Fatalf("double abort: err = %v", err)
	}
}

func TestCorruptStateFailsClosed(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	_, _, _ = e.Next("r1", false) // second save → .bak
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

func TestSchemaV1RejectedNoMigration(t *testing.T) {
	// terminal design: a pre-/2 (oma-ralph/1) document is refused fail-closed,
	// there is no migration layer.
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	path := filepath.Join(e.Dir, "ralph-r1.json")
	if err := os.WriteFile(path, []byte(`{"schema":"oma-ralph/1","id":"r1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Load("r1"); err == nil || !strings.Contains(err.Error(), "oma-ralph/1") {
		t.Fatalf("legacy /1: err = %v", err)
	}
}

func TestLoadValidatesPersistedKeepPolicy(t *testing.T) {
	// codex gate-2 must-fix: a hand-written /2 state with an invalid keep_policy
	// must be refused at Load — it cannot fall through to pass_only behavior.
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	path := filepath.Join(e.Dir, "ralph-r1.json")
	if err := os.WriteFile(path, []byte(`{"schema":"oma-ralph/2","id":"r1","phase":"running","keep_policy":"bogus","max_rounds":10,"plateau_window":3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Load("r1"); err == nil || !strings.Contains(err.Error(), "keep_policy") {
		t.Fatalf("invalid persisted keep_policy: err = %v", err)
	}
	// RecordCheck (via Resolve→Load) must also refuse it, not record as pass_only.
	if _, _, err := e.RecordCheck("r1", 1, nil, "", false); !errors.Is(err, ErrRalph) {
		t.Fatalf("RecordCheck on invalid state must fail-closed: err = %v", err)
	}
}

func TestLoadValidatesScorePlateauWindow(t *testing.T) {
	// score_improvement with a disabled plateau window (<=0) would silently
	// disable the approved stop rule; Load must refuse it.
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	path := filepath.Join(e.Dir, "ralph-r1.json")
	if err := os.WriteFile(path, []byte(`{"schema":"oma-ralph/2","id":"r1","phase":"running","keep_policy":"score_improvement","max_rounds":10,"plateau_window":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Load("r1"); err == nil || !strings.Contains(err.Error(), "plateau_window") {
		t.Fatalf("invalid plateau_window: err = %v", err)
	}
}

func TestResolveSingleActive(t *testing.T) {
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	s, err := e.Resolve("")
	if err != nil || s.ID != "r1" {
		t.Fatalf("resolve: %+v err=%v", s, err)
	}
	if _, err := e.Start("r2", StartOpts{Goal: "second goal"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resolve(""); err == nil || !strings.Contains(err.Error(), "r1, r2") {
		t.Fatalf("ambiguous: err = %v", err)
	}
}

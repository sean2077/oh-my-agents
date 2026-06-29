package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TEST-2: command-level ExitGate(4). These tests drive the REAL exit-4 sites
// (not the loose "interview score / ralph start" names from the handoff):
//   - interview gate  → interview.go:168 Errf(ExitGate, "gate failed: …")
//   - ralph next      → ralph.go:129    Errf(ExitGate, "%s", v.Reason)
//   - ralph check     → ralph.go:174    Errf(ExitGate, "%s", v.Reason)
//
// `interview score` and `ralph start` themselves never reach ExitGate; the gate
// is judged by `interview gate` / `ralph next` / `ralph check`, so those are the
// commands exercised here.

// gateProject mirrors the workflow_dryrun_test.go setup: a unique session id, a
// temp dir with a .git marker, and a chdir into it so state lands in
// <dir>/.oma/state. It returns a helper to write JSON inputs.
func gateProject(t *testing.T, session string) func(name, content string) string {
	t.Helper()
	t.Setenv("OMA_SESSION_ID", session)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return func(name, content string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
}

// TestInterviewGateFailsAboveThreshold pins interview.go:168: a current
// ambiguity ABOVE the threshold must exit ExitGate with "gate failed".
//
// fail-before rationale: if interview.go:168
// (`if !res.Pass && !res.Waived { return Errf(ExitGate, …) }`) were dropped or
// downgraded to a non-gate code, this asserts code==ExitGate and the
// "gate failed" message, so it fails. It is the opposite of the passing edge
// case in TestWorkflowCLIDryRunSnapshot (ambiguity == 0.10 == threshold passes;
// here ambiguity 0.90 > 0.10 fails).
func TestInterviewGateFailsAboveThreshold(t *testing.T) {
	writeJSON := gateProject(t, "gatefail")

	// deep depth => threshold 0.10 (interview.go start: --depth deep).
	if code, out := runOma(t, "interview", "start", "--id", "g1", "--depth", "deep"); code != ExitOK {
		t.Fatalf("start exit %d: %s", code, out)
	}
	// Round 0 locks one active component.
	topo := writeJSON("topo.json", `{"schema":"oma-interview-scores/1","round":0,"topology":{"components":[{"id":"a","name":"a","description":"d","status":"active"}]}}`)
	if code, out := runOma(t, "interview", "score", "--id", "g1", "--input", topo); code != ExitOK {
		t.Fatalf("topology exit %d: %s", code, out)
	}
	// A deliberately HIGH-ambiguity round: all dimensions 0.1 ⇒ clarity 0.1 ⇒
	// ambiguity 0.90, far above the 0.10 deep threshold (score.go weighted math).
	bad := writeJSON("r1.json", `{"schema":"oma-interview-scores/1","round":1,"component_scores":{"a":{"goal":0.1,"constraints":0.1,"criteria":0.1}}}`)
	if code, out := runOma(t, "interview", "score", "--id", "g1", "--input", bad); code != ExitOK {
		t.Fatalf("score exit %d: %s", code, out)
	}

	code, out := runOma(t, "interview", "gate", "--id", "g1")
	if code != ExitGate {
		t.Fatalf("gate exit %d, want ExitGate(%d): %s", code, ExitGate, out)
	}
	if !strings.Contains(out, "gate failed") {
		t.Fatalf("gate output missing %q: %s", "gate failed", out)
	}
}

// TestRalphNextExhaustedExitsGate pins ralph.go:129: a stop verdict from
// `ralph next` (here exhaustion) must exit ExitGate.
//
// fail-before rationale: with --max-rounds 1, the second `next` flips the loop
// to exhausted (ralph.go Next: Round 2 > MaxRounds 1 ⇒ PhaseExhausted,
// Continue=false). The CLI guard at ralph.go:129
// (`if !v.Continue { return Errf(ExitGate, "%s", v.Reason) }`) is the line
// under test: drop it and the exit drops to ExitOK, failing the code==ExitGate
// assertion. The reason text is deterministic ("exhausted: …").
func TestRalphNextExhaustedExitsGate(t *testing.T) {
	_ = gateProject(t, "ralphnext")

	if code, out := runOma(t, "ralph", "start", "--id", "rn", "--goal", "exhaust me", "--max-rounds", "1"); code != ExitOK {
		t.Fatalf("ralph start exit %d: %s", code, out)
	}
	// Round 1 of 1: still continues.
	if code, out := runOma(t, "ralph", "next", "--id", "rn"); code != ExitOK {
		t.Fatalf("first next exit %d, want continue: %s", code, out)
	}
	// Round 2 > max_rounds 1: exhausted stop verdict ⇒ ExitGate.
	code, out := runOma(t, "ralph", "next", "--id", "rn")
	if code != ExitGate {
		t.Fatalf("exhausting next exit %d, want ExitGate(%d): %s", code, ExitGate, out)
	}
	if !strings.Contains(out, "exhausted") {
		t.Fatalf("next output missing %q: %s", "exhausted", out)
	}
}

// TestRalphCheckStalledExitsGate pins ralph.go:174: a stop verdict from
// `ralph check` (here a pass_only stall) must exit ExitGate.
//
// fail-before rationale: --stall-window 1 makes a single failing check with a
// non-empty note a stall (ralph.go stalled(): one exit!=0 same-signature
// failure satisfies the window). RecordCheck sets PhaseStalled ⇒ Continue=false,
// and the CLI guard at ralph.go:174
// (`if !v.Continue { return Errf(ExitGate, "%s", v.Reason) }`) is the line
// under test: drop it and the exit drops to ExitOK, failing code==ExitGate.
func TestRalphCheckStalledExitsGate(t *testing.T) {
	_ = gateProject(t, "ralphcheck")

	if code, out := runOma(t, "ralph", "start", "--id", "rc", "--goal", "stall me", "--stall-window", "1"); code != ExitOK {
		t.Fatalf("ralph start exit %d: %s", code, out)
	}
	// A check needs a round to measure (RecordCheck refuses round 0).
	if code, out := runOma(t, "ralph", "next", "--id", "rc"); code != ExitOK {
		t.Fatalf("next exit %d: %s", code, out)
	}
	// One failing check (exit 1) with a signature note: stall_window 1 ⇒ stalled.
	code, out := runOma(t, "ralph", "check", "--id", "rc", "--verifier-exit", "1", "--note", "same-sig")
	if code != ExitGate {
		t.Fatalf("stalling check exit %d, want ExitGate(%d): %s", code, ExitGate, out)
	}
	if !strings.Contains(out, "stalled") {
		t.Fatalf("check output missing %q: %s", "stalled", out)
	}
}

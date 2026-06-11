package interview

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dirDigest fingerprints a directory tree (paths + contents) so
// zero-write assertions catch .bak/.tmp residue too.
func dirDigest(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		_, _ = fmt.Fprintf(h, "%s|%v|%d\n", path, info.IsDir(), info.Size())
		if !info.IsDir() {
			raw, _ := os.ReadFile(path)
			h.Write(raw)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func TestDryRunMutatorsValidateButNeverWrite(t *testing.T) {
	// review 060 blocker 1: dry-run runs the full validation path and
	// leaves the state tree byte-for-byte unchanged — including the
	// PASSING gate path, which previously persisted the transition.
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	mustScore(t, e, topologyInput("a"))
	mustScore(t, e, scoresInput(1, map[string]map[string]float64{
		"a": {"goal": 0.8, "constraints": 0.8, "criteria": 0.8}, // ambiguity == threshold
	}))
	before := dirDigest(t, e.Dir)

	// Passing gate, dry-run: full judgment, Mutated reported, zero writes.
	st, res, err := e.Gate("t1", false, "", true)
	if err != nil || !res.Pass || !res.Mutated || st.Phase != PhaseGatePassed {
		t.Fatalf("dry-run gate = %+v err=%v", res, err)
	}
	if got := dirDigest(t, e.Dir); got != before {
		t.Fatal("dry-run gate wrote state (review 060 blocker 1)")
	}
	// The persisted phase is untouched: a real reload still interviews.
	loaded, err := e.Load("t1")
	if err != nil || loaded.Phase != PhaseInterviewing {
		t.Fatalf("persisted phase = %s err=%v, want interviewing", loaded.Phase, err)
	}

	// Score dry-run: full math, zero writes.
	if _, rep, err := e.Score("t1", scoresInput(2, map[string]map[string]float64{
		"a": {"goal": 0.9, "constraints": 0.9, "criteria": 0.9},
	}), true); err != nil || rep.Ambiguity <= 0 {
		t.Fatalf("dry-run score: %+v err=%v", rep, err)
	}
	if got := dirDigest(t, e.Dir); got != before {
		t.Fatal("dry-run score wrote state")
	}

	// Waive dry-run: transition computed, zero writes.
	if _, res, err := e.Gate("t1", true, "why", true); err != nil || !res.Waived {
		t.Fatalf("dry-run waive: %+v err=%v", res, err)
	}
	if got := dirDigest(t, e.Dir); got != before {
		t.Fatal("dry-run waive wrote state")
	}

	// Abort dry-run.
	if _, err := e.Abort("t1", true); err != nil {
		t.Fatal(err)
	}
	if got := dirDigest(t, e.Dir); got != before {
		t.Fatal("dry-run abort wrote state")
	}

	// Start dry-run for a NEW id: validates, returns the would-be state,
	// writes nothing.
	if s, err := e.Start("fresh", "greenfield", 0.2, "test", "i", false, true); err != nil || s.ID != "fresh" {
		t.Fatalf("dry-run start: %+v err=%v", s, err)
	}
	if got := dirDigest(t, e.Dir); got != before {
		t.Fatal("dry-run start wrote state")
	}
}

func TestDryRunStillFailsValidation(t *testing.T) {
	// Dry-run must run the SAME checks as real execution — invalid inputs
	// exit nonzero instead of reporting hollow success (review 060).
	e := testEngine(t)
	// no state at all: every mutator refuses even under dry-run
	if _, _, err := e.Score("missing-id", scoresInput(1, nil), true); !errors.Is(err, ErrInterview) {
		t.Fatalf("dry-run score on missing id: %v", err)
	}
	if _, _, err := e.Gate("", true, "", true); !errors.Is(err, ErrInterview) {
		t.Fatalf("dry-run waive with no state: %v", err)
	}
	if _, err := e.Crystallize("missing-id", "no-such-spec.md", true); !errors.Is(err, ErrInterview) {
		t.Fatalf("dry-run crystallize: %v", err)
	}
	if _, err := e.Start("bad*id", "greenfield", 0.2, "t", "i", false, true); !errors.Is(err, ErrInterview) {
		t.Fatalf("dry-run start bad id: %v", err)
	}
	// existing state + missing spec file still refuses under dry-run
	mustStart(t, e, "greenfield", 0.20)
	mustScore(t, e, topologyInput("a"))
	mustScore(t, e, scoresInput(1, map[string]map[string]float64{
		"a": {"goal": 0.9, "constraints": 0.9, "criteria": 0.9},
	}))
	if _, _, err := e.Gate("t1", false, "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Crystallize("t1", filepath.Join(t.TempDir(), "absent.md"), true); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("dry-run crystallize missing spec: %v", err)
	}
}

func TestResolveFailsClosedOnCorruptCandidate(t *testing.T) {
	// review 060 blocker 2: omitted --id must not silently skip corrupt
	// or foreign-major state files.
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	bad := filepath.Join(e.Dir, "interview-bad.json")
	if err := os.WriteFile(bad, []byte(`{"schema":"oma-interview/9","id":"bad","phase":"interviewing"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resolve(""); err == nil || !strings.Contains(err.Error(), "interview-bad.json") {
		t.Fatalf("corrupt candidate: err = %v, want fail-closed naming it", err)
	}
	// Explicit --id on the good instance still works.
	if _, err := e.Resolve("t1"); err != nil {
		t.Fatalf("explicit id must bypass: %v", err)
	}
}

func TestSkillRound0ExampleIsAccepted(t *testing.T) {
	// review 072: the skill claims its round0.json shape is exact — pin
	// doc-code sync by running the LITERAL fenced example through the
	// engine. A future contract change that breaks the example fails here.
	raw, err := os.ReadFile(filepath.Join("..", "..", "assets", "skills", "deep-interview", "SKILL.md"))
	if err != nil {
		t.Skipf("skill asset not present: %v", err)
	}
	var block string
	rest := string(raw)
	for {
		i := strings.Index(rest, "```json\n")
		if i < 0 {
			break
		}
		rest = rest[i+len("```json\n"):]
		j := strings.Index(rest, "```")
		if j < 0 {
			break
		}
		candidate := rest[:j]
		if strings.Contains(candidate, `"round": 0`) {
			block = candidate
			break
		}
		rest = rest[j:]
	}
	if block == "" {
		t.Fatal("round-0 example block not found in SKILL.md")
	}
	path := filepath.Join(t.TempDir(), "round0.json")
	if err := os.WriteFile(path, []byte(block), 0o600); err != nil {
		t.Fatal(err)
	}
	in, err := ParseScoresInput(path)
	if err != nil {
		t.Fatalf("skill example does not parse: %v", err)
	}
	e := testEngine(t)
	mustStart(t, e, "greenfield", 0.20)
	st, _, err := e.Score("t1", in, false)
	if err != nil {
		t.Fatalf("skill example rejected by the engine: %v", err)
	}
	if st.Phase != PhaseInterviewing || st.Topology.Status != "confirmed" {
		t.Fatalf("after example lock: phase=%s topology=%s", st.Phase, st.Topology.Status)
	}
}

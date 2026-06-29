package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S13a budget fence boundary.
//
// The fence decision lives ONLY in the CLI: budget.Measure returns a Report
// with Total and Max but performs no pass/fail comparison, so the ceiling check
// is the single prod line
//
//	internal/cli/budget.go:54  if rep.Total > rep.Max { return Errf(ExitGate, …) }
//
// These tests pin its exact-fit edge with a known measured total. We install one
// skill named "pin" (4 bytes -> 1 token, Tokens = ceil(utf8/4), approx-b4/1) with
// description "abcd" (4 bytes -> 1 token); under `--profile all` the resident
// surface counts skill name+description, so the measured total is exactly 2.
// `--max-resident-tokens` then sets the ceiling precisely against that 2:
//   - max == total (2)     => 2 > 2 is false => pass (ExitOK, no fence error).
//   - max == total-1 (1)   => 2 > 1 is true  => fail (ExitGate, fence error).
//
// fail-before rationale: budget.go:54 uses strict `>`. If `>` were relaxed to
// `>=`, the exact-fit case (total == max == 2) would flip from pass to ExitGate
// and TestBudgetFenceExactFitPasses would fail. The total-1 case guards the
// other side: were `>` tightened so the one-over total stopped tripping the
// gate, TestBudgetFenceOneOverFails would fail. Both ceilings are positive, so
// config validation (budget.max_resident_tokens must be > 0) is satisfied.

// installPinSkill installs a single skill whose resident surface measures to a
// known token total under `doctor budget --profile all`. It mirrors the
// OMA_HOME + `asset install --from` setup in asset_test.go and returns the
// measured total so the boundary ceilings are derived from prod measurement,
// not a guessed constant.
func installPinSkill(t *testing.T) int {
	t.Helper()
	srcRoot := t.TempDir()
	dir := filepath.Join(srcRoot, "skills", "pin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"oma-asset/1","name":"pin","type":"skill","targets":["claude","codex"]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	// name "pin" = 1 token, description "abcd" = 1 token => total 2.
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: pin\ndescription: abcd\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMA_HOME", t.TempDir())
	if code, out := runOma(t, "asset", "install", "--from", srcRoot, "pin"); code != ExitOK {
		t.Fatalf("install exit %d: %s", code, out)
	}

	// Confirm the measured total before testing the boundary, so a change in
	// the approximation surfaces here rather than as a confusing fence result.
	code, out := runOma(t, "doctor", "budget", "--profile", "all", "--json")
	if code != ExitOK {
		t.Fatalf("baseline budget exit %d: %s", code, out)
	}
	var env struct {
		Budget struct {
			Total int `json:"total"`
		} `json:"budget"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("baseline budget json: %v\n%s", err, out)
	}
	if env.Budget.Total != 2 {
		t.Fatalf("measured total = %d, want 2 (pin name+description); items changed?\n%s", env.Budget.Total, out)
	}
	return env.Budget.Total
}

// TestBudgetFenceExactFitPasses pins budget.go:54 on the lower edge: a total that
// exactly equals the ceiling must NOT trip the gate (strict `>`).
func TestBudgetFenceExactFitPasses(t *testing.T) {
	total := installPinSkill(t) // 2

	// max == total: 2 > 2 is false => pass.
	code, out := runOma(t, "doctor", "budget", "--profile", "all", "--max-resident-tokens", "2")
	if code != ExitOK {
		t.Fatalf("exact-fit (total=%d, max=2) exit %d, want ExitOK(%d): %s", total, code, ExitOK, out)
	}
	if strings.Contains(out, "exceeds budget") {
		t.Fatalf("exact-fit must not report a fence error: %s", out)
	}
	if !strings.Contains(out, "total 2 / max 2") {
		t.Fatalf("budget summary missing %q: %s", "total 2 / max 2", out)
	}
}

// TestBudgetFenceOneOverFails pins budget.go:54 on the upper edge: a total one
// above the ceiling must trip the gate with ExitGate and the fence message.
func TestBudgetFenceOneOverFails(t *testing.T) {
	total := installPinSkill(t) // 2

	// max == total-1: 2 > 1 is true => ExitGate.
	code, out := runOma(t, "doctor", "budget", "--profile", "all", "--max-resident-tokens", "1")
	if code != ExitGate {
		t.Fatalf("one-over (total=%d, max=1) exit %d, want ExitGate(%d): %s", total, code, ExitGate, out)
	}
	if !strings.Contains(out, "resident surface 2 tokens exceeds budget 1") {
		t.Fatalf("one-over output missing fence message: %s", out)
	}
}

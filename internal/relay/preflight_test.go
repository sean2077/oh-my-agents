package relay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func claudeIDEnv(k string) string {
	if k == "CLAUDE_CODE_SESSION_ID" {
		return "sess-1"
	}
	return ""
}

func levelOf(r *PreflightReport, name string) string {
	for _, c := range r.Checks {
		if c.Name == name {
			return c.Level
		}
	}
	return "<absent>"
}

func newProbeInput(t *testing.T, root, projectRoot string) PreflightInput {
	t.Helper()
	return PreflightInput{
		ExplicitRoot: root,
		ProjectRoot:  projectRoot,
		Getenv:       claudeIDEnv,
		Now:          newClock().now,
	}
}

func TestPreflightHealthyInitializedLedger(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	// Create + bind a pair so binding and peer resolve.
	claude := testLedger(t, root, "claude", ck)
	mustPair(t, claude, "topic")

	r := Preflight(PreflightInput{ExplicitRoot: root, Getenv: claudeIDEnv, Now: ck.now})
	if r.Fail != 0 {
		t.Fatalf("healthy ledger should not fail: %+v", r.Checks)
	}
	if r.ExitCode() != 0 && r.ExitCode() != 1 {
		t.Fatalf("exit = %d", r.ExitCode())
	}
	if got := levelOf(r, "ledger.sentinel"); got != PFOk {
		t.Fatalf("sentinel = %s", got)
	}
	// Every FS probe should pass on a real temp filesystem.
	for _, name := range []string{"fs.tmp_rename", "fs.symlink", "fs.sha256", "fs.fsync", "fs.posix_mode"} {
		if got := levelOf(r, name); got != PFOk {
			t.Errorf("%s = %s, want ok", name, got)
		}
	}
}

func TestPreflightUninitializedWarns(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".oma", "relay")
	r := Preflight(newProbeInput(t, root, ""))
	if got := levelOf(r, "ledger.sentinel"); got != PFWarn {
		t.Fatalf("uninitialized sentinel = %s, want warn", got)
	}
	if r.Fail != 0 {
		t.Fatalf("uninitialized must not fail: %+v", r.Checks)
	}
	if r.ExitCode() != 1 {
		t.Fatalf("exit = %d, want 1 (warn)", r.ExitCode())
	}
}

func TestPreflightLegacySharedWarnsNotFails(t *testing.T) {
	// review 086 must-fix 2: a legacy .shared/ under the project root is
	// informational; it must never block (the dogfood repo has one).
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".shared", "_relay"), 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(project, ".oma", "relay")
	r := Preflight(newProbeInput(t, root, project))
	if got := levelOf(r, "legacy.shared"); got != PFWarn {
		t.Fatalf("legacy.shared = %s, want warn", got)
	}
	if r.Fail != 0 {
		t.Fatalf("legacy .shared must not cause a failure: %+v", r.Checks)
	}
}

func TestPreflightExplicitV1RootFails(t *testing.T) {
	// review 086 must-fix 2: explicit --ledger-root resolving to a v1
	// tree is a hard refusal (exit 3).
	v1 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(v1, "_relay"), 0o700); err != nil {
		t.Fatal(err)
	}
	r := Preflight(newProbeInput(t, v1, ""))
	if got := levelOf(r, "ledger.v1_root"); got != PFFail {
		t.Fatalf("ledger.v1_root = %s, want fail", got)
	}
	if r.ExitCode() != 3 {
		t.Fatalf("exit = %d, want 3", r.ExitCode())
	}
}

func TestPreflightForeignSentinelFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "relay")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, sentinelName), []byte(`{"schema":"oma-relay/9","created":"2026-06-12T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Preflight(newProbeInput(t, root, ""))
	if got := levelOf(r, "ledger.sentinel"); got != PFFail {
		t.Fatalf("foreign-major sentinel = %s, want fail", got)
	}
	if r.ExitCode() != 3 {
		t.Fatalf("exit = %d, want 3", r.ExitCode())
	}
	// Non-empty dir without a sentinel also fails.
	root2 := filepath.Join(t.TempDir(), "relay")
	if err := os.MkdirAll(filepath.Join(root2, "junk"), 0o700); err != nil {
		t.Fatal(err)
	}
	r2 := Preflight(newProbeInput(t, root2, ""))
	if got := levelOf(r2, "ledger.sentinel"); got != PFFail {
		t.Fatalf("non-empty no-sentinel = %s, want fail", got)
	}
}

func TestPreflightIdentityFailureCaptured(t *testing.T) {
	// No platform signal and no OMA_RELAY_AUTHOR: identity fails, but
	// preflight still reports it as a check rather than aborting.
	root := filepath.Join(t.TempDir(), "relay")
	r := Preflight(PreflightInput{ExplicitRoot: root, Getenv: func(string) string { return "" }, Now: newClock().now})
	if got := levelOf(r, "identity.author"); got != PFFail {
		t.Fatalf("identity.author = %s, want fail", got)
	}
	// Binding/peer checks are skipped without an identity, but FS probes
	// still run (diagnostics continue).
	if levelOf(r, "fs.tmp_rename") == "<absent>" {
		t.Fatal("FS probes must still run when identity fails")
	}
}

func TestPreflightJSONShapeStable(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".oma", "relay")
	r := Preflight(newProbeInput(t, root, ""))
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"schema"`, `"oma-relay-preflight/1"`, `"root"`, `"checks"`, `"pass"`, `"warn"`, `"fail"`, `"level"`, `"name"`, `"message"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("json missing %s: %s", key, raw)
		}
	}
}

func TestPreflightBindingWarnsOnStale(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "gone")
	// Close the pair → the binding now points at a terminal/archived pair.
	if err := claude.Close(s.Pair, "approve", "done", false); err != nil {
		t.Fatal(err)
	}
	r := Preflight(PreflightInput{ExplicitRoot: root, Getenv: claudeIDEnv, Now: ck.now})
	if got := levelOf(r, "pair.binding"); got != PFWarn {
		t.Fatalf("pair.binding after close = %s, want warn", got)
	}
	if r.Fail != 0 {
		t.Fatalf("stale binding must warn, not fail: %+v", r.Checks)
	}
}

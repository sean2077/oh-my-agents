package cli

import (
	"strings"
	"testing"
)

// S8b extends the S8a `hint:` fail-closed convention (state_hint_test.go) to
// more command families. Each migrated refusal keeps its ExitState contract and
// now carries an actionable `hint:` line (command-tree.md §1 convention).

// workflow family: `oma workflow list` outside a git project is a CLI-authored
// fail-closed refusal (workflow.go).
func TestWorkflowListOutsideProjectFailsClosedWithHint(t *testing.T) {
	t.Setenv("OMA_SESSION_ID", "s8b")
	dir := t.TempDir()
	t.Chdir(dir)
	// Only meaningful when the temp dir has no .git ancestor (the
	// outside-project path). If TMPDIR sits inside a checkout, skip rather
	// than assert against a resolved project root.
	if findProjectRoot() != "" {
		t.Skip("temp dir resolved to a project root; outside-project path unreachable here")
	}

	code, out := runOma(t, "workflow", "list")
	if code != ExitState {
		t.Fatalf("exit = %d, want ExitState(%d); out: %s", code, ExitState, out)
	}
	if !strings.Contains(out, "not inside a git project") {
		t.Fatalf("refusal must keep its reason ('not inside a git project'); got: %s", out)
	}
	if !strings.Contains(out, "hint:") {
		t.Fatalf("migrated refusal must carry a `hint:` line; got: %s", out)
	}
}

// asset family: a dev/source build with no --ref/--from has no release to pin
// (asset.go). version.Version is "dev" under `go test`, so this path is
// deterministic and offline (it refuses before any fetch).
func TestAssetInstallNoSourceFailsClosedWithHint(t *testing.T) {
	t.Setenv("OMA_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	code, out := runOma(t, "asset", "install", "demo")
	if code != ExitState {
		t.Fatalf("exit = %d, want ExitState(%d); out: %s", code, ExitState, out)
	}
	if !strings.Contains(out, "no assets source") {
		t.Fatalf("refusal must keep its reason ('no assets source'); got: %s", out)
	}
	if !strings.Contains(out, "hint:") {
		t.Fatalf("migrated refusal must carry a `hint:` line; got: %s", out)
	}
}

// doctor family: the maintenance no-op refusals (`oma doctor state` /
// `oma doctor relay` with no action flag) short-circuit before any project or
// ledger access, so this is deterministic with no setup. ExitState (not
// ExitUsage) is the contract — ExitUsage stays reserved for cobra parse errors.
func TestDoctorMaintenanceNoActionFailsClosedWithHint(t *testing.T) {
	for _, sub := range []string{"state", "relay"} {
		code, out := runOma(t, "doctor", sub)
		if code != ExitState {
			t.Fatalf("doctor %s: exit = %d, want ExitState(%d); out: %s", sub, code, ExitState, out)
		}
		if !strings.Contains(out, "nothing to do") {
			t.Fatalf("doctor %s: refusal must keep its reason ('nothing to do'); got: %s", sub, out)
		}
		if !strings.Contains(out, "hint:") {
			t.Fatalf("doctor %s: migrated refusal must carry a `hint:` line; got: %s", sub, out)
		}
	}
}

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S8a: a migrated fail-closed refusal keeps its exit contract (ExitState) and
// now carries an actionable `hint:` line (command-tree.md §1 convention).
func TestStateGetUnsetKeyFailsClosedWithHint(t *testing.T) {
	t.Setenv("OMA_SESSION_ID", "s8a")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	code, out := runOma(t, "state", "get", "demo/missing")
	if code != ExitState {
		t.Fatalf("exit = %d, want ExitState(%d); out: %s", code, ExitState, out)
	}
	if !strings.Contains(out, "not set") {
		t.Fatalf("fail-closed refusal must keep its reason ('not set'); got: %s", out)
	}
	if !strings.Contains(out, "hint:") {
		t.Fatalf("migrated fail-closed refusal must carry a `hint:` line; got: %s", out)
	}
}

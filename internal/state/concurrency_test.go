package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Multi-process lost-update under the per-namespace lock is already covered by
// TestConcurrentProcessSetsDoNotLoseFields. These tests cover the crash/restart
// recovery dimension: the single-generation backup and tolerance of write
// residue left by a killed process.

// TestStateBackupHoldsPriorGeneration verifies the .bak that makes a crashed
// write recoverable: after a second write, .bak is the first generation and the
// main file is the second, both valid JSON.
func TestStateBackupHoldsPriorGeneration(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)
	st.Now = func() time.Time { return time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC) }
	if _, err := st.Set("ns/a", "1", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Set("ns/a", "2", "", false); err != nil {
		t.Fatal(err)
	}
	if v, ok, rev, _ := st.GetWithRevision("ns/a", ""); v != "2" || !ok || rev != 2 {
		t.Fatalf("main generation = %q rev %d, want \"2\" rev 2", v, rev)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".oma", "state", "ns.json.bak"))
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("backup not valid JSON: %v", err)
	}
	if f.Data["a"] != "1" || f.Revision != 1 {
		t.Fatalf("backup is not the prior generation: %+v", f)
	}
}

// TestStateToleratesCrashTempResidue ensures a leftover unique temp file from a
// killed write neither blocks a later write nor corrupts the namespace.
func TestStateToleratesCrashTempResidue(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)
	st.Now = func() time.Time { return time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC) }
	if _, err := st.Set("ns/a", "1", "", false); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, ".oma", "state")
	if err := os.WriteFile(filepath.Join(stateDir, ".ns.json-stale123.tmp"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Set("ns/b", "2", "", false); err != nil {
		t.Fatalf("set after temp residue: %v", err)
	}
	if v, ok, _, err := st.GetWithRevision("ns/a", ""); err != nil || !ok || v != "1" {
		t.Fatalf("state unreadable after temp residue: %q %v %v", v, ok, err)
	}
}

package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStateFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateSessionScope(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".oma", "state")

	// v0.7.0 "name-suffix" files that must migrate.
	writeStateFixture(t, dir, "myapp-codex-0123456789ab.json",
		`{"schema":"oma-state/1","namespace":"myapp-codex-0123456789ab","revision":2,"data":{"phase":"plan"},"updated":"2026-06-20T00:00:00Z"}`)
	writeStateFixture(t, dir, "interview-loop-claude-abcdef012345.json",
		`{"schema":"oma-interview/1","id":"loop-claude-abcdef012345","round":1}`)
	writeStateFixture(t, dir, "ralph-x-codex-0123456789ab.json",
		`{"schema":"oma-ralph/2","id":"x-codex-0123456789ab","phase":"running"}`)
	// Default instance (bare suffix) and already-migrated files must NOT change.
	writeStateFixture(t, dir, "codex-0123456789ab.json",
		`{"schema":"oma-state/1","namespace":"codex-0123456789ab","revision":1,"data":{},"updated":"2026-06-20T00:00:00Z"}`)
	writeStateFixture(t, dir, "foo--s-codex-0123456789ab.json",
		`{"schema":"oma-state/1","namespace":"foo--s-codex-0123456789ab","revision":1,"data":{},"updated":"2026-06-20T00:00:00Z"}`)

	plan, err := MigrateSessionScope(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 3 {
		t.Fatalf("dry-run plan = %d actions, want 3: %+v", len(plan), plan)
	}
	for _, a := range plan {
		if a.Applied {
			t.Fatalf("dry-run must not apply: %+v", a)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "myapp-codex-0123456789ab.json")); err != nil {
		t.Fatalf("dry-run mutated disk: %v", err)
	}

	applied, err := MigrateSessionScope(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 3 {
		t.Fatalf("apply = %d actions, want 3", len(applied))
	}

	for _, n := range []string{
		"myapp--s-codex-0123456789ab.json",
		"interview-loop--s-claude-abcdef012345.json",
		"ralph-x--s-codex-0123456789ab.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("missing migrated file %s: %v", n, err)
		}
	}
	for _, n := range []string{
		"myapp-codex-0123456789ab.json",
		"interview-loop-claude-abcdef012345.json",
		"ralph-x-codex-0123456789ab.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(err) {
			t.Errorf("old file %s still present: %v", n, err)
		}
		if _, err := os.Stat(filepath.Join(dir, preMigrationDir, n)); err != nil {
			t.Errorf("backup of %s missing: %v", n, err)
		}
	}
	for _, n := range []string{"codex-0123456789ab.json", "foo--s-codex-0123456789ab.json"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("file %s should be untouched: %v", n, err)
		}
	}

	// Generic state is loadable under the new namespace with data intact.
	v, ok, err := New(root).Get("myapp--s-codex-0123456789ab/phase", "")
	if err != nil || !ok || v != "plan" {
		t.Fatalf("migrated state Get = (%q,%v,%v), want (plan,true,nil)", v, ok, err)
	}
	// interview/ralph embedded id is rewritten to the new form.
	raw, err := os.ReadFile(filepath.Join(dir, "interview-loop--s-claude-abcdef012345.json"))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["id"] != "loop--s-claude-abcdef012345" {
		t.Errorf("interview id = %v, want loop--s-claude-abcdef012345", obj["id"])
	}
	// round (an unknown-to-this-migration field) is preserved.
	if obj["round"] == nil {
		t.Errorf("migration dropped the round field: %v", obj)
	}

	again, err := MigrateSessionScope(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second apply = %d actions, want 0 (idempotent)", len(again))
	}
}

func TestMigrateSessionScopeCrashIdempotent(t *testing.T) {
	// Simulate a crash AFTER the migration wrote the new file but BEFORE it
	// removed the old one: a re-run must recognize its own output and finish
	// the cleanup, not fail closed with "target already exists".
	root := t.TempDir()
	dir := filepath.Join(root, ".oma", "state")
	old := `{"schema":"oma-state/1","namespace":"app-codex-0123456789ab","revision":3,"data":{"phase":"plan"},"updated":"2026-06-20T00:00:00Z"}`
	writeStateFixture(t, dir, "app-codex-0123456789ab.json", old)
	want, err := rewriteScopedField([]byte(old), "namespace", "app--s-codex-0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	writeStateFixture(t, dir, "app--s-codex-0123456789ab.json", string(want))

	applied, err := MigrateSessionScope(root, true)
	if err != nil {
		t.Fatalf("crash-idempotent re-apply must finish cleanly, got %v", err)
	}
	if len(applied) != 1 || !applied[0].Applied {
		t.Fatalf("expected 1 applied (cleanup) action, got %+v", applied)
	}
	if _, err := os.Stat(filepath.Join(dir, "app-codex-0123456789ab.json")); !os.IsNotExist(err) {
		t.Errorf("old file must be removed on idempotent finish: %v", err)
	}
}

func TestMigrateSessionScopeFailsClosedOnConflict(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".oma", "state")
	writeStateFixture(t, dir, "myapp-codex-0123456789ab.json",
		`{"schema":"oma-state/1","namespace":"myapp-codex-0123456789ab","revision":1,"data":{},"updated":"2026-06-20T00:00:00Z"}`)
	writeStateFixture(t, dir, "myapp--s-codex-0123456789ab.json",
		`{"schema":"oma-state/1","namespace":"myapp--s-codex-0123456789ab","revision":1,"data":{},"updated":"2026-06-20T00:00:00Z"}`)
	if _, err := MigrateSessionScope(root, true); err == nil {
		t.Fatal("expected fail-closed on target-name collision, got nil")
	}
}

// TestMigrateSessionScopeBackupEnablesRecovery proves the pre-migration backup
// is a byte-exact copy of the original (so a migration can be undone by hand)
// and that an unknown top-level field survives the rewrite untouched.
func TestMigrateSessionScopeBackupEnablesRecovery(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".oma", "state")
	original := `{"schema":"oma-state/1","namespace":"app-codex-0123456789ab","revision":4,"data":{"phase":"impl"},"updated":"2026-06-20T00:00:00Z","future_field":{"keep":true}}`
	writeStateFixture(t, dir, "app-codex-0123456789ab.json", original)

	if _, err := MigrateSessionScope(root, true); err != nil {
		t.Fatal(err)
	}

	backup, err := os.ReadFile(filepath.Join(dir, preMigrationDir, "app-codex-0123456789ab.json"))
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(backup) != original {
		t.Fatalf("backup not byte-identical to original:\n got %q\nwant %q", backup, original)
	}

	migrated, err := os.ReadFile(filepath.Join(dir, "app--s-codex-0123456789ab.json"))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(migrated, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["future_field"] == nil {
		t.Errorf("migration dropped unknown top-level field future_field: %v", obj)
	}
	if obj["namespace"] != "app--s-codex-0123456789ab" {
		t.Errorf("namespace = %v, want rewritten form", obj["namespace"])
	}
}

// TestMigrateSessionScopeAtomicNoResidue proves residue from a crashed earlier
// run (a pre-existing .pre-migration backup dir) does not block a clean
// re-apply, and that a successful migration leaves no temp file behind.
func TestMigrateSessionScopeAtomicNoResidue(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".oma", "state")
	writeStateFixture(t, dir, "app-codex-0123456789ab.json",
		`{"schema":"oma-state/1","namespace":"app-codex-0123456789ab","revision":1,"data":{},"updated":"2026-06-20T00:00:00Z"}`)
	if err := os.MkdirAll(filepath.Join(dir, preMigrationDir), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := MigrateSessionScope(root, true); err != nil {
		t.Fatalf("re-apply over pre-existing backup dir: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp residue left in state dir: %s", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "app--s-codex-0123456789ab.json")); err != nil {
		t.Errorf("migrated file missing: %v", err)
	}
}

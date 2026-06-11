package asset

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sean2077/oh-my-agents/internal/agentdir"
	"github.com/sean2077/oh-my-agents/internal/hookcfg"
)

// writeHookSource builds a hook asset source dir whose fragment injects
// one Stop entry per agent carrying the given command.
func writeHookSource(t *testing.T, root, name, command string) string {
	t.Helper()
	dir := filepath.Join(root, "hooks", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema": "oma-asset/1", "name": "` + name + `", "type": "hook", "targets": ["claude", "codex"]}`
	fragment := `{"schema": "oma-hook-fragment/1",
		"claude": {"Stop": [{"hooks": [{"type": "command", "command": "` + command + `"}]}]},
		"codex": {"Stop": [{"command": "` + command + `"}]}}`
	for file, content := range map[string]string{
		"manifest.json": manifest,
		"fragment.json": fragment,
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func hostPaths(e *Engine) (claude, codex string) {
	return filepath.Join(e.Layout.Home, ".claude", "settings.json"),
		filepath.Join(e.Layout.Home, ".codex", "hooks.json")
}

func TestHookInstallInjectsBothAgentsAndRecordsProjections(t *testing.T) {
	e := newTestEngine(t)
	src := writeHookSource(t, t.TempDir(), "relay-watch", "oma relay wait --hook")
	rep := mustInstall(t, e, src, Options{})

	injectOps := 0
	for _, op := range rep.Ops {
		if op.Kind == "inject" {
			injectOps++
		}
	}
	if injectOps != 2 {
		t.Fatalf("want 2 inject ops, got %+v", rep.Ops)
	}
	claude, codex := hostPaths(e)
	for path, wrap := range map[string]string{claude: "hooks", codex: ""} {
		cmds, err := hookcfg.OwnCommands(path, wrap, "relay-watch")
		if err != nil || len(cmds) != 1 || cmds[0] != "oma relay wait --hook" {
			t.Fatalf("%s: cmds = %v err=%v", path, cmds, err)
		}
	}
	entries, _ := e.List()
	if len(entries[0].Projections) != 2 {
		t.Fatalf("projections = %+v", entries[0].Projections)
	}
	for _, pr := range entries[0].Projections {
		if pr.Kind != agentdir.KindInject {
			t.Fatalf("projection kind = %q, want inject", pr.Kind)
		}
	}
}

func TestHookInstallCorruptHostFailsClosedZeroWrites(t *testing.T) {
	e := newTestEngine(t)
	claude, codex := hostPaths(e)
	if err := os.MkdirAll(filepath.Dir(claude), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claude, []byte(`{"hooks": {`), 0o600); err != nil {
		t.Fatal(err)
	}
	src := writeHookSource(t, t.TempDir(), "relay-watch", "cmd")
	_, err := e.Install(src, Options{})
	if !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("corrupt host: err = %v, want ErrProjectionConflict", err)
	}
	// Pre-check failure means zero writes anywhere: no canonical placement,
	// no registry, no codex-side injection, claude host untouched.
	if _, err := os.Lstat(filepath.Join(e.Layout.CanonicalRoot(), "hooks", "relay-watch")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canonical must not be placed")
	}
	if _, err := os.Lstat(e.Layout.RegistryPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("registry must not be written")
	}
	if _, err := os.Lstat(codex); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("codex host must not be created")
	}
	raw, _ := os.ReadFile(claude)
	if string(raw) != `{"hooks": {` {
		t.Fatalf("claude host mutated: %q", raw)
	}
}

func TestHookRemoveStripsInjectionAndPreservesForeignEntries(t *testing.T) {
	e := newTestEngine(t)
	claude, _ := hostPaths(e)
	if err := os.MkdirAll(filepath.Dir(claude), 0o700); err != nil {
		t.Fatal(err)
	}
	original := `{
  "hooks": {
    "Stop": [
      {
        "command": "user-own-stop"
      }
    ]
  }
}
`
	if err := os.WriteFile(claude, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	src := writeHookSource(t, t.TempDir(), "relay-watch", "cmd")
	mustInstall(t, e, src, Options{})
	if _, err := e.Remove("relay-watch", Options{}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got, _ := os.ReadFile(claude)
	if string(got) != original {
		t.Fatalf("remove did not restore pre-injection bytes:\n--- got ---\n%s--- want ---\n%s", got, original)
	}
	entries, _ := e.List()
	if len(entries) != 0 {
		t.Fatalf("registry after remove: %+v", entries)
	}
}

func TestHookDryRunWritesNothing(t *testing.T) {
	e := newTestEngine(t)
	src := writeHookSource(t, t.TempDir(), "relay-watch", "cmd")
	rep, err := e.Install(src, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, op := range rep.Ops {
		if op.Kind == "inject" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dry-run must report inject ops: %+v", rep.Ops)
	}
	claude, codex := hostPaths(e)
	for _, path := range []string{claude, codex, e.Layout.RegistryPath()} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dry-run wrote %s", path)
		}
	}
}

func TestHookFragmentMissingAgentSectionFailsClosed(t *testing.T) {
	e := newTestEngine(t)
	dir := filepath.Join(t.TempDir(), "hooks", "half")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema": "oma-asset/1", "name": "half", "type": "hook", "targets": ["claude", "codex"]}`
	fragment := `{"schema": "oma-hook-fragment/1", "claude": {"Stop": [{"hooks": [{"type": "command", "command": "x"}]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fragment.json"), []byte(fragment), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := e.Install(dir, Options{})
	if err == nil || !strings.Contains(err.Error(), "no codex section") {
		t.Fatalf("missing section: err = %v, want fail-closed refusal", err)
	}
	if _, err := os.Lstat(e.Layout.RegistryPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("nothing may be written")
	}
}

func TestHookReinstallReplacesEntriesNotAppends(t *testing.T) {
	e := newTestEngine(t)
	root := t.TempDir()
	mustInstall(t, e, writeHookSource(t, root, "relay-watch", "v1-cmd"), Options{})
	src2 := writeHookSource(t, filepath.Join(root, "v2"), "relay-watch", "v2-cmd")
	mustInstall(t, e, src2, Options{})

	claude, _ := hostPaths(e)
	cmds, err := hookcfg.OwnCommands(claude, "hooks", "relay-watch")
	if err != nil || len(cmds) != 1 || cmds[0] != "v2-cmd" {
		t.Fatalf("after reinstall cmds = %v err=%v, want exactly [v2-cmd]", cmds, err)
	}
}

func TestHookRollbackReinjectsRestoredCommands(t *testing.T) {
	e := newTestEngine(t)
	root := t.TempDir()
	mustInstall(t, e, writeHookSource(t, root, "relay-watch", "v1-cmd"), Options{})

	// Drift the canonical fragment, then force-install v2 (records a backup
	// of the drifted v1), then roll back: the v1 command must be live again.
	canonical := filepath.Join(e.Layout.CanonicalRoot(), "hooks", "relay-watch")
	driftFrag := `{"schema": "oma-hook-fragment/1",
		"claude": {"Stop": [{"hooks": [{"type": "command", "command": "v1-drifted"}]}]},
		"codex": {"Stop": [{"command": "v1-drifted"}]}}`
	if err := os.WriteFile(filepath.Join(canonical, "fragment.json"), []byte(driftFrag), 0o600); err != nil {
		t.Fatal(err)
	}
	src2 := writeHookSource(t, filepath.Join(root, "v2"), "relay-watch", "v2-cmd")
	mustInstall(t, e, src2, Options{Force: true})

	if _, err := e.Rollback("relay-watch", "", Options{}); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	claude, codex := hostPaths(e)
	for path, wrap := range map[string]string{claude: "hooks", codex: ""} {
		cmds, err := hookcfg.OwnCommands(path, wrap, "relay-watch")
		if err != nil || len(cmds) != 1 || cmds[0] != "v1-drifted" {
			t.Fatalf("%s after rollback: cmds = %v err=%v, want [v1-drifted]", path, cmds, err)
		}
	}
}

func TestHookInstallNonArrayTargetEventFailsClosedZeroWrites(t *testing.T) {
	// review 048: a host event targeted by the fragment but shaped as a
	// non-array must fail BOTH dry-run and real install before any
	// canonical or registry write — never a post-checkpoint apply error
	// leaving an installed entry with projections=[].
	e := newTestEngine(t)
	claude, codex := hostPaths(e)
	if err := os.MkdirAll(filepath.Dir(claude), 0o700); err != nil {
		t.Fatal(err)
	}
	host := `{"hooks": {"Stop": {}}}`
	if err := os.WriteFile(claude, []byte(host), 0o600); err != nil {
		t.Fatal(err)
	}
	src := writeHookSource(t, t.TempDir(), "relay-watch", "cmd")

	for _, dry := range []bool{true, false} {
		_, err := e.Install(src, Options{DryRun: dry})
		if err == nil || !strings.Contains(err.Error(), "not an array") {
			t.Fatalf("dry=%v: err = %v, want non-array target refusal", dry, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(e.Layout.CanonicalRoot(), "hooks", "relay-watch")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canonical must not be placed")
	}
	if _, err := os.Lstat(e.Layout.RegistryPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("registry must not be written — no installed entry with projections=[]")
	}
	if _, err := os.Lstat(codex); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("codex host must not be created")
	}
	if raw, _ := os.ReadFile(claude); string(raw) != host {
		t.Fatalf("claude host mutated: %q", raw)
	}
}

func TestHookRemoveFailsClosedOnCorruptHost(t *testing.T) {
	// review 046 blocker 2: a failed uninject must not orphan oma residue
	// by dropping canonical/registry state, and --dry-run must run the
	// same validation instead of advertising success.
	e := newTestEngine(t)
	src := writeHookSource(t, t.TempDir(), "relay-watch", "cmd")
	mustInstall(t, e, src, Options{})
	claude, _ := hostPaths(e)
	if err := os.WriteFile(claude, []byte(`{"hooks": {`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := e.Remove("relay-watch", Options{DryRun: true}); err == nil {
		t.Fatal("dry-run remove must fail the same host validation")
	}
	if _, err := e.Remove("relay-watch", Options{}); err == nil {
		t.Fatal("remove must fail closed on corrupt host")
	}
	// Management state intact: canonical still placed, registry still owns.
	if _, err := os.Stat(filepath.Join(e.Layout.CanonicalRoot(), "hooks", "relay-watch")); err != nil {
		t.Fatal("canonical must be left intact after failed uninject")
	}
	entries, _ := e.List()
	if len(entries) != 1 {
		t.Fatalf("registry after failed remove: %+v", entries)
	}
	// Fix the host file: remove now converges (codex side already stripped
	// on the failed attempt is fine — hookcfg.Remove is idempotent).
	if err := os.WriteFile(claude, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Remove("relay-watch", Options{}); err != nil {
		t.Fatalf("remove after fixing host: %v", err)
	}
	entries, _ = e.List()
	if len(entries) != 0 {
		t.Fatalf("registry after converged remove: %+v", entries)
	}
}

func TestHookRemoveFailsClosedOnSymlinkedHost(t *testing.T) {
	e := newTestEngine(t)
	src := writeHookSource(t, t.TempDir(), "relay-watch", "cmd")
	mustInstall(t, e, src, Options{})
	claude, _ := hostPaths(e)
	real := filepath.Join(t.TempDir(), "elsewhere.json")
	if err := os.Rename(claude, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, claude); err != nil {
		t.Skip("symlinks unavailable")
	}
	for _, dry := range []bool{true, false} {
		if _, err := e.Remove("relay-watch", Options{DryRun: dry}); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("dry=%v: err = %v, want symlink refusal", dry, err)
		}
	}
	entries, _ := e.List()
	if len(entries) != 1 {
		t.Fatal("registry must keep the entry after refused remove")
	}
}

func TestHookRemoveFailsClosedOnDuplicateKeyHost(t *testing.T) {
	e := newTestEngine(t)
	src := writeHookSource(t, t.TempDir(), "relay-watch", "cmd")
	mustInstall(t, e, src, Options{})
	claude, _ := hostPaths(e)
	dup := `{"hooks": {"Stop": [{"command": "a", "_oma_asset": "relay-watch"}]}, "hooks": {"Stop": [{"command": "runtime-last"}]}}`
	if err := os.WriteFile(claude, []byte(dup), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Remove("relay-watch", Options{}); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("duplicate-key host: err = %v, want duplicate refusal", err)
	}
	// And the health gate must report the problem, not health.
	entries, _ := e.List()
	if ok, problems := e.VerifyProjections(&entries[0]); ok || len(problems) == 0 {
		t.Fatal("duplicate-key host must fail projection verification")
	}
}

func TestVerifyProjectionsDetectsInjectionDrift(t *testing.T) {
	e := newTestEngine(t)
	src := writeHookSource(t, t.TempDir(), "relay-watch", "cmd")
	mustInstall(t, e, src, Options{})
	entries, _ := e.List()
	if ok, problems := e.VerifyProjections(&entries[0]); !ok {
		t.Fatalf("fresh hook install must verify: %v", problems)
	}
	// Hand-strip the injected entries (registry still records them).
	claude, _ := hostPaths(e)
	if err := hookcfg.Remove(claude, "hooks", "relay-watch"); err != nil {
		t.Fatal(err)
	}
	ok, problems := e.VerifyProjections(&entries[0])
	if ok || len(problems) == 0 {
		t.Fatal("stripped injection must be detected")
	}
	if !strings.Contains(strings.Join(problems, "; "), "injected hook entries missing") {
		t.Fatalf("problems = %v", problems)
	}
}

package asset

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func mustInstall(t *testing.T, e *Engine, src string, opts Options) *Report {
	t.Helper()
	rep, err := e.Install(src, opts)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	return rep
}

func editCanonical(t *testing.T, e *Engine, rel, content string) string {
	t.Helper()
	p := filepath.Join(e.Layout.CanonicalRoot(), filepath.FromSlash(rel))
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDriftedReinstallRefusedThenForced(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "v1")
	mustInstall(t, e, src, Options{})
	editCanonical(t, e, "skills/x/SKILL.md", "user edited")

	if _, err := e.Install(src, Options{}); !errors.Is(err, ErrUnmanagedTarget) {
		t.Fatalf("drifted reinstall: err = %v, want ErrUnmanagedTarget", err)
	}

	rep, err := e.Install(src, Options{Force: true})
	if err != nil {
		t.Fatalf("forced reinstall over drift: %v", err)
	}
	if rep.Ops[0].Kind != "backup" || !strings.Contains(rep.Ops[0].Path, filepath.FromSlash("skills/x")) {
		t.Fatalf("backup op must preserve canonical rel path, got %+v", rep.Ops[0])
	}
	backed, err := os.ReadFile(filepath.Join(rep.Ops[0].Path, "SKILL.md"))
	if err != nil || string(backed) != "user edited" {
		t.Fatalf("backup content = %q err=%v, want user edited", backed, err)
	}
	// digest refreshed: plain reinstall is managed again
	if _, err := e.Install(src, Options{}); err != nil {
		t.Fatalf("reinstall after digest refresh: %v", err)
	}
}

func TestDriftedRemoveRefusedThenForcedBacksUp(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "v1")
	mustInstall(t, e, src, Options{})
	editCanonical(t, e, "skills/x/SKILL.md", "precious user edits")

	if _, err := e.Remove("x", Options{}); !errors.Is(err, ErrUnmanagedTarget) {
		t.Fatalf("drifted remove: err = %v, want ErrUnmanagedTarget", err)
	}
	rep, err := e.Remove("x", Options{Force: true})
	if err != nil {
		t.Fatalf("forced remove: %v", err)
	}
	if rep.Ops[0].Kind != "backup" {
		t.Fatalf("forced remove must back up first, got %+v", rep.Ops)
	}
	backed, err := os.ReadFile(filepath.Join(rep.Ops[0].Path, "SKILL.md"))
	if err != nil || string(backed) != "precious user edits" {
		t.Fatalf("backup content = %q err=%v", backed, err)
	}
}

func TestDriftedRollbackRefused(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "new body")
	canonical := filepath.Join(e.Layout.CanonicalRoot(), "skills", "x")
	if err := os.MkdirAll(canonical, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustInstall(t, e, src, Options{Force: true}) // records a backup of "foreign"
	editCanonical(t, e, "skills/x/SKILL.md", "drifted after install")

	if _, err := e.Rollback("x", "", Options{}); !errors.Is(err, ErrRollbackConflict) {
		t.Fatalf("drifted rollback: err = %v, want ErrRollbackConflict", err)
	}
}

func TestCorruptedManifestNameTraversalRefused(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	mustInstall(t, e, src, Options{})

	victim := filepath.Join(e.Layout.Home, "precious")
	if err := os.MkdirAll(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	// Hostile-but-schema-valid registry: name and embedded manifest name
	// both set to a traversal string, canonical_path pointed at the victim.
	reg, err := LoadRegistry(e.Layout.RegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	ent := reg.Find("x")
	ent.Name = "../../precious"
	ent.Manifest.Name = "../../precious"
	ent.CanonicalPath = victim
	if err := reg.Save(e.Layout.RegistryPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Remove("../../precious", Options{Force: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("traversal remove: err = %v, want ErrInvalid", err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("victim outside .agents must not be deleted")
	}
}

func TestPayloadSpecialFileRefused(t *testing.T) {
	e := newTestEngine(t)
	root := t.TempDir()
	src := writeSkillSource(t, root, "x", "body")
	if err := syscall.Mkfifo(filepath.Join(src, "pipe"), 0o600); err != nil {
		t.Skip("mkfifo unavailable on this platform")
	}
	_, err := e.Install(src, Options{})
	if err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("special-file payload: err = %v, want non-regular refusal", err)
	}
}

func TestCLIRejectsTraversalNamesBeforePathWork(t *testing.T) {
	for _, n := range []string{"../escape", "a/b", "..", "UPPER"} {
		if ValidName(n) {
			t.Errorf("ValidName(%q) = true, want false", n)
		}
	}
}

func TestTamperedRegistryPathRefused(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	mustInstall(t, e, src, Options{})

	victim := filepath.Join(e.Layout.Home, "precious")
	if err := os.MkdirAll(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegistry(e.Layout.RegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	reg.Find("x").CanonicalPath = victim
	if err := reg.Save(e.Layout.RegistryPath()); err != nil {
		t.Fatal(err)
	}

	if _, err := e.Remove("x", Options{Force: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered canonical_path: err = %v, want ErrInvalid", err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("victim path must not be deleted")
	}
}

func TestCanonicalRootSymlinkEscapeRefused(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".agents")); err != nil {
		t.Skip("symlinks unavailable")
	}
	e := NewEngine(home)
	e.Now = func() time.Time { return time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC) }
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	if _, err := e.Install(src, Options{}); err == nil || !strings.Contains(err.Error(), "outside home") {
		t.Fatalf("root escape: err = %v, want outside-home refusal", err)
	}
}

func TestBackupCollisionFailsClosed(t *testing.T) {
	e := newTestEngine(t)
	fixed := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	e.Now = func() time.Time { return fixed } // constant clock → same backup id
	src := writeSkillSource(t, t.TempDir(), "x", "v1")
	mustInstall(t, e, src, Options{})

	editCanonical(t, e, "skills/x/SKILL.md", "edit one")
	if _, err := e.Install(src, Options{Force: true}); err != nil {
		t.Fatalf("first forced reinstall: %v", err)
	}
	editCanonical(t, e, "skills/x/SKILL.md", "edit two")
	if _, err := e.Install(src, Options{Force: true}); err == nil || !strings.Contains(err.Error(), "backup target already exists") {
		t.Fatalf("backup collision: err = %v, want fail-closed", err)
	}
}

func TestRegistrySaveKeepsSingleGenerationBak(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "v1")
	mustInstall(t, e, src, Options{})
	first, err := os.ReadFile(e.Layout.RegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	mustInstall(t, e, src, Options{}) // managed reinstall rewrites registry
	bak, err := os.ReadFile(e.Layout.RegistryPath() + ".bak")
	if err != nil {
		t.Fatalf("registry .bak missing: %v", err)
	}
	if string(bak) != string(first) {
		t.Fatal(".bak must hold the previous generation")
	}
}

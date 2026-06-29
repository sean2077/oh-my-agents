package asset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sean2077/oh-my-agents/internal/agentdir"
)

// S2b / SEC-1: the canonical-mutation and copy-refresh siblings of S2's
// projection TOCTOU. Each test makes a parent valid at plan/precheck time, then
// swaps it to an escaping symlink in the window before the guarded mutation, and
// asserts the immediate revalidation refuses the operation and writes/deletes
// nothing outside the trusted root.

func requireSymlinks(t *testing.T, outside string) {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "probe")
	if err := os.Symlink(outside, probe); err != nil {
		t.Skip("symlinks unavailable")
	}
	_ = os.Remove(probe)
}

// swapToExternalSymlink replaces dir (whatever it currently is) with a symlink
// pointing at outside — the post-check parent swap an attacker would race in.
func swapToExternalSymlink(t *testing.T, dir, outside string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Errorf("swap setup remove: %v", err)
		return
	}
	if err := os.Symlink(outside, dir); err != nil {
		t.Errorf("swap setup symlink: %v", err)
	}
}

func TestCanonicalPlaceRevalidatesAfterPlanTimeParentSwap(t *testing.T) {
	e := newTestEngine(t)
	outside := t.TempDir()
	requireSymlinks(t, outside)
	agentsSkills := filepath.Join(e.Layout.Home, ".agents", "skills")
	if err := os.MkdirAll(agentsSkills, 0o700); err != nil {
		t.Fatal(err)
	}
	e.beforeWriteHook = func(stage string) {
		if stage == "canonical-place" {
			swapToExternalSymlink(t, agentsSkills, outside)
		}
	}
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	if _, err := e.Install(src, Options{}); err == nil || !strings.Contains(err.Error(), "intermediate symlink escape") {
		t.Fatalf("install canonical place: err = %v, want escape refusal", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("nothing may be written through the swapped canonical parent: %v", entries)
	}
}

func TestRemoveCanonicalDeleteRevalidatesAfterParentSwap(t *testing.T) {
	e := newTestEngine(t)
	outside := t.TempDir()
	requireSymlinks(t, outside)
	// Sentinel at the path RemoveAll would hit if it followed the swapped parent.
	if err := os.WriteFile(filepath.Join(outside, "x"), []byte("external sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	if _, err := e.Install(src, Options{}); err != nil {
		t.Fatal(err)
	}
	agentsSkills := filepath.Join(e.Layout.Home, ".agents", "skills")
	e.beforeWriteHook = func(stage string) {
		if stage == "canonical-delete" {
			swapToExternalSymlink(t, agentsSkills, outside)
		}
	}
	if _, err := e.Remove("x", Options{}); err == nil || !strings.Contains(err.Error(), "intermediate symlink escape") {
		t.Fatalf("remove canonical delete: err = %v, want escape refusal", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "x")); err != nil {
		t.Fatalf("external content must not be deleted through the swapped parent: %v", err)
	}
}

func TestRollbackCanonicalPlaceRevalidatesAfterParentSwap(t *testing.T) {
	e := newTestEngine(t)
	outside := t.TempDir()
	requireSymlinks(t, outside)
	// Force-install over foreign content so a backup is recorded for rollback.
	canonical := filepath.Join(e.Layout.Home, ".agents", "skills", "x")
	if err := os.MkdirAll(canonical, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := writeSkillSource(t, t.TempDir(), "x", "new body")
	if _, err := e.Install(src, Options{Force: true}); err != nil {
		t.Fatal(err)
	}
	agentsSkills := filepath.Join(e.Layout.Home, ".agents", "skills")
	e.beforeWriteHook = func(stage string) {
		if stage == "canonical-place" {
			swapToExternalSymlink(t, agentsSkills, outside)
		}
	}
	if _, err := e.Rollback("x", "", Options{}); err == nil || !strings.Contains(err.Error(), "intermediate symlink escape") {
		t.Fatalf("rollback canonical place: err = %v, want escape refusal", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("nothing may be written through the swapped canonical parent: %v", entries)
	}
}

func TestRollbackCopyRefreshRevalidatesAfterParentSwap(t *testing.T) {
	e := newTestEngine(t)
	outside := t.TempDir()
	requireSymlinks(t, outside)
	// Force-install over foreign content so a backup exists, then convert the
	// projection to a KindCopy (Windows-shaped) entry the way the dry-run copy
	// refresh test does — POSIX installs only produce symlink projections.
	canonical := filepath.Join(e.Layout.Home, ".agents", "skills", "x")
	if err := os.MkdirAll(canonical, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "SKILL.md"), []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := writeSkillSource(t, t.TempDir(), "x", "new body")
	if _, err := e.Install(src, Options{Force: true}); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegistry(e.Layout.RegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	entry := reg.Find("x")
	if entry == nil || len(entry.Projections) == 0 {
		t.Fatalf("missing projection: %+v", reg)
	}
	proj := entry.Projections[0]
	if err := removeExistingProjectionTargetBestEffort(proj.Path); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(entry.CanonicalPath, proj.Path); err != nil {
		t.Fatal(err)
	}
	entry.Projections = []Projection{{Agent: proj.Agent, Path: proj.Path, Kind: agentdir.KindCopy}}
	if err := reg.Save(e.Layout.RegistryPath()); err != nil {
		t.Fatal(err)
	}

	projParent := filepath.Dir(proj.Path) // e.g. ~/.claude/skills
	e.beforeWriteHook = func(stage string) {
		if stage == "copy-refresh" {
			swapToExternalSymlink(t, projParent, outside)
		}
	}
	if _, err := e.Rollback("x", "", Options{}); err == nil || !strings.Contains(err.Error(), "intermediate symlink escape") {
		t.Fatalf("rollback copy refresh: err = %v, want escape refusal", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("nothing may be written through the swapped projection parent: %v", entries)
	}
}

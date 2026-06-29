package asset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S2 / SEC-1: a projection parent that is valid when planProjections checks it
// but is swapped to an escaping symlink before the projection write must be
// caught by the revalidation at the top of applyProjection. The static escape
// tests (TestNestedProjectionSymlinkEscapeRefused) only cover a symlink present
// at plan time; without the recheck, applyProjection would follow the swapped-in
// parent and create the link outside the agent root.
func TestProjectionWriteRevalidatesAfterPlanTimeParentSwap(t *testing.T) {
	e := newTestEngine(t)
	outside := t.TempDir()

	// Skip cleanly where symlinks are unavailable (matches the static tests).
	probe := filepath.Join(t.TempDir(), "probe")
	if err := os.Symlink(outside, probe); err != nil {
		t.Skip("symlinks unavailable")
	}
	_ = os.Remove(probe)

	skillsDir := filepath.Join(e.Layout.Home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Fire in the window between the plan-time root check and the write: swap the
	// still-empty, still-valid parent to a symlink pointing outside the root.
	e.beforeWriteHook = func(stage string) {
		if stage != "projection" {
			return
		}
		if err := os.Remove(skillsDir); err != nil {
			t.Errorf("swap setup: %v", err)
			return
		}
		if err := os.Symlink(outside, skillsDir); err != nil {
			t.Errorf("swap setup: %v", err)
		}
	}

	src := writeSkillSource(t, t.TempDir(), "x", "body")
	_, err := e.Install(src, Options{Agents: []string{"claude"}})
	if err == nil || !strings.Contains(err.Error(), "intermediate symlink escape") {
		t.Fatalf("post-plan parent swap: err = %v, want intermediate-symlink-escape refusal", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("nothing may be written through the swapped-in escaping parent: %v", entries)
	}
}

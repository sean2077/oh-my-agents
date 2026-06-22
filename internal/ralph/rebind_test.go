package ralph

import (
	"testing"
	"time"
)

// TestBranchSwitchDetectionAndRebind pins P2-7: a loop bound to one branch is
// refused from a different branch in the SAME worktree, and rebind-worktree is
// the explicit, mutating way to move it.
func TestBranchSwitchDetectionAndRebind(t *testing.T) {
	dir := t.TempDir()
	e := NewEngine(dir)
	e.Now = func() time.Time { return time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC) }
	e.WorktreeRoot, e.ProjectRoot, e.Branch, e.BaseCommit = "/wt/a", "/wt/a", "feat-1", "c0ffee"

	s, err := e.Start("loop", StartOpts{Goal: "ship it"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Branch != "feat-1" || s.BaseCommit != "c0ffee" {
		t.Fatalf("start did not record branch/base_commit: %+v", s)
	}

	// Same worktree, switched branch → mutating access refused.
	e.Branch = "feat-2"
	if _, err := e.loadResolved(s.ID); err == nil {
		t.Fatal("expected branch-switch refusal from loadResolved")
	}

	// Rebind moves it explicitly; afterwards the loop tracks feat-2.
	if _, err := e.Rebind(s.ID, false); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	got, err := e.loadResolved(s.ID)
	if err != nil {
		t.Fatalf("loadResolved after rebind: %v", err)
	}
	if got.Branch != "feat-2" {
		t.Fatalf("rebind did not update branch: %+v", got)
	}
}

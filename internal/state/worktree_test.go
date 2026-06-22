package state

import (
	"testing"
	"time"
)

func TestBindAndCheckWorktree(t *testing.T) {
	root := t.TempDir()
	st := New(root)
	st.Now = func() time.Time { return time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC) }

	// Unbound namespace: the check is a no-op until BindWorktree runs.
	if err := st.CheckWorktree("autopilot", "", "/wt/a"); err != nil {
		t.Fatalf("unbound check should pass: %v", err)
	}
	if _, err := st.BindWorktree("autopilot", "", root, "/wt/a", false); err != nil {
		t.Fatal(err)
	}
	if err := st.CheckWorktree("autopilot", "", "/wt/a"); err != nil {
		t.Fatalf("same-worktree check should pass: %v", err)
	}
	if err := st.CheckWorktree("autopilot", "", "/wt/b"); err == nil {
		t.Fatal("different-worktree check should fail closed")
	}

	// A normal set must not clobber the worktree binding.
	if _, err := st.Set("autopilot/phase", "plan", "", false); err != nil {
		t.Fatal(err)
	}
	if err := st.CheckWorktree("autopilot", "", "/wt/a"); err != nil {
		t.Fatalf("worktree binding lost after set: %v", err)
	}
	if v, ok, err := st.Get("autopilot/phase", ""); err != nil || !ok || v != "plan" {
		t.Fatalf("data Get = (%q,%v,%v), want (plan,true,nil)", v, ok, err)
	}

	// Re-binding refreshes the worktree.
	if _, err := st.BindWorktree("autopilot", "", root, "/wt/b", false); err != nil {
		t.Fatal(err)
	}
	if err := st.CheckWorktree("autopilot", "", "/wt/b"); err != nil {
		t.Fatalf("after rebind, /wt/b should pass: %v", err)
	}
}

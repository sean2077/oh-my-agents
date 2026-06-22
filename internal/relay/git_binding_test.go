package relay

import "testing"

func TestPairBindsWorktreeAndRefusesForeignClose(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	claude.GitContext = GitContext{WorktreeRoot: "/wt/a", Branch: "feat-x", HeadCommit: "deadbeef"}

	s := mustPair(t, claude, "bind")
	if s.WorktreeRoot != "/wt/a" || s.Branch != "feat-x" || s.BaseCommit != "deadbeef" {
		t.Fatalf("pair not bound to creator worktree: %+v", s)
	}
	reloaded, err := claude.LoadSession(s.Pair)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.WorktreeRoot != "/wt/a" || reloaded.BaseCommit != "deadbeef" {
		t.Fatalf("worktree binding not persisted: %+v", reloaded)
	}

	// Closing from a different worktree is refused...
	claude.GitContext = GitContext{WorktreeRoot: "/wt/b"}
	if err := claude.Close(s.Pair, "abandon", "stop", false); err == nil {
		t.Fatal("close from a foreign worktree should be refused")
	}
	// ...unless explicitly allowed.
	claude.GitContext = GitContext{WorktreeRoot: "/wt/b", AllowWorktreeChange: true}
	if err := claude.Close(s.Pair, "abandon", "stop", false); err != nil {
		t.Fatalf("close with --allow-worktree-change: %v", err)
	}
}

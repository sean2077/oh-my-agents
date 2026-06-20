package projectroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectRootRegularCheckout(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := ProjectRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("ProjectRoot = %s, want %s", got, root)
	}
}

func TestProjectRootLinkedWorktreeUsesPrimaryCheckout(t *testing.T) {
	main := t.TempDir()
	gitDir := filepath.Join(main, ".git", "worktrees", "feature")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../..\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(wt, "sub")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	info, err := Resolve(nested)
	if err != nil {
		t.Fatal(err)
	}
	if info.ProjectRoot != main {
		t.Fatalf("ProjectRoot = %s, want primary checkout %s", info.ProjectRoot, main)
	}
	if info.WorktreeRoot != wt || !info.Linked {
		t.Fatalf("linked info = %+v, want worktree %s linked", info, wt)
	}
}

func TestProjectRootLinkedWorktreeWithoutCommondirFallback(t *testing.T) {
	main := t.TempDir()
	gitDir := filepath.Join(main, ".git", "worktrees", "feature")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ProjectRoot(wt)
	if err != nil {
		t.Fatal(err)
	}
	if got != main {
		t.Fatalf("ProjectRoot = %s, want %s", got, main)
	}
}

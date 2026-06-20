// Package projectroot resolves oma's project anchor.
//
// A normal checkout anchors at its own .git directory. A linked worktree
// anchors back to the primary checkout that owns the common .git directory, so
// all worktrees for one repository share one project-level .oma tree.
package projectroot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Info describes both the current checkout and oma's shared project anchor.
type Info struct {
	WorktreeRoot string
	ProjectRoot  string
	GitDir       string
	CommonGitDir string
	Linked       bool
}

// Resolve walks up from dir and returns the current git worktree plus the
// shared oma project root. Linked worktrees are detected through their .git
// file and mapped back to the primary checkout's common .git directory.
func Resolve(dir string) (Info, error) {
	worktreeRoot, gitPath, err := findGitPath(dir)
	if err != nil {
		return Info{}, err
	}
	info, err := os.Stat(gitPath)
	if err != nil {
		return Info{}, err
	}
	if info.IsDir() {
		return Info{
			WorktreeRoot: worktreeRoot,
			ProjectRoot:  worktreeRoot,
			GitDir:       gitPath,
			CommonGitDir: gitPath,
		}, nil
	}
	if !info.Mode().IsRegular() {
		return Info{}, fmt.Errorf(".git at %s is neither directory nor file", gitPath)
	}

	gitDir, err := readGitDirFile(gitPath, worktreeRoot)
	if err != nil {
		return Info{}, err
	}
	commonGitDir, err := commonDir(gitDir)
	if err != nil {
		return Info{}, err
	}
	projectRoot := worktreeRoot
	if filepath.Base(commonGitDir) == ".git" {
		projectRoot = filepath.Dir(commonGitDir)
	}
	return Info{
		WorktreeRoot: worktreeRoot,
		ProjectRoot:  projectRoot,
		GitDir:       gitDir,
		CommonGitDir: commonGitDir,
		Linked:       projectRoot != worktreeRoot,
	}, nil
}

// ProjectRoot returns oma's shared project anchor.
func ProjectRoot(dir string) (string, error) {
	info, err := Resolve(dir)
	if err != nil {
		return "", err
	}
	return info.ProjectRoot, nil
}

func findGitPath(dir string) (root, gitPath string, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	for {
		gitPath := filepath.Join(dir, ".git")
		if _, statErr := os.Stat(gitPath); statErr == nil {
			return dir, gitPath, nil
		} else if !os.IsNotExist(statErr) {
			return "", "", statErr
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("not inside a git checkout (no .git found above %s)", dir)
		}
		dir = parent
	}
}

func readGitDirFile(gitFile, worktreeRoot string) (string, error) {
	raw, err := os.ReadFile(gitFile)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(raw))
	path, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return "", fmt.Errorf("%s is not a gitdir file", gitFile)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%s has an empty gitdir", gitFile)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(worktreeRoot, path)
	}
	return filepath.Abs(path)
}

func commonDir(gitDir string) (string, error) {
	if raw, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		path := strings.TrimSpace(string(raw))
		if path == "" {
			return "", fmt.Errorf("%s has an empty commondir", gitDir)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(gitDir, path)
		}
		return filepath.Abs(path)
	} else if !os.IsNotExist(err) {
		return "", err
	}

	// Test fixtures and some minimal worktree setups omit commondir; the
	// standard gitdir shape still points at <primary>/.git/worktrees/<name>.
	if filepath.Base(filepath.Dir(gitDir)) == "worktrees" &&
		filepath.Base(filepath.Dir(filepath.Dir(gitDir))) == ".git" {
		return filepath.Dir(filepath.Dir(gitDir)), nil
	}
	return gitDir, nil
}

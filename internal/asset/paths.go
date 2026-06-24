package asset

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Layout resolves every path the asset domain touches, anchored at a home
// directory that tests replace with t.TempDir() (never the real home).
type Layout struct {
	Home string
}

// CanonicalRoot is the agent-neutral asset root shared with the npx-skills
// ecosystem (~/.agents).
func (l Layout) CanonicalRoot() string { return filepath.Join(l.Home, ".agents") }

// ConfigDir holds oma-owned state (registry, backups) under XDG config.
func (l Layout) ConfigDir() string { return filepath.Join(l.Home, ".config", "oma") }

// RegistryPath is the install registry file (docs/reference/schemas.md §2).
func (l Layout) RegistryPath() string { return filepath.Join(l.ConfigDir(), "registry.json") }

// BackupDir is the root for pre-overwrite backups (docs/reference/security-contract.md §2).
func (l Layout) BackupDir(id string) string { return filepath.Join(l.ConfigDir(), "backups", id) }

// CanonicalPath maps an asset to its canonical location
// (docs/reference/adapter-conformance.md §2): skills and hooks are directories,
// subagents and prompts are single markdown files.
func (l Layout) CanonicalPath(m *Manifest) (string, error) {
	rel, err := l.CanonicalRel(m)
	if err != nil {
		return "", err
	}
	return filepath.Join(l.CanonicalRoot(), filepath.FromSlash(rel)), nil
}

// CanonicalRel is the root-relative canonical path; backups preserve it
// (docs/reference/security-contract.md §2).
func (l Layout) CanonicalRel(m *Manifest) (string, error) {
	switch m.Type {
	case TypeSkill:
		return "skills/" + m.Name, nil
	case TypeHook:
		return "hooks/" + m.Name, nil
	case TypeSubagent:
		return "agents/" + m.Name + ".md", nil
	case TypePrompt:
		return "prompts/" + m.Name + ".md", nil
	default:
		return "", fmt.Errorf("%w: no canonical layout for type %q", ErrInvalid, m.Type)
	}
}

// SafeCanonicalTarget recomputes the canonical path from the entry's
// manifest, verifies the registry's recorded path agrees, and enforces the
// trusted-root constraints of docs/reference/security-contract.md §3. Destructive
// operations must use the returned path, never the registry string
// (B3 review blocker 3; embedded-manifest revalidation per recheck 020).
func (l Layout) SafeCanonicalTarget(e *Entry) (string, error) {
	// The registry is schema-checked but its content is still untrusted
	// input for destructive paths: revalidate the embedded manifest so a
	// corrupted name like "../../x" cannot reach CanonicalRel.
	if err := e.Manifest.Validate(); err != nil {
		return "", fmt.Errorf("%w: registry-embedded manifest invalid: %v", ErrInvalid, err)
	}
	if e.Manifest.Name != e.Name {
		return "", fmt.Errorf("%w: registry entry %q carries manifest for %q", ErrInvalid, e.Name, e.Manifest.Name)
	}
	// The top-level entry.Type drives projection-path recomputation (Remove /
	// Rollback) while canonical deletion uses the validated Manifest; cross-check
	// them so a tampered Type cannot diverge the two halves of one operation.
	if e.Type != e.Manifest.Type {
		return "", fmt.Errorf("%w: registry entry %q type %q disagrees with its manifest type %q (corrupt or tampered registry)", ErrInvalid, e.Name, e.Type, e.Manifest.Type)
	}
	expected, err := l.CanonicalPath(&e.Manifest)
	if err != nil {
		return "", err
	}
	if !pathWithin(expected, l.CanonicalRoot()) {
		return "", fmt.Errorf("%w: computed target %q escapes canonical root %q", ErrInvalid, expected, l.CanonicalRoot())
	}
	if filepath.Clean(e.CanonicalPath) != expected {
		return "", fmt.Errorf("%w: registry canonical_path %q does not match expected %q (corrupt or tampered registry)",
			ErrInvalid, e.CanonicalPath, expected)
	}
	if err := l.checkRootEscape(); err != nil {
		return "", err
	}
	if err := checkAncestorWritable(filepath.Dir(expected)); err != nil {
		return "", err
	}
	return expected, nil
}

// pathWithin reports whether path is lexically inside root after cleaning.
func pathWithin(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	return path != root && strings.HasPrefix(path, root+string(filepath.Separator))
}

// ValidName reports whether s is a safe asset name (CLI argument guard).
func ValidName(s string) bool { return nameRe.MatchString(s) }

// checkRootEscape refuses operation when the canonical root resolves
// outside the home directory (e.g. ~/.agents symlinked elsewhere).
func (l Layout) checkRootEscape() error {
	home, err := resolveExisting(l.Home)
	if err != nil {
		return err
	}
	root, err := resolveExisting(l.CanonicalRoot())
	if err != nil {
		return err
	}
	if root != home && !strings.HasPrefix(root+string(filepath.Separator), home+string(filepath.Separator)) {
		return fmt.Errorf("%w: canonical root %s resolves outside home %s", ErrInvalid, root, home)
	}
	return nil
}

// rejectsWorldWritable reports a world-writable directory on POSIX. On Windows,
// os.FileMode permission bits are an approximation over ACLs and commonly
// report user-owned directories as 0777, so ACL hardening is left to the
// Windows sandbox and the existing trusted-root checks. Both install
// (canonical placement) and projection feed their parents through
// checkAncestorWritable so the canonical store gets the same bar as the host
// projection dirs — no asymmetry where one walks ancestors and the other does
// not.
func rejectsWorldWritable(info os.FileInfo) bool {
	return runtime.GOOS != "windows" && info.Mode().Perm()&0o002 != 0
}

// resolveExisting eval-symlinks the nearest existing ancestor of path and
// rejoins the non-existing remainder, so escape checks work before the
// target is created.
func resolveExisting(path string) (string, error) {
	path = filepath.Clean(path)
	var tail []string
	for {
		resolved, err := resolveExistingPath(path)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("no existing ancestor for %s", path)
		}
		tail = append(tail, filepath.Base(path))
		path = parent
	}
}

func resolveExistingPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	dest, linkErr := os.Readlink(path)
	if linkErr != nil {
		return resolved, nil
	}
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(filepath.Dir(path), dest)
	}
	if linkResolved, err := filepath.EvalSymlinks(dest); err == nil {
		return linkResolved, nil
	}
	return filepath.Clean(dest), nil
}

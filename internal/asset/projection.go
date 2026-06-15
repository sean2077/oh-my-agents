package asset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/agentdir"
)

// ErrProjectionConflict marks a projection destination occupied by
// something oma does not own. Projection never stomps foreign content;
// --force applies to canonical placement only (B4 decision per review-022
// guidance: only replace expected oma links).
var ErrProjectionConflict = errors.New("projection target exists and is not the expected oma symlink")

// Skip records a requested projection an agent cannot take, reported
// rather than silently dropped (docs/config.md §4b).
type Skip struct {
	Agent  string `json:"agent"`
	Reason string `json:"reason"`
}

// plannedProjection is one symlink projection to create.
type plannedProjection struct {
	target    agentdir.Target
	canonical string
}

// planProjections intersects manifest targets with requested agents and
// resolves agent paths. Final set = manifest.targets ∩ requested; "shared"
// never projects; unsupported combinations (including hook assets, which
// are canonical-only) become Skips.
func (e *Engine) planProjections(m *Manifest, canonical string, requested []string) ([]plannedProjection, []Skip, error) {
	if len(requested) == 0 {
		requested = []string{agentClaude, agentCodex}
	}
	var plans []plannedProjection
	var skips []Skip
	for _, agent := range requested {
		if agent != agentClaude && agent != agentCodex {
			return nil, nil, fmt.Errorf("%w: unknown agent %q", ErrInvalid, agent)
		}
		if !m.HasTarget(agent) {
			skips = append(skips, Skip{Agent: agent, Reason: fmt.Sprintf("manifest targets %v do not include %s", m.Targets, agent)})
			continue
		}
		tgt, ok, reason := agentdir.For(e.Layout.Home, agent, m.Type, m.Name)
		if !ok {
			skips = append(skips, Skip{Agent: agent, Reason: reason})
			continue
		}
		if !pathWithin(tgt.Path, agentdir.AgentRoot(e.Layout.Home, agent)) {
			return nil, nil, fmt.Errorf("%w: projection %q escapes agent root", ErrInvalid, tgt.Path)
		}
		// Resolved trusted-root + ancestor-permission checks: the lexical
		// check above is not enough when the agent root is a symlink or a
		// world-writable directory (review 030 blocker 1).
		if err := e.checkProjectionRoot(agent, tgt.Path); err != nil {
			return nil, nil, err
		}
		plans = append(plans, plannedProjection{target: tgt, canonical: canonical})
	}
	return plans, skips, nil
}

// checkProjectionRoot enforces the resolved trusted-root constraints for
// one agent's projection area: the agent root must not resolve outside the
// home directory (symlinked ~/.claude → elsewhere is refused), the FULL
// target path — resolving any existing intermediate symlink components
// such as .claude/skills → elsewhere — must stay inside the resolved agent
// root (review 032), and the nearest existing ancestor of the target must
// not be world-writable.
func (e *Engine) checkProjectionRoot(agent, target string) error {
	home, err := resolveExisting(e.Layout.Home)
	if err != nil {
		return err
	}
	root, err := resolveExisting(agentdir.AgentRoot(e.Layout.Home, agent))
	if err != nil {
		return err
	}
	if root != home && !strings.HasPrefix(root+string(filepath.Separator), home+string(filepath.Separator)) {
		return fmt.Errorf("%w: agent root for %s resolves outside home: %s", ErrInvalid, agent, root)
	}
	// Resolve the PARENT directory's existing components — the final
	// component is the oma-owned symlink that legitimately points at
	// canonical (outside the agent root), so resolving the target itself
	// would false-positive. Intermediate escapes (.claude/skills ->
	// elsewhere) are caught here (review 032).
	resolvedParent, err := resolveExisting(filepath.Dir(target))
	if err != nil {
		return err
	}
	if resolvedParent != root && !strings.HasPrefix(resolvedParent+string(filepath.Separator), root+string(filepath.Separator)) {
		return fmt.Errorf("%w: projection path %s resolves outside agent root %s (intermediate symlink escape)",
			ErrInvalid, filepath.Join(resolvedParent, filepath.Base(target)), root)
	}
	return checkAncestorWritable(filepath.Dir(target))
}

// checkAncestorWritable walks up to the nearest existing ancestor and
// refuses world-writable directories (the direct parent may not exist yet
// when projection dirs are created on demand).
func checkAncestorWritable(dir string) error {
	for {
		info, err := os.Stat(dir)
		if err == nil {
			if info.Mode().Perm()&0o002 != 0 {
				return fmt.Errorf("%w: directory %s is world-writable", ErrInvalid, dir)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

// checkProjection pre-validates one symlink destination with zero writes:
// free, or already the expected oma symlink (idempotent reinstall) —
// anything else conflicts.
func checkProjection(p plannedProjection) error {
	info, err := os.Lstat(p.target.Path)
	if errors.Is(err, os.ErrNotExist) {
		return checkParentWritable(filepath.Dir(p.target.Path))
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%w: %s", ErrProjectionConflict, p.target.Path)
	}
	dest, err := os.Readlink(p.target.Path)
	if err != nil {
		return err
	}
	if filepath.Clean(dest) != filepath.Clean(p.canonical) {
		return fmt.Errorf("%w: %s -> %s (foreign link)", ErrProjectionConflict, p.target.Path, dest)
	}
	return nil
}

// applyProjection creates (or refreshes) the symlink.
func applyProjection(p plannedProjection) error {
	if err := os.MkdirAll(filepath.Dir(p.target.Path), 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(p.target.Path); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%w: %s", ErrProjectionConflict, p.target.Path)
		}
		if err := os.Remove(p.target.Path); err != nil {
			return err
		}
	}
	return os.Symlink(p.canonical, p.target.Path)
}

// removeProjection deletes a recorded projection only when the recorded
// path matches the expected projection path recomputed from the entry's
// manifest (registry content is persisted untrusted input — review 030
// blocker 3, same lesson as canonical paths in B3) and the link still
// points at canonical. Anything else is left intact with a warning.
func (e *Engine) removeProjection(entry *Entry, pr Projection, canonical string) (removed bool, warn string, err error) {
	expected, ok, _ := agentdir.For(e.Layout.Home, pr.Agent, entry.Type, entry.Name)
	if !ok || filepath.Clean(pr.Path) != expected.Path {
		return false, fmt.Sprintf("recorded projection %s does not match expected path; left intact", pr.Path), nil
	}
	if rootErr := e.checkProjectionRoot(pr.Agent, expected.Path); rootErr != nil {
		return false, fmt.Sprintf("projection root check failed for %s: %v", expected.Path, rootErr), nil
	}
	info, statErr := os.Lstat(expected.Path)
	if errors.Is(statErr, os.ErrNotExist) {
		return false, "", nil
	}
	if statErr != nil {
		return false, fmt.Sprintf("%s: %v", expected.Path, statErr), nil
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, fmt.Sprintf("%s is not a symlink; left intact", expected.Path), nil
	}
	dest, linkErr := os.Readlink(expected.Path)
	if linkErr != nil || filepath.Clean(dest) != filepath.Clean(canonical) {
		return false, fmt.Sprintf("%s points elsewhere; left intact", expected.Path), nil
	}
	if rmErr := os.Remove(expected.Path); rmErr != nil {
		return false, fmt.Sprintf("%s: %v", expected.Path, rmErr), nil
	}
	return true, "", nil
}

// VerifyProjectionSecurity re-runs the trusted-root constraints over an
// entry's recorded projections, catching drift introduced AFTER install
// (review 038 blocker 1): swapped intermediate symlinks, world-writable
// roots, or tampered recorded paths. Violations are fail-grade.
func (e *Engine) VerifyProjectionSecurity(entry *Entry) []string {
	var problems []string
	for _, pr := range entry.Projections {
		expected, ok, _ := agentdir.For(e.Layout.Home, pr.Agent, entry.Type, entry.Name)
		if !ok || filepath.Clean(pr.Path) != expected.Path {
			problems = append(problems, fmt.Sprintf("%s: recorded projection %s does not match expected path", pr.Agent, pr.Path))
			continue
		}
		if err := e.checkProjectionRoot(pr.Agent, expected.Path); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", pr.Agent, err))
		}
	}
	return problems
}

// VerifyProjections reports per-projection health for an entry (used by
// list --installed now, doctor in B6).
func (e *Engine) VerifyProjections(entry *Entry) (ok bool, problems []string) {
	ok = true
	if _, err := os.Lstat(entry.CanonicalPath); err != nil {
		return false, []string{fmt.Sprintf("canonical missing: %s", entry.CanonicalPath)}
	}
	// An installed entry with ZERO projections while its manifest could
	// project somewhere is inert — typically a TOCTOU-interrupted install
	// checkpoint. Degrade explicitly instead of reporting healthy
	// (review 048 adjacent hardening, approved in 050).
	if len(entry.Projections) == 0 {
		for _, agent := range []string{agentClaude, agentCodex} {
			if !entry.Manifest.HasTarget(agent) {
				continue
			}
			if _, supported, _ := agentdir.For(e.Layout.Home, agent, entry.Type, entry.Name); supported {
				return false, []string{"installed but no projections applied (interrupted install? rerun install to converge)"}
			}
		}
	}
	for _, pr := range entry.Projections {
		info, err := os.Lstat(pr.Path)
		if err != nil {
			ok = false
			problems = append(problems, fmt.Sprintf("%s projection missing: %s", pr.Agent, pr.Path))
			continue
		}
		if pr.Kind != agentdir.KindSymlink {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			ok = false
			problems = append(problems, fmt.Sprintf("%s not a symlink", pr.Path))
			continue
		}
		if dest, err := os.Readlink(pr.Path); err != nil || filepath.Clean(dest) != filepath.Clean(entry.CanonicalPath) {
			ok = false
			problems = append(problems, fmt.Sprintf("%s points to %s, want canonical", pr.Path, dest))
		}
	}
	return ok, problems
}

const (
	agentClaude = "claude"
	agentCodex  = "codex"
)

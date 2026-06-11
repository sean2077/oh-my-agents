package asset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// plannedProjection is one symlink to create.
type plannedProjection struct {
	target    agentdir.Target
	canonical string
}

// planProjections intersects manifest targets with requested agents and
// resolves agent paths. Final set = manifest.targets ∩ requested; "shared"
// never projects; unsupported combinations become Skips.
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
		plans = append(plans, plannedProjection{target: tgt, canonical: canonical})
	}
	return plans, skips, nil
}

// checkProjection pre-validates one destination: free, or already the
// expected oma symlink (idempotent reinstall). Anything else conflicts.
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

// removeProjection deletes a recorded projection only when it is still the
// expected oma symlink; foreign content is left intact with a warning.
func removeProjection(pr Projection, canonical string) (removed bool, warn string) {
	info, err := os.Lstat(pr.Path)
	if errors.Is(err, os.ErrNotExist) {
		return false, ""
	}
	if err != nil {
		return false, fmt.Sprintf("%s: %v", pr.Path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, fmt.Sprintf("%s is not a symlink; left intact", pr.Path)
	}
	dest, err := os.Readlink(pr.Path)
	if err != nil || filepath.Clean(dest) != filepath.Clean(canonical) {
		return false, fmt.Sprintf("%s points elsewhere; left intact", pr.Path)
	}
	if err := os.Remove(pr.Path); err != nil {
		return false, fmt.Sprintf("%s: %v", pr.Path, err)
	}
	return true, ""
}

// VerifyProjections reports per-projection health for an entry (used by
// list --installed now, doctor in B6).
func (e *Engine) VerifyProjections(entry *Entry) (ok bool, problems []string) {
	ok = true
	if _, err := os.Lstat(entry.CanonicalPath); err != nil {
		return false, []string{fmt.Sprintf("canonical missing: %s", entry.CanonicalPath)}
	}
	for _, pr := range entry.Projections {
		info, err := os.Lstat(pr.Path)
		if err != nil {
			ok = false
			problems = append(problems, fmt.Sprintf("%s/%s missing", pr.Agent, pr.Path))
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

package asset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/agentdir"
	"github.com/sean2077/oh-my-agents/internal/hookcfg"
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

// plannedProjection is one projection to create: a symlink, or a hook
// fragment injection into the agent's host config.
type plannedProjection struct {
	target    agentdir.Target
	canonical string
	asset     string                       // asset name = ownership marker value (inject only)
	wrapKey   string                       // host config shape (inject only)
	events    map[string][]json.RawMessage // entries to inject (inject only)
}

// planProjections intersects manifest targets with requested agents and
// resolves agent paths. Final set = manifest.targets ∩ requested; "shared"
// never projects; unsupported combinations become Skips. Hook assets must
// supply a fragment section for every projected agent (a target with no
// entries is a packaging bug — fail closed).
func (e *Engine) planProjections(m *Manifest, canonical string, requested []string, frag *hookcfg.Fragment) ([]plannedProjection, []Skip, error) {
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
		p := plannedProjection{target: tgt, canonical: canonical}
		if tgt.Kind == agentdir.KindInject {
			if frag == nil || len(frag.Events[agent]) == 0 {
				return nil, nil, fmt.Errorf("%w: hook asset %q targets %s but fragment.json has no %s section", ErrInvalid, m.Name, agent, agent)
			}
			p.asset, p.wrapKey, p.events = m.Name, agentdir.HookWrapKey(agent), frag.Events[agent]
		}
		plans = append(plans, p)
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

// checkProjection pre-validates one destination with zero writes.
// Symlink kind: free, or already the expected oma symlink (idempotent
// reinstall) — anything else conflicts. Inject kind: the host config must
// be absent or safely editable (valid JSON object, regular file) so a
// damaged host file aborts the install before any filesystem change
// (review 044 forward note).
func checkProjection(p plannedProjection) error {
	if p.target.Kind == agentdir.KindInject {
		if _, err := hookcfg.OwnCommands(p.target.Path, p.wrapKey, p.asset); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrProjectionConflict, p.target.Path, err)
		}
		return checkParentWritable(filepath.Dir(p.target.Path))
	}
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

// applyProjection creates (or refreshes) the symlink, or injects the hook
// fragment with replace-own semantics (idempotent reinstall: the asset's
// previous entries are replaced, never duplicated).
func applyProjection(p plannedProjection) error {
	if p.target.Kind == agentdir.KindInject {
		return hookcfg.Inject(p.target.Path, p.wrapKey, p.asset, p.events)
	}
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
func (e *Engine) removeProjection(entry *Entry, pr Projection, canonical string) (removed bool, warn string) {
	expected, ok, _ := agentdir.For(e.Layout.Home, pr.Agent, entry.Type, entry.Name)
	if !ok || filepath.Clean(pr.Path) != expected.Path {
		return false, fmt.Sprintf("recorded projection %s does not match expected path; left intact", pr.Path)
	}
	if err := e.checkProjectionRoot(pr.Agent, expected.Path); err != nil {
		return false, fmt.Sprintf("projection root check failed for %s: %v", expected.Path, err)
	}
	if expected.Kind == agentdir.KindInject {
		// Own-marker filtering IS the ownership verification: only entries
		// stamped "_oma_asset": <name> leave the host file; foreign hooks
		// and other assets' entries are byte-untouched.
		if err := hookcfg.Remove(expected.Path, agentdir.HookWrapKey(pr.Agent), entry.Name); err != nil {
			return false, fmt.Sprintf("%s: %v", expected.Path, err)
		}
		return true, ""
	}
	info, err := os.Lstat(expected.Path)
	if errors.Is(err, os.ErrNotExist) {
		return false, ""
	}
	if err != nil {
		return false, fmt.Sprintf("%s: %v", expected.Path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, fmt.Sprintf("%s is not a symlink; left intact", expected.Path)
	}
	dest, err := os.Readlink(expected.Path)
	if err != nil || filepath.Clean(dest) != filepath.Clean(canonical) {
		return false, fmt.Sprintf("%s points elsewhere; left intact", expected.Path)
	}
	if err := os.Remove(expected.Path); err != nil {
		return false, fmt.Sprintf("%s: %v", expected.Path, err)
	}
	return true, ""
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
	for _, pr := range entry.Projections {
		info, err := os.Lstat(pr.Path)
		if err != nil {
			ok = false
			problems = append(problems, fmt.Sprintf("%s projection missing: %s", pr.Agent, pr.Path))
			continue
		}
		if pr.Kind == agentdir.KindInject {
			// Liveness for injections: the host file must still hold at
			// least one entry marked for this asset.
			cmds, err := hookcfg.OwnCommands(pr.Path, agentdir.HookWrapKey(pr.Agent), entry.Name)
			if err != nil {
				ok = false
				problems = append(problems, fmt.Sprintf("%s host config unreadable: %v", pr.Agent, err))
				continue
			}
			if len(cmds) == 0 {
				ok = false
				problems = append(problems, fmt.Sprintf("%s injected hook entries missing from %s", pr.Agent, pr.Path))
			}
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

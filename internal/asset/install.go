package asset

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sean2077/oh-my-agents/internal/agentdir"
	"github.com/sean2077/oh-my-agents/internal/hookcfg"
)

// ErrUnmanagedTarget marks a destination that exists but is not owned by
// oma — no registry record, or content drifted from the recorded digest
// (docs/security-contract.md §2). Refused without --force.
var ErrUnmanagedTarget = errors.New("destination is not oma-managed (missing record or content drift); use --force to back up and replace")

// ErrRollbackConflict marks a rollback target whose current content is not
// the oma-managed state: restoring would silently destroy newer non-oma
// content (docs/security-contract.md §2).
var ErrRollbackConflict = errors.New("rollback conflict: current content is not the managed state; resolve manually or reinstall first")

// Op is one planned filesystem operation; --dry-run prints these and
// writes nothing (docs/security-contract.md §1).
type Op struct {
	Kind string `json:"kind"` // create | replace | backup | delete | restore | link | unlink | inject | uninject
	Path string `json:"path"`
}

// Options control mutating asset operations.
type Options struct {
	DryRun bool
	Force  bool
	Source string   // registry source label: dir | dev-link | release
	Agents []string // requested projection agents; final = manifest.targets ∩ Agents (docs/config.md §4b)
}

// Report describes what an operation did (or would do, under --dry-run).
type Report struct {
	Ops      []Op     `json:"ops"`
	Skips    []Skip   `json:"skips,omitempty"`    // requested-but-unsupported projections, reported not silent
	Warnings []string `json:"warnings,omitempty"` // non-fatal anomalies (foreign content left intact)
	Name     string   `json:"name"`
	DryRun   bool     `json:"dry_run"`
}

// Engine performs canonical-placement operations anchored at Home.
// Per-agent projection lives in the agentdir package (B4).
type Engine struct {
	Layout Layout
	Now    func() time.Time // injected for deterministic backup IDs in tests
}

// NewEngine builds an Engine for the given home directory.
func NewEngine(home string) *Engine {
	return &Engine{Layout: Layout{Home: home}, Now: time.Now}
}

// Install validates the asset at srcDir (manifest.json + payload) and
// places it at its canonical path, recording it in the registry.
func (e *Engine) Install(srcDir string, opts Options) (*Report, error) {
	m, err := LoadManifest(filepath.Join(srcDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	payload, err := e.payloadPath(srcDir, m)
	if err != nil {
		return nil, err
	}
	dest, err := e.Layout.CanonicalPath(m)
	if err != nil {
		return nil, err
	}
	if err := e.Layout.checkRootEscape(); err != nil {
		return nil, err
	}
	if err := checkParentWritable(filepath.Dir(dest)); err != nil {
		return nil, err
	}
	reg, err := LoadRegistry(e.Layout.RegistryPath())
	if err != nil {
		return nil, err
	}
	rel, _ := e.Layout.CanonicalRel(m)

	rep := &Report{Name: m.Name, DryRun: opts.DryRun}
	frag, err := e.loadFragment(srcDir, m)
	if err != nil {
		return nil, err
	}
	plans, skips, err := e.planProjections(m, dest, opts.Agents, frag)
	if err != nil {
		return nil, err
	}
	rep.Skips = skips
	for _, p := range plans {
		// Pre-check every projection destination before any write so a
		// conflict aborts with zero filesystem changes.
		if err := checkProjection(p); err != nil {
			return nil, err
		}
	}
	entry := Entry{
		Name:          m.Name,
		Type:          m.Type,
		Version:       m.Version,
		InstalledAt:   e.Now().UTC(),
		Source:        sourceLabel(opts.Source),
		CanonicalPath: dest,
		Projections:   []Projection{},
		Manifest:      *m,
	}

	_, destErr := os.Lstat(dest)
	destExists := destErr == nil
	managed := false
	if prev := reg.Find(m.Name); prev != nil && destExists && filepath.Clean(prev.CanonicalPath) == dest {
		// Ownership requires both the record and an intact digest
		// (review 018 blocker 1: drifted content is non-managed).
		if cur, err := DigestTree(dest); err == nil && cur == prev.Digest && prev.Digest != "" {
			managed = true
		}
	}

	switch {
	case destExists && !managed && !opts.Force:
		return nil, fmt.Errorf("%w: %s", ErrUnmanagedTarget, dest)
	case destExists && !managed && opts.Force:
		backupPath, id, err := e.planBackup(rel)
		if err != nil {
			return nil, err
		}
		rep.Ops = append(rep.Ops, Op{"backup", backupPath})
		entry.Backups = []Backup{{ID: id, Path: backupPath}}
		if !opts.DryRun {
			if err := copyTree(dest, backupPath); err != nil {
				return nil, fmt.Errorf("backup before overwrite: %w", err)
			}
		}
		rep.Ops = append(rep.Ops, Op{"replace", dest})
	case destExists && managed:
		rep.Ops = append(rep.Ops, Op{"replace", dest})
	default:
		rep.Ops = append(rep.Ops, Op{"create", dest})
	}
	for _, p := range plans {
		rep.Ops = append(rep.Ops, Op{projectionOpKind(p.target.Kind), p.target.Path})
	}
	rep.Ops = append(rep.Ops, Op{"replace", e.Layout.RegistryPath()})

	if opts.DryRun {
		return rep, nil
	}
	if err := place(payload, dest, destExists, e.backupID()); err != nil {
		return nil, err
	}
	digest, err := DigestTree(dest)
	if err != nil {
		return nil, fmt.Errorf("digest installed asset: %w", err)
	}
	entry.Digest = digest
	// Managed checkpoint before projections (review 030 blocker 2): if a
	// projection fails mid-way the canonical content is already registry-
	// owned, so a rerun converges instead of leaving unmanaged state.
	reg.Upsert(entry)
	if err := reg.Save(e.Layout.RegistryPath()); err != nil {
		return nil, err
	}
	var projErr error
	for _, p := range plans {
		if err := applyProjection(p); err != nil {
			projErr = err
			break
		}
		entry.Projections = append(entry.Projections, Projection{Agent: p.target.Agent, Path: p.target.Path, Kind: p.target.Kind})
	}
	reg.Upsert(entry)
	if err := reg.Save(e.Layout.RegistryPath()); err != nil {
		return nil, err
	}
	if projErr != nil {
		return nil, fmt.Errorf("projection incomplete (canonical is managed; rerun install to converge): %w", projErr)
	}
	return rep, nil
}

// Remove deletes a managed asset's canonical placement and registry entry.
// Drifted content is non-managed: refused without --force, which backs the
// drifted content up first. Backups on disk are kept (doctor reports
// orphans).
func (e *Engine) Remove(name string, opts Options) (*Report, error) {
	reg, err := LoadRegistry(e.Layout.RegistryPath())
	if err != nil {
		return nil, err
	}
	entry := reg.Find(name)
	if entry == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotManaged, name)
	}
	target, err := e.Layout.SafeCanonicalTarget(entry)
	if err != nil {
		return nil, err
	}
	rep := &Report{Name: name, DryRun: opts.DryRun}

	if _, statErr := os.Lstat(target); statErr == nil {
		cur, err := DigestTree(target)
		drifted := err != nil || cur != entry.Digest || entry.Digest == ""
		if drifted && !opts.Force {
			return nil, fmt.Errorf("%w: %s (content drifted from managed state)", ErrUnmanagedTarget, target)
		}
		if drifted && opts.Force {
			rel, relErr := e.Layout.CanonicalRel(&entry.Manifest)
			if relErr != nil {
				return nil, relErr
			}
			backupPath, _, err := e.planBackup(rel)
			if err != nil {
				return nil, err
			}
			rep.Ops = append(rep.Ops, Op{"backup", backupPath})
			if !opts.DryRun {
				if err := copyTree(target, backupPath); err != nil {
					return nil, fmt.Errorf("backup before remove: %w", err)
				}
			}
		}
	}

	for _, pr := range entry.Projections {
		rep.Ops = append(rep.Ops, Op{removalOpKind(pr.Kind), pr.Path})
	}
	rep.Ops = append(rep.Ops, Op{"delete", target}, Op{"replace", e.Layout.RegistryPath()})
	if opts.DryRun {
		return rep, nil
	}
	for _, pr := range entry.Projections {
		if removed, warn := e.removeProjection(entry, pr, target); !removed && warn != "" {
			rep.Warnings = append(rep.Warnings, warn)
		}
	}
	if err := os.RemoveAll(target); err != nil {
		return nil, err
	}
	reg.Drop(name)
	if err := reg.Save(e.Layout.RegistryPath()); err != nil {
		return nil, err
	}
	return rep, nil
}

// Rollback restores a recorded backup over the canonical path. It refuses
// when current content is not the managed state (review 018 blocker 2):
// restoring would silently destroy newer non-oma content.
func (e *Engine) Rollback(name, backupID string, opts Options) (*Report, error) {
	reg, err := LoadRegistry(e.Layout.RegistryPath())
	if err != nil {
		return nil, err
	}
	entry := reg.Find(name)
	if entry == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotManaged, name)
	}
	target, err := e.Layout.SafeCanonicalTarget(entry)
	if err != nil {
		return nil, err
	}
	if len(entry.Backups) == 0 {
		return nil, fmt.Errorf("%w: no backups recorded for %s", ErrInvalid, name)
	}
	b := entry.Backups[len(entry.Backups)-1]
	if backupID != "" {
		found := false
		for _, cand := range entry.Backups {
			if cand.ID == backupID {
				b, found = cand, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: backup id %q not recorded for %s", ErrInvalid, backupID, name)
		}
	}

	destExists := false
	if _, statErr := os.Lstat(target); statErr == nil {
		destExists = true
		cur, err := DigestTree(target)
		if err != nil || cur != entry.Digest || entry.Digest == "" {
			return nil, fmt.Errorf("%w: %s", ErrRollbackConflict, target)
		}
	}

	rep := &Report{Name: name, DryRun: opts.DryRun}
	rep.Ops = append(rep.Ops, Op{"restore", target}, Op{"replace", e.Layout.RegistryPath()})
	if opts.DryRun {
		return rep, nil
	}
	if err := place(b.Path, target, destExists, e.backupID()); err != nil {
		return nil, err
	}
	digest, err := DigestTree(target)
	if err != nil {
		return nil, fmt.Errorf("digest restored asset: %w", err)
	}
	entry.Digest = digest
	entry.RestoredFrom = b.ID // provenance: content is restored, not a fresh install
	if err := reg.Save(e.Layout.RegistryPath()); err != nil {
		return nil, err
	}
	// Symlink projections track the restored content automatically (same
	// canonical path); injected hook entries are copies and must be
	// re-injected from the restored fragment (replace-own semantics).
	if entry.Type == TypeHook {
		frag, err := e.loadFragment(target, &entry.Manifest)
		if err != nil {
			return nil, fmt.Errorf("restored hook asset: %w", err)
		}
		for _, pr := range entry.Projections {
			if pr.Kind != agentdir.KindInject {
				continue
			}
			expected, ok, _ := agentdir.For(e.Layout.Home, pr.Agent, entry.Type, entry.Name)
			if !ok || filepath.Clean(pr.Path) != expected.Path {
				return nil, fmt.Errorf("%w: recorded projection %s does not match expected path", ErrInvalid, pr.Path)
			}
			if err := e.checkProjectionRoot(pr.Agent, expected.Path); err != nil {
				return nil, err
			}
			if err := hookcfg.Inject(expected.Path, agentdir.HookWrapKey(pr.Agent), entry.Name, frag.Events[pr.Agent]); err != nil {
				return nil, fmt.Errorf("re-inject restored hooks: %w", err)
			}
		}
	}
	return rep, nil
}

// List returns the registry entries.
func (e *Engine) List() ([]Entry, error) {
	reg, err := LoadRegistry(e.Layout.RegistryPath())
	if err != nil {
		return nil, err
	}
	return reg.Assets, nil
}

// loadFragment reads fragment.json for hook assets (nil for other types).
// Validation happens here — before any write — so a broken fragment fails
// the whole install closed.
func (e *Engine) loadFragment(dir string, m *Manifest) (*hookcfg.Fragment, error) {
	if m.Type != TypeHook {
		return nil, nil
	}
	frag, err := hookcfg.LoadFragment(filepath.Join(dir, "fragment.json"))
	if err != nil {
		return nil, fmt.Errorf("%w: hook asset %s: %v", ErrInvalid, m.Name, err)
	}
	return frag, nil
}

// projectionOpKind / removalOpKind label the planned operation for
// --dry-run and reports.
func projectionOpKind(kind string) string {
	if kind == agentdir.KindInject {
		return "inject"
	}
	return "link"
}

func removalOpKind(kind string) string {
	if kind == agentdir.KindInject {
		return "uninject"
	}
	return "unlink"
}

// payloadPath resolves what gets copied: directory assets ship the whole
// source dir; file assets ship exactly "<name>.md".
func (e *Engine) payloadPath(srcDir string, m *Manifest) (string, error) {
	switch m.Type {
	case TypeSkill, TypeHook:
		return srcDir, nil
	case TypeSubagent, TypePrompt:
		p := filepath.Join(srcDir, m.Name+".md")
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%w: %s asset must contain %s.md: %v", ErrInvalid, m.Type, m.Name, err)
		}
		return p, nil
	default:
		return "", fmt.Errorf("%w: type %q", ErrInvalid, m.Type)
	}
}

// planBackup allocates a collision-checked backup path preserving the
// canonical-relative layout (review 018 non-blocking 1).
func (e *Engine) planBackup(rel string) (path, id string, err error) {
	id = e.backupID()
	path = filepath.Join(e.Layout.BackupDir(id), filepath.FromSlash(rel))
	if _, statErr := os.Lstat(path); statErr == nil {
		return "", "", fmt.Errorf("%w: backup target already exists: %s", ErrInvalid, path)
	}
	return path, id, nil
}

func (e *Engine) backupID() string { return e.Now().UTC().Format("20060102T150405.000000000Z") }

func sourceLabel(s string) string {
	if s == "" {
		return "dir"
	}
	return s
}

// place puts src at dest: plain stage+rename for fresh creates, recoverable
// swap for replacements (review 018 blocker 4 — no window where dest is
// missing without a restorable sibling).
func place(src, dest string, destExists bool, swapID string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	stage := dest + ".oma-stage"
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := copyTree(src, stage); err != nil {
		_ = os.RemoveAll(stage)
		return err
	}
	if !destExists {
		return os.Rename(stage, dest)
	}
	old := dest + ".oma-old-" + swapID
	// A pre-existing old sibling is a stale recovery artifact from an
	// interrupted swap: fail closed rather than destroy it (recheck 020).
	if _, statErr := os.Lstat(old); statErr == nil {
		_ = os.RemoveAll(stage)
		return fmt.Errorf("%w: recovery sibling already exists: %s (recover or clean it first)", ErrInvalid, old)
	}
	if err := os.Rename(dest, old); err != nil {
		_ = os.RemoveAll(stage)
		return err
	}
	if err := os.Rename(stage, dest); err != nil {
		_ = os.Rename(old, dest) // restore previous content
		_ = os.RemoveAll(stage)
		return fmt.Errorf("swap failed, previous content restored: %w", err)
	}
	_ = os.RemoveAll(old)
	syncDir(filepath.Dir(dest))
	return nil
}

// syncDir best-effort fsyncs a directory so renames become durable.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// copyTree copies a file or directory tree with 0700 dirs and 0600 files,
// refusing symlinks inside payloads (path-constraint defense,
// docs/security-contract.md §3).
func copyTree(src, dest string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("%w: refusing symlink in payload: %s", ErrInvalid, src)
	case info.IsDir():
		if err := os.MkdirAll(dest, 0o700); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, ent := range entries {
			if err := copyTree(filepath.Join(src, ent.Name()), filepath.Join(dest, ent.Name())); err != nil {
				return err
			}
		}
		return nil
	default:
		// Reject FIFOs, devices, sockets before opening: opening a special
		// file could hang or perform unintended IO ahead of the digest
		// layer's rejection (B3 re-recheck finding 1).
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: refusing non-regular file in payload: %s", ErrInvalid, src)
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
}

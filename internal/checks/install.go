package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/agentdir"
	"github.com/sean2077/oh-my-agents/internal/asset"
	"github.com/sean2077/oh-my-agents/internal/relay"
)

// InstallChecks builds the asset-domain doctor checks for one home/project.
// commands is the registered CLI surface for refcheck validation.
func InstallChecks(home, projectRoot string, commands CommandSet) []Check {
	eng := asset.NewEngine(home)
	return []Check{
		{Name: "registry-consistency", Run: func() []Finding { return registryConsistency(eng) }},
		{Name: "permissions", Run: func() []Finding { return permissions(eng) }},
		{Name: "orphan-backups", Run: func() []Finding { return orphanBackups(eng) }},
		{Name: "legacy-relay-v1", Run: func() []Finding { return legacyRelayV1(projectRoot) }},
		{Name: "relay-v2-residue", Run: func() []Finding { return relayV2Residue(projectRoot) }},
		{Name: "refcheck-installed", Run: func() []Finding {
			return Refcheck(filepath.Join(eng.Layout.CanonicalRoot(), "skills"), commands)
		}},
	}
}

// relayV2Residue reports stale drafts, leftover reservations and unready
// formal files across active v2 pairs (docs/reference/command-tree.md §5: doctor
// surfaces them; `oma doctor relay --clean-stale` removes them). Identity
// is irrelevant for reading residue, so a fixed probe identity is used;
// a missing/uninitialized ledger is silent (nothing to check).
func relayV2Residue(projectRoot string) []Finding {
	if projectRoot == "" {
		return nil
	}
	id, err := relay.ResolveIdentity(func(k string) string {
		if k == "OMA_RELAY_AUTHOR" {
			return "doctor-probe"
		}
		return ""
	})
	if err != nil {
		return nil
	}
	l := relay.NewLedger(filepath.Join(projectRoot, ".oma", "relay"), id)
	if err := l.Open(); err != nil {
		return nil // not initialized or not v2: the legacy check covers v1
	}
	slugs, err := l.AllPairs()
	if err != nil {
		return nil
	}
	var fs []Finding
	for _, slug := range slugs {
		actions, err := l.CleanStale(slug, true) // dry-run: report only
		if err != nil {
			fs = append(fs, Finding{Level: LevelWarn, Message: fmt.Sprintf("relay %s: %v", slug, err)})
			continue
		}
		for _, a := range actions {
			fs = append(fs, Finding{Level: LevelWarn, Message: fmt.Sprintf("relay %s: %s (oma doctor relay --clean-stale)", slug, a)})
		}
	}
	return fs
}

// registryConsistency verifies every registry entry: canonical present,
// digest intact, projections healthy.
func registryConsistency(eng *asset.Engine) []Finding {
	entries, err := eng.List()
	if err != nil {
		return []Finding{{Level: LevelFail, Message: fmt.Sprintf("registry unreadable: %v", err)}}
	}
	var fs []Finding
	for i := range entries {
		e := &entries[i]
		if _, err := os.Lstat(e.CanonicalPath); err != nil {
			fs = append(fs, Finding{Level: LevelFail, Message: fmt.Sprintf("%s: canonical missing (%s)", e.Name, e.CanonicalPath)})
			continue
		}
		if cur, err := asset.DigestTree(e.CanonicalPath); err != nil || cur != e.Digest {
			fs = append(fs, Finding{Level: LevelWarn, Message: fmt.Sprintf("%s: content drifted from managed state (reinstall or remove --force)", e.Name)})
		}
		if ok, problems := eng.VerifyProjections(e); !ok {
			fs = append(fs, Finding{Level: LevelWarn, Message: fmt.Sprintf("%s: %s", e.Name, strings.Join(problems, "; "))})
		}
		// trusted-root drift after install is fail-grade (review 038)
		for _, p := range eng.VerifyProjectionSecurity(e) {
			fs = append(fs, Finding{Level: LevelFail, Message: fmt.Sprintf("%s: %s", e.Name, p)})
		}
	}
	return fs
}

// permissions reports mode drift on oma-owned roots and agent trusted
// roots. POSIX world-writable paths are fail-grade; on Windows, Go's mode bits
// are only an ACL approximation and commonly report user-owned directories as
// 0777, so this check is skipped.
func permissions(eng *asset.Engine) []Finding {
	if runtime.GOOS == "windows" {
		return nil
	}
	var fs []Finding
	dirs := []string{eng.Layout.CanonicalRoot(), eng.Layout.ConfigDir()}
	for _, agent := range []string{"claude", "codex"} {
		dirs = append(dirs, agentdir.AgentRoot(eng.Layout.Home, agent))
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			fs = append(fs, Finding{Level: LevelWarn, Message: fmt.Sprintf("%s: %v", dir, err)})
			continue
		}
		if perm := info.Mode().Perm(); perm&0o002 != 0 {
			fs = append(fs, Finding{Level: LevelFail, Message: fmt.Sprintf("%s is world-writable (%o)", dir, perm)})
		}
	}
	return fs
}

// orphanBackups reports backup directories no registry entry references.
func orphanBackups(eng *asset.Engine) []Finding {
	backupRoot := filepath.Join(eng.Layout.ConfigDir(), "backups")
	ids, err := os.ReadDir(backupRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return []Finding{{Level: LevelWarn, Message: err.Error()}}
	}
	entries, err := eng.List()
	if err != nil {
		return []Finding{{Level: LevelFail, Message: fmt.Sprintf("registry unreadable: %v", err)}}
	}
	referenced := map[string]bool{}
	for _, e := range entries {
		for _, b := range e.Backups {
			referenced[b.ID] = true
		}
	}
	var fs []Finding
	for _, id := range ids {
		if !referenced[id.Name()] {
			fs = append(fs, Finding{Level: LevelWarn,
				Message: fmt.Sprintf("orphan backup %s (no registry reference; safe to archive or delete manually)", filepath.Join(backupRoot, id.Name()))})
		}
	}
	return fs
}

// legacyRelayV1 reports an agent-ledger v1 .shared/ tree in the project:
// archival/manual-reference only, oma never reads or writes it
// (docs/reference/relay-v2-protocol.md §1).
func legacyRelayV1(projectRoot string) []Finding {
	if projectRoot == "" {
		return nil
	}
	shared := filepath.Join(projectRoot, ".shared")
	if _, err := os.Stat(filepath.Join(shared, "_relay")); err == nil {
		return []Finding{{Level: LevelWarn,
			Message: fmt.Sprintf("legacy agent-ledger v1 ledger at %s: archival/manual reference only; oma relay uses .oma/relay/", shared)}}
	}
	return nil
}

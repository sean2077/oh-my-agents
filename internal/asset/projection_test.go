package asset

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sean2077/oh-my-agents/internal/agentdir"
	"github.com/sean2077/oh-my-agents/internal/hookcfg"
)

func TestInstallProjectsToBothAgents(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	rep := mustInstall(t, e, src, Options{})
	if len(rep.Skips) != 0 {
		t.Fatalf("unexpected skips: %+v", rep.Skips)
	}
	canonical := filepath.Join(e.Layout.CanonicalRoot(), "skills", "x")
	for _, link := range []string{
		filepath.Join(e.Layout.Home, ".claude", "skills", "x"),
		filepath.Join(e.Layout.Home, ".codex", "skills", "x"),
	} {
		dest, err := os.Readlink(link)
		if err != nil || dest != canonical {
			t.Fatalf("projection %s -> %q err=%v, want %s", link, dest, err, canonical)
		}
	}
	entries, _ := e.List()
	if len(entries[0].Projections) != 2 {
		t.Fatalf("registry projections = %+v", entries[0].Projections)
	}
}

func TestAgentNarrowingIntersectsManifestTargets(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	rep := mustInstall(t, e, src, Options{Agents: []string{"claude"}})
	if len(rep.Skips) != 0 {
		t.Fatalf("narrowed install skips: %+v", rep.Skips)
	}
	if _, err := os.Lstat(filepath.Join(e.Layout.Home, ".codex", "skills", "x")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("codex projection must not exist when narrowed to claude")
	}
}

func TestClaudeOnlySubagentSkipsCodexWithReason(t *testing.T) {
	e := newTestEngine(t)
	dir := t.TempDir()
	manifest := `{"schema": "oma-asset/1", "name": "explorer", "type": "subagent",
		"targets": ["claude"], "fallback": "codex explores inline"}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "explorer.md"), []byte("agent def"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep := mustInstall(t, e, dir, Options{})
	if len(rep.Skips) != 1 || rep.Skips[0].Agent != "codex" {
		t.Fatalf("want one codex skip, got %+v", rep.Skips)
	}
	link := filepath.Join(e.Layout.Home, ".claude", "agents", "explorer.md")
	if _, err := os.Readlink(link); err != nil {
		t.Fatalf("claude subagent projection missing: %v", err)
	}
}

func TestProjectionConflictAbortsWithZeroWrites(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	foreign := filepath.Join(e.Layout.Home, ".claude", "skills", "x")
	if err := os.MkdirAll(foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := e.Install(src, Options{})
	if !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("err = %v, want ErrProjectionConflict", err)
	}
	if _, err := os.Lstat(filepath.Join(e.Layout.CanonicalRoot(), "skills", "x")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canonical must not be placed when projection pre-check fails")
	}
	if _, err := os.Lstat(e.Layout.RegistryPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("registry must not be written when projection pre-check fails")
	}
	// --force does not override projection conflicts (canonical-only semantics)
	if _, err := e.Install(src, Options{Force: true}); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("force must not stomp projections: %v", err)
	}
}

func TestReinstallRefreshesProjectionIdempotently(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "v1")
	mustInstall(t, e, src, Options{})
	mustInstall(t, e, src, Options{}) // managed reinstall, same links
	link := filepath.Join(e.Layout.Home, ".claude", "skills", "x")
	if dest, err := os.Readlink(link); err != nil || dest != filepath.Join(e.Layout.CanonicalRoot(), "skills", "x") {
		t.Fatalf("link after reinstall: %q err=%v", dest, err)
	}
}

func TestRemoveDeletesOnlyOwnLinks(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	mustInstall(t, e, src, Options{})

	// replace the codex projection with a foreign regular file
	codexLink := filepath.Join(e.Layout.Home, ".codex", "skills", "x")
	if err := os.Remove(codexLink); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexLink, []byte("user's own file"), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := e.Remove("x", Options{})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(rep.Warnings) != 1 {
		t.Fatalf("want one warning for foreign file, got %+v", rep.Warnings)
	}
	if _, err := os.Stat(codexLink); err != nil {
		t.Fatal("foreign file must be left intact")
	}
	if _, err := os.Lstat(filepath.Join(e.Layout.Home, ".claude", "skills", "x")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("own claude link must be removed")
	}
}

func TestVerifyProjectionsDetectsBreakage(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	mustInstall(t, e, src, Options{})
	entries, _ := e.List()
	if ok, problems := e.VerifyProjections(&entries[0]); !ok {
		t.Fatalf("fresh install must verify: %v", problems)
	}
	if err := os.Remove(filepath.Join(e.Layout.Home, ".claude", "skills", "x")); err != nil {
		t.Fatal(err)
	}
	if ok, problems := e.VerifyProjections(&entries[0]); ok || len(problems) == 0 {
		t.Fatal("broken projection must be detected")
	}
}

func TestProjectionRootSymlinkEscapeRefused(t *testing.T) {
	e := newTestEngine(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(e.Layout.Home, ".claude")); err != nil {
		t.Skip("symlinks unavailable")
	}
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	_, err := e.Install(src, Options{Agents: []string{"claude"}})
	if err == nil || !strings.Contains(err.Error(), "outside home") {
		t.Fatalf("symlinked agent root: err = %v, want outside-home refusal", err)
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("nothing may be written through the escaping root")
	}
}

func TestProjectionRootWorldWritableRefused(t *testing.T) {
	e := newTestEngine(t)
	claudeRoot := filepath.Join(e.Layout.Home, ".claude")
	if err := os.MkdirAll(claudeRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(claudeRoot, 0o777); err != nil {
		t.Fatal(err)
	}
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	_, err := e.Install(src, Options{Agents: []string{"claude"}})
	if err == nil || !strings.Contains(err.Error(), "world-writable") {
		t.Fatalf("world-writable nearest ancestor: err = %v, want refusal", err)
	}
}

func TestNestedProjectionSymlinkEscapeRefused(t *testing.T) {
	e := newTestEngine(t)
	outside := t.TempDir()
	claudeRoot := filepath.Join(e.Layout.Home, ".claude")
	if err := os.MkdirAll(claudeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(claudeRoot, "skills")); err != nil {
		t.Skip("symlinks unavailable")
	}
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	_, err := e.Install(src, Options{Agents: []string{"claude"}})
	if err == nil || !strings.Contains(err.Error(), "intermediate symlink escape") {
		t.Fatalf("nested symlink escape: err = %v, want refusal", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("nothing may be written through the nested escaping component")
	}
}

func TestRemoveRefusesNestedSymlinkEscape(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	mustInstall(t, e, src, Options{Agents: []string{"claude"}})

	// swap the real skills dir for an escaping symlink after install
	outside := t.TempDir()
	skillsDir := filepath.Join(e.Layout.Home, ".claude", "skills")
	if err := os.RemoveAll(skillsDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, skillsDir); err != nil {
		t.Skip("symlinks unavailable")
	}
	canonical := filepath.Join(e.Layout.CanonicalRoot(), "skills", "x")
	if err := os.Symlink(canonical, filepath.Join(outside, "x")); err != nil {
		t.Fatal(err)
	}

	rep, err := e.Remove("x", Options{})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(rep.Warnings) == 0 {
		t.Fatalf("escaping projection path must warn, got %+v", rep)
	}
	if _, err := os.Lstat(filepath.Join(outside, "x")); err != nil {
		t.Fatal("link behind the escaping component must be left intact")
	}
}

func TestPartialProjectionConvergesToManaged(t *testing.T) {
	e := newTestEngine(t)
	codexRoot := filepath.Join(e.Layout.Home, ".codex")
	if err := os.MkdirAll(codexRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(codexRoot, 0o500); err != nil { // not writable: codex projection will fail
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(codexRoot, 0o700) })

	src := writeSkillSource(t, t.TempDir(), "x", "body")
	_, err := e.Install(src, Options{})
	if err == nil || !strings.Contains(err.Error(), "rerun install to converge") {
		t.Fatalf("partial projection: err = %v, want converge hint", err)
	}
	// canonical must be registry-owned despite the failure
	entries, lerr := e.List()
	if lerr != nil || len(entries) != 1 {
		t.Fatalf("registry after partial apply: %+v err=%v", entries, lerr)
	}
	// rerun converges once the obstacle is removed
	if err := os.Chmod(codexRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Install(src, Options{}); err != nil {
		t.Fatalf("converging reinstall: %v", err)
	}
	entries, _ = e.List()
	if len(entries[0].Projections) != 2 {
		t.Fatalf("after convergence projections = %+v", entries[0].Projections)
	}
}

func TestRemoveProjectionRevalidatesRecordedPath(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	mustInstall(t, e, src, Options{})

	outside := filepath.Join(t.TempDir(), "outside-link")
	canonical := filepath.Join(e.Layout.CanonicalRoot(), "skills", "x")
	if err := os.Symlink(canonical, outside); err != nil {
		t.Skip("symlinks unavailable")
	}
	reg, err := LoadRegistry(e.Layout.RegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	reg.Find("x").Projections = []Projection{{Agent: "claude", Path: outside, Kind: "symlink"}}
	if err := reg.Save(e.Layout.RegistryPath()); err != nil {
		t.Fatal(err)
	}

	rep, err := e.Remove("x", Options{})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(rep.Warnings) == 0 {
		t.Fatalf("tampered projection path must warn, got %+v", rep)
	}
	if _, err := os.Lstat(outside); err != nil {
		t.Fatal("outside link must be left intact")
	}
}

func TestZeroProjectionEntryDegradesHealth(t *testing.T) {
	// review 048 adjacent hardening (approved in 050): an installed entry
	// whose manifest could project somewhere but holds zero projections is
	// inert and must not report healthy.
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	mustInstall(t, e, src, Options{})
	entries, _ := e.List()

	reg, err := LoadRegistry(e.Layout.RegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	reg.Find("x").Projections = nil // simulate a TOCTOU-interrupted checkpoint
	if err := reg.Save(e.Layout.RegistryPath()); err != nil {
		t.Fatal(err)
	}
	entries, _ = e.List()
	ok, problems := e.VerifyProjections(&entries[0])
	if ok || len(problems) == 0 || !strings.Contains(problems[0], "no projections applied") {
		t.Fatalf("zero-projection entry: ok=%v problems=%v", ok, problems)
	}

	// shared-only assets legitimately project nowhere: stays healthy.
	dir := t.TempDir()
	manifest := `{"schema": "oma-asset/1", "name": "shared-thing", "type": "skill", "targets": ["shared"]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustInstall(t, e, dir, Options{})
	entries, _ = e.List()
	for i := range entries {
		if entries[i].Name != "shared-thing" {
			continue
		}
		if ok, problems := e.VerifyProjections(&entries[i]); !ok {
			t.Fatalf("shared-only must stay healthy: %v", problems)
		}
	}
}

// conformanceFixture describes the expected projection layout for one agent
// (testdata/conformance/<agent>.json, docs/adapter-conformance.md §6).
type conformanceFixture struct {
	Agent string `json:"agent"`
	Cases []struct {
		Manifest       json.RawMessage `json:"manifest"`
		PayloadFile    string          `json:"payload_file"`
		PayloadContent string          `json:"payload_content"` // "" = placeholder bytes
		WantRelHome    string          `json:"want_rel_home"`   // "" = no projection expected
		WantKind       string          `json:"want_kind"`       // "" = symlink
	} `json:"cases"`
}

func TestConformanceFixtures(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "conformance", agent+".json"))
		if err != nil {
			t.Fatalf("fixture %s: %v", agent, err)
		}
		var fx conformanceFixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatalf("fixture %s: %v", agent, err)
		}
		for _, c := range fx.Cases {
			m, err := ParseManifest(c.Manifest)
			if err != nil {
				t.Fatalf("%s fixture manifest: %v", agent, err)
			}
			e := newTestEngine(t)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "manifest.json"), c.Manifest, 0o600); err != nil {
				t.Fatal(err)
			}
			content := c.PayloadContent
			if content == "" {
				content = "payload"
			}
			if err := os.WriteFile(filepath.Join(dir, c.PayloadFile), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			rep, err := e.Install(dir, Options{Agents: []string{agent}})
			if err != nil {
				t.Fatalf("%s/%s install: %v", agent, m.Name, err)
			}
			want := c.WantRelHome
			if want == "" {
				if len(rep.Skips) == 0 {
					t.Errorf("%s/%s: expected a skip, got projections", agent, m.Name)
				}
				continue
			}
			link := filepath.Join(e.Layout.Home, filepath.FromSlash(want))
			if c.WantKind == agentdir.KindInject {
				cmds, err := hookcfg.OwnCommands(link, agentdir.HookWrapKey(agent), m.Name)
				if err != nil || len(cmds) == 0 {
					t.Errorf("%s/%s: expected injected entries in %s: cmds=%v err=%v", agent, m.Name, want, cmds, err)
				}
				continue
			}
			dest, err := os.Readlink(link)
			if err != nil {
				t.Errorf("%s/%s: expected projection at %s: %v", agent, m.Name, want, err)
				continue
			}
			if filepath.Clean(dest) != filepath.Clean(filepath.Join(e.Layout.CanonicalRoot(), filepath.FromSlash(mustRel(t, e, m)))) {
				t.Errorf("%s/%s: link target %s unexpected", agent, m.Name, dest)
			}
		}
	}
}

func mustRel(t *testing.T, e *Engine, m *Manifest) string {
	t.Helper()
	rel, err := e.Layout.CanonicalRel(m)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

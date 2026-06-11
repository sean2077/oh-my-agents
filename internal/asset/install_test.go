package asset

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestEngine anchors everything in a temp home with a fixed clock.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e := NewEngine(t.TempDir())
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	n := 0
	e.Now = func() time.Time { n++; return base.Add(time.Duration(n) * time.Second) }
	return e
}

// writeSkillSource builds a minimal skill asset source dir.
func writeSkillSource(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema": "oma-asset/1", "name": "` + name + `", "type": "skill", "targets": ["claude", "codex"]}`
	for file, content := range map[string]string{
		"manifest.json":          manifest,
		"SKILL.md":               body,
		"references/protocol.md": "details",
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestInstallSkillCreatesCanonicalAndRegistry(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "deep-interview", "skill body")

	rep, err := e.Install(src, Options{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if rep.Ops[0].Kind != "create" {
		t.Fatalf("first op = %+v, want create", rep.Ops[0])
	}
	canonical := filepath.Join(e.Layout.Home, ".agents", "skills", "deep-interview")
	for _, f := range []string{"SKILL.md", "references/protocol.md", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(canonical, f)); err != nil {
			t.Errorf("canonical missing %s: %v", f, err)
		}
	}
	entries, err := e.List()
	if err != nil || len(entries) != 1 || entries[0].Name != "deep-interview" {
		t.Fatalf("registry entries = %+v, err=%v", entries, err)
	}
	if entries[0].Manifest.Schema != ManifestSchema {
		t.Fatal("registry must snapshot the manifest")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	rep, err := e.Install(src, Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run install: %v", err)
	}
	if len(rep.Ops) == 0 || !rep.DryRun {
		t.Fatalf("dry-run report = %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(e.Layout.Home, ".agents")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry-run must not create canonical root")
	}
	if _, err := os.Stat(e.Layout.RegistryPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry-run must not write registry")
	}
}

func TestUnmanagedCollisionRefusedThenForcedWithBackupAndRollback(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "new body")

	canonical := filepath.Join(e.Layout.Home, ".agents", "skills", "x")
	if err := os.MkdirAll(canonical, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(canonical, "SKILL.md")
	if err := os.WriteFile(foreign, []byte("foreign content"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := e.Install(src, Options{}); !errors.Is(err, ErrUnmanagedTarget) {
		t.Fatalf("collision: err = %v, want ErrUnmanagedTarget", err)
	}

	rep, err := e.Install(src, Options{Force: true})
	if err != nil {
		t.Fatalf("force install: %v", err)
	}
	if rep.Ops[0].Kind != "backup" {
		t.Fatalf("force must back up first, got %+v", rep.Ops)
	}
	got, _ := os.ReadFile(foreign)
	if string(got) != "new body" {
		t.Fatalf("canonical SKILL.md = %q, want new body", got)
	}

	if _, err := e.Rollback("x", "", Options{}); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	got, _ = os.ReadFile(foreign)
	if string(got) != "foreign content" {
		t.Fatalf("after rollback SKILL.md = %q, want foreign content", got)
	}
}

func TestManagedReinstallNeedsNoForce(t *testing.T) {
	e := newTestEngine(t)
	root := t.TempDir()
	src := writeSkillSource(t, root, "x", "v1")
	if _, err := e.Install(src, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Install(src, Options{}); err != nil {
		t.Fatalf("managed reinstall: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(e.Layout.Home, ".agents", "skills", "x", "SKILL.md"))
	if string(got) != "v2" {
		t.Fatalf("reinstall content = %q, want v2", got)
	}
}

func TestRemoveDeletesCanonicalAndEntry(t *testing.T) {
	e := newTestEngine(t)
	src := writeSkillSource(t, t.TempDir(), "x", "body")
	if _, err := e.Install(src, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Remove("x", Options{}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.Layout.Home, ".agents", "skills", "x")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canonical must be deleted")
	}
	if entries, _ := e.List(); len(entries) != 0 {
		t.Fatalf("registry should be empty, got %+v", entries)
	}
	if _, err := e.Remove("x", Options{}); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("double remove: err = %v, want ErrNotManaged", err)
	}
}

func TestSubagentFileLayout(t *testing.T) {
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
	if _, err := e.Install(dir, Options{}); err != nil {
		t.Fatalf("install subagent: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(e.Layout.Home, ".agents", "agents", "explorer.md"))
	if err != nil || string(got) != "agent def" {
		t.Fatalf("canonical subagent file: %q err=%v", got, err)
	}
}

func TestPayloadSymlinkRefused(t *testing.T) {
	e := newTestEngine(t)
	root := t.TempDir()
	src := writeSkillSource(t, root, "x", "body")
	if err := os.Symlink("/etc/passwd", filepath.Join(src, "evil")); err != nil {
		t.Skip("symlinks unavailable")
	}
	_, err := e.Install(src, Options{})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("payload symlink: err = %v, want refusal", err)
	}
}

func TestRegistryUnknownMajorFailsClosed(t *testing.T) {
	e := newTestEngine(t)
	path := e.Layout.RegistryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema": "oma-registry/9", "assets": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.List(); !errors.Is(err, ErrUnknownSchema) {
		t.Fatalf("unknown registry major: err = %v, want ErrUnknownSchema", err)
	}
}

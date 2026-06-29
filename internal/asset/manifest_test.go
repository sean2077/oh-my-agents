package asset

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func valid() string {
	return `{
		"schema": "oma-asset/1",
		"name": "deep-interview",
		"type": "skill",
		"targets": ["claude", "codex"],
		"description_budget_tokens": 80
	}`
}

func TestParseValidManifest(t *testing.T) {
	m, err := ParseManifest([]byte(valid()))
	if err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	if m.Name != "deep-interview" || !m.HasTarget(TargetCodex) {
		t.Fatalf("unexpected parse result: %+v", m)
	}
}

func TestUnknownSchemaMajorFailsClosed(t *testing.T) {
	for _, schema := range []string{"oma-asset/2", "oma-asset/x", "oma-registry/1", ""} {
		raw := `{"schema": "` + schema + `", "name": "x", "type": "skill", "targets": ["codex"]}`
		_, err := ParseManifest([]byte(raw))
		if !errors.Is(err, ErrUnknownSchema) {
			t.Errorf("schema %q: err = %v, want ErrUnknownSchema", schema, err)
		}
	}
}

func TestNameRejectsTraversalAndUppercase(t *testing.T) {
	for _, name := range []string{"../escape", "a/b", "UPPER", "-lead", "", "x x"} {
		raw := `{"schema": "oma-asset/1", "name": "` + name + `", "type": "skill", "targets": ["codex"]}`
		if _, err := ParseManifest([]byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Errorf("name %q: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestTargetValidation(t *testing.T) {
	cases := []struct {
		targets string
		wantErr bool
	}{
		{`["claude", "codex"]`, false},
		{`["shared"]`, false},
		{`[]`, true},
		{`["windsurf"]`, true},
		{`["codex", "codex"]`, true},
	}
	for _, tc := range cases {
		raw := `{"schema": "oma-asset/1", "name": "x", "type": "hook", "targets": ` + tc.targets + `}`
		_, err := ParseManifest([]byte(raw))
		if (err != nil) != tc.wantErr {
			t.Errorf("targets %s: err = %v, wantErr=%v", tc.targets, err, tc.wantErr)
		}
	}
}

func TestClaudeOnlyRequiresFallback(t *testing.T) {
	raw := `{"schema": "oma-asset/1", "name": "explorer", "type": "subagent", "targets": ["claude"]}`
	if _, err := ParseManifest([]byte(raw)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("claude-only without fallback: err = %v, want ErrInvalid", err)
	}
	withFallback := `{"schema": "oma-asset/1", "name": "explorer", "type": "subagent",
		"targets": ["claude"], "fallback": "codex runs the exploration inline, single-threaded"}`
	if _, err := ParseManifest([]byte(withFallback)); err != nil {
		t.Fatalf("claude-only with fallback rejected: %v", err)
	}
}

func TestClaudeSharedStillRequiresFallback(t *testing.T) {
	raw := `{"schema": "oma-asset/1", "name": "x", "type": "skill", "targets": ["claude", "shared"]}`
	if _, err := ParseManifest([]byte(raw)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("claude+shared without fallback: err = %v, want ErrInvalid", err)
	}
	withFallback := `{"schema": "oma-asset/1", "name": "x", "type": "skill",
		"targets": ["claude", "shared"], "fallback": "codex reads the canonical copy manually"}`
	if _, err := ParseManifest([]byte(withFallback)); err != nil {
		t.Fatalf("claude+shared with fallback rejected: %v", err)
	}
}

func TestUnknownFieldsTolerated(t *testing.T) {
	raw := `{"schema": "oma-asset/1", "name": "x", "type": "prompt", "targets": ["codex"],
		"future_minor_field": {"nested": true}}`
	if _, err := ParseManifest([]byte(raw)); err != nil {
		t.Fatalf("minor-additive field rejected: %v", err)
	}
}

func TestDigestFramingUnambiguous(t *testing.T) {
	// The classic concatenation-ambiguity pair: one file "a" containing
	// x\0b\0y, versus two files a=x and b=y. Length-prefixed framing must
	// give them different digests (B3 recheck blocker 2).
	treeA := t.TempDir()
	if err := os.WriteFile(filepath.Join(treeA, "a"), []byte("x\x00b\x00y"), 0o600); err != nil {
		t.Fatal(err)
	}
	treeB := t.TempDir()
	if err := os.WriteFile(filepath.Join(treeB, "a"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(treeB, "b"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	da, err := DigestTree(treeA)
	if err != nil {
		t.Fatal(err)
	}
	db, err := DigestTree(treeB)
	if err != nil {
		t.Fatal(err)
	}
	if da == db {
		t.Fatal("ambiguous tree pair produced the same digest")
	}
}

func TestDigestDetectsEmptyDirChange(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := DigestTree(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	after, err := DigestTree(base)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("adding an empty directory must change the digest")
	}
}

func TestLoadManifestFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(valid()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if _, err := LoadManifest(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file must error")
	}
}

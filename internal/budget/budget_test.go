package budget

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sean2077/oh-my-agents/internal/asset"
)

func TestTokensCeilSemantics(t *testing.T) {
	cases := map[string]int{"": 0, "abcd": 1, "abcde": 2, "一二": 2 /* 6 utf8 bytes */}
	for s, want := range cases {
		if got := Tokens(s); got != want {
			t.Errorf("Tokens(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestReadFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	md := `---
name: deep-interview
description: >-
  Socratic interview with
  ambiguity gating.
---
body text`
	if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	fm, err := ReadFrontmatterFile(path)
	if err != nil {
		t.Fatalf("frontmatter: %v", err)
	}
	if fm["name"] != "deep-interview" {
		t.Fatalf("name = %q", fm["name"])
	}
	if fm["description"] != "Socratic interview with ambiguity gating." {
		t.Fatalf("description = %q", fm["description"])
	}
}

func TestReadFrontmatterRejectsMissingBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("just body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFrontmatterFile(path); err == nil {
		t.Fatal("missing frontmatter must error")
	}
}

func installSkillWithFrontmatter(t *testing.T, home, name, description string) *asset.Engine {
	t.Helper()
	eng := asset.NewEngine(home)
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	n := 0
	eng.Now = func() time.Time { n++; return base.Add(time.Duration(n) * time.Second) }

	src := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"oma-asset/1","name":"` + name + `","type":"skill","targets":["claude","codex"]}`
	if err := os.WriteFile(filepath.Join(src, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: " + description + "\n---\nbody"
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Install(src, asset.Options{}); err != nil {
		t.Fatalf("install: %v", err)
	}
	return eng
}

func TestMeasureCountsInstalledProfileMembers(t *testing.T) {
	home := t.TempDir()
	desc := strings.Repeat("d", 80) // 80 bytes → 20 tokens
	eng := installSkillWithFrontmatter(t, home, "deep-interview", desc)

	rep, err := Measure(eng, "claude", "core4", 2000)
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	// name "deep-interview" = 14 bytes → 4 tokens; description 20 tokens
	if rep.Total != 24 {
		t.Fatalf("total = %d, want 24; items %+v", rep.Total, rep.Items)
	}
	if len(rep.Missing) != 3 {
		t.Fatalf("missing = %v, want 3 absent core4 members", rep.Missing)
	}
	if rep.Algo != AlgoVersion {
		t.Fatalf("algo = %q", rep.Algo)
	}
}

func TestMeasureSkipsAssetsNotProjectedToAgent(t *testing.T) {
	home := t.TempDir()
	eng := installSkillWithFrontmatter(t, home, "deep-interview", "x")
	rep, err := Measure(eng, "codex", "core4", 2000)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total == 0 {
		t.Fatal("codex projection exists; should count")
	}
	// remove codex projection from registry by narrowing a fresh install
	home2 := t.TempDir()
	eng2 := asset.NewEngine(home2)
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	n := 0
	eng2.Now = func() time.Time { n++; return base.Add(time.Duration(n) * time.Second) }
	src := filepath.Join(t.TempDir(), "deep-interview")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"oma-asset/1","name":"deep-interview","type":"skill","targets":["claude","codex"]}`
	if err := os.WriteFile(filepath.Join(src, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: deep-interview\ndescription: x\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := eng2.Install(src, asset.Options{Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	rep2, err := Measure(eng2, "codex", "core4", 2000)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Total != 0 {
		t.Fatalf("not projected to codex but counted: %+v", rep2.Items)
	}
}

func TestMeasureUnknownProfileFails(t *testing.T) {
	eng := asset.NewEngine(t.TempDir())
	if _, err := Measure(eng, "claude", "bogus", 2000); !errors.Is(err, ErrBudget) {
		t.Fatalf("unknown profile: err = %v", err)
	}
}

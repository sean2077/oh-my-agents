package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sean2077/oh-my-agents/internal/asset"
	"github.com/sean2077/oh-my-agents/internal/budget"
	"github.com/sean2077/oh-my-agents/internal/checks"
	"go.yaml.in/yaml/v3"
)

// TestRealAssetsPassReleaseGates is the Phase C release-blocking gate
// (docs/reference/adapter-conformance.md §4): every shipped asset under assets/
// must install cleanly, project healthily, pass refcheck with ZERO
// exemptions against the real command surface, and keep the core4
// resident budget within the threshold.
func TestRealAssetsPassReleaseGates(t *testing.T) {
	repoAssets := filepath.Join("..", "..", "assets", "skills")
	entries, err := os.ReadDir(repoAssets)
	if err != nil {
		t.Skipf("no shipped assets yet: %v", err)
	}
	assertNpxSkillGroupingManifest(t, repoAssets, entries)

	home := t.TempDir()
	eng := asset.NewEngine(home)
	base := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	n := 0
	eng.Now = func() time.Time { n++; return base.Add(time.Duration(n) * time.Second) }

	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		src := filepath.Join(repoAssets, ent.Name())
		assertPortableSkillFrontmatter(t, filepath.Join(src, "SKILL.md"))
		m, err := asset.LoadManifest(filepath.Join(src, "manifest.json"))
		if err != nil {
			t.Fatalf("manifest %s: %v", ent.Name(), err)
		}
		// Core workflow skills must stay dual-agent (adapter-conformance
		// §3); an accidental single-target drop fails the release gate
		// (review 068 follow-up).
		if !m.HasTarget("claude") || !m.HasTarget("codex") {
			t.Fatalf("skill %s targets %v: core workflow skills must target both claude and codex", ent.Name(), m.Targets)
		}
		rep, err := eng.Install(src, asset.Options{})
		if err != nil {
			t.Fatalf("install %s: %v", ent.Name(), err)
		}
		if len(rep.Skips) != 0 {
			t.Fatalf("install %s skipped projections: %+v", ent.Name(), rep.Skips)
		}
	}

	// Projection health over everything installed.
	installed, err := eng.List()
	if err != nil {
		t.Fatal(err)
	}
	for i := range installed {
		if ok, problems := eng.VerifyProjections(&installed[i]); !ok {
			t.Fatalf("%s projections unhealthy: %v", installed[i].Name, problems)
		}
		if problems := eng.VerifyProjectionSecurity(&installed[i]); len(problems) != 0 {
			t.Fatalf("%s projection security: %v", installed[i].Name, problems)
		}
	}

	// refcheck against the REAL command tree — zero exemptions.
	set := commandPaths(newRootCmd())
	findings := checks.Refcheck(filepath.Join(eng.Layout.CanonicalRoot(), "skills"), set)
	for _, f := range findings {
		t.Errorf("refcheck: %s", f.Message)
	}

	// default-path conformance (adapter-conformance §6): codex-targeted skills
	// must not reference Claude-only affordances outside a CC-acceleration block.
	for _, f := range checks.DefaultPathConformance(filepath.Join(eng.Layout.CanonicalRoot(), "skills")) {
		t.Errorf("default-path conformance: %s", f.Message)
	}

	// core4 resident budget (missing members are fine pre-completion;
	// installed ones must measure and stay inside the gate).
	rep, err := budget.Measure(eng, "claude", "core4", 2000)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if rep.Total > 2000 {
		t.Fatalf("core4 resident budget %d > 2000", rep.Total)
	}
	// Phase C complete: every core4 member ships — a missing one is a
	// release blocker, not a pre-completion note (review 074).
	if len(rep.Missing) != 0 {
		t.Fatalf("core4 members missing from assets/: %v", rep.Missing)
	}
	t.Logf("core4 resident budget: %d tokens, all members present", rep.Total)
}

func assertNpxSkillGroupingManifest(t *testing.T, repoAssets string, entries []os.DirEntry) {
	t.Helper()

	manifestPath := filepath.Join(repoAssets, "..", "..", ".claude-plugin", "plugin.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read npx skills grouping manifest: %v", err)
	}
	var manifest struct {
		Name   string   `json:"name"`
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse npx skills grouping manifest: %v", err)
	}
	if manifest.Name != "oh-my-agents" {
		t.Fatalf("npx skills grouping name = %q, want oh-my-agents", manifest.Name)
	}

	want := make([]string, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			want = append(want, "./assets/skills/"+ent.Name())
		}
	}
	slices.Sort(want)
	if !slices.Equal(manifest.Skills, want) {
		t.Fatalf("npx skills grouping paths = %v, want %v", manifest.Skills, want)
	}
}

func assertPortableSkillFrontmatter(t *testing.T, path string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill frontmatter %s: %v", path, err)
	}
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		t.Fatalf("skill frontmatter %s: file must start with ---", path)
	}
	body := raw[len("---\n"):]
	end := bytes.Index(body, []byte("\n---\n"))
	if end < 0 {
		t.Fatalf("skill frontmatter %s: block never closed with ---", path)
	}

	strictFM, err := budget.ReadFrontmatterFile(path)
	if err != nil {
		t.Fatalf("skill frontmatter %s is outside oma's supported YAML subset: %v", path, err)
	}
	if strings.TrimSpace(strictFM["name"]) == "" || strings.TrimSpace(strictFM["description"]) == "" {
		t.Fatalf("skill frontmatter %s: name and description are required", path)
	}

	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(body[:end], &fm); err != nil {
		t.Fatalf("skill frontmatter %s is not portable YAML: %v", path, err)
	}
	if strings.TrimSpace(fm.Name) == "" || strings.TrimSpace(fm.Description) == "" {
		t.Fatalf("skill frontmatter %s: name and description are required", path)
	}
}

package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sean2077/oh-my-agents/internal/asset"
	"github.com/sean2077/oh-my-agents/internal/assetaudit"
	"github.com/sean2077/oh-my-agents/internal/budget"
	"github.com/sean2077/oh-my-agents/internal/checks"
	"go.yaml.in/yaml/v3"
)

const (
	shippedCore4Budget            = 400
	defaultSkillDescriptionBudget = 80
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
	assertTriggerEvalCoversActiveSkills(t, repoAssets, entries)

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
		if err := shippedSkillDescriptionViolation(filepath.Join(src, "SKILL.md"), m); err != nil {
			t.Errorf("skill %s description gate: %v", ent.Name(), err)
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
	rep, err := budget.Measure(eng, "claude", "core4", shippedCore4Budget)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if rep.Total > shippedCore4Budget {
		t.Fatalf("core4 resident budget %d > %d", rep.Total, shippedCore4Budget)
	}
	// Phase C complete: every core4 member ships — a missing one is a
	// release blocker, not a pre-completion note (review 074).
	if len(rep.Missing) != 0 {
		t.Fatalf("core4 members missing from assets/: %v", rep.Missing)
	}
	t.Logf("core4 resident budget: %d tokens, all members present", rep.Total)
}

func TestShippedSkillParallelismMatrix(t *testing.T) {
	const (
		parallelMarker                = "> **Parallel acceleration (optional, capability-gated)**:"
		maxParallelAccelerationTokens = 230
	)
	requiredParallelClauses := []string{
		"lifecycle-controllable subagent tools",
		"at least two",
		"critical-path benefit",
		"no lane waits on",
		"shared single-writer state",
		"stop conditions",
		"no nested delegation",
		"evidence, not a verdict",
		"final verification",
		"continue sequentially",
	}
	type matrixEntry struct {
		mode        string
		wantMarkers int
	}

	matrix := make(map[string]matrixEntry)
	add := func(mode string, wantMarkers int, names ...string) {
		t.Helper()
		for _, name := range names {
			if _, exists := matrix[name]; exists {
				t.Fatalf("skill %s appears more than once in parallelism matrix", name)
			}
			matrix[name] = matrixEntry{mode: mode, wantMarkers: wantMarkers}
		}
	}
	add("proactive-parallel", 1,
		"analyze", "trace", "deep-interview", "best-practice-research",
		"code-review", "autopilot", "ultraqa", "skillify")
	add("read-only-inventory-parallel", 1, "ai-slop-cleaner")
	add("sequential", 0, "ralph", "research-mission", "prototype", "pair-delivery")

	repoAssets := filepath.Join("..", "..", "assets", "skills")
	entries, err := os.ReadDir(repoAssets)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		want, ok := matrix[name]
		if !ok {
			t.Errorf("shipped skill %s is missing from the 8+1+4 parallelism matrix", name)
			continue
		}
		seen[name] = true
		raw, err := os.ReadFile(filepath.Join(repoAssets, name, "SKILL.md"))
		if err != nil {
			t.Errorf("read skill %s: %v", name, err)
			continue
		}
		if got := strings.Count(string(raw), parallelMarker); got != want.wantMarkers {
			t.Errorf("skill %s mode=%s has %d exact Parallel acceleration markers, want %d", name, want.mode, got, want.wantMarkers)
		}
		if want.wantMarkers == 1 {
			var block string
			for _, line := range strings.Split(string(raw), "\n") {
				line = strings.TrimSuffix(line, "\r")
				if strings.HasPrefix(line, parallelMarker) {
					block = line
					break
				}
			}
			if block == "" {
				continue
			}
			if tokens := budget.Tokens(block); tokens > maxParallelAccelerationTokens {
				t.Errorf("skill %s Parallel acceleration block is %d tokens, want <= %d", name, tokens, maxParallelAccelerationTokens)
			}
			for _, clause := range requiredParallelClauses {
				if !strings.Contains(block, clause) {
					t.Errorf("skill %s Parallel acceleration block is missing contract clause %q", name, clause)
				}
			}
		}
	}
	for name, want := range matrix {
		if !seen[name] {
			t.Errorf("parallelism matrix skill %s mode=%s is not shipped", name, want.mode)
		}
	}
}

func assertTriggerEvalCoversActiveSkills(t *testing.T, repoAssets string, entries []os.DirEntry) {
	t.Helper()

	path := filepath.Join(repoAssets, "..", "..", "eval", "cases", "triggering.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open triggering eval fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	type triggerCase struct {
		ID       string `json:"id"`
		Expected string `json:"expected"`
	}
	covered := map[string]bool{}
	ids := map[string]bool{}
	hasDecoy := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var c triggerCase
		if err := json.Unmarshal(scanner.Bytes(), &c); err != nil {
			t.Fatalf("parse triggering eval fixture: %v", err)
		}
		if strings.TrimSpace(c.ID) == "" || ids[c.ID] {
			t.Fatalf("triggering eval case id must be non-empty and unique: %q", c.ID)
		}
		ids[c.ID] = true
		if c.Expected == "none" {
			hasDecoy = true
		} else {
			covered[c.Expected] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read triggering eval fixture: %v", err)
	}
	if !hasDecoy {
		t.Fatal("triggering eval must retain at least one none decoy")
	}

	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		m, err := asset.LoadManifest(filepath.Join(repoAssets, ent.Name(), "manifest.json"))
		if err != nil {
			t.Fatalf("manifest %s while checking eval coverage: %v", ent.Name(), err)
		}
		if m.StatusOrDefault() == asset.StatusActive && !covered[m.Name] {
			t.Errorf("active skill %s has no expected case in eval/cases/triggering.jsonl", m.Name)
		}
	}
}

// shippedSkillDescriptionViolation is deliberately independent of the audit's
// single advisory label. An oversized active skill remains a release violation
// even when ORPHAN wins the audit label precedence.
func shippedSkillDescriptionViolation(skillPath string, m *asset.Manifest) error {
	if m.StatusOrDefault() != asset.StatusActive {
		return nil
	}
	fm, err := budget.ReadFrontmatterFile(skillPath)
	if err != nil {
		return err
	}
	description := fm["description"]
	if !strings.HasPrefix(description, "Use when ") {
		return fmt.Errorf("must begin exactly %q", "Use when ")
	}
	maxTokens := m.DescriptionBudgetTokens
	if maxTokens <= 0 {
		maxTokens = defaultSkillDescriptionBudget
	}
	if tokens := budget.Tokens(description); tokens > maxTokens {
		return fmt.Errorf("description is %d tokens, exceeds manifest budget %d", tokens, maxTokens)
	}
	return nil
}

func TestShippedSkillDescriptionGate(t *testing.T) {
	write := func(t *testing.T, description string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "SKILL.md")
		text := "---\nname: demo\ndescription: " + description + "\n---\nbody\n"
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("accepts when-first description", func(t *testing.T) {
		m := &asset.Manifest{Status: asset.StatusActive, DescriptionBudgetTokens: 80}
		if err := shippedSkillDescriptionViolation(write(t, "Use when a focused workflow is needed."), m); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects non-trigger lead", func(t *testing.T) {
		m := &asset.Manifest{Status: asset.StatusActive, DescriptionBudgetTokens: 80}
		err := shippedSkillDescriptionViolation(write(t, "Build a workflow when requested."), m)
		if err == nil || !strings.Contains(err.Error(), `must begin exactly "Use when "`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("inactive skill is outside release gate", func(t *testing.T) {
		m := &asset.Manifest{Status: asset.StatusDeprecated, DescriptionBudgetTokens: 1}
		if err := shippedSkillDescriptionViolation(write(t, "not a trigger and far over budget"), m); err != nil {
			t.Fatal(err)
		}
	})
}

func TestShippedDescriptionGateRejectsOversizedOrphan(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skills", "orphan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"schema":"oma-asset/1","name":"orphan","type":"skill","targets":["claude","codex"],"description_budget_tokens":80}`)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	description := "Use when " + strings.Repeat("x", 400)
	skillPath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: orphan\ndescription: "+description+"\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	audit, err := assetaudit.Audit(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 1 || audit[0].Label != assetaudit.LabelOrphan {
		t.Fatalf("audit = %+v, want one ORPHAN", audit)
	}
	m, err := asset.LoadManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := shippedSkillDescriptionViolation(skillPath, m); err == nil || !strings.Contains(err.Error(), "exceeds manifest budget") {
		t.Fatalf("oversized orphan must fail release description gate, got %v", err)
	}
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

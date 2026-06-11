package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sean2077/oh-my-agents/internal/asset"
	"github.com/sean2077/oh-my-agents/internal/budget"
	"github.com/sean2077/oh-my-agents/internal/checks"
)

// TestRealAssetsPassReleaseGates is the Phase C release-blocking gate
// (docs/implementation-plan.md §5.4): every shipped asset under assets/
// must install cleanly, project healthily, pass refcheck with ZERO
// exemptions against the real command surface, and keep the core4
// resident budget within the threshold.
func TestRealAssetsPassReleaseGates(t *testing.T) {
	repoAssets := filepath.Join("..", "..", "assets", "skills")
	entries, err := os.ReadDir(repoAssets)
	if err != nil {
		t.Skipf("no shipped assets yet: %v", err)
	}

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

	// core4 resident budget (missing members are fine pre-completion;
	// installed ones must measure and stay inside the gate).
	rep, err := budget.Measure(eng, "claude", "core4", 2000)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if rep.Total > 2000 {
		t.Fatalf("core4 resident budget %d > 2000", rep.Total)
	}
	t.Logf("core4 resident budget: %d tokens (missing: %v)", rep.Total, rep.Missing)
}

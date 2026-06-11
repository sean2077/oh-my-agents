package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeConfig writes a config.toml under the given root's expected location.
func writeUserConfig(t *testing.T, home, content string) {
	t.Helper()
	p := filepath.Join(home, ".config", "oma", "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeProjectConfig(t *testing.T, root, content string) {
	t.Helper()
	p := filepath.Join(root, ".oma", "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultsWhenNoSources(t *testing.T) {
	cfg, err := Load(t.TempDir(), "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Relay.StaleAfter != 15*time.Minute || cfg.Budget.MaxResidentTokens != 2000 {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if cfg.Interview.Threshold != 0.20 || cfg.Interview.ThresholdSource != "default(standard)" {
		t.Fatalf("threshold default wrong: %v %s", cfg.Interview.Threshold, cfg.Interview.ThresholdSource)
	}
	if cfg.Sources["relay.stale_after"] != SourceDefault {
		t.Fatalf("source = %s, want default", cfg.Sources["relay.stale_after"])
	}
}

func TestPrecedenceChainLayerByLayer(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()

	// user layer overrides default
	writeUserConfig(t, home, "[budget]\nmax_resident_tokens = 1500\n")
	cfg, err := Load(home, project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Budget.MaxResidentTokens != 1500 || cfg.Sources["budget.max_resident_tokens"] != SourceUser {
		t.Fatalf("user layer: %+v %s", cfg.Budget, cfg.Sources["budget.max_resident_tokens"])
	}

	// project layer overrides user
	writeProjectConfig(t, project, "[budget]\nmax_resident_tokens = 1200\n")
	cfg, err = Load(home, project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Budget.MaxResidentTokens != 1200 || cfg.Sources["budget.max_resident_tokens"] != SourceProject {
		t.Fatalf("project layer: %+v %s", cfg.Budget, cfg.Sources["budget.max_resident_tokens"])
	}

	// env overrides project
	t.Setenv("OMA_BUDGET_MAX_TOKENS", "900")
	cfg, err = Load(home, project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Budget.MaxResidentTokens != 900 || cfg.Sources["budget.max_resident_tokens"] != SourceEnv {
		t.Fatalf("env layer: %+v %s", cfg.Budget, cfg.Sources["budget.max_resident_tokens"])
	}
}

func TestSyntaxErrorFailsClosed(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, "this is not toml ===")
	if _, err := Load(home, ""); !errors.Is(err, ErrConfig) {
		t.Fatalf("syntax error: err = %v, want ErrConfig", err)
	}
}

func TestUnknownSchemaMajorFailsClosed(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, "schema = \"oma-config/9\"\n")
	if _, err := Load(home, ""); !errors.Is(err, ErrConfig) {
		t.Fatalf("schema major: err = %v, want ErrConfig", err)
	}
}

func TestRangeViolationsFailClosed(t *testing.T) {
	cases := []string{
		"[interview]\nthreshold = 1.5\n",
		"[relay]\nstale_after = \"-5m\"\n",
		"[asset]\ndefault_agents = [\"windsurf\"]\n",
		"[budget]\nmax_resident_tokens = 0\n",
	}
	for _, content := range cases {
		home := t.TempDir()
		writeUserConfig(t, home, content)
		if _, err := Load(home, ""); !errors.Is(err, ErrConfig) {
			t.Errorf("content %q: err = %v, want ErrConfig", content, err)
		}
	}
}

func TestTOMLTypeMismatchesFailClosed(t *testing.T) {
	// viper's coercive getters must never weakly coerce (review 028 blocker):
	// string-as-int, string-as-float, scalar-as-list, number-as-string.
	cases := []string{
		"[budget]\nmax_resident_tokens = \"2500\"\n",
		"[interview]\nthreshold = \"0.42\"\n",
		"[asset]\ndefault_agents = \"claude\"\n",
		"[asset]\ndefault_agents = [1, 2]\n",
		"[relay]\nledger_root = 42\n",
		"[relay]\nstale_after = 15\n", // TOML int for a duration-string key
		"[interview]\ndepth = 2\n",
	}
	for _, content := range cases {
		home := t.TempDir()
		writeUserConfig(t, home, content)
		if _, err := Load(home, ""); !errors.Is(err, ErrConfig) {
			t.Errorf("content %q: err = %v, want ErrConfig (strict type)", content, err)
		}
	}
}

func TestIntegerThresholdIsGenuineNumber(t *testing.T) {
	// A bare TOML integer for a float key is a real number, not coercion.
	home := t.TempDir()
	writeUserConfig(t, home, "[interview]\nthreshold = 1\n")
	cfg, err := Load(home, "")
	if err != nil {
		t.Fatalf("integer threshold: %v", err)
	}
	if cfg.Interview.Threshold != 1.0 {
		t.Fatalf("threshold = %v, want 1.0", cfg.Interview.Threshold)
	}
}

func TestFlagLayerBeatsEverything(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	writeProjectConfig(t, project, "[relay]\nwait_timeout = \"30m\"\n[asset]\ndefault_agents = [\"claude\"]\n")
	t.Setenv("OMA_RELAY_WAIT_TIMEOUT", "45m")
	cfg, err := Load(home, project)
	if err != nil {
		t.Fatal(err)
	}
	wt := 5 * time.Minute
	lr := "/tmp/elsewhere"
	if err := cfg.ApplyFlags(FlagOverrides{WaitTimeout: &wt, LedgerRoot: &lr, Agents: []string{"codex"}}); err != nil {
		t.Fatal(err)
	}
	if cfg.Relay.WaitTimeout != 5*time.Minute || cfg.Sources["relay.wait_timeout"] != SourceFlag {
		t.Fatalf("flag timeout: %v %s", cfg.Relay.WaitTimeout, cfg.Sources["relay.wait_timeout"])
	}
	if cfg.Relay.LedgerRoot != lr || cfg.Sources["relay.ledger_root"] != SourceFlag {
		t.Fatalf("flag ledger root: %v", cfg.Relay.LedgerRoot)
	}
	if len(cfg.Asset.DefaultAgents) != 1 || cfg.Asset.DefaultAgents[0] != "codex" || cfg.Sources["asset.default_agents"] != SourceFlag {
		t.Fatalf("flag agents: %v", cfg.Asset.DefaultAgents)
	}
}

func TestFlagDepthBeatsEnvThreshold(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OMA_INTERVIEW_THRESHOLD", "0.42")
	cfg, err := Load(home, "")
	if err != nil {
		t.Fatal(err)
	}
	depth := "deep"
	if err := cfg.ApplyFlags(FlagOverrides{Depth: &depth}); err != nil {
		t.Fatal(err)
	}
	if cfg.Interview.Threshold != 0.10 || cfg.Sources["interview.threshold"] != SourceFlag {
		t.Fatalf("flag depth must beat env threshold: %v (%s)", cfg.Interview.Threshold, cfg.Interview.ThresholdSource)
	}
}

func TestFlagThresholdBeatsFlagDepth(t *testing.T) {
	home := t.TempDir()
	cfg, err := Load(home, "")
	if err != nil {
		t.Fatal(err)
	}
	th, depth := 0.25, "deep"
	if err := cfg.ApplyFlags(FlagOverrides{Threshold: &th, Depth: &depth}); err != nil {
		t.Fatal(err)
	}
	if cfg.Interview.Threshold != 0.25 || !strings.Contains(cfg.Interview.ThresholdSource, "depth ignored") {
		t.Fatalf("same-layer flags: %v %s", cfg.Interview.Threshold, cfg.Interview.ThresholdSource)
	}
}

func TestApplyFlagsRevalidates(t *testing.T) {
	home := t.TempDir()
	cfg, err := Load(home, "")
	if err != nil {
		t.Fatal(err)
	}
	bad := 1.5
	if err := cfg.ApplyFlags(FlagOverrides{Threshold: &bad}); !errors.Is(err, ErrConfig) {
		t.Fatalf("out-of-range flag: err = %v, want ErrConfig", err)
	}
}

func TestRelayAuthorInConfigFileRefused(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, "[relay]\nauthor = \"codex\"\n")
	_, err := Load(home, "")
	if err == nil || !strings.Contains(err.Error(), "author") {
		t.Fatalf("relay.author in file: err = %v, want refusal", err)
	}
}

func TestDepthNotShadowedByThresholdDefault(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, "[interview]\ndepth = \"deep\"\n")
	cfg, err := Load(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interview.Threshold != 0.10 {
		t.Fatalf("depth=deep must yield 0.10, got %v (source %s)", cfg.Interview.Threshold, cfg.Interview.ThresholdSource)
	}
}

func TestThresholdBeatsDepthWithinLayer(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, "[interview]\ndepth = \"deep\"\nthreshold = 0.25\n")
	cfg, err := Load(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interview.Threshold != 0.25 || !strings.Contains(cfg.Interview.ThresholdSource, "depth ignored") {
		t.Fatalf("same-layer: %v %s", cfg.Interview.Threshold, cfg.Interview.ThresholdSource)
	}
}

func TestEnvThresholdBeatsProjectDepth(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	writeProjectConfig(t, project, "[interview]\ndepth = \"quick\"\n")
	t.Setenv("OMA_INTERVIEW_THRESHOLD", "0.12")
	cfg, err := Load(home, project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interview.Threshold != 0.12 || cfg.Sources["interview.threshold"] != SourceEnv {
		t.Fatalf("env threshold: %v %s", cfg.Interview.Threshold, cfg.Sources["interview.threshold"])
	}
}

func TestProjectDepthBeatsUserThreshold(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	writeUserConfig(t, home, "[interview]\nthreshold = 0.30\n")
	writeProjectConfig(t, project, "[interview]\ndepth = \"deep\"\n")
	cfg, err := Load(home, project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interview.Threshold != 0.10 || cfg.Sources["interview.threshold"] != SourceProject {
		t.Fatalf("project depth must beat user threshold: %v %s", cfg.Interview.Threshold, cfg.Sources["interview.threshold"])
	}
}

func TestDurationAcceptsBareSeconds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OMA_RELAY_STALE_AFTER", "900")
	cfg, err := Load(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Relay.StaleAfter != 15*time.Minute {
		t.Fatalf("bare seconds: %v", cfg.Relay.StaleAfter)
	}
}

func TestUnknownKeysTolerated(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, "future_key = true\n[relay]\nledger_root = \".oma/relay\"\nfuture_nested = 1\n")
	if _, err := Load(home, ""); err != nil {
		t.Fatalf("minor-additive keys rejected: %v", err)
	}
}

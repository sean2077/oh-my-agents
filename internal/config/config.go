// Package config implements the user-intent configuration layer
// (docs/config.md). It owns the precedence chain
// flag > env > project config > user config > built-in default,
// with explicit per-key source tracking. Schema persisted data
// (registry/state/ledger/manifest) never flows through here.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ConfigSchema is the optional schema string in config files; only major 1
// is accepted (missing tolerated for hand-written files).
const ConfigSchema = "oma-config/1"

// ErrConfig marks any fail-closed configuration error (syntax, schema,
// type, range). CLI maps it to ExitState.
var ErrConfig = errors.New("invalid configuration")

// Source identifies where an effective value came from (per-key map kept
// during controlled layer application — never inferred from merged state).
type Source string

const (
	SourceDefault Source = "default"
	SourceUser    Source = "user config"
	SourceProject Source = "project config"
	SourceEnv     Source = "env"
	SourceFlag    Source = "flag"
)

// Depth aliases map onto threshold values (docs/config.md §4a).
var depthThresholds = map[string]float64{
	"quick":    0.30,
	"standard": 0.20,
	"deep":     0.10,
}

// Config is the strongly-typed effective configuration. viper never leaks
// past this package.
type Config struct {
	Relay struct {
		LedgerRoot  string
		StaleAfter  time.Duration
		WaitTimeout time.Duration
	}
	Budget struct {
		MaxResidentTokens int
	}
	Interview struct {
		Threshold       float64
		ThresholdSource string // human-readable provenance, e.g. "project config interview.depth=deep"
	}
	Asset struct {
		DefaultAgents []string
	}

	Sources     map[string]Source // canonical key -> winning source
	UserPath    string            // resolved user config path (may not exist)
	ProjectPath string            // resolved project config path ("" when no project root)
}

// Load assembles the chain for the given home and project root (projectRoot
// may be empty when outside a project). Flag overrides are applied by
// commands afterwards via the Set* helpers so cobra stays in the CLI layer.
func Load(home, projectRoot string) (*Config, error) {
	cfg := &Config{Sources: map[string]Source{}}
	cfg.UserPath = filepath.Join(home, ".config", "oma", "config.toml")
	if projectRoot != "" {
		cfg.ProjectPath = filepath.Join(projectRoot, ".oma", "config.toml")
	}

	// Layer 5: built-in defaults.
	cfg.Relay.LedgerRoot = ".oma/relay"
	cfg.Relay.StaleAfter = 15 * time.Minute
	cfg.Relay.WaitTimeout = 60 * time.Minute
	cfg.Budget.MaxResidentTokens = 2000
	cfg.Asset.DefaultAgents = []string{"claude", "codex"}
	for _, k := range []string{"relay.ledger_root", "relay.stale_after", "relay.wait_timeout",
		"budget.max_resident_tokens", "asset.default_agents"} {
		cfg.Sources[k] = SourceDefault
	}

	// Interview threshold/depth resolve specially (§4a): record explicit
	// settings per layer, then take the first explicit source.
	var layers []interviewLayer // appended low → high priority

	// Layer 4 then 3: user file, project file.
	for _, fl := range []struct {
		path   string
		source Source
	}{{cfg.UserPath, SourceUser}, {cfg.ProjectPath, SourceProject}} {
		if fl.path == "" {
			continue
		}
		v, ok, err := readFileLayer(fl.path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if err := applyFileLayer(cfg, v, fl.source); err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrConfig, fl.path, err)
		}
		il := interviewLayer{source: fl.source}
		if v.IsSet("interview.threshold") {
			t := v.GetFloat64("interview.threshold")
			il.threshold, il.thresholdKey = &t, "interview.threshold"
		}
		if v.IsSet("interview.depth") {
			d := v.GetString("interview.depth")
			il.depth, il.depthKey = &d, "interview.depth"
		}
		layers = append(layers, il)
	}

	// Layer 2: environment (explicit per-key reads; no AutomaticEnv).
	if err := applyEnvLayer(cfg); err != nil {
		return nil, err
	}
	if raw, ok := os.LookupEnv("OMA_INTERVIEW_THRESHOLD"); ok {
		t, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: OMA_INTERVIEW_THRESHOLD %q: %v", ErrConfig, raw, err)
		}
		layers = append(layers, interviewLayer{source: SourceEnv, threshold: &t, thresholdKey: "OMA_INTERVIEW_THRESHOLD"})
	}

	if err := resolveInterview(cfg, layers); err != nil {
		return nil, err
	}
	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// readFileLayer loads one TOML file into an isolated viper instance.
// Missing file → (nil,false,nil); any other failure is fail-closed.
func readFileLayer(path string) (*viper.Viper, bool, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("%w: stat %s: %v", ErrConfig, path, err)
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	if err := v.ReadInConfig(); err != nil {
		return nil, false, fmt.Errorf("%w: parse %s: %v", ErrConfig, path, err)
	}
	if v.IsSet("schema") {
		schema := v.GetString("schema")
		if major, ok := schemaMajor(schema, "oma-config"); !ok || major != 1 {
			return nil, false, fmt.Errorf("%w: %s schema %q, want %s", ErrConfig, path, schema, ConfigSchema)
		}
	}
	return v, true, nil
}

// applyFileLayer copies known keys from one file layer into cfg, recording
// the source for each key it sets. Unknown keys are tolerated
// (minor-additive read policy).
func applyFileLayer(cfg *Config, v *viper.Viper, src Source) error {
	if v.IsSet("relay.ledger_root") {
		cfg.Relay.LedgerRoot = v.GetString("relay.ledger_root")
		cfg.Sources["relay.ledger_root"] = src
	}
	if v.IsSet("relay.stale_after") {
		d, err := parseDuration(v.GetString("relay.stale_after"))
		if err != nil {
			return fmt.Errorf("relay.stale_after: %v", err)
		}
		cfg.Relay.StaleAfter = d
		cfg.Sources["relay.stale_after"] = src
	}
	if v.IsSet("relay.wait_timeout") {
		d, err := parseDuration(v.GetString("relay.wait_timeout"))
		if err != nil {
			return fmt.Errorf("relay.wait_timeout: %v", err)
		}
		cfg.Relay.WaitTimeout = d
		cfg.Sources["relay.wait_timeout"] = src
	}
	if v.IsSet("budget.max_resident_tokens") {
		cfg.Budget.MaxResidentTokens = v.GetInt("budget.max_resident_tokens")
		cfg.Sources["budget.max_resident_tokens"] = src
	}
	if v.IsSet("asset.default_agents") {
		cfg.Asset.DefaultAgents = v.GetStringSlice("asset.default_agents")
		cfg.Sources["asset.default_agents"] = src
	}
	if v.IsSet("relay.author") {
		// Identity is bootstrap-level only (docs/config.md §4): a config
		// file must never select the relay participant.
		return fmt.Errorf("relay.author is not configurable via config files (identity is platform/env only)")
	}
	return nil
}

// applyEnvLayer reads the explicit OMA_* variables (docs/config.md §4).
func applyEnvLayer(cfg *Config) error {
	if raw, ok := os.LookupEnv("OMA_RELAY_LEDGER_ROOT"); ok {
		cfg.Relay.LedgerRoot = raw
		cfg.Sources["relay.ledger_root"] = SourceEnv
	}
	if raw, ok := os.LookupEnv("OMA_RELAY_STALE_AFTER"); ok {
		d, err := parseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: OMA_RELAY_STALE_AFTER %q: %v", ErrConfig, raw, err)
		}
		cfg.Relay.StaleAfter = d
		cfg.Sources["relay.stale_after"] = SourceEnv
	}
	if raw, ok := os.LookupEnv("OMA_RELAY_WAIT_TIMEOUT"); ok {
		d, err := parseDuration(raw)
		if err != nil {
			return fmt.Errorf("%w: OMA_RELAY_WAIT_TIMEOUT %q: %v", ErrConfig, raw, err)
		}
		cfg.Relay.WaitTimeout = d
		cfg.Sources["relay.wait_timeout"] = SourceEnv
	}
	if raw, ok := os.LookupEnv("OMA_BUDGET_MAX_TOKENS"); ok {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("%w: OMA_BUDGET_MAX_TOKENS %q: %v", ErrConfig, raw, err)
		}
		cfg.Budget.MaxResidentTokens = n
		cfg.Sources["budget.max_resident_tokens"] = SourceEnv
	}
	if raw, ok := os.LookupEnv("OMA_ASSET_AGENTS"); ok {
		cfg.Asset.DefaultAgents = splitCSV(raw)
		cfg.Sources["asset.default_agents"] = SourceEnv
	}
	return nil
}

// resolveInterview applies §4a: walk layers from highest priority down and
// take the first explicit threshold/depth; threshold beats depth within a
// layer; the built-in default is standard (0.20) with no independent
// threshold default.
func resolveInterview(cfg *Config, layers []interviewLayer) error {
	for i := len(layers) - 1; i >= 0; i-- {
		l := layers[i]
		switch {
		case l.threshold != nil:
			cfg.Interview.Threshold = *l.threshold
			cfg.Interview.ThresholdSource = fmt.Sprintf("%s %s", l.source, l.thresholdKey)
			if l.depth != nil {
				cfg.Interview.ThresholdSource += " (depth ignored: threshold is more precise)"
			}
			cfg.Sources["interview.threshold"] = l.source
			return nil
		case l.depth != nil:
			t, ok := depthThresholds[*l.depth]
			if !ok {
				return fmt.Errorf("%w: %s %s=%q not in {quick,standard,deep}", ErrConfig, l.source, l.depthKey, *l.depth)
			}
			cfg.Interview.Threshold = t
			cfg.Interview.ThresholdSource = fmt.Sprintf("%s %s=%s", l.source, l.depthKey, *l.depth)
			cfg.Sources["interview.threshold"] = l.source
			return nil
		}
	}
	cfg.Interview.Threshold = depthThresholds["standard"]
	cfg.Interview.ThresholdSource = "default(standard)"
	cfg.Sources["interview.threshold"] = SourceDefault
	return nil
}

// interviewLayer records one layer's explicit interview settings for the
// §4a first-explicit-source resolution.
type interviewLayer struct {
	source                 Source
	threshold              *float64
	depth                  *string
	thresholdKey, depthKey string
}

// validate enforces ranges after the merge (docs/config.md §5).
func validate(cfg *Config) error {
	if cfg.Interview.Threshold < 0 || cfg.Interview.Threshold > 1 {
		return fmt.Errorf("%w: interview.threshold %.3f outside [0,1] (%s)", ErrConfig, cfg.Interview.Threshold, cfg.Interview.ThresholdSource)
	}
	if cfg.Relay.StaleAfter <= 0 {
		return fmt.Errorf("%w: relay.stale_after must be positive", ErrConfig)
	}
	if cfg.Relay.WaitTimeout <= 0 {
		return fmt.Errorf("%w: relay.wait_timeout must be positive", ErrConfig)
	}
	if cfg.Budget.MaxResidentTokens <= 0 {
		return fmt.Errorf("%w: budget.max_resident_tokens must be positive", ErrConfig)
	}
	if strings.TrimSpace(cfg.Relay.LedgerRoot) == "" {
		return fmt.Errorf("%w: relay.ledger_root must not be empty", ErrConfig)
	}
	for _, a := range cfg.Asset.DefaultAgents {
		if a != "claude" && a != "codex" {
			return fmt.Errorf("%w: asset.default_agents entry %q not in {claude,codex}", ErrConfig, a)
		}
	}
	if len(cfg.Asset.DefaultAgents) == 0 {
		return fmt.Errorf("%w: asset.default_agents must not be empty", ErrConfig)
	}
	return nil
}

// parseDuration accepts Go duration strings and bare integer seconds (env
// compatibility with the protocol doc's seconds-based variables).
func parseDuration(raw string) (time.Duration, error) {
	if secs, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		return time.Duration(secs) * time.Second, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("want duration (e.g. 15m) or integer seconds: %v", err)
	}
	return d, nil
}

func splitCSV(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// schemaMajor mirrors the strict parser in internal/asset (digit-sequence
// only, no signs/leading zeros, major >= 1).
func schemaMajor(schema, wantDomain string) (int, bool) {
	domain, ver, found := strings.Cut(schema, "/")
	if !found || domain != wantDomain || ver == "" || ver[0] == '0' {
		return 0, false
	}
	for i := 0; i < len(ver); i++ {
		if ver[i] < '0' || ver[i] > '9' {
			return 0, false
		}
	}
	major, err := strconv.Atoi(ver)
	if err != nil || major < 1 {
		return 0, false
	}
	return major, true
}

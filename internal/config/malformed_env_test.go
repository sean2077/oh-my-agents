package config

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestMalformedEnvFailsClosed pins fail-closed parsing of malformed OMA_*
// environment values: each typed env var must reject a non-parsable value
// with a wrapped ErrConfig rather than silently falling back to its built-in
// default. Each variable is parsed by an explicit prod read whose error is
// returned (never discarded):
//
//   - OMA_BUDGET_MAX_TOKENS   strconv.Atoi        config.go:311 (err returned :313)
//   - OMA_RELAY_STALE_AFTER   parseDuration       config.go:295 (err returned :297)
//   - OMA_INTERVIEW_THRESHOLD strconv.ParseFloat  config.go:138 (err returned :140)
//
// fail-before rationale: if any of those reads dropped its error — e.g. the
// budget read became `n, _ := strconv.Atoi(raw)` at config.go:311 — then
// OMA_BUDGET_MAX_TOKENS="abc" would yield n==0 and Load would silently fall
// back toward the default instead of failing. This table asserts a non-nil
// (ErrConfig) error per variable, so dropping the error makes that row fail.
func TestMalformedEnvFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  string // clearly malformed for this var's type
	}{
		{"budget_max_tokens_not_int", "OMA_BUDGET_MAX_TOKENS", "abc"},
		{"relay_stale_after_not_duration", "OMA_RELAY_STALE_AFTER", "xyz"},
		{"interview_threshold_not_float", "OMA_INTERVIEW_THRESHOLD", "nope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv isolates and restores the env per subtest; only this
			// one var is malformed so the failure is attributable to it.
			t.Setenv(tc.key, tc.val)
			_, err := Load(t.TempDir(), "")
			if err == nil {
				t.Fatalf("%s=%q: Load succeeded, want fail-closed error (silent default = bug)", tc.key, tc.val)
			}
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("%s=%q: err = %v, want wrapped ErrConfig", tc.key, tc.val, err)
			}
			// The fail-closed error should name the offending key and value so
			// the operator can find the bad setting.
			if msg := err.Error(); !strings.Contains(msg, tc.key) || !strings.Contains(msg, tc.val) {
				t.Fatalf("%s=%q: err %q should name both key and value", tc.key, tc.val, msg)
			}
		})
	}
}

// TestEnvUnsetLoadSucceeds is the control: with none of the malformed vars set
// (defaults only), Load must succeed. This distinguishes malformed-rejection
// above from an always-failing Load — the table fails closed *because* the
// value is malformed, not unconditionally.
func TestEnvUnsetLoadSucceeds(t *testing.T) {
	// Defensively clear the typed env vars so an inherited ambient value can't
	// make this control flaky. t.Setenv records the prior value and registers a
	// Cleanup that restores it; os.Unsetenv then removes the placeholder so the
	// var is genuinely absent during Load (an empty string would itself be
	// malformed for the int/float/duration parsers).
	for _, k := range []string{
		"OMA_BUDGET_MAX_TOKENS",
		"OMA_RELAY_STALE_AFTER",
		"OMA_INTERVIEW_THRESHOLD",
	} {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unset %s: %v", k, err)
		}
	}
	if _, err := Load(t.TempDir(), ""); err != nil {
		t.Fatalf("control: Load with no malformed env must succeed, got %v", err)
	}
}

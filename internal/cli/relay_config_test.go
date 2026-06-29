package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sean2077/oh-my-agents/internal/config"
)

// jsonPathInOutput reports whether JSON blob `out` contains `path` encoded as a
// JSON string value, so a Windows path's backslashes match their `\\` escaping
// in the JSON (a raw strings.Contains over the path would miss that).
func jsonPathInOutput(t *testing.T, out, path string) bool {
	t.Helper()
	b, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(out, string(b[1:len(b)-1]))
}

// S3 / DRIFT-1: relay commands consume the config layer. These tests prove the
// three approved fields (stale_after, wait_timeout, ledger_root) actually affect
// relay behavior via config files and env — and that the duration form the old
// integer-seconds-only parser silently dropped now works.

func writeUserConfig(t *testing.T, home, body string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "oma")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func relayIdentityEnv(t *testing.T, home, session string) {
	t.Helper()
	t.Setenv("OMA_HOME", home)
	t.Setenv("OMA_RELAY_AUTHOR", "claude")
	t.Setenv("OMA_RELAY_SESSION_ID", session)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "")
}

func TestRelayLedgerConsumesConfigFile(t *testing.T) {
	home := t.TempDir()
	relayIdentityEnv(t, home, "cfg-file")
	ledgerRoot := filepath.Join(t.TempDir(), "custom-relay")
	// The path goes in a TOML LITERAL string (single quotes) so a Windows path's
	// backslashes are taken verbatim instead of parsed as TOML escapes.
	writeUserConfig(t, home,
		"relay.stale_after = \"30m\"\nrelay.ledger_root = '"+ledgerRoot+"'\n")

	l, err := relayLedger("", false)
	if err != nil {
		t.Fatal(err)
	}
	if l.StaleAfter != 30*time.Minute {
		t.Fatalf("StaleAfter = %v, want 30m from config", l.StaleAfter)
	}
	if l.Root != ledgerRoot {
		t.Fatalf("Root = %q, want explicit config ledger_root %q", l.Root, ledgerRoot)
	}
}

func TestRelayLedgerStaleFromEnvAcceptsDuration(t *testing.T) {
	home := t.TempDir()
	relayIdentityEnv(t, home, "env-dur")
	// "45m" is exactly the Go-duration form the old strconv.Atoi path dropped
	// (silently falling back to the 15m default).
	t.Setenv("OMA_RELAY_STALE_AFTER", "45m")

	l, err := relayLedger("", false)
	if err != nil {
		t.Fatal(err)
	}
	if l.StaleAfter != 45*time.Minute {
		t.Fatalf("StaleAfter = %v, want 45m from env duration (regression: the old int-only parser dropped it)", l.StaleAfter)
	}
}

func TestRelayWaitTimeoutResolution(t *testing.T) {
	cfg := &config.Config{}
	cfg.Relay.WaitTimeout = 20 * time.Minute
	var rootFlag string
	cmd := newRelayWaitCmd(&rootFlag)

	// Flag unset → the configured wait_timeout is used.
	if got := relayWaitTimeout(cmd, cfg, 0); got != 20*time.Minute {
		t.Fatalf("wait timeout = %v, want 20m from config", got)
	}
	// Explicit --timeout wins over config.
	if err := cmd.Flags().Set("timeout", "120"); err != nil {
		t.Fatal(err)
	}
	if got := relayWaitTimeout(cmd, cfg, 120); got != 120*time.Second {
		t.Fatalf("explicit --timeout = %v, want 120s", got)
	}
}

// preflight is a relay command too: its reported/probed root must honor the
// same relay.ledger_root precedence (config + env), not just --ledger-root.
func TestRelayPreflightHonorsConfigLedgerRoot(t *testing.T) {
	home := t.TempDir()
	relayIdentityEnv(t, home, "preflight-cfg")
	customRoot := filepath.Join(t.TempDir(), "cfg-relay")
	writeUserConfig(t, home, "relay.ledger_root = '"+customRoot+"'\n")

	// Uninitialized custom root → preflight warns; the assertion is the root.
	code, out := runRelay(t, "relay", "preflight", "--json")
	if code != ExitOK && code != ExitWarn && code != ExitState {
		t.Fatalf("preflight exit %d: %s", code, out)
	}
	if !jsonPathInOutput(t, out, customRoot) {
		t.Fatalf("preflight --json must report config ledger_root %q; got: %s", customRoot, out)
	}
}

func TestRelayPreflightHonorsEnvLedgerRoot(t *testing.T) {
	home := t.TempDir()
	relayIdentityEnv(t, home, "preflight-env")
	customRoot := filepath.Join(t.TempDir(), "env-relay")
	t.Setenv("OMA_RELAY_LEDGER_ROOT", customRoot)

	code, out := runRelay(t, "relay", "preflight", "--json")
	if code != ExitOK && code != ExitWarn && code != ExitState {
		t.Fatalf("preflight exit %d: %s", code, out)
	}
	if !jsonPathInOutput(t, out, customRoot) {
		t.Fatalf("preflight --json must report env ledger_root %q; got: %s", customRoot, out)
	}
}

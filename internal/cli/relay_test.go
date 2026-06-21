package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runRelay executes one oma invocation against an isolated ledger root
// and returns (exit, stdout).
func runRelay(t *testing.T, args ...string) (int, string) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	code := executeWith(root, &out)
	return code, out.String()
}

func setManualRelayIdentity(t *testing.T, author, session string) {
	t.Helper()
	t.Setenv("OMA_RELAY_AUTHOR", author)
	t.Setenv("OMA_RELAY_SESSION_ID", session)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "")
}

func TestRelayCLIRoundTrip(t *testing.T) {
	setManualRelayIdentity(t, "claude", "cli-roundtrip")
	ledger := filepath.Join(t.TempDir(), "relay")

	if code, out := runRelay(t, "relay", "init", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("init exit %d: %s", code, out)
	}
	if code, out := runRelay(t, "relay", "pair", "new", "cli-smoke", "--ledger-root", ledger); code != ExitOK || !strings.Contains(out, "pair join") {
		t.Fatalf("pair new exit %d: %s", code, out)
	}
	code, out := runRelay(t, "relay", "draft", "--kind", "plan", "--ledger-root", ledger)
	if code != ExitOK {
		t.Fatalf("draft exit %d: %s", code, out)
	}
	draft := strings.TrimSpace(out)

	body := filepath.Join(t.TempDir(), "body.md")
	prompt := filepath.Join(t.TempDir(), "next.md")
	if err := os.WriteFile(body, []byte("the plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, []byte("review it\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out := runRelay(t, "relay", "publish", draft, "--body-file", body, "--prompt-file", prompt, "--touched", "internal/x.go", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("publish exit %d: %s", code, out)
	}
	if code, out := runRelay(t, "relay", "status", "--json", "--ledger-root", ledger); code != ExitOK || !strings.Contains(out, `"next_seq": 2`) {
		t.Fatalf("status exit %d: %s", code, out)
	}
	// wait with a zero-second window: peer never answers → contract 10.
	if code, _ := runRelay(t, "relay", "wait", "--timeout", "0", "--ledger-root", ledger); code != 10 {
		t.Fatalf("wait timeout exit = %d, want 10", code)
	}
	if code, out := runRelay(t, "relay", "close", "--outcome", "abandon", "--reason", "smoke done", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("close exit %d: %s", code, out)
	}
	// terminal pair: wait resolves to 12 via explicit --pair on the
	// archived slug being gone → binding error is also acceptable (exit 3).
	code, _ = runRelay(t, "relay", "wait", "--timeout", "0", "--ledger-root", ledger)
	if code != 12 && code != ExitState {
		t.Fatalf("wait after close exit = %d, want 12 or 3", code)
	}
}

func TestRelayCLIIdentityFailure(t *testing.T) {
	t.Setenv("OMA_RELAY_AUTHOR", "")
	t.Setenv("OMA_RELAY_SESSION_ID", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "")
	ledger := filepath.Join(t.TempDir(), "relay")
	if code, out := runRelay(t, "relay", "init", "--ledger-root", ledger); code != ExitState || !strings.Contains(out, "cannot resolve author") {
		t.Fatalf("identity failure exit %d: %s", code, out)
	}
}

func TestRelayCLIIgnoresWorkflowSessionFlag(t *testing.T) {
	setManualRelayIdentity(t, "claude", "workflow-session-ignored")
	t.Setenv("OMA_SESSION_ID", "")
	ledger := filepath.Join(t.TempDir(), "relay")

	if code, out := runRelay(t, "--session", "current", "relay", "init", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("relay init must ignore workflow --session, exit %d: %s", code, out)
	}
}

func TestRelayCLIParallelPairsUseDistinctSessionPairs(t *testing.T) {
	t.Setenv("OMA_RELAY_AUTHOR", "")
	t.Setenv("OMA_RELAY_SESSION_ID", "")
	t.Setenv("OMA_SESSION_ID", "")
	ledger := filepath.Join(t.TempDir(), "relay")

	setCodex := func(id string) {
		t.Helper()
		t.Setenv("CODEX_THREAD_ID", id)
		t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	}
	setClaude := func(id string) {
		t.Helper()
		t.Setenv("CODEX_THREAD_ID", "")
		t.Setenv("CLAUDE_CODE_SESSION_ID", id)
	}
	newPair := func(codexSession, topic string) string {
		t.Helper()
		setCodex(codexSession)
		code, out := runRelay(t, "--session", "current", "relay", "pair", "new", topic, "--ledger-root", ledger)
		if code != ExitOK {
			t.Fatalf("%s pair new exit %d: %s", codexSession, code, out)
		}
		return strings.Split(strings.TrimSpace(out), "\n")[0]
	}
	joinPair := func(claudeSession, pair string) {
		t.Helper()
		setClaude(claudeSession)
		if code, out := runRelay(t, "--session", "current", "relay", "pair", "join", pair, "--ledger-root", ledger); code != ExitOK {
			t.Fatalf("%s pair join exit %d: %s", claudeSession, code, out)
		}
	}
	assertShows := func(sessionKind, sessionID, pair, peer string) {
		t.Helper()
		if sessionKind == "codex" {
			setCodex(sessionID)
		} else {
			setClaude(sessionID)
		}
		code, out := runRelay(t, "--session", "current", "relay", "pair", "show", "--ledger-root", ledger)
		if code != ExitOK || !strings.Contains(out, "pair: "+pair) || !strings.Contains(out, "peer: "+peer) {
			t.Fatalf("%s/%s show exit %d: %s", sessionKind, sessionID, code, out)
		}
	}

	setCodex("codex-one")
	if code, out := runRelay(t, "--session", "current", "relay", "init", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("codex init exit %d: %s", code, out)
	}
	pairOne := newPair("codex-one", "pair-one")
	joinPair("claude-one", pairOne)
	pairTwo := newPair("codex-two", "pair-two")
	joinPair("claude-two", pairTwo)

	assertShows("codex", "codex-one", pairOne, "claude")
	assertShows("claude", "claude-one", pairOne, "codex")
	assertShows("codex", "codex-two", pairTwo, "claude")
	assertShows("claude", "claude-two", pairTwo, "codex")

	entries, err := os.ReadDir(filepath.Join(ledger, "_bindings"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("bindings = %d, want two codex + two claude author-session bindings", len(entries))
	}
}

func TestRelayCLIDryRunPublishZeroWrites(t *testing.T) {
	setManualRelayIdentity(t, "claude", "dry-run-publish")
	ledger := filepath.Join(t.TempDir(), "relay")
	runRelay(t, "relay", "init", "--ledger-root", ledger)
	runRelay(t, "relay", "pair", "new", "dry-smoke", "--ledger-root", ledger)
	_, out := runRelay(t, "relay", "draft", "--kind", "note", "--ledger-root", ledger)
	draft := strings.TrimSpace(out)

	body := filepath.Join(t.TempDir(), "b.md")
	prompt := filepath.Join(t.TempDir(), "p.md")
	_ = os.WriteFile(body, []byte("b\n"), 0o600)
	_ = os.WriteFile(prompt, []byte("p\n"), 0o600)
	if code, out := runRelay(t, "--dry-run", "relay", "publish", draft, "--body-file", body, "--prompt-file", prompt, "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("dry-run publish exit %d: %s", code, out)
	}
	pairDir := filepath.Dir(filepath.Dir(draft))
	if _, err := os.Stat(filepath.Join(pairDir, "001-claude-note.md")); !os.IsNotExist(err) {
		t.Fatal("dry-run publish must not create the formal artifact")
	}
}

func TestRelayPreflightCLI(t *testing.T) {
	setManualRelayIdentity(t, "claude", "preflight")

	// Initialized ledger: pass/warn but never fail → exit 0 or 1.
	ledger := filepath.Join(t.TempDir(), "relay")
	if code, _ := runRelay(t, "relay", "init", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("init exit %d", code)
	}
	code, out := runRelay(t, "relay", "preflight", "--ledger-root", ledger)
	if code != ExitOK && code != ExitWarn {
		t.Fatalf("preflight exit %d: %s", code, out)
	}
	if !strings.Contains(out, "relay preflight") || !strings.Contains(out, "summary:") {
		t.Fatalf("preflight table missing: %s", out)
	}

	// --json carries the stable schema.
	if code, out := runRelay(t, "relay", "preflight", "--ledger-root", ledger, "--json"); (code != ExitOK && code != ExitWarn) || !strings.Contains(out, `"oma-relay-preflight/1"`) {
		t.Fatalf("preflight --json exit %d: %s", code, out)
	}

	// Explicit --ledger-root to a v1 tree fails closed → exit 3 (ExitState).
	v1 := filepath.Join(t.TempDir(), "v1")
	if err := os.MkdirAll(filepath.Join(v1, "_relay"), 0o700); err != nil {
		t.Fatal(err)
	}
	if code, _ := runRelay(t, "relay", "preflight", "--ledger-root", v1); code != ExitState {
		t.Fatalf("v1-root preflight exit %d, want %d", code, ExitState)
	}
}

func TestRelayStatuslineCLI(t *testing.T) {
	setManualRelayIdentity(t, "claude", "statusline")

	// Render path must never error into the status bar: uninitialized
	// ledger (bad --ledger-root parent) degrades to a line, exit 0.
	ledger := filepath.Join(t.TempDir(), "relay")
	if code, out := runRelay(t, "relay", "statusline", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("uninitialized statusline exit %d: %s", code, out)
	}
	if code, out := runRelay(t, "relay", "init", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("init %d: %s", code, out)
	}
	if code, out := runRelay(t, "relay", "pair", "new", "demo", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("pair new %d: %s", code, out)
	}
	code, out := runRelay(t, "relay", "statusline", "--ledger-root", ledger, "--no-color")
	if code != ExitOK || !strings.Contains(out, "demo") || !strings.Contains(out, "new pair") {
		t.Fatalf("statusline exit %d: %s", code, out)
	}
	if code, out := runRelay(t, "relay", "statusline", "--ledger-root", ledger, "--json"); code != ExitOK || !strings.Contains(out, `"oma-relay-statusline/1"`) {
		t.Fatalf("statusline --json exit %d: %s", code, out)
	}
}

// TestRelayCloseApproveGateMissExit4 pins R4: an unsatisfied approve quality
// gate exits 4 (ErrGate → ExitGate), distinct from exit 3 for corrupt state.
func TestRelayCloseApproveGateMissExit4(t *testing.T) {
	setManualRelayIdentity(t, "claude", "close-gate")
	ledger := filepath.Join(t.TempDir(), "relay")
	if code, out := runRelay(t, "relay", "init", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("init %d: %s", code, out)
	}
	if code, out := runRelay(t, "relay", "pair", "new", "gate", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("pair new %d: %s", code, out)
	}
	// No decision/approve review yet → approve close is an unsatisfied gate → exit 4.
	if code, out := runRelay(t, "relay", "close", "--outcome", "approve", "--reason", "x", "--ledger-root", ledger); code != ExitGate {
		t.Fatalf("gate-miss approve close exit = %d (want %d): %s", code, ExitGate, out)
	}
	// abandon needs no receipt → succeeds.
	if code, out := runRelay(t, "relay", "close", "--outcome", "abandon", "--reason", "drop", "--ledger-root", ledger); code != ExitOK {
		t.Fatalf("abandon close exit = %d: %s", code, out)
	}
}

func TestRelayHookDispatchHiddenAndSilentWhenUnconfigured(t *testing.T) {
	setManualRelayIdentity(t, "claude", "hook-hidden")
	// The dispatcher must never error into the host even with no ledger.
	dir := t.TempDir()
	t.Chdir(dir)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(`{"hook_event_name":"Stop","stop_hook_active":false}`))
	root.SetArgs([]string{"relay", "hook", "Stop"})
	if code := executeWith(root, &out); code != ExitOK {
		t.Fatalf("dispatch with no ledger exit %d: %s", code, out.String())
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("dispatch with no ledger must be silent, got %q", out.String())
	}
	// The dispatcher is hidden from help.
	var help bytes.Buffer
	h := newRootCmd()
	h.SetOut(&help)
	h.SetArgs([]string{"relay", "--help"})
	_ = h.Execute()
	if strings.Contains(help.String(), "hook ") && strings.Contains(help.String(), "<event>") {
		t.Fatalf("dispatcher should be hidden from relay --help:\n%s", help.String())
	}
}

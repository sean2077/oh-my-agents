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

func TestRelayCLIRoundTrip(t *testing.T) {
	t.Setenv("OMA_RELAY_AUTHOR", "claude")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "")
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
	if code, out := runRelay(t, "relay", "close", "--outcome", "approve", "--reason", "smoke done", "--ledger-root", ledger); code != ExitOK {
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
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "")
	ledger := filepath.Join(t.TempDir(), "relay")
	if code, out := runRelay(t, "relay", "init", "--ledger-root", ledger); code != ExitState || !strings.Contains(out, "cannot resolve author") {
		t.Fatalf("identity failure exit %d: %s", code, out)
	}
}

func TestRelayCLIDryRunPublishZeroWrites(t *testing.T) {
	t.Setenv("OMA_RELAY_AUTHOR", "claude")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "")
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

package relay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatuslineBindingScopedAndPureRead(t *testing.T) {
	// review 086 must-fix 4: an unbound window shows nothing — never the
	// lone active pair — and the render mutates nothing.
	ck := newClock()
	root, _ := initRoot(t, ck)
	creator := testLedger(t, root, "claude", ck)
	mustPair(t, creator, "topic") // one active pair exists, bound to creator

	// A DIFFERENT claude session (distinct session key → no binding file of
	// its own) must not surface the lone active pair.
	otherID, err := makeIdentity("claude", "a-different-window")
	if err != nil {
		t.Fatal(err)
	}
	unbound := NewLedger(root, otherID)
	unbound.Now = ck.now
	unbound.Getenv = func(string) string { return "" }
	if unbound.bindingPath() == creator.bindingPath() {
		t.Fatal("test bug: sessions must differ")
	}
	before := treeDigest(t, root)
	st := unbound.Statusline("")
	if st.Bound || !strings.Contains(st.Text, "no pair") {
		t.Fatalf("unbound window must show 'no pair', got %+v", st)
	}
	// The bound creator sees the pair.
	st = creator.Statusline("")
	if !st.Bound || st.Pair != "20260611-topic" {
		t.Fatalf("bound creator: %+v", st)
	}
	// Pure-read: no ledger bytes changed by either render.
	if after := treeDigest(t, root); after != before {
		t.Fatal("Statusline mutated the ledger (must be pure-read)")
	}
}

func TestStatuslineTurnLogic(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "turns")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}

	// No artifacts yet → creator's turn (write first).
	if st := claude.Statusline(""); st.Turn != "you" {
		t.Fatalf("fresh pair turn = %q, want you", st.Turn)
	}
	// claude publishes → claude now awaits codex; codex sees its turn.
	mustPublish(t, claude, s.Pair, "plan", "body", "review")
	if st := claude.Statusline(""); st.Turn != "codex" {
		t.Fatalf("after my publish, turn = %q, want codex", st.Turn)
	}
	if st := codex.Statusline(""); st.Turn != "you" {
		t.Fatalf("peer published to me, turn = %q, want you", st.Turn)
	}
	if st := claude.Statusline(""); st.LatestSeq != 1 || st.LatestKind != "plan" || st.LatestAuthor != "claude" {
		t.Fatalf("latest = %d %s %s", st.LatestSeq, st.LatestKind, st.LatestAuthor)
	}
	// Close → done.
	if err := claude.Close(s.Pair, "approve", "x", false); err != nil {
		t.Fatal(err)
	}
	if st := claude.Statusline(s.Pair); st.Turn != "done" {
		t.Fatalf("closed turn = %q, want done", st.Turn)
	}
}

func TestStatuslineJSONShape(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	mustPair(t, claude, "json")
	raw, _ := json.Marshal(claude.Statusline(""))
	for _, key := range []string{`"oma-relay-statusline/1"`, `"bound"`, `"turn"`, `"text"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("json missing %s: %s", key, raw)
		}
	}
}

func TestStatuslineInstallLifecycle(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	// A hostile binary path: spaces and a single quote must survive POSIX
	// quoting (review 099 must-fix 2).
	exe := "/opt/o m a's bin/oma"

	// absent → install sets ours.
	if state, _ := DoctorStatusline(settings, exe); state != StatuslineAbsent {
		t.Fatalf("initial state = %s", state)
	}
	if err := InstallStatusline(settings, exe, false); err != nil {
		t.Fatal(err)
	}
	if state, _ := DoctorStatusline(settings, exe); state != StatuslineOwned {
		t.Fatalf("after install = %s, want owned", state)
	}
	// The installed command embeds the guarded, single-quoted absolute
	// path and the dispatcher invocation (decode the JSON: shell quoting
	// must hold AFTER JSON unescaping).
	raw, _ := os.ReadFile(settings)
	var host struct {
		StatusLine struct {
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal(raw, &host); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`'/opt/o m a'\''s bin/oma'`, "|| exit 0", statuslineInvocation} {
		if !strings.Contains(host.StatusLine.Command, want) {
			t.Fatalf("installed command missing %q: %q", want, host.StatusLine.Command)
		}
	}
	// idempotent reinstall.
	if err := InstallStatusline(settings, exe, false); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	// A DIFFERENT binary sees the slot as owned-but-drifted (mismatch,
	// warn-grade — reinstall refreshes; review 099).
	if state, _ := DoctorStatusline(settings, "/elsewhere/oma"); state != StatuslineMismatch {
		t.Fatalf("other-binary view = %s, want mismatch", state)
	}
	// uninstall removes ours.
	if state, err := UninstallStatusline(settings, exe); err != nil || state != StatuslineOwned {
		t.Fatalf("uninstall = %s err=%v", state, err)
	}
	if state, _ := DoctorStatusline(settings, exe); state != StatuslineAbsent {
		t.Fatalf("after uninstall = %s, want absent", state)
	}
}

func TestStatuslineNeverClobbersForeign(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	foreign := `{
  "statusLine": {
    "type": "command",
    "command": "my-own-statusline"
  },
  "model": "opus"
}
`
	if err := os.WriteFile(settings, []byte(foreign), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, _ := DoctorStatusline(settings, "/bin/oma"); state != StatuslineForeign {
		t.Fatalf("foreign state = %s", state)
	}
	// install without --force refuses and writes nothing.
	if err := InstallStatusline(settings, "/bin/oma", false); err == nil {
		t.Fatal("must refuse a foreign statusLine without --force")
	}
	if got, _ := os.ReadFile(settings); string(got) != foreign {
		t.Fatalf("refused install mutated the file:\n%s", got)
	}
	// uninstall leaves the foreign one intact.
	if state, _ := UninstallStatusline(settings, "/bin/oma"); state != StatuslineForeign {
		t.Fatalf("uninstall foreign = %s", state)
	}
	if got, _ := os.ReadFile(settings); string(got) != foreign {
		t.Fatal("uninstall removed a foreign statusLine")
	}
	// --force replaces it; the model key is preserved.
	if err := InstallStatusline(settings, "/bin/oma", true); err != nil {
		t.Fatal(err)
	}
	if state, _ := DoctorStatusline(settings, "/bin/oma"); state != StatuslineOwned {
		t.Fatalf("after force = %s", state)
	}
	if got, _ := os.ReadFile(settings); !strings.Contains(string(got), `"model"`) {
		t.Fatal("force install dropped the foreign model key")
	}
}

func TestStatuslineMismatchReinstalls(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	// owned marker but a drifted command.
	drifted := `{"statusLine": {"type": "command", "command": "oma relay statusline --old", "_oma_relay": "statusline"}}`
	if err := os.WriteFile(settings, []byte(drifted), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, _ := DoctorStatusline(settings, "/bin/oma"); state != StatuslineMismatch {
		t.Fatalf("drifted = %s, want mismatch", state)
	}
	// install (no force) fixes our own drifted entry.
	if err := InstallStatusline(settings, "/bin/oma", false); err != nil {
		t.Fatal(err)
	}
	if state, _ := DoctorStatusline(settings, "/bin/oma"); state != StatuslineOwned {
		t.Fatalf("after fix = %s", state)
	}
}

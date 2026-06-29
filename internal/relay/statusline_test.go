package relay

import (
	"encoding/json"
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
	if err := claude.Close(s.Pair, "abandon", "x", false); err != nil {
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

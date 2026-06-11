package relay

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRolesLeadValidation(t *testing.T) {
	base := func() *Session {
		return &Session{
			Schema: Schema, Pair: "20260611-x", Project: "p",
			Participants: []string{"claude", "codex"},
			Roles:        map[string]string{"lead": "claude"},
			Status:       "active", Created: time.Now(),
		}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("valid session rejected: %v", err)
	}
	noLead := base()
	delete(noLead.Roles, "lead")
	if err := noLead.Validate(); err == nil || !strings.Contains(err.Error(), "lead") {
		t.Fatalf("missing lead: err = %v", err)
	}
	outsider := base()
	outsider.Roles["lead"] = "stranger"
	if err := outsider.Validate(); err == nil || !strings.Contains(err.Error(), "not a participant") {
		t.Fatalf("outsider lead: err = %v", err)
	}
	unknownRole := base()
	unknownRole.Roles["scribe"] = "someone-else" // minor-additive tolerance
	if err := unknownRole.Validate(); err != nil {
		t.Fatalf("unknown role must be tolerated: %v", err)
	}
	same := base()
	same.Participants = []string{"claude", "claude"}
	if err := same.Validate(); err == nil {
		t.Fatal("same-name participants must be refused")
	}
}

func TestNewPairDefaultsLeadToCreator(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, codex, "led-by-codex")
	if s.Roles["lead"] != "codex" || s.Roles["reviewer"] != "claude" {
		t.Fatalf("roles = %v, want creator lead", s.Roles)
	}
	if s.Participants[0] != "codex" || s.Participants[1] != "claude" {
		t.Fatalf("participants = %v", s.Participants)
	}
}

func TestSecretScanBlocksPublishDraftSurvives(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	draft, err := claude.CreateDraft(s.Pair, "note", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	body := "deploy key is AKIAIOSFODNN7EXAMPLE ok"
	if _, err := claude.Publish(draft, PublishInput{Body: body, Prompt: "next"}, false); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v, want secret refusal", err)
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatal("draft must survive a refused publish (durable intent)")
	}
	formal := filepath.Join(claude.PairDir(s.Pair), ArtifactName(1, "claude", "note"))
	if _, statErr := os.Stat(formal); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("no formal file may exist after a refused publish")
	}
}

func TestFrontmatterRoundTrip(t *testing.T) {
	in := 41
	cor := 7
	fm := &Frontmatter{
		Schema: Schema, Seq: 42, Author: "claude", Peer: "codex",
		Kind: "correction", Status: "timed_out",
		Created:       time.Date(2026, 6, 11, 13, 0, 0, 0, time.UTC),
		InReplyTo:     &in,
		Corrects:      &cor,
		PromptForNext: "line one\n\n@user: need a decision\nlast line",
		TouchedPaths:  []string{"internal/relay/relay.go", "docs/spec.md"},
	}
	body := "# Body\n\nwith **markdown** and --- inside\n"
	raw := Render(fm, body)
	got, gotBody, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gotBody != body {
		t.Fatalf("body round-trip:\n%q\nvs\n%q", body, gotBody)
	}
	if !reflect.DeepEqual(fm, got) {
		t.Fatalf("frontmatter round-trip:\n%+v\nvs\n%+v", fm, got)
	}
	// Unknown keys fail closed.
	bad := strings.Replace(string(raw), "corrects: 7", "corrects: 7\nevil_key: x", 1)
	if _, _, err := Parse([]byte(bad)); err == nil || !strings.Contains(err.Error(), "unknown frontmatter key") {
		t.Fatalf("unknown key: err = %v", err)
	}
}

func TestWaitTimeoutAndTerminal(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	mustPublish(t, claude, s.Pair, "plan", "body", "next")

	res, err := claude.Wait(s.Pair, 2*time.Second)
	if err != nil || res.Code != WaitTimeout {
		t.Fatalf("wait = %+v err=%v, want timeout 10", res, err)
	}

	if err := claude.Close(s.Pair, "approve", "done", false); err != nil {
		t.Fatal(err)
	}
	// Archived with no undelivered peer artifact: deterministic 12
	// (review 054 blocker 2: archive must not surface as an error).
	res, err = claude.Wait(s.Pair, time.Second)
	if err != nil || res.Code != WaitTerminal {
		t.Fatalf("wait on archived pair = %+v err=%v, want 12", res, err)
	}
}

func TestWaitDeliversUnconsumedDecisionAfterCloseArchive(t *testing.T) {
	// review 054 blocker 2: peer publishes a decision and immediately
	// closes+archives — ready-priority must still deliver the decision.
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "close-smoke")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "body", "review")
	mustPublish(t, codex, s.Pair, "decision", "approved, shipping", "none")
	if err := codex.Close(s.Pair, "approve", "done", false); err != nil {
		t.Fatal(err)
	}

	// Explicit slug, bound binding, both resolve through the archive.
	res, err := claude.Wait(s.Pair, time.Second)
	if err != nil || res.Code != WaitNewArtifact {
		t.Fatalf("wait = %+v err=%v, want undelivered decision", res, err)
	}
	if !strings.Contains(res.ArtifactPath, "_archive") || !strings.Contains(res.ArtifactPath, "002-codex-decision.md") {
		t.Fatalf("artifact path = %s", res.ArtifactPath)
	}
	if _, _, err := ReadArtifact(res.ArtifactPath); err != nil {
		t.Fatalf("archived artifact unreadable: %v", err)
	}
	// Bound (no explicit slug) resolution takes the binding→archive path.
	res, err = claude.Wait("", time.Second)
	if err != nil || res.Code != WaitNewArtifact {
		t.Fatalf("bound wait = %+v err=%v", res, err)
	}
}

func TestResolvePairFailsClosedOnCorruptSession(t *testing.T) {
	// review 060 blocker 2 applied consistently: auto-adopt must not
	// silently skip a corrupt pair (it could bind the wrong one).
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	mustPair(t, claude, "good")
	if err := os.Remove(claude.bindingPath()); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(root, "20990101-bad")
	if err := os.MkdirAll(badDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "session.json"), []byte(`{"schema":"oma-relay/9"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := claude.ResolvePair("", true); err == nil || !strings.Contains(err.Error(), "20990101-bad") {
		t.Fatalf("corrupt pair candidate: err = %v, want fail-closed naming it", err)
	}
}

func TestDraftExplicitPairBeatsBindingState(t *testing.T) {
	// review 054 blocker 1: an explicit --pair must never depend on
	// binding or auto-disambiguation state.
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s1 := mustPair(t, claude, "alpha")
	ck2 := newClock()
	ck2.advance(48 * time.Hour)
	claude2 := testLedger(t, root, "claude", ck2)
	if _, err := claude2.NewPair("beta", "", "p", false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(claude.bindingPath()); err != nil {
		t.Fatal(err)
	}
	draft, err := claude.CreateDraft(s1.Pair, "note", nil, nil, false)
	if err != nil {
		t.Fatalf("explicit --pair with two actives and no binding: %v", err)
	}
	if !strings.Contains(draft, s1.Pair) {
		t.Fatalf("draft landed in %s, want %s", draft, s1.Pair)
	}
}

func TestCleanStalePreservesResumablePublish(t *testing.T) {
	// review 054 blocker 3: an interrupted publish (formal exists, no
	// .ready, draft alive) must survive --clean-stale even when the
	// author's heartbeat is stale; the rerun then converges.
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	draft, err := claude.CreateDraft(s.Pair, "plan", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("kill")
	claude.StepHook = func(step string) error {
		if step == "formal-renamed" {
			return boom
		}
		return nil
	}
	if _, err := claude.Publish(draft, PublishInput{Body: "body", Prompt: "next"}, false); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	claude.StepHook = nil
	ck.advance(20 * time.Minute) // heartbeat goes stale

	actions, err := claude.CleanStale(s.Pair, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(actions, "; "), "resumable") {
		t.Fatalf("actions = %v, want resumable notice", actions)
	}
	formal := filepath.Join(claude.PairDir(s.Pair), ArtifactName(1, "claude", "plan"))
	for _, path := range []string{draft, formal, filepath.Join(claude.PairDir(s.Pair), ".seq", "001")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("recovery path destroyed: %s missing", path)
		}
	}
	if _, err := claude.Publish(draft, PublishInput{Body: "body", Prompt: "next"}, false); err != nil {
		t.Fatalf("resumed publish: %v", err)
	}
	if _, _, err := ReadArtifact(formal); err != nil {
		t.Fatalf("converged artifact: %v", err)
	}
}

func TestArchivedSessionTamperFailsClosed(t *testing.T) {
	// review 056 blocker 1: the archive fallback must hold the same
	// consistency bar as the active path — pair==directory and terminal
	// status — or a tampered archive redirects wait.
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "tamper")
	mustPublish(t, claude, s.Pair, "plan", "body", "next")
	if err := claude.Close(s.Pair, "approve", "done", false); err != nil {
		t.Fatal(err)
	}
	sessPath := filepath.Join(root, "_archive", s.Pair, "session.json")

	tamper := func(replace, with string) {
		t.Helper()
		raw, err := os.ReadFile(sessPath)
		if err != nil {
			t.Fatal(err)
		}
		out := strings.Replace(string(raw), replace, with, 1)
		if out == string(raw) {
			t.Fatalf("tamper target %q not found", replace)
		}
		if err := os.WriteFile(sessPath, []byte(out), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	original, _ := os.ReadFile(sessPath)
	tamper(`"pair": "`+s.Pair+`"`, `"pair": "20990101-other"`)
	if _, err := claude.Wait(s.Pair, time.Second); err == nil || !strings.Contains(err.Error(), "does not match directory") {
		t.Fatalf("pair-mismatch archive: err = %v, want fail-closed", err)
	}
	if err := os.WriteFile(sessPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	tamper(`"status": "closed"`, `"status": "active"`)
	if _, err := claude.Wait(s.Pair, time.Second); err == nil || !strings.Contains(err.Error(), "non-terminal") {
		t.Fatalf("active-status archive: err = %v, want fail-closed", err)
	}
}

func TestCleanStaleQuarantinesFormalWithoutDraft(t *testing.T) {
	// review 056 blocker 2: a formal without .ready AND without a draft
	// is not resumable — the reservation must go and the formal must be
	// quarantined so doctor converges.
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	draft, err := claude.CreateDraft(s.Pair, "plan", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("kill")
	claude.StepHook = func(step string) error {
		if step == "formal-renamed" {
			return boom
		}
		return nil
	}
	if _, err := claude.Publish(draft, PublishInput{Body: "body", Prompt: "next"}, false); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	claude.StepHook = nil
	if err := os.Remove(draft); err != nil { // the draft is lost: not resumable
		t.Fatal(err)
	}
	ck.advance(20 * time.Minute)

	actions, err := claude.CleanStale(s.Pair, false)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(actions, "; ")
	if strings.Contains(joined, "resumable") {
		t.Fatalf("actions = %v: must not claim resumability without a draft", actions)
	}
	pairDir := claude.PairDir(s.Pair)
	if _, err := os.Stat(filepath.Join(pairDir, ".seq", "001")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("orphan .seq reservation must be removed")
	}
	if _, err := os.Stat(filepath.Join(pairDir, ArtifactName(1, "claude", "plan")+".stale")); err != nil {
		t.Fatal("formal must be quarantined to .stale")
	}
	// Doctor converges: a second pass finds nothing left to do.
	actions, err = claude.CleanStale(s.Pair, false)
	if err != nil || len(actions) != 0 {
		t.Fatalf("second pass actions = %v err=%v, want clean convergence", actions, err)
	}
}

func TestSecretScannerShapes(t *testing.T) {
	// review 054 blocker 4: provider prefixes and unquoted assignments
	// must block; benign prose must not (no-bypass design demands a low
	// false-positive floor).
	blocked := []string{
		"OPENAI_API_KEY=sk-proj-FAKE0000000000000000abcd",
		"export ANTHROPIC_KEY=sk-ant-FAKE0000000000000000",
		"stripe sk_live_FAKE000000000000004242",
		"MY_ACCESS_TOKEN=Ab1-0000000000000000",
		`api_key: "quoted-secret-000"`,
		"AKIAIOSFODNN7EXAMPLE",
	}
	for _, line := range blocked {
		if got := ScanSecrets([]byte(line)); len(got) == 0 {
			t.Errorf("must block: %q", line)
		}
	}
	benign := []string{
		"the token = approximately four utf-8 bytes",
		"budget_tokens: 2000",
		"description_budget_tokens: 80",
		"password rules require 12 characters minimum",
		"sk-learn pipelines are unrelated",
		"OMA_RELAY_STALE_AFTER=900",
		"in_reply_to: 50",
		"resident token surface counts name+description",
	}
	for _, line := range benign {
		if got := ScanSecrets([]byte(line)); len(got) != 0 {
			t.Errorf("false positive on %q: %v", line, got)
		}
	}
}

func TestParseRejectsDuplicateFrontmatterKeys(t *testing.T) {
	fm := &Frontmatter{Schema: Schema, Seq: 1, Author: "claude", Peer: "codex",
		Kind: "note", Status: "ready", Created: time.Date(2026, 6, 11, 13, 0, 0, 0, time.UTC),
		TouchedPaths: []string{}, PromptForNext: "x"}
	raw := string(Render(fm, "body"))
	dup := strings.Replace(raw, "kind: note\n", "kind: note\nkind: decision\n", 1)
	if _, _, err := Parse([]byte(dup)); err == nil || !strings.Contains(err.Error(), "duplicate frontmatter key") {
		t.Fatalf("err = %v, want duplicate-key refusal", err)
	}
}

func TestWaitReturnsImmediatelyOnUnconsumedPeerArtifact(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "body", "review please")
	formal := mustPublish(t, codex, s.Pair, "review", "lgtm", "proceed")

	res, err := claude.Wait(s.Pair, time.Hour)
	if err != nil || res.Code != WaitNewArtifact || res.ArtifactPath != formal {
		t.Fatalf("wait = %+v err=%v, want immediate artifact", res, err)
	}
}

func TestCloseArchivesAndRestoreBringsBack(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	mustPublish(t, claude, s.Pair, "decision", "final synthesis", "none")

	if err := claude.Close(s.Pair, "approve", "shipped", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(claude.PairDir(s.Pair)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("closed pair must be archived away")
	}
	archived := filepath.Join(root, "_archive", s.Pair)
	if _, err := os.Stat(filepath.Join(archived, "CLOSED")); err != nil {
		t.Fatal("CLOSED sentinel missing in archive")
	}
	if err := claude.Restore(s.Pair, false); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := claude.LoadSession(s.Pair)
	if err != nil || !got.Terminal() {
		t.Fatalf("restored session = %+v err=%v (must stay terminal)", got, err)
	}
	// Double close refused.
	if err := claude.Close(s.Pair, "approve", "again", false); err == nil {
		t.Fatal("closing a terminal pair must refuse")
	}
}

func TestDraftRequiresFillBeforePublish(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	draft, err := claude.CreateDraft(s.Pair, "note", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claude.Publish(draft, PublishInput{}, false); err == nil || !strings.Contains(err.Error(), "TODO") {
		t.Fatalf("placeholder publish: err = %v", err)
	}
	if _, err := claude.Publish(draft, PublishInput{Body: "real body"}, false); err == nil || !strings.Contains(err.Error(), "prompt_for_next") {
		t.Fatalf("empty prompt: err = %v", err)
	}
	if _, err := claude.Publish(draft, PublishInput{Body: "real body", Prompt: "do X", Status: "bogus"}, false); err == nil {
		t.Fatal("invalid status must refuse")
	}
}

func TestCorrectionRequiresCorrects(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	if _, err := claude.CreateDraft(s.Pair, "correction", nil, nil, false); err == nil || !strings.Contains(err.Error(), "corrects") {
		t.Fatalf("correction without --corrects: err = %v", err)
	}
	cor := 1
	if _, err := claude.CreateDraft(s.Pair, "correction", nil, &cor, false); err != nil {
		t.Fatalf("correction with corrects: %v", err)
	}
}

func TestDryRunZeroWrites(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	before := treeDigest(t, root)

	if _, err := claude.CreateDraft(s.Pair, "note", nil, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := claude.NewPair("another", "", "p", true); err != nil {
		t.Fatal(err)
	}
	if err := claude.Close(s.Pair, "approve", "why", true); err != nil {
		t.Fatal(err)
	}
	if got := treeDigest(t, root); got != before {
		t.Fatal("dry-run paths must write nothing")
	}

	draft, err := claude.CreateDraft(s.Pair, "note", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	mid := treeDigest(t, root)
	if _, err := claude.Publish(draft, PublishInput{Body: "b", Prompt: "p"}, true); err != nil {
		t.Fatal(err)
	}
	if got := treeDigest(t, root); got != mid {
		t.Fatal("dry-run publish must write nothing")
	}
}

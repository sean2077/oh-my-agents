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
	// The pair is archived now; a fresh wait resolves via explicit slug
	// and reports terminal.
	res, err = claude.Wait(s.Pair, time.Second)
	if err == nil {
		// Resolution may fail (archived = not found) — both shapes are
		// acceptable as long as the result is terminal, never a hang.
		if res.Code != WaitTerminal {
			t.Fatalf("wait on closed pair = %+v", res)
		}
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
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

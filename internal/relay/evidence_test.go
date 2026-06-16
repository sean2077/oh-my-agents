package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func evBlock(json string) string {
	return "review header\n\n```oma-review-evidence/1\n" + json + "\n```\n"
}

func mkEvidence(t *testing.T, json string) *Evidence {
	t.Helper()
	ev, err := ParseEvidence(evBlock(json))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return ev
}

func TestParseEvidenceExtractionFailClosed(t *testing.T) {
	cases := map[string]string{
		"no block":     "just prose, no fence",
		"unterminated": "x\n```oma-review-evidence/1\n{}\n",
		"two blocks":   evBlock(`{"schema":"oma-review-evidence/1"}`) + evBlock(`{"schema":"oma-review-evidence/1"}`),
	}
	for name, body := range cases {
		if _, err := ParseEvidence(body); err == nil {
			t.Errorf("%s: expected fail-closed", name)
		}
	}
}

func TestParseEvidenceStrictJSON(t *testing.T) {
	if _, err := ParseEvidence(evBlock(`{"schema":"oma-review-evidence/1","bogus":1}`)); err == nil {
		t.Fatal("unknown JSON key must fail closed")
	}
	if _, err := ParseEvidence(evBlock(`{"schema":"oma-review-evidence/2"}`)); err == nil {
		t.Fatal("wrong evidence schema must fail")
	}
	if _, err := ParseEvidence(evBlock(`{"schema":"oma-review-evidence/1"} trailing`)); err == nil {
		t.Fatal("trailing content must fail")
	}
}

func TestValidateEvidenceByVerdict(t *testing.T) {
	// revise / approve-with-changes require non-empty findings
	empty := mkEvidence(t, `{"schema":"oma-review-evidence/1","findings":[],"basis_refs":[],"commands_run":["x"],"limitations":["y"]}`)
	if err := ValidateEvidence(empty, VerdictRevise); err == nil {
		t.Fatal("revise with no findings must fail")
	}
	if err := ValidateEvidence(empty, VerdictApproveWithChanges); err == nil {
		t.Fatal("approve-with-changes with no findings must fail")
	}
	// approve may have empty findings but must carry basis_refs + commands + limitations
	if err := ValidateEvidence(empty, VerdictApprove); err == nil {
		t.Fatal("approve with no basis_refs must fail (evidence-free approve)")
	}
	okApprove := mkEvidence(t, `{"schema":"oma-review-evidence/1","findings":[],"basis_refs":[{"type":"repo","ref":"a.go:1"}],"commands_run":["go test -> ok"],"limitations":["none"]}`)
	if err := ValidateEvidence(okApprove, VerdictApprove); err != nil {
		t.Fatalf("valid approve evidence rejected: %v", err)
	}
	// approve missing commands_run / limitations
	noCmd := mkEvidence(t, `{"schema":"oma-review-evidence/1","findings":[],"basis_refs":[{"type":"repo","ref":"a.go:1"}],"commands_run":[],"limitations":["none"]}`)
	if err := ValidateEvidence(noCmd, VerdictApprove); err == nil {
		t.Fatal("approve without commands_run must fail")
	}
}

func TestValidateEvidenceRefsAndPlaceholders(t *testing.T) {
	bad := map[string]string{
		"unknown severity":   `{"schema":"oma-review-evidence/1","findings":[{"severity":"BOGUS","confidence":"high","claim":"c","refs":[{"type":"repo","ref":"a.go:1"}]}],"basis_refs":[],"commands_run":["x"],"limitations":["y"]}`,
		"unknown confidence": `{"schema":"oma-review-evidence/1","findings":[{"severity":"low","confidence":"BOGUS","claim":"c","refs":[{"type":"repo","ref":"a.go:1"}]}],"basis_refs":[],"commands_run":["x"],"limitations":["y"]}`,
		"finding no ref":     `{"schema":"oma-review-evidence/1","findings":[{"severity":"low","confidence":"low","claim":"c","refs":[]}],"basis_refs":[],"commands_run":["x"],"limitations":["y"]}`,
		"placeholder claim":  `{"schema":"oma-review-evidence/1","findings":[{"severity":"low","confidence":"low","claim":"TODO","refs":[{"type":"repo","ref":"a.go:1"}]}],"basis_refs":[],"commands_run":["x"],"limitations":["y"]}`,
		"absolute repo ref":  `{"schema":"oma-review-evidence/1","findings":[{"severity":"low","confidence":"low","claim":"c","refs":[{"type":"repo","ref":"/abs/a.go:1"}]}],"basis_refs":[],"commands_run":["x"],"limitations":["y"]}`,
		"dotdot repo ref":    `{"schema":"oma-review-evidence/1","findings":[{"severity":"low","confidence":"low","claim":"c","refs":[{"type":"repo","ref":"../a.go:1"}]}],"basis_refs":[],"commands_run":["x"],"limitations":["y"]}`,
		"unknown ref type":   `{"schema":"oma-review-evidence/1","findings":[{"severity":"low","confidence":"low","claim":"c","refs":[{"type":"blog","ref":"a.go:1"}]}],"basis_refs":[],"commands_run":["x"],"limitations":["y"]}`,
		"non-url external":   `{"schema":"oma-review-evidence/1","findings":[{"severity":"low","confidence":"low","claim":"c","refs":[{"type":"official","ref":"not a url"}]}],"basis_refs":[],"commands_run":["x"],"limitations":["y"]}`,
		"placeholder cmd":    `{"schema":"oma-review-evidence/1","findings":[{"severity":"low","confidence":"low","claim":"c","refs":[{"type":"repo","ref":"a.go:1"}]}],"basis_refs":[],"commands_run":["tbd"],"limitations":["y"]}`,
	}
	for name, json := range bad {
		if err := ValidateEvidence(mkEvidence(t, json), VerdictRevise); err == nil {
			t.Errorf("%s: expected validation failure", name)
		}
	}
	// valid: a finding citing both a repo range and an external URL with date
	ok := mkEvidence(t, `{"schema":"oma-review-evidence/1","findings":[{"severity":"high","confidence":"medium","claim":"real issue","refs":[{"type":"repo","ref":"internal/relay/publish.go:83-90"},{"type":"official","ref":"https://example.com/doc","version_or_date":"2026-06"}]}],"basis_refs":[],"commands_run":["go test ./internal/relay -> fail"],"limitations":["did not run e2e"]}`)
	if err := ValidateEvidence(ok, VerdictRevise); err != nil {
		t.Fatalf("valid multi-ref evidence rejected: %v", err)
	}
}

func TestEvidenceHashStableAndRecomputable(t *testing.T) {
	body := evBlock(`{"schema":"oma-review-evidence/1","findings":[],"basis_refs":[{"type":"repo","ref":"a.go:1"}],"commands_run":["go test -> ok"],"limitations":["none"]}`)
	h1, err := reviewEvidenceHash(body, VerdictApprove)
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := reviewEvidenceHash(body, VerdictApprove)
	if h1 != h2 || !strings.HasPrefix(h1, "sha256:") {
		t.Fatalf("evidence hash unstable/malformed: %q vs %q", h1, h2)
	}
}

func TestPublishReviewRequiresEvidence(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "the plan", "review it") // seq 1
	one := 1
	draft, err := codex.CreateDraft(s.Pair, "review", &one, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codex.Publish(draft, PublishInput{Body: "prose only, no evidence block", Prompt: "p", Verdict: VerdictApprove, ReviewTarget: &one}, false); err == nil {
		t.Fatal("a ready review without an evidence block must be refused")
	}
}

func TestApproveCloseRefusesTamperedEvidence(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "fix", "the work", "review it")    // seq 1
	revPath := mustPublishReview(t, codex, s.Pair, VerdictApprove, 1) // seq 2
	mustPublish(t, claude, s.Pair, "decision", "ship it", "done")     // seq 3
	if err := claude.Close(s.Pair, "approve", "clean", true); err != nil {
		t.Fatalf("clean approve close must pass first: %v", err)
	}
	// Tamper the review body's evidence (change a ref) and refresh its .sha256,
	// leaving the frontmatter evidence_hash stale. The close gate recomputes
	// the body evidence hash and must refuse the mismatch.
	fm, _, err := ReadArtifact(revPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(reviewBody(VerdictApprove), "receipt.go:30", "receipt.go:99", 1)
	rendered := Render(fm, tampered)
	if err := os.WriteFile(revPath, rendered, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(rendered)
	if err := os.WriteFile(revPath+".sha256", []byte(hex.EncodeToString(sum[:])+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := claude.Close(s.Pair, "approve", "tampered", true); err == nil {
		t.Fatal("approve close must refuse a tampered approve review")
	}
}

// TestDecisionReceiptBindsEvidenceHash: a decision receipt binds the approve
// review's evidence hash, and the clean approve close passes the additive
// evidence triple-check (recompute == review frontmatter == receipt).
func TestDecisionReceiptBindsEvidenceHash(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "fix", "the work", "review it")
	rev := mustPublishReview(t, codex, s.Pair, VerdictApprove, 1)
	dec := mustPublish(t, claude, s.Pair, "decision", "ship it", "done")
	rfm, _, err := ReadArtifact(rev)
	if err != nil {
		t.Fatal(err)
	}
	dfm, _, err := ReadArtifact(dec)
	if err != nil {
		t.Fatal(err)
	}
	if rfm.EvidenceHash == "" {
		t.Fatal("approve review must carry an evidence_hash")
	}
	if dfm.QualityGateEvidenceHash != rfm.EvidenceHash {
		t.Fatalf("decision must bind the review evidence_hash: %q vs %q", dfm.QualityGateEvidenceHash, rfm.EvidenceHash)
	}
	if err := claude.Close(s.Pair, "approve", "shipped", true); err != nil {
		t.Fatalf("clean approve close must pass the evidence triple-check: %v", err)
	}
}

package relay

import (
	"os"
	"strings"
	"testing"
)

// S1 / SEC-3: positive + boundary coverage for the secret-scanner patterns that
// TestSecretScannerShapes does not exercise (private-key block, GitHub, Slack,
// JWT), plus end-to-end proof that the pre-publish scan covers the
// prompt_for_next and touched_paths channels — not just the body. A regex edit
// that silently stops matching one pattern, or a refactor that narrows the scan
// to the body, fails here.

func TestSecretScannerResidualPatterns(t *testing.T) {
	positive := []struct {
		name, line, wantPattern string
	}{
		{"rsa-private-key", "-----BEGIN RSA PRIVATE KEY-----", "private-key-block"},
		{"openssh-private-key", "-----BEGIN OPENSSH PRIVATE KEY-----", "private-key-block"},
		{"ec-private-key", "-----BEGIN EC PRIVATE KEY-----", "private-key-block"},
		{"github-pat", "ghp_" + strings.Repeat("A", 36), "github-token"},
		{"github-oauth", "gho_" + strings.Repeat("b", 36), "github-token"},
		{"slack-bot", "xoxb-" + strings.Repeat("0", 12), "slack-token"},
		{"jwt", "eyJ" + strings.Repeat("a", 12) + ".eyJ" + strings.Repeat("b", 12) + ".sig", "jwt"},
	}
	for _, c := range positive {
		got := ScanSecrets([]byte(c.line))
		if len(got) == 0 {
			t.Errorf("%s: must block %q", c.name, c.line)
			continue
		}
		if joined := strings.Join(got, ","); !strings.Contains(joined, c.wantPattern) {
			t.Errorf("%s: blocked by %q, want pattern %q", c.name, joined, c.wantPattern)
		}
	}

	// Boundary cases just under each pattern's threshold must NOT match, so the
	// no-bypass scanner keeps a low false-positive floor.
	boundary := []struct {
		name, line string
	}{
		{"github-one-short", "ghp_" + strings.Repeat("A", 35)},  // 35 < 36 required
		{"slack-bad-prefix", "xoxz-" + strings.Repeat("0", 12)}, // z not in [baprs]
		{"jwt-single-segment", "eyJ" + strings.Repeat("a", 12) + ".not-a-jwt"},
		{"benign-certificate", "-----BEGIN CERTIFICATE-----"}, // not a PRIVATE KEY block
	}
	for _, c := range boundary {
		if got := ScanSecrets([]byte(c.line)); len(got) != 0 {
			t.Errorf("%s: false positive on %q: %v", c.name, c.line, got)
		}
	}
}

// A secret carried by a non-body channel must still be caught: the pre-publish
// scan covers the whole rendered artifact (prompt_for_next + touched_paths),
// not just the body. Each case keeps the body clean so a refactor narrowing the
// scan to the body alone would fail here.
func TestSecretScanBlocksPublishViaNonBodyChannels(t *testing.T) {
	cases := []struct {
		name string
		in   PublishInput
	}{
		{
			// prompt_for_next is what the peer reads every round.
			name: "prompt_for_next",
			in:   PublishInput{Body: "clean handoff body", Prompt: "ssh in with AKIAIOSFODNN7EXAMPLE then run make"},
		},
		{
			// touched_paths is rendered as a list; a contrived path proves it is scanned.
			name: "touched_paths",
			in:   PublishInput{Body: "clean body", Prompt: "clean prompt", Touched: []string{"secrets/AKIAIOSFODNN7EXAMPLE"}},
		},
	}
	for _, c := range cases {
		ck := newClock()
		root, _ := initRoot(t, ck)
		claude := testLedger(t, root, "claude", ck)
		s := mustPair(t, claude, "topic")
		draft, err := claude.CreateDraft(s.Pair, "note", nil, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := claude.Publish(draft, c.in, false); err == nil || !strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: err = %v, want secret refusal", c.name, err)
		}
		if _, err := os.Stat(draft); err != nil {
			t.Errorf("%s: draft must survive a refused publish (durable intent)", c.name)
		}
	}
}

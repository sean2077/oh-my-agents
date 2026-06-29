package relay

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestOwnDraftPathGuards pins every refusal/return branch of
// ownDraftPath (publish.go:248-273). ownDraftPath operates on path
// strings via filepath.Abs, so no files need exist on disk; the Ledger
// only needs a known Root and a known Identity.Author. Each error case
// asserts the exact fmt.Errorf substring so a removed guard fails this
// test instead of silently returning a wrong (pairDir, seq, kind).
func TestOwnDraftPathGuards(t *testing.T) {
	root := t.TempDir()
	id, err := makeIdentity("claude", "session-claude")
	if err != nil {
		t.Fatal(err)
	}
	l := NewLedger(root, id) // l.Root = root, l.Identity.Author = "claude"

	// A pair directly under the ledger root, the only well-located shape.
	const pair = "20260629-topic"
	pairDir := filepath.Join(root, pair)

	// Branch 5 inputs (happy path), reused below for the negative cases so
	// each negative differs from the happy path in exactly one dimension.
	const happyName = "007-claude-fix.md"
	happyPath := filepath.Join(pairDir, ".draft", happyName)

	t.Run("not a .draft path (publish.go:254-255)", func(t *testing.T) {
		// Parent dir is "notdraft", not ".draft" → first guard fires before
		// any root or filename check.
		p := filepath.Join(pairDir, "notdraft", happyName)
		_, _, _, err := l.ownDraftPath(p)
		if err == nil || !strings.Contains(err.Error(), "is not a .draft/ path") {
			t.Fatalf("err = %v, want %q", err, "is not a .draft/ path")
		}
	})

	t.Run("not under ledger root (publish.go:262-263)", func(t *testing.T) {
		// A correctly-shaped .draft path, but its pair dir's parent is a
		// foreign directory, not l.Root → second guard fires.
		other := t.TempDir()
		p := filepath.Join(other, pair, ".draft", happyName)
		_, _, _, err := l.ownDraftPath(p)
		if err == nil || !strings.Contains(err.Error(), "not under ledger root") {
			t.Fatalf("err = %v, want %q", err, "not under ledger root")
		}
	})

	t.Run("malformed filename (publish.go:266-267)", func(t *testing.T) {
		// Correctly located .draft file, but the basename is not
		// NNN-<author>-<kind>.md (no kind segment) → ParseArtifactName
		// returns ok=false and the third guard fires. Author segment is
		// still our own so a regression could only fall through to the
		// happy path, never the wrong-author branch.
		p := filepath.Join(pairDir, ".draft", "007-claude.md")
		_, _, _, err := l.ownDraftPath(p)
		if err == nil || !strings.Contains(err.Error(), "is not NNN-<author>-<kind>.md") {
			t.Fatalf("err = %v, want %q", err, "is not NNN-<author>-<kind>.md")
		}
	})

	t.Run("wrong author (publish.go:269-270)", func(t *testing.T) {
		// Well-formed draft filename, correctly located, but authored by a
		// different participant → fourth guard fires (drafts are private to
		// their author).
		p := filepath.Join(pairDir, ".draft", "007-codex-fix.md")
		_, _, _, err := l.ownDraftPath(p)
		if err == nil {
			t.Fatalf("err = nil, want wrong-author refusal")
		}
		if !strings.Contains(err.Error(), "belongs to") ||
			!strings.Contains(err.Error(), "drafts are private to their author") {
			t.Fatalf("err = %v, want substrings %q and %q",
				err, "belongs to", "drafts are private to their author")
		}
	})

	t.Run("happy path (publish.go:272)", func(t *testing.T) {
		// Well-formed draft of OUR author, correctly located under root:
		// returns the parsed parts with no error.
		gotPair, gotSeq, gotKind, err := l.ownDraftPath(happyPath)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		// pairDir is returned by filepath.Dir(filepath.Dir(abs)); compare
		// against the absolute form for an OS-independent, link-free match.
		wantPair, aerr := filepath.Abs(pairDir)
		if aerr != nil {
			t.Fatal(aerr)
		}
		if gotPair != wantPair {
			t.Fatalf("pairDir = %q, want %q", gotPair, wantPair)
		}
		if gotSeq != 7 {
			t.Fatalf("seq = %d, want 7", gotSeq)
		}
		if gotKind != "fix" {
			t.Fatalf("kind = %q, want %q", gotKind, "fix")
		}
	})
}

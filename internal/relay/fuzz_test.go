package relay

import (
	"bytes"
	"testing"
	"time"
)

// FuzzParse asserts the strict artifact parser never panics on arbitrary
// bytes, and that whenever it accepts input the canonical render is a fixed
// point: Parse → Render → Parse → Render yields identical bytes. That property
// is what lets the close gate trust a re-read artifact without re-deriving it.
func FuzzParse(f *testing.F) {
	valid := Render(&Frontmatter{
		Schema:        ArtifactSchema,
		Seq:           1,
		Author:        "claude",
		Peer:          "codex",
		AuthorSession: "0123456789ab",
		Kind:          "plan",
		Status:        "ready",
		Created:       time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
		TouchedPaths:  []string{},
	}, "a body line\nanother line")
	f.Add(valid)
	f.Add([]byte("not an artifact"))
	f.Add([]byte("---\n---\n"))
	f.Add([]byte("---\nschema: oma-relay/4\nseq: 1\n---\nbody"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		fm, body, err := Parse(raw)
		if err != nil {
			return // rejecting malformed input is the expected, fail-closed path
		}
		r1 := Render(fm, body)
		fm2, body2, err := Parse(r1)
		if err != nil {
			t.Fatalf("re-parse of canonical render failed: %v\n%s", err, r1)
		}
		r2 := Render(fm2, body2)
		if !bytes.Equal(r1, r2) {
			t.Fatalf("Render is not idempotent on accepted input:\n--- r1 ---\n%s\n--- r2 ---\n%s", r1, r2)
		}
	})
}

// FuzzScanSecrets asserts the publish-time secret scanner never panics, is
// deterministic, and — the property that actually matters — never loses a
// planted credential no matter what precedes it. A scanner that can be blinded
// by a crafted prefix would let secrets into the ledger.
func FuzzScanSecrets(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("just some prose\nwith multiple lines"))
	f.Add([]byte("budget_tokens: 2000\n# not a secret"))

	f.Fuzz(func(t *testing.T, content []byte) {
		a := ScanSecrets(content)
		if b := ScanSecrets(content); len(a) != len(b) {
			t.Fatalf("ScanSecrets not deterministic: %d vs %d findings", len(a), len(b))
		}
		planted := append(append([]byte{}, content...), []byte("\nAKIAIOSFODNN7EXAMPLE\n")...)
		if got := ScanSecrets(planted); len(got) == 0 {
			t.Fatalf("planted AWS key missed after %d bytes of prefix", len(content))
		}
	})
}

// FuzzParseArtifactName asserts the ledger filename parser never panics and
// round-trips: any name it accepts re-renders to a name that parses back to the
// same (seq, author, kind).
func FuzzParseArtifactName(f *testing.F) {
	f.Add("001-claude-plan.md")
	f.Add("999-codex-review.md")
	f.Add("garbage.md")
	f.Add("abc-claude-plan.md")

	f.Fuzz(func(t *testing.T, name string) {
		seq, author, kind, ok := ParseArtifactName(name)
		if !ok {
			return
		}
		rt := ArtifactName(seq, author, kind)
		seq2, author2, kind2, ok2 := ParseArtifactName(rt)
		if !ok2 || seq2 != seq || author2 != author || kind2 != kind {
			t.Fatalf("round-trip mismatch: %q -> (%d,%q,%q) -> %q -> (%d,%q,%q,%v)",
				name, seq, author, kind, rt, seq2, author2, kind2, ok2)
		}
	})
}

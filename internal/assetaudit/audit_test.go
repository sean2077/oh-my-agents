package assetaudit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSkill lays down assets/skills/<name>/{manifest.json,SKILL.md}.
func writeSkill(t *testing.T, root, name, description, status, canonical, body string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mf := `{"schema":"oma-asset/1","name":"` + name + `","type":"skill","targets":["claude","codex"],"description_budget_tokens":80`
	if status != "" {
		mf += `,"status":"` + status + `"`
	}
	if canonical != "" {
		mf += `,"canonical":"` + canonical + `"`
	}
	mf += "}\n"
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(mf), 0o644); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: " + name + "\ndescription: " + description + "\n---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}
}

func find(entries []AuditEntry, name string) *AuditEntry {
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i]
		}
	}
	return nil
}

func TestAuditLabels(t *testing.T) {
	root := t.TempDir()
	// keeper: short description, referenced by `referencer` below.
	writeSkill(t, root, "keeper", "short and sweet", "", "", "body")
	// referencer: its body cites `keeper` (an exact token ref).
	writeSkill(t, root, "referencer", "also short", "", "", "use the keeper skill and the fat skill for X")
	// orphan: short description, nobody references it.
	writeSkill(t, root, "lonely", "short desc", "", "", "body")
	// retire: deprecated status.
	writeSkill(t, root, "old-thing", "short", "deprecated", "", "body")
	// oversized: description longer than the 80-token (320-byte) budget.
	writeSkill(t, root, "fat", strings.Repeat("verbose ", 50), "", "", "body")

	entries, err := Audit(root)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"keeper":     LabelKeep,
		"lonely":     LabelOrphan,
		"old-thing":  LabelRetire,
		"fat":        LabelOversized,
		"referencer": LabelOrphan, // nobody references referencer itself
	}
	for name, want := range cases {
		e := find(entries, name)
		if e == nil {
			t.Fatalf("%s missing from audit", name)
		}
		if e.Label != want {
			t.Errorf("%s label = %s, want %s (reason: %s, refs=%d, tok=%d)", name, e.Label, want, e.Reason, e.RefCount, e.ResidentTokens)
		}
	}
	// keeper must have been referenced exactly once and not count itself.
	if e := find(entries, "keeper"); e.RefCount != 1 {
		t.Errorf("keeper ref_count = %d, want 1", e.RefCount)
	}
	if got := find(entries, "lonely").Reason; got != "no inbound references from other assets — direct-use evidence is outside this audit" {
		t.Errorf("lonely reason = %q, want a scope-bounded advisory", got)
	}
}

func TestAuditBodyTokensExcludeYAMLFrontmatter(t *testing.T) {
	for _, tc := range []struct {
		name      string
		eol       string
		delimiter string
	}{
		{name: "lf", eol: "\n", delimiter: "---"},
		{name: "crlf", eol: "\r\n", delimiter: "---"},
		{name: "lf trailing spaces", eol: "\n", delimiter: "---  "},
		{name: "crlf trailing tab", eol: "\r\n", delimiter: "---\t"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, "demo", "short", "", "", "placeholder")
			path := filepath.Join(root, "skills", "demo", "SKILL.md")
			raw := strings.Join([]string{tc.delimiter, "name: demo", "description: short", tc.delimiter, "abcdefgh"}, tc.eol)
			if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}

			entries, err := Audit(root)
			if err != nil {
				t.Fatal(err)
			}
			if got := find(entries, "demo").BodyTokens; got != 2 { // 8 UTF-8 bytes under approx-b4/1
				t.Fatalf("body_tokens = %d, want 2", got)
			}
		})
	}
}

func TestAuditBodyTokensAreLineEndingInvariant(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "demo", "short", "", "", "placeholder")
	path := filepath.Join(root, "skills", "demo", "SKILL.md")

	variants := map[string]string{
		"lf":    "---\nname: demo\ndescription: short\n---\na\nb\nc\nd\n",
		"crlf":  "---\r\nname: demo\r\ndescription: short\r\n---\r\na\r\nb\r\nc\r\nd\r\n",
		"mixed": "---\r\nname: demo\ndescription: short\r\n---\na\r\nb\nc\r\nd\n",
	}

	for name, raw := range variants {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}

			entries, err := Audit(root)
			if err != nil {
				t.Fatal(err)
			}
			if got := find(entries, "demo").BodyTokens; got != 2 {
				t.Fatalf("body_tokens = %d, want 2 after canonical line-ending measurement", got)
			}
		})
	}
}

func TestAuditDescriptionBudgetExcludesAssetName(t *testing.T) {
	root := t.TempDir()
	longName := "skill-" + strings.Repeat("x", 58) // 64 bytes; valid asset name
	description := strings.Repeat("abcd", 70)      // exactly 70 tokens
	writeSkill(t, root, longName, description, "", "", "body")
	writeSkill(t, root, "referencer", "short", "", "", "use "+longName)

	entries, err := Audit(root)
	if err != nil {
		t.Fatal(err)
	}
	e := find(entries, longName)
	if e == nil {
		t.Fatalf("%s missing from audit", longName)
	}
	if e.ResidentTokens <= 80 {
		t.Fatalf("fixture resident_tokens = %d, want > 80", e.ResidentTokens)
	}
	if e.DescriptionTokens != 70 || e.DescriptionBudgetTokens != 80 {
		t.Fatalf("description budget metrics = %d/%d, want 70/80", e.DescriptionTokens, e.DescriptionBudgetTokens)
	}
	if e.Label != LabelKeep {
		t.Fatalf("label = %s, want KEEP: description itself is within its 80-token budget", e.Label)
	}
}

func TestAuditRefExcludesSelfAndCountsCanonical(t *testing.T) {
	root := t.TempDir()
	// successor referenced only via an alias's canonical pointer.
	writeSkill(t, root, "successor", "short", "", "", "successor mentions successor successor") // self refs ignored
	writeSkill(t, root, "old-alias", "short", "alias", "successor", "body")

	entries, err := Audit(root)
	if err != nil {
		t.Fatal(err)
	}
	succ := find(entries, "successor")
	// self-references in successor's own body do NOT count; the alias canonical edge does.
	if succ.RefCount != 1 {
		t.Fatalf("successor ref_count = %d, want 1 (only the canonical edge)", succ.RefCount)
	}
	if a := find(entries, "old-alias"); a.Label != LabelRetire {
		t.Errorf("old-alias label = %s, want RETIRE", a.Label)
	}
}

func TestAuditReferenceMetricsDistinguishOccurrencesAndReferrers(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "target-skill", "short", "", "", "target-skill target-skill")
	writeSkill(t, root, "noisy-referrer", "short", "", "", "target-skill, target-skill; target-skill")
	writeSkill(t, root, "old-alias", "short", "alias", "target-skill", "body")

	entries, err := Audit(root)
	if err != nil {
		t.Fatal(err)
	}
	target := find(entries, "target-skill")
	if target.RefCount != 4 {
		t.Fatalf("target-skill ref_count = %d, want 4 reference occurrences", target.RefCount)
	}
	if target.ReferrerCount != 2 {
		t.Fatalf("target-skill referrer_count = %d, want 2 distinct other assets", target.ReferrerCount)
	}
}

func TestAuditFailsClosedOnBadManifest(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skills", "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// name disagrees with directory → catalog (and thus audit) fails closed.
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"schema":"oma-asset/1","name":"mismatch","type":"skill","targets":["claude"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Audit(root); err == nil {
		t.Fatal("audit must fail closed on a manifest whose name disagrees with its directory")
	}
}

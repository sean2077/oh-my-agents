package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureSet mirrors a future command tree so refcheck tests do not depend
// on which commands the real CLI has grown yet (A4 §4 mandatory fixtures).
func fixtureSet() CommandSet {
	return CommandSet{
		// groups (not runnable)
		"oma": false, "oma asset": false, "oma relay": false,
		"oma relay pair": false, "oma state": false,
		// runnables ("oma doctor" is both runnable and a parent of budget)
		"oma asset install": true, "oma asset list": true, "oma asset link": true,
		"oma doctor": true, "oma doctor budget": true,
		"oma relay pair ensure": true, "oma relay publish": true,
		"oma state get": true, "oma state set": true,
	}
}

func refsFrom(md string) [][]string { return ExtractOmaRefs(md) }

func TestValidNestedCommandAccepted(t *testing.T) {
	md := "Run:\n```bash\noma relay pair ensure --json\n```\n"
	for _, ref := range refsFrom(md) {
		if bad := validateRef(ref, fixtureSet()); bad != "" {
			t.Fatalf("valid three-level ref rejected: %q", bad)
		}
	}
}

func TestInvalidLeafAfterValidPrefixFails(t *testing.T) {
	md := "Run `oma relay pair typo` to break things."
	refs := refsFrom(md)
	if len(refs) != 1 {
		t.Fatalf("refs = %v", refs)
	}
	if bad := validateRef(refs[0], fixtureSet()); bad != "oma relay pair typo" {
		t.Fatalf("invalid leaf: got %q, want oma relay pair typo", bad)
	}
}

func TestFlagsStopTheWalk(t *testing.T) {
	md := "`oma asset link --dev` and `oma doctor budget --agent claude --max-resident-tokens 2000`"
	for _, ref := range refsFrom(md) {
		if bad := validateRef(ref, fixtureSet()); bad != "" {
			t.Fatalf("flag-stopped ref rejected: %v -> %q", ref, bad)
		}
	}
}

func TestArgumentsAfterRunnableAreValid(t *testing.T) {
	// "deep-interview" looks like a subcommand token but follows the
	// RUNNABLE "oma asset install", so it is an argument (A4 §4).
	md := "`oma asset install deep-interview`"
	refs := refsFrom(md)
	if bad := validateRef(refs[0], fixtureSet()); bad != "" {
		t.Fatalf("argument after runnable rejected: %q", bad)
	}
}

func TestGroupEndingRefIsDocumentedExample(t *testing.T) {
	md := "See the `oma relay pair` group."
	refs := refsFrom(md)
	if bad := validateRef(refs[0], fixtureSet()); bad != "" {
		t.Fatalf("group-ending example rejected: %q", bad)
	}
}

func TestMultilineShellSnippet(t *testing.T) {
	md := "```sh\nset -e\noma state set wf/phase planning\noma relay publish draft.md | tee log\necho done; oma asset list\n```"
	refs := refsFrom(md)
	if len(refs) != 3 {
		t.Fatalf("want 3 refs, got %v", refs)
	}
	for _, ref := range refs {
		if bad := validateRef(ref, fixtureSet()); bad != "" {
			t.Fatalf("ref %v rejected: %q", ref, bad)
		}
	}
}

func TestUnknownLeafWithDigitsFails(t *testing.T) {
	// review 038 blocker 2: digits/underscores must not stop the token
	// walk before validation sees the unknown leaf.
	for _, md := range []string{"`oma bogus2`", "`oma relay pair typo_2`"} {
		refs := refsFrom(md)
		if len(refs) != 1 {
			t.Fatalf("%s: refs = %v", md, refs)
		}
		if bad := validateRef(refs[0], fixtureSet()); bad == "" {
			t.Fatalf("%s: unknown leaf with digits slipped through", md)
		}
	}
}

func TestArgumentsWithSlashesAndExtensionsValid(t *testing.T) {
	for _, md := range []string{"`oma state set wf/phase planning`", "`oma relay publish draft.md`"} {
		refs := refsFrom(md)
		if bad := validateRef(refs[0], fixtureSet()); bad != "" {
			t.Fatalf("%s rejected: %q", md, bad)
		}
	}
}

func TestUnknownTopGroupFails(t *testing.T) {
	md := "`oma autopilot step`"
	refs := refsFrom(md)
	if bad := validateRef(refs[0], fixtureSet()); bad != "oma autopilot" {
		t.Fatalf("unknown group: got %q, want oma autopilot", bad)
	}
}

func TestRefcheckWalksInstalledSkills(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "demo")
	if err := os.MkdirAll(filepath.Join(skill, "references"), 0o700); err != nil {
		t.Fatal(err)
	}
	good := "Use `oma state get demo/phase`."
	bad := "Then run `oma nonexistent verb` happily."
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "references", "deep.md"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := Refcheck(root, fixtureSet())
	if len(fs) != 1 || fs[0].Level != LevelFail || !strings.Contains(fs[0].Message, "oma nonexistent") {
		t.Fatalf("findings = %+v", fs)
	}
}

func TestRefcheckMissingRootIsClean(t *testing.T) {
	if fs := Refcheck(filepath.Join(t.TempDir(), "absent"), fixtureSet()); len(fs) != 0 {
		t.Fatalf("missing root must be clean, got %+v", fs)
	}
}

func TestProseOutsideCodeIgnored(t *testing.T) {
	md := "The oma totally-fake command is discussed in prose without code formatting."
	if refs := refsFrom(md); len(refs) != 0 {
		t.Fatalf("prose must not produce refs: %v", refs)
	}
}

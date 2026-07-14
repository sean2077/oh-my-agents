package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readPairHandoffContractFile(t *testing.T, parts ...string) string {
	t.Helper()
	pathParts := append([]string{"..", ".."}, parts...)
	raw, err := os.ReadFile(filepath.Join(pathParts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestPairHandoffUsesCompactDeltaContract(t *testing.T) {
	skill := readPairHandoffContractFile(t, "assets", "skills", "pair-delivery", "SKILL.md")

	for _, want := range []string{
		"prompt_for_next is a compact delta contract",
		"**Fixed point**",
		"**Delta + next task**",
		"**Locked decisions / non-goals**",
		"**Acceptance + validation expected**",
		"**Reply kind + stop conditions**",
		"**Review independence** *(review prompts only)*",
		"Non-review prompts MUST omit it",
		"self-contained relative to the fixed point",
		"make the receiver walk earlier ledger artifacts",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("pair-delivery handoff contract lost %q", want)
		}
	}

	for _, oldField := range []string{
		"**Task**:",
		"**Acceptance criteria**:",
		"**Validation expected back**:",
		"**Reply kind**:",
		"**Stop conditions**:",
	} {
		if strings.Contains(skill, oldField) {
			t.Fatalf("pair-delivery retained superseded prompt field %q", oldField)
		}
	}
}

func TestPairHandoffWorkflowSpecMatchesSkill(t *testing.T) {
	workflows := readPairHandoffContractFile(t, "docs", "reference", "workflows.md")
	for _, want := range []string{
		"compact delta-based `prompt_for_next`",
		"fixed artifact/commit/spec baseline",
		"locked decisions and non-goals",
		"only for review, an independence request",
		"never has to reconstruct them by walking earlier ledger artifacts",
	} {
		if !strings.Contains(workflows, want) {
			t.Fatalf("pair-delivery workflow spec lost handoff rule %q", want)
		}
	}
}

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readPairDeliverySkill(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "assets", "skills", "pair-delivery", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestPairDeliverySkillCodexStopHookFirst(t *testing.T) {
	text := readPairDeliverySkill(t)
	for _, want := range []string{
		"the Stop hook is the main self-continuation path",
		"Stop hook dispatcher is wired in `~/.codex/hooks.json` and trusted through `/hooks`",
		"Use held/re-polled `oma relay wait` only as the fallback path",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pair-delivery skill lost Codex Stop-hook-first guardrail %q", want)
		}
	}
}

func TestPairDeliverySkillReviewerAntiPrejudgingContract(t *testing.T) {
	text := readPairDeliverySkill(t)
	for _, want := range []string{
		"Reviewer contract (anti-prejudging)",
		"read-only review of the checkout",
		"two separate judgments",
		"Spec compliance",
		"Quality verdict",
		"`file:line`",
		"Do not pre-judge the reviewer",
		"`don't flag`",
		"`at most Minor`",
		"stop signal",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pair-delivery skill lost reviewer anti-prejudging guardrail %q", want)
		}
	}
}

func TestPairDeliverySkillReviewReceptionDiscipline(t *testing.T) {
	text := readPairDeliverySkill(t)
	for _, want := range []string{
		"Review reception discipline",
		"Never reply with performative agreement",
		"`You're absolutely right`",
		"Clarify ALL unclear findings before acting",
		"Verify each finding before implementing",
		"Record a disposition for every finding",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pair-delivery skill lost review-reception guardrail %q", want)
		}
	}
}

func TestPairDeliverySkillFreshEvidenceCompletionGate(t *testing.T) {
	text := readPairDeliverySkill(t)
	for _, want := range []string{
		"Fresh-evidence completion gate",
		"No completion claim without fresh evidence run in this message",
		"after the final edit",
		"Check the VCS diff yourself",
		"completion receipt is not a substitute for fresh evidence",
		"Never claim completion, publish a decision, or ask to close without fresh verification evidence from the current turn",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pair-delivery skill lost fresh-evidence completion guardrail %q", want)
		}
	}
}

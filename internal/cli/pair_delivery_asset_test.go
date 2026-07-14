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

func readPairDeliveryContinuationReference(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "assets", "skills", "pair-delivery", "references", "continuation-and-recovery.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAutopilotRoutesResumeDetailOnDemand(t *testing.T) {
	skill := readBorrowedContractFile(t, "assets", "skills", "autopilot", "SKILL.md")
	for _, want := range []string{
		"references/resume-and-recovery.md",
		"only when",
		"missing or inconsistent state",
		"Do not load it for a fresh missing/`done` run",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("autopilot skill lost conditional resume route %q", want)
		}
	}
	for _, moved := range []string{
		"oma state list autopilot --json",
		"--expected-revision <n>",
		"recoverable corrupt workflow state",
	} {
		if strings.Contains(skill, moved) {
			t.Fatalf("autopilot skill re-inlined on-demand resume detail %q", moved)
		}
	}

	ref := readBorrowedContractFile(t, "assets", "skills", "autopilot", "references", "resume-and-recovery.md")
	for _, want := range []string{
		"oma state list autopilot --json",
		"ask which namespace to resume rather than guessing",
		"recoverable corrupt workflow state",
		"--expected-revision <n>",
	} {
		if !strings.Contains(ref, want) {
			t.Fatalf("autopilot resume reference lost recovery guardrail %q", want)
		}
	}
	if strings.Contains(ref, "](") {
		t.Fatal("autopilot resume reference must remain one hop from SKILL.md")
	}
}

func TestPairDeliverySkillRoutesContinuationDetailOnDemand(t *testing.T) {
	skill := readPairDeliverySkill(t)
	for _, want := range []string{
		"references/continuation-and-recovery.md",
		"only when",
		"hook wiring or trust is unclear",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("pair-delivery skill lost conditional continuation route %q", want)
		}
	}
	for _, moved := range []string{
		"oma relay wait --timeout 3600",
		"~/.codex/hooks.json",
		"oma doctor relay --clean-stale",
	} {
		if strings.Contains(skill, moved) {
			t.Fatalf("pair-delivery skill re-inlined on-demand continuation detail %q", moved)
		}
	}

	text := readPairDeliveryContinuationReference(t)
	for _, want := range []string{
		"the Stop hook is the main self-continuation path",
		"dispatcher is wired in",
		"trusted through `/hooks`",
		"held or re-polled `oma relay wait` only when hook wiring/trust is unavailable",
		"oma doctor relay --clean-stale",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pair-delivery continuation reference lost host/recovery guardrail %q", want)
		}
	}
	if strings.Contains(text, "](") {
		t.Fatal("pair-delivery continuation reference must remain one hop from SKILL.md")
	}
}

func TestPairDeliverySkillReviewerAntiPrejudgingContract(t *testing.T) {
	text := readPairDeliverySkill(t)
	for _, want := range []string{
		"Reviewer contract (anti-prejudging)",
		"read-only review of the checkout",
		"three explicitly separate sections",
		"Spec compliance",
		"Standards & quality",
		"Verdict",
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

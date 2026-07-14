package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readBorrowedContractFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// These fixtures protect the repository's explicit judgment contracts. They do
// not claim to predict or score model behavior.
func TestBorrowedSkillContractsRemainExplicit(t *testing.T) {
	cases := []struct {
		name  string
		path  []string
		wants []string
	}{
		{
			name: "local self-review",
			path: []string{"CONTRIBUTING.md"},
			wants: []string{
				"four explicit headings",
				"Spec compliance",
				"Standards & quality",
				"Verification",
				"Limitations",
				"self-review, not an independent review",
				"Never describe same-host assistance as cross-reviewed",
			},
		},
		{
			name: "pair delivery remains optional",
			path: []string{"CONTRIBUTING.md"},
			wants: []string{
				"optional rather than a prerequisite",
				"If no independent peer host is available, continue",
				"do not block ordinary local work",
			},
		},
		{
			name: "missing peer cannot block repository work",
			path: []string{"AGENTS.md"},
			wants: []string{
				"Cross-review is optional and risk-based",
				"A missing peer host must not block ordinary local work",
				"Never present same-host assistance as independent cross-host evidence",
			},
		},
		{
			name: "deep interview decision choices",
			path: []string{"assets", "skills", "deep-interview", "SKILL.md"},
			wants: []string{
				"2–4 concrete choices plus free text",
				"Mark exactly one `Recommended` only when inspected evidence favors it",
				"No reliable default",
				"The user's answer — not the recommendation — is the decision",
			},
		},
		{
			name: "deep interview prototype evidence lane",
			path: []string{"assets", "skills", "deep-interview", "SKILL.md"},
			wants: []string{
				"experiential uncertainty",
				"could change a CRITICAL axis",
				"offer available `$prototype` as a bounded handoff",
				"[from-prototype]",
				"if unavailable or declined, stay in the interview",
				"Keep the crystallized spec durable",
			},
		},
		{
			name: "deep interview research value gate",
			path: []string{"assets", "skills", "deep-interview", "SKILL.md"},
			wants: []string{
				"Research only decision-bearing gaps",
				"could change a CRITICAL axis or eliminate that user-owned question",
				"leave non-decision-bearing background out of the interview",
			},
		},
		{
			name: "trace red loop boundary",
			path: []string{"assets", "skills", "trace", "SKILL.md"},
			wants: []string{
				"Entry gate: tight red-capable loop",
				"Missing or unconfirmed loop",
				"reproduction/minimization plan",
				"This gate does not apply to configuration or routing behavior, architecture or postmortem questions",
			},
		},
		{
			name: "autopilot test seam",
			path: []string{"assets", "skills", "autopilot", "SKILL.md"},
			wants: []string{
				"Test seam",
				"establish RED first",
				"turns it GREEN",
				"no meaningful automated seam exists",
				"self-review under four headings",
			},
		},
		{
			name: "autopilot premise verification",
			path: []string{"assets", "skills", "autopilot", "SKILL.md"},
			wants: []string{
				"confirm its premise with effort proportional to uncertainty and impact",
				"requested capability already exists",
				"use `$trace` for hard causality when available",
				"otherwise use the smallest safe probe",
				"actually delivers what it claims",
				"explicit rejected or deferred decisions",
				"this is not a universal research phase",
			},
		},
		{
			name: "pair review axes",
			path: []string{"assets", "skills", "pair-delivery", "SKILL.md"},
			wants: []string{
				"three explicitly separate sections",
				"Spec compliance",
				"Standards & quality",
				"Verdict",
			},
		},
		{
			name: "on-demand code review boundary",
			path: []string{"assets", "skills", "code-review", "SKILL.md"},
			wants: []string{
				"read-only and terminal",
				"## Establish the target",
				"do not edit files",
				"Spec compliance",
				"Standards & quality",
				"precise `file:line`",
				"## Verification",
				"## Limitations",
				"self-review, never independent review",
				"does not satisfy a `pair-delivery` gate",
				"Do not start or publish to an `oma relay`",
			},
		},
		{
			name: "code review target drift",
			path: []string{"assets", "skills", "code-review", "SKILL.md"},
			wants: []string{
				"lightweight review baseline",
				"exact file set in scope",
				"diff/snapshot identity",
				"Immediately before the final report",
				"re-review the changed material",
				"do not claim that material was reviewed",
			},
		},
		{
			name: "prototype evidence contract",
			path: []string{"assets", "skills", "prototype", "SKILL.md"},
			wants: []string{
				"one unresolved design question",
				"Logic / state prototype",
				"UI / interaction prototype",
				"Construct a discriminating condition",
				"plausible in the intended production context",
				"## Assumptions and discriminating condition",
				"Provide one run command",
				"Keep data in memory by default",
				"## Production disposition",
				"silently relabeling prototype code as production code",
			},
		},
		{
			name: "cleanup architecture boundary",
			path: []string{"assets", "skills", "ai-slop-cleaner", "SKILL.md"},
			wants: []string{
				"hand it off as architecture work",
				"improve locality",
				"Apply the deletion test",
				"same interface callers use",
			},
		},
		{
			name: "research persistence handoff",
			path: []string{"assets", "skills", "best-practice-research", "SKILL.md"},
			wants: []string{
				"Persistence Handoff",
				"no persistence warranted",
				"current guidance",
				"dated snapshot",
				"refresh trigger",
				"this research skill never writes the file",
			},
		},
		{
			name: "skill authoring discipline",
			path: []string{"assets", "skills", "skillify", "SKILL.md"},
			wants: []string{
				"main path and invariants before exceptions",
				"first words of headings and steps expose the controlling action or branch",
				"one-hop references loaded only when needed",
				"Prune any sentence whose removal changes no trigger, decision, action, artifact, stop, or verifier",
			},
		},
		{
			name: "positive steering authoring contract",
			path: []string{"docs", "skill-authoring.md"},
			wants: []string{
				"Lead with positive steering",
				"Use a prohibition only for a hard boundary",
				"pair it with the concrete alternative or recovery action",
				"Desired behavior comes first",
			},
		},
		{
			name: "proportionate skill validation boundary",
			path: []string{"docs", "skill-authoring.md"},
			wants: []string{
				"A missing live-agent behavior eval does not block a bounded, locally verified judgment-layer improvement",
				"when no efficacy claim is being made",
				"not a universal prerequisite for local skill improvement",
			},
		},
		{
			name: "workflow documentation router",
			path: []string{"README.md"},
			wants: []string{
				"### Which workflow?",
				"This is a documentation router, not another skill",
				"**deep-interview**",
				"**autopilot**",
				"**trace**",
				"**code-review**",
				"**prototype**",
				"**research-mission**",
				"**skillify**",
			},
		},
		{
			name: "commandless judgment skill boundary",
			path: []string{"README.md"},
			wants: []string{
				"Install the `oma` CLI for mechanical workflows",
				"a skill that names an `oma` command stops at that command",
				"Judgment-only on-demand skills with no CLI command",
				"Optional handoffs are best-effort",
				"the parent workflow continues",
			},
		},
		{
			name: "post-approval domain handoff",
			path: []string{"assets", "skills", "deep-interview", "SKILL.md"},
			wants: []string{
				"After approval, you may offer a separate docs handoff",
				"`Term:` / `Decision:` records",
				"approved spec path",
				"`domain-modeling` when available",
				"Never invoke it automatically",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := readBorrowedContractFile(t, tc.path...)
			normalized := strings.Join(strings.Fields(text), " ")
			for _, want := range tc.wants {
				if !strings.Contains(text, want) && !strings.Contains(normalized, strings.Join(strings.Fields(want), " ")) {
					t.Fatalf("%s lost borrowed contract %q", strings.Join(tc.path, "/"), want)
				}
			}
		})
	}
}

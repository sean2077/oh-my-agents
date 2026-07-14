# Borrowing selected methods from Matt Pocock's skills

> **Status: historical — implemented.** This is the decision record for borrowing selected methods from `mattpocock/skills`, not a normative specification. The executable contracts live in the relevant canonical oma skills and authoring guide; contribution policy lives in [`../../AGENTS.md`](../../AGENTS.md) and [`../../CONTRIBUTING.md`](../../CONTRIBUTING.md).
>
> Decision date: 2026-07-14. Upstream pin: [`mattpocock/skills@66898f60e8c744e269f8ce06c2b2b99ce7660d5f`](https://github.com/mattpocock/skills/tree/66898f60e8c744e269f8ce06c2b2b99ce7660d5f).

## Ruler

OMA keeps one canonical implementation per workflow. For this local skill set, the pinned upstream comparison plus inspection of the existing contracts is sufficient design evidence for bounded judgment-layer improvements and one narrow on-demand review skill. They add no command, state machine, or core4 resident surface. Mechanical state and gates stay in `oma`; judgment stays in skills.

The research report compared whole skills. The current repository already contains much of the proposed shape: `deep-interview` asks one question per round and separates brownfield research from user judgment; `trace` ranks evidence and ends at a probe; `autopilot` already owns resumable delivery; `pair-delivery` already separates spec compliance from quality. Duplicating those contracts would add wording without a demonstrated behavior change.

## Pinned upstream inputs

| Upstream method | Pinned source | Decision |
|---|---|---|
| One question, discover facts locally, user owns decisions, recommended answer | [`skills/productivity/grilling/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/productivity/grilling/SKILL.md) | Adopt evidence-backed answer choices without inventing a default, then retain settled terms and decisions in the spec. |
| Establish a tight red-capable loop before debugging | [`skills/engineering/diagnosing-bugs/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/diagnosing-bugs/SKILL.md) | Adopt a scoped red-loop entry gate for bugs, regressions, and performance failures. |
| Vertical tracer-bullet tickets and blocker edges | [`skills/engineering/to-tickets/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/to-tickets/SKILL.md) | Adopt adaptive vertical slices, blocker edges, and per-slice verification. |
| Test-first behavior through a meaningful seam | [`skills/engineering/tdd/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/tdd/SKILL.md) | Put focused RED → GREEN discipline in `autopilot` implementation, not in ralph's stop engine. |
| Standards / Spec review axes and mutable-target awareness | [`skills/engineering/code-review/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/code-review/SKILL.md) | Ship a narrow on-demand, read-only review skill around the four-axis local rubric; pin and recheck a mutable review target; never represent it as cross-review or pair evidence. |
| Deletion test, locality, and interface-as-test-surface | [`skills/engineering/codebase-design/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/codebase-design/SKILL.md) | Add only cleanup-relevant heuristics and an architecture handoff boundary to `ai-slop-cleaner`. |
| Durable cited research notes | [`skills/engineering/research/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/research/SKILL.md) | Return an artifact-ready persistence handoff without letting the read-only research skill write files. |
| Predictability, positive steering, information hierarchy, progressive disclosure, and pruning | [`skills/productivity/writing-great-skills/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/productivity/writing-great-skills/SKILL.md) | Adopt as authoring guidance and `skillify` gates, including desired behavior before exceptional prohibitions, not as another resident skill. |
| Inline context/glossary/ADR maintenance | [`skills/engineering/domain-modeling/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/domain-modeling/SKILL.md) | Keep project-doc mutation separate, while retaining stabilized terminology and decisions inside the interview spec. |
| Runnable throwaway evidence for design uncertainty | [`skills/engineering/prototype/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/prototype/SKILL.md) | Add one on-demand OMA-native prototype lane and let interview hand off only material experience-dependent uncertainty. |
| Verify inherited premises before committing to a workflow | [`skills/engineering/triage/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/triage/SKILL.md) | Add proportional premise verification to `autopilot`, without importing an issue-label state machine or mandatory research. |
| Route by the user's actual outcome | [`skills/engineering/ask-matt/SKILL.md`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/ask-matt/SKILL.md) | Add a compact README decision table instead of a resident router skill. |

This repository paraphrases the methods; it does not copy an upstream skill wholesale.

## Implemented outcome

- [`deep-interview`](../../assets/skills/deep-interview/SKILL.md) now gives every user-owned question 2–4 concrete choices plus free text, makes evidence-backed recommendations without deciding for the user, carries stabilized terminology and decisions into the existing spec artifact, and may offer a separate docs/domain handoff after approval.
- [`trace`](../../assets/skills/trace/SKILL.md) now requires bug/regression/performance investigations to confirm a tight agent-runnable red loop before causal ranking; a missing loop stops at a reproduction/minimization probe, while configuration and architecture questions remain free of a test prerequisite.
- [`autopilot`](../../assets/skills/autopilot/SKILL.md) now gives large plans a concrete tracer-bullet template with observable outcomes, blockers, affected surfaces, test seams, per-slice status, and observed verification evidence; meaningful seams use focused RED → GREEN, while fully specified small edits stay lightweight. Its sequential path resumes recorded work or selects the first ready slice in plan order, while explicit independent work may still run in parallel; it never invents dependencies just to manufacture a unique frontier.
- [`ai-slop-cleaner`](../../assets/skills/ai-slop-cleaner/SKILL.md) now uses the deletion test, locality, and caller-facing test surfaces to judge simplification, and stops when cleanup would become architecture design.
- [`best-practice-research`](../../assets/skills/best-practice-research/SKILL.md) now decides whether findings deserve an artifact-ready persistence handoff, including lifecycle and refresh context, without writing the file itself.
- [`skillify`](../../assets/skills/skillify/SKILL.md) and [`docs/skill-authoring.md`](../skill-authoring.md) now require action-bearing leading words, main-path-first information hierarchy, one-hop progressive disclosure, and deletion of wording with no observable behavioral effect.
- Every shipped skill description is now pure trigger metadata beginning exactly with `Use when`; the release fixture checks that shape and each active description's manifest budget instead of leaving the authoring rule as prose.
- [`autopilot`](../../assets/skills/autopilot/SKILL.md) and [`pair-delivery`](../../assets/skills/pair-delivery/SKILL.md) keep their complete normal path in `SKILL.md` while moving rare resume/concurrency/corrupt-state and host-loop/recovery detail into branch-loaded, one-hop references. At implementation time their default bodies fell by 294 and 642 `approx-b4/1` tokens respectively.
- `oma asset audit` now reports loaded `body_tokens` separately from the resident name-plus-description surface and exposes the description/budget pair that drives `OVERSIZED`. The core4 release ceiling is 400 tokens and measured 169 at implementation time; body size remains a review signal rather than a new manifest field or arbitrary hard limit.
- [`deep-interview`](../../assets/skills/deep-interview/SKILL.md) starts a research lane only when its answer can change a CRITICAL axis or eliminate a user-owned question, so background gathering does not become unconditional interview context.
- The same authoring surfaces now lead with desired behavior. A prohibition is reserved for a hard boundary that cannot be expressed positively and names the concrete alternative, so safety/integrity constraints do not crowd out useful action.
- [`prototype`](../../assets/skills/prototype/SKILL.md) is an on-demand evidence lane: one explicit design question, one runnable throwaway artifact, a condition that distinguishes the plausible choices, observable state or meaningful UI variants, and a recorded verdict/disposition. It has no CLI state machine and does not silently become production code.
- [`deep-interview`](../../assets/skills/deep-interview/SKILL.md) may pause on a material experience-dependent unknown and offer an available `prototype` handoff, then resume with `[from-prototype]` evidence; otherwise it stays in the interview. It never makes prototyping a universal gate.
- [`autopilot`](../../assets/skills/autopilot/SKILL.md) proportionately verifies inherited bug, issue, PR, and change premises before planning, while small concrete edits remain lightweight. An unavailable `trace` handoff reduces to the smallest safe probe.
- [`code-review`](../../assets/skills/code-review/SKILL.md) is a narrow on-demand, read-only review lane over an existing diff. It records and rechecks a mutable target, reports Spec compliance, Standards & quality, Verification, and Limitations with precise evidence, but never edits, invokes relay, or counts as independent pair review.
- The README now routes common outcomes to the existing skills. No router skill or resident description was added.
- `pair-delivery` remains available, but cross-review is optional and availability-aware. Without an independent peer, local delivery uses the same four-axis self-review, inline or through `code-review`; neither path can satisfy a pair gate.
- When pair delivery is chosen, its handoff is a compact, self-contained delta from one fixed artifact/commit/spec baseline: next task, locked decisions/non-goals, acceptance and expected validation, reply kind and stop conditions, plus an unbiased independence request only for review. A receiver never has to replay earlier ledger entries just to reconstruct the assignment.
- A small [static contract fixture](../../internal/cli/borrowed_skill_contract_test.go) protects these explicit sections and stop rules from accidental deletion; it does not score or predict model behavior.
- Structural fixtures now protect the durable plan and compact handoff contracts, the required high-level shape of selected complex skills, and the one-hop progressive-disclosure graph. They validate inspectable authoring properties, not exact prose or model efficacy.
- The existing [triggering fixture](../../eval/cases/triggering.jsonl) now covers every active shipped skill, including the `code-review`/`pair-delivery` and `prototype`/simple-ideation boundaries; the release gate rejects catalog additions with no expected trigger case. These labels make future live runs comparable but are not themselves efficacy evidence.
- No change-specific live-agent A/B harness was added or made a prerequisite. The existing optional triggering scorer remains available, but this local adoption makes no statistical or cross-host efficacy claim without real provenance-bearing runs.

## Adoption decisions

### Adopt: `deep-interview`

Offer 2–4 concrete answers to the single question; name a recommended answer and rationale only when inspected evidence supports one; for pure preference, explicitly state that no reliable default exists. The recommendation remains advisory: the user's answer owns the decision. Facts discoverable from the repository remain the agent's responsibility.

Research is decision-bearing, not a default phase. Start a bounded repo or external lane before the first user-answer round only when the result can change a CRITICAL axis or eliminate that user-owned question; record useful findings as evidence and omit background that cannot affect the decision.

Reuse the persisted round answer and the spec's existing Ontology / Decision Boundaries instead of adding a state schema: stabilized terms retain canonical meanings, and locked decisions retain rationale/evidence, owner, and revisit triggers. Do not add a quick mode, a second grilling entry point, or a new CLI/state path. Project glossary/ADR mutation remains excluded because interview is spec-only; its useful knowledge-shaping effect is retained inside the pending-approval spec.

After the user approves the spec, the interview may offer — never auto-start — a separate handoff carrying the relevant `Term:` / `Decision:` records and spec path to an available domain-modeling or user-chosen docs workflow. Interview completion does not depend on that optional promotion.

### Adopt: `trace`

For bugs, regressions, and performance failures, confirm a tight agent-runnable red-capable loop before ranking causes. Use a confirmed loop as tier-1 evidence and the preferred basis for the discriminating probe. If the loop is missing or cannot be confirmed, stop at a reproduction/minimization plan plus one loop-building probe rather than speculating about root cause. Configuration, routing, architecture, postmortem, and other non-code causal questions continue through the normal evidence hierarchy without a failing-test prerequisite. `trace` remains read-only and ends before fixing.

### Adopt: `autopilot`

Large or nonlinear work writes tracer-bullet slices that each leave an observable behavior working, with explicit `Status`, `Blocked by`, `Touches`, `Test seam`, `Verification`, and `Result/Evidence` fields; wide compatibility refactors use expand → migrate → contract. The sequential path continues recorded `in_progress` work or takes the first ready slice in plan order; multiple ready slices are valid and explicit independent work may run in parallel. A failed verifier remains visible and cannot become `done`. For new behavior or a confirmed reproducible bug, a meaningful seam uses focused RED → GREEN; otherwise the plan records why and names an alternative verifier. Ralph remains the stop engine, not the testing-method skill. Small concrete edits remain lightweight. The existing plan file, five phases, and state keys remain the complete orchestration contract; there is no frontier command or state schema.

For inherited bug reports, external issues, existing PRs, and claimed changes, verify the premise before committing to a plan: inspect whether the capability already exists, confirm the reported behavior, validate what a referenced change actually does, and consult explicit rejected/deferred decisions when the repository has them. Scale that check to the uncertainty and consequence; it is not a universal research phase.

### Adopt: prototype evidence

Use `prototype` only when one runnable artifact can settle a material design uncertainty that inspection, research, and conversation cannot. Logic/state questions get a tiny interactive model with visible state; UI/interaction questions get meaningfully different variants switchable in one place. The run must state its assumptions and include a condition under which the plausible choices predict different observations; otherwise its disposition is `rework`, not a comparative verdict. The result records the question, run command, observations, verdict, and production disposition. It is evidence for a later decision, not a hidden production implementation.

`deep-interview` may offer this as a separate user-authorized handoff for a CRITICAL axis and resume from its `[from-prototype]` result. It does not write prototype code itself, and interview completion does not generally depend on a prototype.

### Adopt: local self-review and `code-review`

When no independent peer is available, review the final diff under Spec compliance, Standards & quality, Verification, and Limitations. The on-demand `code-review` skill packages that review-only path for an existing diff without editing or opening a relay. Inline or skill-driven, this remains local self-review: it neither becomes independent evidence nor satisfies `pair-delivery` gates. Peer availability is not a prerequisite for otherwise verified local work, and adopting this skill does not restore mandatory pair delivery.

### Adopt: cleanup architecture heuristics

Keep `ai-slop-cleaner` narrow and behavior-preserving. Use a deletion test to distinguish shallow wrappers from layers that contain real complexity, prefer changes that improve locality, and check preserved behavior through caller-facing interfaces. If simplification requires a new public seam, cross-module ownership change, or new caller knowledge, stop and hand off architecture work instead of redesigning inside cleanup.

### Adopt: research persistence handoff

Keep research terminal and read-only. When findings should outlive the task, return a suggested repository-conventional path, classify the note as current guidance or a dated snapshot, name its refresh trigger, and identify the user-authorized docs workflow that may write it. Otherwise state that no persistence is warranted.

### Adopt: skill authoring, `skillify`, and workflow routing

Keep OMA's canonical `skillify` plus authoring guide. Add Matt's author-review vocabulary — WHEN-only trigger descriptions, leading words, information hierarchy, progressive disclosure, and no-op pruning — as concrete checks, not as a new installed authoring skill. This improves future skill text without adding another trigger or resident description. The release fixture enforces the trigger prefix and manifest description budget; the audit exposes body size so disclosure decisions are visible without mechanizing them.

Lead with the behavior the skill should perform. Reserve negative wording for a hard integrity or scope boundary that cannot be made equally clear as a positive instruction, and pair it with the intended alternative. Keep workflow selection as a README table that routes outcomes to existing skills; a router skill would only add another trigger and context surface.

## Rejected and deferred

Rejected in this adoption:

- Replacing any canonical oma skill with its upstream counterpart.
- Bundling `grilling` or adding another default interview entry.
- Adding an `oma autopilot` command, task-frontier state, schema, or migration layer.
- Making glossary/ADR mutations an inline `deep-interview` side effect.
- Treating local self-review as independent evidence or using it to satisfy a `pair-delivery` gate.
- Making `pair-delivery` mandatory again because an on-demand local review skill exists.
- Adding a second TDD loop or moving testing-method judgment into ralph's stop engine.
- Turning `ai-slop-cleaner` into an architecture scanner or report generator.
- Letting `best-practice-research` write repository files automatically.
- Adding a live-agent A/B harness as a prerequisite for these local wording changes.
- Automatically creating prototypes during interview, persisting prototype state in `oma`, or treating throwaway code as production-ready.
- Importing upstream triage labels, lifecycle state, or a mandatory research phase into `autopilot`.
- Adding a workflow-router skill when a documentation decision table is sufficient.
- Enforcing a universal skill-body ceiling or an exact context snapshot for every wording change; body size remains an audit signal, while trigger budgets and one-hop disclosure are the release contracts.
- Adding more fallback prose, availability state, or dispatch machinery merely to handle an absent optional skill or peer.

Deferred to separate falsifiable work:

- A separate architecture-scan skill, if repeated explicit architecture exploration establishes a distinct trigger.

## Validation boundary

These are judgment-layer workflow changes, not a benchmark claim. Acceptance means the upstream ideas were translated into unambiguous OMA-native decisions and artifacts while preserving existing CLI/state boundaries. Automated checks validate skill structure, commands, budgets, repository mechanics, and the continued presence of the explicit contract text; they neither prove model behavior nor make the absence of deterministic behavior eval a blocker. Dogfood or forward runs may guide later iteration, but any efficacy claim must report its actual host provenance honestly.

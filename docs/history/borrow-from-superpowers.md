# Borrowing from superpowers into oma — the authoring / research vein (plan)

> **Status: historical — implemented / superseded.** This was the planning artifact for borrowing superpowers' skill-authoring wisdom into oma; that work has landed (see [`../skill-authoring.md`](../skill-authoring.md) and the hardened skill contracts). It is kept under `docs/history/` as a design note, **not** a normative spec — the authoritative contracts live in [`../reference/`](../reference/).
> Date: 2026-06-17 · Source: `obra/superpowers` v6.0.1 (`a21956e`).
> Ruler: every candidate is judged against [`../design-philosophy.md`](../design-philosophy.md) — minimal resident footprint (accuracy first, cost second), mechanical-vs-judgment split, agent-neutral default, zero host-config mutation, terminal-state + fail-closed.
> Relation to prior borrow rounds: earlier rounds borrowed omx/omc's **delivery loop** (relay receipts / quality gates / trace / ai-slop-cleaner) and its **research vein** (score keep-policy / research-mission / analyze); superpowers sits on a **third, orthogonal axis** those two never mined.

## 0. Background & core judgment

superpowers is a multi-harness "complete software-development methodology" (14 resident skills + an always-on bootstrap). Read through oma's ruler, **almost none of its delivery architecture is borrowable** — it is the opposite pole on nearly every axis (always-resident bootstrap, host-config auto-wired through the plugin installer, host-specific subagent defaults). That contrast is already documented in `design-philosophy.md §5`.

The transferable value is one superpowers has that oma entirely lacks: **meta-knowledge of how to write skills that agents actually obey** — empirically-tested authoring techniques (Match-the-Form-to-the-Failure, persuasion principles, description=WHEN, rationalization/Red-Flags tables, adversarial subagent skill-testing). This is high-value *precisely because it costs zero resident tokens*: it shapes how a skill is **written**, then drops away. That is the cleanest possible fit with oma's anti-bloat thesis.

**Core judgment: borrow superpowers' writing wisdom; do not borrow its delivery architecture.** A secondary tier hardens the prompt contracts of oma's *existing* skills (reviewer anti-pre-judging, anti-sycophancy review-reception, an Iron-Law verification gate, file-handoff anti-bloat discipline) — all pure markdown, agent-neutral, ~0 resident. A third, optional tier folds three tactics into `trace` and sketches a multi-harness expansion path.

**The irony worth stating:** superpowers itself would fail `oma doctor budget` — its always-on bootstrap + 14 resident skills is exactly the OMC-style context tax oma exists to kill. We take its lessons on *authoring*, not its model of *residence*.

## 1. Decision summary (borrow set — proposed, pending gate-1 review)

**Tier 1 — skill-authoring meta-knowledge → a new doc `docs/skill-authoring.md`. Zero resident cost, zero kernel coupling, highest ROI.**

| # | Borrow | Source (file:line) | Verdict | oma landing form |
|---|---|---|---|---|
| S1 | **Match the Form to the Failure**: prohibitions backfire on *wrong-shape* output (measured worse than no-guidance); use recipes/contracts for shape, prohibitions only for *discipline* failures | `writing-skills/SKILL.md:459-480` ✓ | adopt | keystone section of `skill-authoring.md` |
| S2 | **description = WHEN, not WHAT**: a description that summarizes the workflow makes agents follow it and skip the body (measured: "two reviews" collapsed to one) | `writing-skills/SKILL.md:99,154` ✓ | adopt | authoring doc + a hard rule in `skillify` / manifest authoring |
| S3 | **Persuasion principles + ban Liking/Reciprocity for discipline skills** (anti-sycophancy), per-skill-type combination table | `writing-skills/persuasion-principles.md:7,130` ✓ | adopt | authoring doc (combination table by skill type) |
| S4 | **Rationalization table + Red-Flags/STOP + "letter = spirit" + close-every-loophole** | `test-driven-development/SKILL.md:14,258-288`; `writing-skills/SKILL.md:484-514` | adopt | authoring-doc templates for discipline skills |

**Tier 2 — prompt-contract & gate hardening of EXISTING oma skills. Pure markdown, agent-neutral, ~0 resident.**

| # | Borrow | Source (file:line) | Verdict | oma landing form |
|---|---|---|---|---|
| S5 | **Reviewer contract**: split spec-compliance ✅/❌ vs quality verdict; read-only on checkout; file:line required; **"do not pre-judge the reviewer"** (a prompt containing "don't flag" / "at most Minor" is a stop signal) | `task-reviewer-prompt.md:3-5` + Part 1/2 `:78,:93` (two-verdict split); `:60-62` (rationale never downgrades severity); `subagent-driven-development/SKILL.md:166-173` (controller "don't pre-judge") | adopt | harden `pair-delivery` reviewer contract + the `code-review` command prompt |
| S6 | **Review-reception discipline**: forbid performative/sycophantic replies ("You're absolutely right"); verify-before-implement; **clarify ALL unclear items before acting** | `receiving-code-review/SKILL.md:12,27-38,40-48` | adopt | fold a few lines into `pair-delivery` review-reception step |
| S7 | **Iron-Law verification gate**: no completion claim without fresh evidence run *in this message*; "agent reports success → check the VCS diff yourself" | `verification-before-completion/SKILL.md:16-22` ✓ | adopt | a resident-light shared "deliver-gate" line referenced by autopilot/ralph/pair-delivery; leans on existing `ralph` exit-code + relay receipt. **No new `oma verify` command** (oma has none) |
| S8 | **File-handoff anti-bloat discipline**: "what you paste in / a subagent prints back stays resident and is re-read every later turn — hand artifacts over as files"; deterministic `task-brief` / `review-package` extraction to a collision-free path | `subagent-driven-development/SKILL.md:222-223` ✓ + `…/scripts/` | adapt | this is oma's own thesis applied to handoff; an OPTIONAL CC-subagent branch in `pair-delivery`; the mechanical extraction can be an `oma` helper |

**Tier 3 — trace tactics + multi-harness expansion. Optional.**

| # | Borrow | Source (file:line) | Verdict | oma landing form |
|---|---|---|---|---|
| S9 | **trace tactics**: backward call-stack-to-source; boundary instrumentation (log each boundary, run once, evidence before theory); **"3 fixes failed → question the architecture"** stop-condition | `root-cause-tracing.md:45-64,130-154` (backward tracing); `systematic-debugging/SKILL.md:72-87` (boundary instrumentation), `:192-213` (3-fixes → question architecture) | adapt | fold as tactics + a stop-condition into `trace` |
| S10 | **Action-vocabulary discipline** (skills name *actions*, not tools) + per-harness tool-mapping refs + the Shape A/B/C porting taxonomy | `docs/porting-to-a-new-harness.md:37-296` (rule 2 "never edit the user's files" `:65` ✓) | adapt (docs) | a `docs/porting-*` analog; note Codex/Copilot/Gemini already read `~/.agents/skills/`, so expansion is cheap; bootstrap stays manual (oma's line) |
| S11 | **skillify 4th gate — adversarial efficacy test**: RED (watch a fresh subagent fail *without* the skill, capture rationalizations verbatim) → GREEN → REFACTOR; **no-guidance control**; **variance-as-metric** | `testing-skills-with-subagents.md:30-40` (RED/GREEN/REFACTOR); `writing-skills/SKILL.md:576-585` (micro-test + no-guidance control) | adapt | optional efficacy gate in `skillify` (orthogonal to its existing 3-question *value* gate); mechanical tally driven by `research-mission`/`ralph` |

Legend: ✓ = quote personally verified in-repo this session; unmarked = from the parallel deep-read agents, to be re-verified at gate 1.

## 2. Explicitly excluded / skip (with the ruler's reason)

- **`using-superpowers` resident bootstrap** — an always-resident 1%-trigger spine is the OMC-style context tax oma exists to kill (violates footprint, the doc's §1). Borrow its Red-Flags *style*, never the resident skill.
- **Plugin-manifest auto-loading of a SessionStart hook** (`hooks/hooks*.json`) — superpowers' single crossing of oma's line: it relies on the host plugin loader auto-reading a manifest-declared hook = host-config auto-wiring. oma keeps hook-wiring manual/documented (`design-philosophy.md §3.3`). Skip.
- **subagent-driven-development / dispatching-parallel-agents as DEFAULTS** — host-only (CC subagents) as the default path violates agent-neutral. Keep only as a clearly-marked OPTIONAL accel; the value (prompt contracts, file-handoff) is already captured in S5/S8.
- **`find-polluter.sh`, `condition-based-waiting` as standalone skills** — narrow (JS test-pollution / async waits); not general enough for oma's small catalog.
- **Codex fork-sync rsync, `.version-bump.json` multi-manifest sync** — irrelevant to a Go single-binary; at most steal the `bump --audit` "grep the repo for a stale version string" idea into `semver-release`.
- **Drill eval harness (private submodule)** — concept only (a skill-trigger assertion); folded into S11. No heavy dependency.

## 3. Architecture constraints (verified against current oma)

1. **11 skills today** in `assets/skills/` (ai-slop-cleaner, analyze, autopilot, best-practice-research, deep-interview, pair-delivery, ralph, research-mission, skillify, trace, ultraqa). Adding *resident* skills is expensive — so Tier 1 is a **doc, not a skill**, and Tier 2 enhances existing skills in place. Run `oma doctor budget` after any wave that touches resident surface.
2. **No `oma verify` command exists** (only `internal/relay/receipt.go` `verifyApproveClose`). S7's Iron-Law lands as skill-level discipline + the existing `ralph`/relay receipt mechanics, **not** a new command.
3. **No authoring guide exists** (grep of `assets/`/`docs/` for persuasion/red-flag/behavior-shaping — excluding this planning doc itself — returns nothing) — S1–S4 fill a real gap, not a duplicate.
4. **`git-worktree` is not an oma-shipped skill** — superpowers' worktree/finish tactics are out of scope here.
5. **`skillify` exists** (`assets/skills/skillify/`) and already runs a 3-question value gate — S11 is an *additional, optional* gate, terminal-state, no rewrite of the existing one.
6. **Agent-neutral is non-negotiable**: every Tier-2 contract must read identically for Claude and Codex; any CC-subagent branch (S8) is OPTIONAL and clearly marked.

## 4. Phased implementation plan

### wave SP-1 — `docs/skill-authoring.md` (S1–S4) + skillify description rule
Zero kernel coupling, no binary, no new resident skill. The single highest-ROI item. Authoring doc captures Match-the-Form-to-the-Failure (keystone), description=WHEN, persuasion + anti-sycophancy combination table, and the rationalization/Red-Flags/letter=spirit templates. Add the description=WHEN rule to `skillify` as a hard authoring check. **This is the keystone — propose doing it first and alone.**

### wave SP-2 — prompt-contract & gate hardening (S5, S6, S7, S9)
Pure markdown over existing skills. Harden `pair-delivery`'s reviewer contract (S5) and review-reception step (S6); add the Iron-Law deliver-gate line (S7) referenced from autopilot/ralph/pair-delivery; fold S9 tactics + the "3 fixes → question architecture" stop-condition into `trace`. All ~0 resident, agent-neutral.

### wave SP-3 — optional, has a mechanical part (S11, S8)
S11: an optional efficacy gate in `skillify` — the mechanical tally (compliance rate / variance across N subagent reps, fail-closed if the no-guidance control doesn't fail) is the only candidate for the binary, and can instead ride `research-mission`/`ralph`. S8: a file-handoff helper only if the OPTIONAL CC-subagent branch in `pair-delivery` proves worth it.

### wave SP-4 — future, docs (S10)
A `docs/porting-*` analog encoding action-vocabulary discipline + the Shape A/B/C taxonomy, to cheaply extend oma past Claude+Codex (Codex/Copilot/Gemini already read `~/.agents/skills/`). Bootstrap stays manual.

## 5. Global principles & per-wave acceptance (inherited)

- **Terminal-state**: docs/skills/commands built to final shape; no migration layers.
- **fail-closed**: any new parser/gate rejects unknown input.
- **agent-neutral**: skill default = plain `oma` + markdown; CC accel is an OPTIONAL branch.
- **zero host mutation**: nothing here writes host config; S10 keeps bootstrap manual.
- **budget gate**: after SP-1 (a doc) and any SP-2/3 skill edits, run `oma doctor budget` — the core set's resident footprint must not break threshold. Tier 1 being a doc is *designed* to keep this at zero.
- **per-item delivery**: each wave is its own Codex plan-review + code-review pair. Two gates, not skippable.
- **don't borrow the delivery architecture**: the recurring acceptance check — if a candidate would add resident weight or host-mutation, it is wrong by construction.

## 6. Source index

- **superpowers** (`a21956e`): `skills/writing-skills/{SKILL.md,persuasion-principles.md,testing-skills-with-subagents.md,anthropic-best-practices.md}`; `skills/test-driven-development/{SKILL.md,testing-anti-patterns.md}`; `skills/verification-before-completion/SKILL.md`; `skills/subagent-driven-development/{SKILL.md,task-reviewer-prompt.md,implementer-prompt.md,scripts/}`; `skills/requesting-code-review/{SKILL.md,code-reviewer.md}`; `skills/receiving-code-review/SKILL.md`; `skills/systematic-debugging/{SKILL.md,root-cause-tracing.md,condition-based-waiting.md,defense-in-depth.md}`; `skills/using-superpowers/SKILL.md`; `docs/porting-to-a-new-harness.md`; `CLAUDE.md:104` (Drill).
- **oma verification points**: `assets/skills/` (11 skills); `internal/relay/receipt.go` (no `verify` cmd); `internal/budget/` (`doctor budget`); `assets/skills/skillify/`; `docs/design-philosophy.md` (the ruler).
- **This round's research**: three parallel deep-reads (delivery skills / skill-craft+testing / projection+evals), each pre-filtered through oma's lens; load-bearing quotes (S1/S2/S3/S7/S8 + porting rule 2) personally re-verified in-repo.

---
name: ai-slop-cleaner
description: Regression-safe, deletion-first cleanup of AI code slop — lock behavior with tests, then remove duplication / dead code / needless abstraction / boundary leaks one pass at a time. Completion requires a green verifier (or a user-approved no-test rationale). Usable as ralph's deslop gate.
---

# ai-slop-cleaner

Clean AI-generated code slop — bloated, repetitive, weakly-tested, over-abstracted code that *works* — without drifting scope or changing intended behavior. Deletion-first, regression-safe, one smell at a time.

## When to use

- The user says `deslop` / `anti-slop` / "clean the AI slop".
- Code is noisy, repetitive, over-abstracted, or weakly tested but behaviorally correct.
- A prior implementation left duplicate logic, dead code, wrapper layers, boundary leaks, or thin coverage.
- As a bounded post-implementation cleanup pass (e.g. ralph's deslop step).

## When NOT to use

- It's a new feature / product change, or a broad redesign.
- A generic refactor with no simplification intent.
- Behavior is too unclear to protect with tests or a concrete verification plan — clarify or `trace` first.

## Posture

- Preserve behavior unless the user explicitly asks to change it.
- Lock behavior with focused regression tests FIRST; plan before editing; prefer deletion over addition.
- Reuse existing utilities/patterns before adding any; no new dependencies unless asked.
- Keep diffs small, reversible, smell-focused; inspect → edit → verify → report.

## Workflow

1. **Protect behavior first.** Identify what must stay identical. Add or run the narrowest regression tests that lock it BEFORE editing. If tests genuinely can't come first, write the explicit verification plan before touching code (and see the completion gate — a no-test path needs the user's sign-off).
2. **Plan before code.** Bound the pass to the requested files/area. List the concrete smells to remove. Order safest-deletion → riskier-consolidation.
3. **Classify the slop:**
   - **Duplication** — repeated logic, copy-paste branches, redundant helpers.
   - **Dead code** — unused/unreachable code, stale flags, debug leftovers.
   - **Needless abstraction** — pass-through wrappers, speculative indirection, single-use layers.
   - **Boundary violations** — hidden coupling, misplaced responsibility, wrong-layer imports/side-effects.
   - **Missing tests** — unlocked behavior, weak coverage, edge-case gaps.
   - **Masking fallbacks** — catch-all/default branches that hide failures instead of surfacing them; grep for `quick hack` / `temporary workaround`, swallowed exceptions (bare `except:` / empty `catch {}`), silent default returns, `|| true` (distinguish from *grounded* compatibility fallbacks that are intentional and documented).
4. **One smell-focused pass at a time**, re-verifying between passes: dead-code deletion → duplicate removal → naming & error-handling → test reinforcement. Never bundle unrelated refactors into one edit set.
5. **Quality gates.** Keep regression green; run the relevant lint/typecheck/tests for the touched area; run existing static/security checks. If a gate fails, fix it or back out the risky cleanup — never force it through.
6. **Close with an evidence-dense report:** changed files · simplifications · behavior-lock / verification run (commands + result) · remaining risks.

## Completion gate (do not skip)

A deslop pass is **not done** until either:

- a verifier you actually ran is **green** over the touched area (record the command + exit code — via `oma ralph check` when this runs inside ralph, otherwise report it directly), OR
- the user has **explicitly approved a no-test rationale** for why a verifier cannot apply here.

Cleanup without one of these is a permission slip, not a finished pass — keep going or escalate. Never declare done on "looks cleaner".

## Reviewer-only mode (`--review`)

A reviewer pass after cleanup is drafted, preserving writer/reviewer separation (the same pass must not both write and self-approve high-impact cleanup):

1. Don't edit. Review the cleanup plan, changed files, and regression coverage.
2. Check for: leftover dead code / unused exports; duplication that should have merged; needless wrappers still blurring boundaries; missing or weak tests for preserved behavior; cleanup that changed behavior without intent.
3. Produce a verdict + required follow-ups; hand changes back to a separate writer pass — never fix-and-approve in one step.

## Scope discipline

Can be bounded to an explicit file list or a session's changed files. Preserve the same regression-safe workflow even for a short list. Never silently expand a changed-file scope into broader cleanup unless the user asks.

## Hard rules

1. Behavior is preserved unless the user asks otherwise; tests lock it before edits.
2. Deletion over addition; reuse over new abstraction; no new deps unprompted.
3. One smell per pass; re-verify between passes.
4. Not done without a green verifier or a user-approved no-test rationale.
5. When invoked as ralph's deslop step, do not spawn a nested cleanup or ralph loop — run the pass, record it via `oma ralph check`, and return.

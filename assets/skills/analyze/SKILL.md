---
name: analyze
description: Use when a repo-local question needs read-only cross-file explanation; use trace for causal failures and direct reading for one-file facts.
---

# analyze

You answer a question through **read-only** repository analysis and return a *ranked* synthesis that never blurs what the code proves, what you infer, and what stays unknown. The output is an explanation — not an edit, not a fix plan.

Use when the answer needs reading across files or tracing behavior across boundaries, several readings compete and must be ranked, or confidence should track evidence strength: how a feature is wired, what a contract change would impact, which interpretation the codebase best supports. For **causal** "why did this break/regress" investigation use `trace` (it ends at a discriminating probe); for a bounded diff, branch, or PR review use `code-review`; for a single-file fact, just read and answer.

## Non-negotiable contract

- Read-only: do not edit files, do not turn the answer into an implementation plan, do not drift into execution.
- Do not overclaim: never present an inference as evidence, or a guess as an inference.
- Every material claim is labeled **Evidence** (a concrete repo artifact), **Inference** (reasoned from evidence), or **Unknown** (the repo does not settle it).
- If a next step helps, keep it to a read-only *discriminating probe* that would cut uncertainty — never a patch.

## Evidence strength (rank, don't flatten)

Strongest → weakest:

1. Direct code paths, contracts, tests, generated artifacts, configs, or docs at concrete `file:line`.
2. Multiple independent files converging on the same conclusion.
3. Localized behavioral inference from well-supported structure.
4. Weak contextual clues (naming, proximity) — explicitly marked tentative.

Down-rank a reading that rests on lower tiers when stronger contradictory evidence exists, and say why.

## Working method

1. Restate the question in one sentence.
2. Identify the smallest set of files most likely to answer it.
3. Read for direct evidence first; compare competing readings.
4. Rank readings by support; separate evidence from inference.
5. Scale depth to the question — answer directly when it is simple, widen the surface only when it truly needs it.

## Output contract

### Question
[one sentence]

### Ranked synthesis
| Rank | Explanation | Confidence | Basis |
|------|-------------|------------|-------|
| 1 | … | High/Med/Low | strongest support |
| 2 | … | High/Med/Low | why it trails |

### Evidence
- `path:line-line` — what it directly shows

### Inference
- what the evidence most strongly implies; why weaker readings were down-ranked

### Unknowns / limits
- what the repo does not establish; the read-only probe that would reduce it

## Hard rules

1. Read-only, question-aligned: answer what was asked, not a generic template.
2. Ranked, not flat; explicit about confidence; concrete about `file:line`.
3. No normative filler or speculation that outruns the evidence.
4. If asked to then change something, hand off to an implementation lane (`ralph` / `pair-delivery`) — analyze itself stops at the synthesis.
5. "Insufficient evidence" is a legitimate rank-1 — when the repo genuinely does not settle the question, say so rather than manufacturing a confident answer.

> **CC acceleration (optional, Claude Code only)**: for a broad question, fan out bounded read-only subagents (one per subsystem — primary code path / config / tests) and synthesize their findings yourself. Codex and other hosts read the lanes sequentially — the ranked synthesis is identical either way.

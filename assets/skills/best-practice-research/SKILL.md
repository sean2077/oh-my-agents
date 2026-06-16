---
name: best-practice-research
description: Bounded external best-practice research — official / upstream / standards evidence with source URLs and version/date context, returned as a cited recommendation, then hand off. Terminal, read-only, never implements. Use when a decision needs current external or version-aware guidance; for repo-local facts use analyze.
---

# best-practice-research

You produce a **cited, reusable** best-practice answer from current external evidence — keeping official/upstream sources ahead of third-party summaries — and then hand off. This is a research wrapper, not an implementation lane: it gathers evidence and stops.

Use when correctness depends on current external practice: recommended approach, version-aware behavior, migration/lifecycle rules, API/usage guidance for an already-chosen technology, standards/compliance. For repo-local facts use `analyze`; for causal investigation use `trace`.

## Terminal and read-only

This skill gathers evidence and produces a cited recommendation + handoff, then **stops**. Do not edit files, commit, or run mutating commands — even when the question has obvious implementation implications. When implementation is warranted, hand off (name the next lane) rather than continuing.

## Source-quality rules

- Prefer official documentation, upstream source, release notes, changelogs, standards, and maintainer guidance.
- Include **source URLs** for every material claim.
- State **date / version / release-channel** context for every best-practice claim — guidance rots.
- Label third-party summaries as supplemental; never rank them above official/upstream.
- Flag stale, conflicting, undocumented, or version-mismatched evidence instead of smoothing it over.
- Do not over-fetch: gather the smallest evidence set that supports the decision.

## Working method

1. Classify the question: conceptual best practice / implementation guidance / migration-version / standards-compliance / mixed local+external.
2. Gather repo-local context first (read/grep, or `analyze`) when local usage or constraints shape the answer.
3. Gather external evidence — agent-neutral: use whatever your host provides (web search, official docs, or an MCP docs server such as context7).
4. Synthesize with source quality, version/date context, caveats, and a handoff.
5. Stop when the answer is grounded enough for the caller; otherwise report the exact blocker.

## Output contract

````md
## Best-Practice Research: <question>

### Direct Recommendation
<actionable guidance or decision support>

### Evidence Used
- Official/upstream: <URL> — <what it establishes> (<version/date>)
- Supplemental, if any: <URL> — <why it is secondary>

### Version / Date Context
<versions, dates, release channels, or stated unknowns>

### Repo-Local Context
<facts from analyze/read, or "not needed">

### Boundaries / Non-goals
<what this research does not decide>

### Handoff
<planning → `deep-interview` / `pair-delivery` plan; execution → `ralph` / `research-mission` / `autopilot`; note this skill stops here unless the user switches workflow>
````

## Hard rules

1. Official/upstream before third-party; every material claim carries a source URL.
2. Always state version/date context — undated best-practice is a liability.
3. Terminal: never implement. After the recommendation + handoff, stop; resume only when the user explicitly switches to a named workflow.
4. Do not over-fetch or polish wording that will not change the recommendation.
5. Adopt / replace / compare-dependency questions are surfaced here as *evidence*, never decided here — the choice itself is a planning call; hand it to `deep-interview` / `pair-delivery`.

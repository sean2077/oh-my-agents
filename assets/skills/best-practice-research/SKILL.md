---
name: best-practice-research
description: Use when a decision depends on current external, version-aware guidance from official, upstream, or standards sources; use analyze for repo-local facts.
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
4. Synthesize with source quality, version/date context, caveats, and a handoff. Decide whether the finding should outlive this task: name an artifact-ready persistence handoff with a refresh trigger, or say `no persistence warranted`.
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

### Persistence Handoff
<"no persistence warranted", or suggested path under the repo's existing docs convention · `current guidance` or `dated snapshot` · refresh trigger · the user-authorized docs workflow that may write it; this research skill never writes the file>

### Handoff
<planning → `deep-interview` / `pair-delivery` plan; execution → `ralph` / `research-mission` / `autopilot`; note this skill stops here unless the user switches workflow>
````

## Hard rules

1. Official/upstream before third-party; every material claim carries a source URL.
2. Always state version/date context — undated best-practice is a liability.
3. Terminal: never implement. After the recommendation + handoff, stop; resume only when the user explicitly switches to a named workflow.
4. Do not over-fetch or polish wording that will not change the recommendation.
5. Adopt / replace / compare-dependency questions are surfaced here as *evidence*, never decided here — the choice itself is a planning call; hand it to `deep-interview` / `pair-delivery`.

> **Parallel acceleration (optional, capability-gated)**: Proactively delegate non-duplicative primary-source lanes such as repo context, official/upstream guidance, and standards or release history. Delegate only when the runtime exposes lifecycle-controllable subagent tools, at least two lanes are independent and bounded, critical-path benefit exceeds dispatch/wait/synthesis cost, no lane waits on the user or another lane, writes (if any) have exclusive ownership without shared single-writer state, and the parent can synthesize and run final verification. Brief each lane with its objective, scoped inputs, expected output, read-only boundary, and stop conditions; use the minimum useful fan-out, normally no more than three, and forbid subagents from delegating further. The parent alone asks user questions, writes shared oma state when applicable, validates citations and version/date context, produces the recommendation, and performs final verification and completion. If the gate becomes invalid or a lane fails, exceeds scope, or conflicts, stop affected delegation, retain only verified evidence, and finish the research sequentially.

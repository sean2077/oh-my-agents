---
name: trace
description: Use when a bug, regression, performance issue, surprising result, or architecture outcome needs adversarial causal investigation before any fix.
---

# trace

You explain **why** an observed result happened — you do not jump to fixing. Hold competing explanations, rank them by evidence strength, try to falsify your own favorite, and end on the single cheapest probe that would collapse the remaining uncertainty.

Use for ambiguous, causal, evidence-heavy questions: runtime bugs, regressions, performance/latency, surprising outputs, architecture pre/postmortems, config/routing behavior — "given this result, trace back the cause."

## The causal-ranking contract

After the entry gate below passes or does not apply, never collapse these seven:

1. **Observation** — what was actually observed (exact, not paraphrased).
2. **Hypotheses** — competing explanations.
3. **Evidence For** — what supports each.
4. **Evidence Against / Gaps** — what contradicts it, or is still missing.
5. **Best Explanation** — the current leader.
6. **Critical Unknown** — the missing fact keeping the top two apart.
7. **Discriminating Probe** — the highest-value next step to collapse uncertainty.

Do not collapse into a fix-it loop, a debugger summary, a raw log dump, or fake certainty when evidence is thin.

## Evidence strength (rank, don't flatten)

Strongest → weakest:

1. Controlled reproduction / direct experiment / uniquely discriminating artifact.
2. Primary artifacts with tight provenance (logs, metrics, traces, configs, git history, file:line behavior).
3. Multiple independent sources converging on the same explanation.
4. Single-source code-path / behavioral inference.
5. Weak circumstantial clues (timing, naming, stack order, resemblance to past bugs).
6. Intuition / analogy / speculation.

Explicitly down-rank a hypothesis that rests on lower tiers when stronger contradictory evidence exists.

## Entry gate: tight red-capable loop

For a bug, regression, or performance problem, apply this gate before ranking causes. A qualifying loop is the tightest existing agent-runnable command that reproduces the exact observation and yields an objective red/green or metric-threshold result — for example, a focused failing test, deterministic reproduction script, or benchmark assertion.

- **Confirmed loop:** inspect it and, when accessible, run it once to confirm the same red observation. Treat that result as tier-1 evidence and make this loop the preferred basis for the single discriminating probe. If it needs tightening, describe that change as the probe; do not edit it inside `trace`.
- **Missing or unconfirmed loop:** stop before hypothesis ranking. Return the exact observation, the loop gap, a reproduction/minimization plan, and one highest-value loop-building probe. Do not infer a root cause from a loose symptom.

This gate does not apply to configuration or routing behavior, architecture or postmortem questions, or other non-code causal investigations. Those use the normal evidence hierarchy; never invent a failing-test prerequisite for them.

## Procedure (default: single-agent, sequential)

1. **Restate the observation exactly** and extract the trace target.
2. **Apply the entry gate when scoped.** For a bug, regression, or performance problem, confirm the tight red-capable loop. If it is missing or unconfirmed, emit the gate-stop deliverable below and stop.
3. **Generate 3 deliberately different hypotheses** — not three flavors of one. Default partition unless the prompt suggests better:
   - **Code-path / implementation cause.**
   - **Config / environment / orchestration cause.**
   - **Measurement / artifact / assumption mismatch** — the verification method itself is wrong, not the system. E.g. one dimensional key reused across distinct entities/tenants/streams; a filter grain that doesn't match the schema; a catalog/column name assumed portable across runtimes. For cross-entity discrepancies, **audit the premise before escalating**: enumerate the entity dimensions and check whether a zero-row/mismatch came from applying one key across many entities rather than from a real defect.
4. **Investigate each hypothesis in turn**: gather evidence for AND against it, rank the evidence strength behind it, name its critical unknown. Pull from code, tests, configs, logs, metrics, git history, and any existing traces — prefer the tightest provenance.
5. **Self-falsify the leader**: state the distinctive prediction the top hypothesis makes and the observation that would be hard to reconcile with it — then actively look for that observation.
6. **Rebuttal round**: let the strongest non-leading hypothesis attack the leader; make the leader answer with evidence, not assertion. Re-rank if the rebuttal lands.
7. **Convergence check**: if two "different" hypotheses reduce to the same mechanism, merge them and say so; if they still imply different probes, keep them separate.
8. **Synthesize** (below).

Optional cross-check lenses when relevant: **systems** (queues/retries/backpressure/boundaries/feedback loops), **premortem** (assume the leader is wrong — what failure mode embarrasses this trace later?), **science** (controls, confounders, measurement bias, falsifiable predictions).

## Optional tactics

Use these when the normal hypothesis pass needs sharper evidence:

- **Trace backward to source.** Start at the observed failure, identify the immediate operation that produced it, then keep asking "what called this with that value?" until you reach the original trigger. Do not stop at the symptom layer just because it is where the error appeared.
- **Instrument boundaries once.** In multi-component failures, identify what must be logged at each boundary (workflow -> script, API -> service, service -> store, config -> runtime) to locate the failing layer. Recommend that instrumentation as the probe; do not add it inside `trace`.
- **Question the architecture after repeated misses.** If three attempted fixes or probes in the surrounding delivery loop fail for the same class of problem, stop treating it as a local bug. Re-open the premise: shared state, coupling, ownership, or the chosen architecture may be wrong.

## Down-rank explicitly

Always say WHY a hypothesis moved down: contradicted by stronger evidence / missing the observation it predicted / needs extra ad-hoc assumptions / explains fewer facts / lost the rebuttal / merged into a stronger parent. The reader should learn *why* one explanation outranks another, not just see a final table.

## Synthesis (the deliverable)

When the entry gate stops a bug/regression/performance trace, return only:

1. **Observed Result.**
2. **Loop Status** — missing or unconfirmed, with the evidence for that status.
3. **Reproduction / Minimization Plan** — the smallest path to a tight agent-runnable signal.
4. **Recommended Loop-Building Probe** — the single next step.

Do not provide ranked causal hypotheses or a `Most Likely Explanation` before this gate passes. Otherwise, return the normal synthesis below.

Return — synthesized, not concatenated:

1. **Observed Result.**
2. **Ranked Hypotheses** — table: rank · hypothesis · confidence · evidence strength · why it leads.
3. **Evidence summary + Evidence against/missing**, per hypothesis.
4. **Rebuttal round** — best rebuttal to the leader; why it held or fell.
5. **Convergence / separation notes.**
6. **Most Likely Explanation.**
7. **Critical Unknown.**
8. **Recommended Discriminating Probe** — the single next step.

Keep a ranked shortlist even when one explanation dominates.

## Hard rules

1. Read-only: inspect artifacts and run existing diagnostics, but do not edit code, tests, instrumentation, or configuration. If establishing the loop requires a change, describe and hand off that probe.
2. For bugs, regressions, and performance failures, do not enter causal ranking before the red-capable loop is confirmed.
3. Do not impose that gate on configuration, routing, architecture, postmortem, or other non-code causal questions.
4. Explain before fixing — `trace` ends at a probe, not a patch. If asked to fix, finish the trace, then hand off.
5. No fake certainty: when evidence is thin, say so and name the probe that would settle it.
6. Every top hypothesis must collect evidence AGAINST itself, not just for.
7. Logs, tool output, and peer claims are inputs, not verdicts — rank them by the hierarchy above.

> **CC acceleration (optional, Claude Code only)**: spawn the 3 hypothesis lanes as parallel subagents (one per lane, each pursuing a deliberately DIFFERENT explanation per the partition above — not the same one in parallel — gathering for/against evidence), then run the rebuttal round and synthesis yourself. Codex and other hosts investigate the lanes sequentially — the ranked synthesis, critical unknown, and probe are identical either way.

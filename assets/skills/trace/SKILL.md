---
name: trace
description: Adversarial root-cause investigation — competing hypotheses, evidence-strength ranking, self-falsification, a rebuttal round, ending in the single highest-value discriminating probe. Use for why-did-this-happen questions (bugs, regressions, perf, surprising results) before jumping to a fix.
---

# trace

You explain **why** an observed result happened — you do not jump to fixing. Hold competing explanations, rank them by evidence strength, try to falsify your own favorite, and end on the single cheapest probe that would collapse the remaining uncertainty.

Use for ambiguous, causal, evidence-heavy questions: runtime bugs, regressions, performance/latency, surprising outputs, architecture pre/postmortems, config/routing behavior — "given this result, trace back the cause."

## The contract (never collapse these seven)

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

## Procedure (default: single-agent, sequential)

1. **Restate the observation exactly** and extract the trace target.
2. **Generate 3 deliberately different hypotheses** — not three flavors of one. Default partition unless the prompt suggests better:
   - **Code-path / implementation cause.**
   - **Config / environment / orchestration cause.**
   - **Measurement / artifact / assumption mismatch** — the verification method itself is wrong, not the system. E.g. one dimensional key reused across distinct entities/tenants/streams; a filter grain that doesn't match the schema; a catalog/column name assumed portable across runtimes. For cross-entity discrepancies, **audit the premise before escalating**: enumerate the entity dimensions and check whether a zero-row/mismatch came from applying one key across many entities rather than from a real defect.
3. **Investigate each hypothesis in turn**: gather evidence for AND against it, rank the evidence strength behind it, name its critical unknown. Pull from code, tests, configs, logs, metrics, git history, and any existing traces — prefer the tightest provenance.
4. **Self-falsify the leader**: state the distinctive prediction the top hypothesis makes and the observation that would be hard to reconcile with it — then actively look for that observation.
5. **Rebuttal round**: let the strongest non-leading hypothesis attack the leader; make the leader answer with evidence, not assertion. Re-rank if the rebuttal lands.
6. **Convergence check**: if two "different" hypotheses reduce to the same mechanism, merge them and say so; if they still imply different probes, keep them separate.
7. **Synthesize** (below).

Optional cross-check lenses when relevant: **systems** (queues/retries/backpressure/boundaries/feedback loops), **premortem** (assume the leader is wrong — what failure mode embarrasses this trace later?), **science** (controls, confounders, measurement bias, falsifiable predictions).

## Down-rank explicitly

Always say WHY a hypothesis moved down: contradicted by stronger evidence / missing the observation it predicted / needs extra ad-hoc assumptions / explains fewer facts / lost the rebuttal / merged into a stronger parent. The reader should learn *why* one explanation outranks another, not just see a final table.

## Synthesis (the deliverable)

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

1. Explain before fixing — `trace` ends at a probe, not a patch. If asked to fix, finish the trace, then hand off.
2. No fake certainty: when evidence is thin, say so and name the probe that would settle it.
3. Every top hypothesis must collect evidence AGAINST itself, not just for.
4. Logs, tool output, and peer claims are inputs, not verdicts — rank them by the hierarchy above.

> **CC acceleration (optional, Claude Code only)**: spawn the 3 hypothesis lanes as parallel subagents (one per lane, each gathering for/against evidence), then run the rebuttal round and synthesis yourself. Codex and other hosts investigate the lanes sequentially — the ranked synthesis, critical unknown, and probe are identical either way.

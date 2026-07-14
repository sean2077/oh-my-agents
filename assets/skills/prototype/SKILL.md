---
name: prototype
description: Use when the user explicitly requests a throwaway runnable experiment, state-logic spike, or side-by-side UI variants before production implementation.
---

# prototype

Turn one unresolved design question into the smallest runnable artifact that can answer it. The prototype is disposable evidence for a decision, not an early production implementation.

## Establish the experiment

1. **Confirm authorization.** Start writing only when the user explicitly asks for a prototype. Otherwise, explain that a prototype could reduce the uncertainty and stop before creating files.
2. **State one question.** Write a single falsifiable design question. If several questions remain, choose the one whose answer changes the decision most and defer the others.
3. **State assumptions.** Name the conditions under which the answer is expected to hold, especially dependencies or context the artifact will not model.
4. **Construct a discriminating condition.** Include at least one run where the plausible choices predict different observations, and state why that condition is plausible in the intended production context. If no relevant condition can be exercised, use the `rework` disposition and state the next condition to test instead of claiming a comparative verdict.
5. **Isolate the artifact.** Put it in a clearly labelled temporary surface appropriate to the repository — for example a scratch directory, demo route, sandbox, or user-named location — so its lifecycle remains explicit.

## Choose the prototype type

- **Logic / state prototype:** build a tiny interactive terminal artifact. Show the current state, accepted input, transition taken, and resulting state so the behavior can be inspected directly.
- **UI / interaction prototype:** build several meaningfully different variants and make them switchable in one place. Label the variants and expose the differences being compared; cosmetic copies do not count as alternatives.

Choose the branch that answers the question with less machinery. Do not combine both unless the question genuinely depends on both.

## Build and observe

1. **Provide one run command.** The artifact must start from one documented command; keep setup to the minimum already supported by the repository.
2. **Keep data in memory by default.** Add persistence only when persistence is the question under test or the user explicitly requests it.
3. **Expose the evidence.** Make relevant state, transitions, inputs, outputs, or active UI variant visible during the run.
4. **Skip production polish.** Implement only what distinguishes the choices. Leave out deployment, migration, compatibility layers, telemetry, hardening, and broad test architecture unless one is the design question.
5. **Run the artifact.** Record observed behavior from the actual run. Separate observation from interpretation and name any condition that could not be exercised.

## Close the experiment

Choose an explicit production disposition:

- **discard** — the question is answered and the artifact has no further value;
- **rework** — the experiment was inconclusive and the next question is stated;
- **hand off** — the verdict supports a separate production implementation.

Hand-off means rebuilding the chosen behavior under production contracts. It never means silently relabeling prototype code as production code. If the user wants to retain prototype code, pause and make that lifecycle decision explicit first.

## Output contract

```md
## Question
<one explicit design question>

## Prototype type
<logic/state | UI/interaction>

## Assumptions and discriminating condition
<what the artifact models and omits, the run that separates plausible choices, and why that condition is relevant>

## Entry point / run command
<artifact path and one command>

## Observations
<what the run actually showed, including limits>

## Verdict
<answer to the question and confidence>

## Production disposition
<discard | rework | hand off, with the next action>
```

## Hard rules

1. Answer one question per prototype.
2. Treat the runnable artifact as evidence, not production.
3. Keep the default disposable: isolated, in-memory, and minimally polished.
4. Claim a comparative verdict only from an observed discriminating run; otherwise choose `rework` and name the missing condition.

---
name: ultraqa
description: Adversarial end-to-end QA as a ralph profile — drive oma ralph with a hostile-scenario matrix (malformed input, cancel/resume, injection, boundary, adversarial user) until every scenario behaves safely. Use to harden a feature before shipping, when happy-path passing is not enough.
---

# ultraqa

Adversarial end-to-end QA, run as a **ralph profile**: you drive `oma ralph` with a deliberately hostile scenario matrix and loop until every scenario behaves safely. The CLI owns the loop mechanics (counting, stall detection, stop judgment); you own the adversarial scenario design and run the checks yourself. ultraqa adds **no new engine** — it is `oma ralph` with an adversarial goal.

Use to harden a feature/flow before shipping — when "it works on the happy path" isn't enough.

## Start

Frame the QA bar as verifiable, then start a ralph loop:

```
oma ralph start --goal "all ultraqa scenarios pass: <feature>" --max-rounds 10 --stall-window 3 --id qa-<slug>
```

## The scenario matrix (design before looping)

Build a matrix across these axes — pick the ones that fit the target:

- **Malformed / hostile input** — empty, oversized, wrong-type, injection (SQL / shell / prompt), unicode & encoding edge cases.
- **Lifecycle** — cancel mid-operation, resume after interruption, double-submit, out-of-order calls, concurrency.
- **Boundary** — zero / one / max / max+1, empty collection, missing optional fields, expired or reused tokens.
- **State / environment** — stale cache, partial write, missing dependency, permission denied, network failure / timeout.
- **Adversarial user** — someone trying to break it, escalate privilege, or reach another tenant's data.

Each scenario states: setup → action → **expected safe behavior** (reject / degrade / recover — never crash, corrupt, or leak).

## Each round

1. `oma ralph next --json` — `continue:false` (exit 4) means a terminal state; act on it, don't push on.
2. Run the next batch of scenarios YOURSELF; compare actual vs expected behavior.
3. Fix the defects the round surfaced.
4. Record the result:

   ```
   oma ralph check --verifier-exit <0|nonzero> --note "<failing scenario signature>"
   ```

   exit 0 only when EVERY scenario passes. The `--note` is the stall signature: same scenario failing → same note; a new failure → a new note.

## Terminal states (from ralph)

- **passed** — every scenario behaves safely. Report the matrix + outcomes.
- **exhausted** — budget ran out: list scenarios still failing and the closest-safe state; ask to raise the bound or stop.
- **stalled** — the same scenario keeps failing the same way: the fix approach is wrong; present 2–3 genuinely different strategies.

## Hard rules

1. No new engine — never re-implement counting / stall / stop; that is `oma ralph`.
2. A scenario "passes" only on observed safe behavior, never assumption. You run the checks; you never fake an exit code.
3. Expand the matrix when a defect reveals a missed axis — coverage grows with what you learn.
4. Done = every matrix scenario green, recorded via `oma ralph check`.

---
name: ultraqa
description: Use when a feature or flow needs adversarial end-to-end hardening before shipping because happy-path verification is insufficient.
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
- **Misleading success** — a 0 exit with failure text in the output, a SUCCESS message over a partial write, a swallowed error reported as done. The exit code and the observable result must agree.

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

**Coverage-scored variant (optional)** — for a large matrix where partial progress matters, drive the loop under `--keep-policy score_improvement` and report `--score <fraction safe>` (e.g. 7 of 9 → 0.78) each round instead of a binary pass; ralph keeps the best coverage and reports `plateaued` when fixes stop raising it (see the ralph skill).

## Terminal states (from ralph)

- **passed** — every scenario behaves safely. Report the matrix + outcomes.
- **exhausted** — budget ran out: list scenarios still failing and the closest-safe state; ask to raise the bound or stop.
- **stalled** — the same scenario keeps failing the same way: the fix approach is wrong; present 2–3 genuinely different strategies.
- **plateaued** (coverage-scored runs) — safe-scenario coverage stopped improving for plateau_window rounds: the current fixes are mined out; report best coverage and switch to 2–3 different strategies.

## Hard rules

1. No new engine — never re-implement counting / stall / stop; that is `oma ralph`.
2. A scenario "passes" only on observed safe behavior, never assumption. You run the checks; you never fake an exit code.
3. Expand the matrix when a defect reveals a missed axis — coverage grows with what you learn.
4. Done = every matrix scenario green, recorded via `oma ralph check`.
5. Keep your OWN probes inside safe bounds: bounded timeouts, no destructive / production / secret-exfil actions, no unbounded spawns. You are firing hostile inputs — don't let the harness become the incident.

> **Parallel acceleration (optional, capability-gated)**: Delegate isolated, non-destructive scenario execution from the defined matrix; keep fixes sequential, and let the parent own the matrix and aggregate `oma ralph check`. Gate requires lifecycle-controllable subagent tools; at least two independent bounded lanes whose critical-path benefit beats coordination; no lane waits on user, peer, or unstable input; exclusive file/worktree writes, no generated/shared single-writer state; parent synthesis and final verification. Brief objective, inputs, output, boundary, stop conditions; normally no more than three, no nested delegation. Lane output is evidence, not a verdict. Parent owns questions, shared state, integration, and completion. If gate/lane fails, breaches scope, or conflicts: stop affected lanes, keep verified evidence, continue sequentially.

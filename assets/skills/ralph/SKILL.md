---
name: ralph
description: Use when work must iterate until a verifier passes or a deterministic stop condition is reached, especially for keep-going, loop-on-this, or do-not-stop-until-green requests.
---

# ralph

You iterate on a goal until a verifier proves it done, with the stop judgment solidified in the CLI. The split is strict:

- **You (the agent) do the work AND run the verifier**: oma never executes verifier commands — that is a security boundary, not a convenience. You run the check (tests, build, linter, whatever proves the goal), inspect its output yourself, and report only the exit code plus a failure signature.
- **The CLI counts and judges stop**: rounds, the exhaustion bound, stall detection over failure signatures, terminal-state persistence. Never second-guess a stop verdict by looping anyway.

## Start

```
oma ralph start --goal "<what done means>" --max-rounds 10 --stall-window 3 --id <slug>
```

`--goal` is required and should be verifiable ("go test ./... passes", not "make it better"). The default current workflow session isolates loops inside the shared project `.oma/state/`; omitting `--id` uses that session's default loop, while `--id` creates an explicit loop that must keep using the same id on later commands. Ralph records the worktree and branch where the loop starts and refuses later `next`/`check`/`abort` from another worktree or a switched branch; move it intentionally with `oma ralph rebind-worktree`. Use `oma ralph status --allow-worktree-change` to inspect a loop bound elsewhere read-only. Inspect state after an interruption with `oma ralph status --json`, or `oma ralph status --id <slug> --json` when you supplied an id; resume the loop by continuing the `next → work → check` cycle below.

## Each round

1. Advance and check the verdict:

   ```
   oma ralph next --json
   ```

   `continue: false` (exit 4) means the loop already reached a terminal state — act on it (below), never push on.

2. Work the goal: make the smallest change you believe moves the verifier toward passing.

3. Run the verifier YOURSELF and capture its exit code. Then record it:

   ```
   oma ralph check --verifier-exit <code> --note "<failure signature>" --json
   ```

   The `--note` is the stall detector's input: give the SAME signature for the same failure (first failing test name, error class — e.g. `TestFooBar fails`), a DIFFERENT one when the failure changed, and a non-empty one whenever the exit code is nonzero. Raw log dumps and empty notes both blind the detector. Never report an exit code you did not actually observe.

## Score-gated loops (optional)

When "done" is a quality *score* climbing rather than a binary pass — beating a baseline, optimizing a metric — start under the `score_improvement` keep-policy instead of the default `pass_only`:

```
oma ralph start --goal "<score bar>" --keep-policy score_improvement --plateau-window 3 --id <slug>
```

Then report a finite `--score` (required under this policy) alongside the exit code every round:

```
oma ralph check --verifier-exit <code> --score <n> --json
```

The CLI keeps the strict-best (`best_round`/`best_score`) and stops with the `plateaued` terminal (below) once the score stops improving. For a full optimization scaffold — mission spec, evaluator contract, candidate ledger — use the `research-mission` skill, which wraps this policy.

## Terminal states are instructions, not labels

- **passed** (verifier exit 0): report what was done and how many rounds it took. Stop.
- **exhausted** (round > max_rounds): the budget ran out. Summarize what was attempted per round, name the closest-to-green state, and ask the user: raise the bound, change approach, or stop here.
- **stalled** (stall_window consecutive identical signatures): repeating the same fix is disproven. STOP iterating on the current strategy; present the stuck signature, 2–3 genuinely different strategies, and let the user pick (or pick one yourself only if the user pre-authorized strategy changes).
- **plateaued** (score_improvement only: no strict `best_score` gain for plateau_window rounds): the current approach is mined out — same posture as `stalled`, but the signal is a stuck score, not a repeated failure. Present `best_score`@`best_round`, 2–3 genuinely different strategies, and let the user pick.
- **aborted**: only via `oma ralph abort` on the user's instruction.

`next`/`check` on a terminal loop never advance it: `next` reports the stop idempotently, `check` is refused.

## Hard rules

1. oma never runs your verifier; you never fake its exit code.
2. One `check` per meaningful attempt — recording noise burns the stall window's signal.
3. A stop verdict (exit 4) ends the loop NOW; renegotiating it is the user's call, not yours.
4. The goal text is the contract: if the work drifts somewhere else, abort and restart with the real goal.

> **`/goal` driver (optional, host-native)**: only when the current interactive surface exposes a native `/goal` loop, you may let the host auto-continue rounds instead of re-prompting each one. Availability can vary by host surface, release channel, and experimental configuration; never infer it from a minimum host version. Otherwise keep the ordinary `next → work → check` loop. Keep the stop judgment in oma: phrase the goal so the host's evaluator only has to confirm oma's verdict, e.g. `/goal advance one ralph round each turn until 'oma ralph status --json' reports a terminal state (passed/exhausted/stalled/plateaued); print that JSON every turn`. The round count, exhaustion bound, stall detection and terminal persistence still come from `oma ralph` — deterministic and identical across hosts — and no Stop-hook wiring is needed to keep the loop alive.

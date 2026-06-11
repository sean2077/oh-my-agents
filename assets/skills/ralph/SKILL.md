---
name: ralph
description: Persistent improvement loop with deterministic stop judgment via oma ralph. Use when work should iterate until a verifier passes — user says keep going until tests pass, loop on this, ralph it, don't stop until green.
---

# ralph

You iterate on a goal until a verifier proves it done, with the stop judgment solidified in the CLI. The split is strict:

- **You (the agent) do the work AND run the verifier**: oma never executes verifier commands — that is a security boundary, not a convenience. You run the check (tests, build, linter, whatever proves the goal), inspect its output yourself, and report only the exit code plus a failure signature.
- **The CLI counts and judges stop**: rounds, the exhaustion bound, stall detection over failure signatures, terminal-state persistence. Never second-guess a stop verdict by looping anyway.

## Start

```
oma ralph start --goal "<what done means>" --max-rounds 10 --stall-window 3 --id <slug>
```

`--goal` is required and should be verifiable ("go test ./... passes", not "make it better"). Resume after an interruption with `oma ralph status --json`.

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

## Terminal states are instructions, not labels

- **passed** (verifier exit 0): report what was done and how many rounds it took. Stop.
- **exhausted** (round > max_rounds): the budget ran out. Summarize what was attempted per round, name the closest-to-green state, and ask the user: raise the bound, change approach, or stop here.
- **stalled** (stall_window consecutive identical signatures): repeating the same fix is disproven. STOP iterating on the current strategy; present the stuck signature, 2–3 genuinely different strategies, and let the user pick (or pick one yourself only if the user pre-authorized strategy changes).
- **aborted**: only via `oma ralph abort` on the user's instruction.

`next`/`check` on a terminal loop never advance it: `next` reports the stop idempotently, `check` is refused.

## Hard rules

1. oma never runs your verifier; you never fake its exit code.
2. One `check` per meaningful attempt — recording noise burns the stall window's signal.
3. A stop verdict (exit 4) ends the loop NOW; renegotiating it is the user's call, not yours.
4. The goal text is the contract: if the work drifts somewhere else, abort and restart with the real goal.

> **CC acceleration (optional, Claude Code only)**: long verifier runs may go through a background shell task while you prepare the next change. Codex and other hosts run the verifier in the foreground — the recorded exit codes and signatures are identical either way.

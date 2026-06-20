---
name: research-mission
description: Scaffold a falsifiable research/optimization mission (mission spec + deterministic evaluator contract + candidate ledger) and drive it with ralph's score_improvement keep-policy. Use to beat a baseline score or pass an evaluator under budget — optimize a benchmark, tune params, search for a better solution — not a one-shot fix.
---

# research-mission

You turn an open-ended research/optimization problem into a falsifiable loop: a mission spec, a deterministic evaluator contract, and a candidate ledger — then drive iterations with `oma ralph` under the `score_improvement` keep-policy. The split mirrors ralph: **you do the work and run the evaluator; the CLI counts, tracks the best, and judges the stop.**

Use this when "done" means *beat a baseline / pass an evaluator under a budget* (optimize a benchmark, tune hyperparameters, search for a better solution), not a one-shot fix. For a single pass/fail verifier, use `ralph` directly.

## 1. Scaffold the mission

Write a mission file (`<work>/mission.md`) the loop reasons against:

- **Goal**: one sentence — what "better" means (e.g. "beat the kept baseline score on `<benchmark>` under a fixed budget").
- **Primary targets**: the files the search may change.
- **Success**: the evaluator's pass/score bar, plus invariants (deterministic, budget-respecting).
- **Allowed / forbidden changes**: scope the search (e.g. allow strategy/hyperparameters; forbid new deps, forbid inflating the budget to win).

## 2. Define the evaluator contract

The evaluator is a command YOU run (oma never runs it — security boundary) that prints a single JSON line:

```
{"pass": <bool>, "score": <finite number>}
```

- **command**: deterministic, repo-local, fixed budget. Higher `score` is better (no enforced range).
- **keep_policy**:
  - `score_improvement` — keep the strict-best score; stop when it plateaus. The research default.
  - `pass_only` — only the boolean matters (then prefer plain `ralph`).

Record it as a small contract block (its own `<work>/sandbox.md`, or a fenced block in the mission file) so a resume — or a peer agent — recovers the exact command:

```yaml
evaluator:
  command: "<deterministic repo-local command printing one JSON line>"
  format: json                    # {"pass": bool, "score": finite}
  keep_policy: score_improvement  # or pass_only
  scope: "<files/areas the search may touch>"
```

## 3. Drive it with ralph

```
oma --session current ralph start --keep-policy score_improvement --goal "<mission goal>" --plateau-window 3 --max-rounds <N> --id <slug>
```

This IS the "research profile" — a ralph preset expressed through `--keep-policy score_improvement`, not a separate command. Global `--session current` is the host-session boundary inside the shared project `.oma/state/`; `--id` names the mission within that session. `--max-rounds` bounds the search (the `exhausted` terminal below); set it deliberately, since the default of 10 is often too few for a real optimization run.

Each round:

1. `oma --session current ralph next --json` — `continue: false` (exit 4) means a terminal state; act on it.
2. Form ONE hypothesis and implement the smallest change that tests it.
3. Run the evaluator YOURSELF, parse its JSON, and record it:

   ```
   oma --session current ralph check --verifier-exit <0-if-pass-else-1> --score <score> --json
   ```

   `--score` is required every round under `score_improvement` and must be finite. The CLI keeps the strict-best (`best_round`/`best_score`) and plateaus after `--plateau-window` rounds with no strict improvement.

## 4. Keep a candidate ledger

Append one row per attempt to a skill-owned ledger (`<work>/candidates.md` or the mission file) — NOT oma state, which holds only the deterministic ralph fields:

```
| round | hypothesis | change | pass | score | decision |
```

`decision` is keep (a new strict best) or discard. The ledger is the research narrative; ralph's `best_round`/`best_score` is the machine truth.

## Terminal states

- **passed**: the evaluator's `pass` went true. Report the kept best and stop.
- **plateaued**: `plateau_window` rounds bought no strict improvement — the current approach is mined out. Present `best_score`@`best_round`, 2–3 genuinely different strategies, and let the user pick.
- **exhausted**: the round budget ran out. Report the kept best — `best_score`@`best_round` from `oma --session current ralph status --json` (it earns a receipt) — and ask: raise the bound, change approach, or stop.

## Hard rules

1. oma never runs your evaluator; never report a score you did not observe.
2. The evaluator must be deterministic and budget-bounded — a noisy or budget-inflating evaluator makes `score_improvement` meaningless.
3. One `check` per real attempt; `--score` finite and honest.
4. A stop verdict (exit 4) ends the loop NOW — renegotiating it is the user's call.
5. The candidate ledger lives in the mission/skill files, never in oma state.

> **CC acceleration (optional, Claude Code only)**: a long evaluator run may go through a background shell task while you draft the next hypothesis. Other hosts run it in the foreground — the recorded scores are identical.

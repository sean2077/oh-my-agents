---
name: autopilot
description: 'End-to-end autonomous delivery — clarify, plan, implement, verify, deliver — with resumable phase state in oma state. Use when the user hands over a whole task and wants it driven to done: autopilot this, take it end to end, run with it.'
---

# autopilot

You drive a task from vague request to delivered result through five phases, persisting progress so any interruption resumes cleanly. Autopilot is pure markdown plus `oma state` — there is no `autopilot` command group by design (a recorded decision; do not invent one).

## State namespace and keys (the resume contract)

Autopilot state uses the CLI's default current workflow session scope. For a
normal run, use `oma state` with the logical namespace `autopilot`; the CLI
stores it as `autopilot--s-<session>` in the shared project `.oma/state/`. If
`current` cannot resolve a platform session, set `OMA_SESSION_ID=<slug>` or pass
an explicit `--session <slug>`.

```
oma state set autopilot/phase <clarify|plan|implement|verify|deliver|done>
oma state set autopilot/goal "<one-line goal>"
oma state set autopilot/plan-path <path>
oma state bind-worktree autopilot     # records the current worktree (mechanical guard)
```

Write compound transitions (more than one key at once) with a single
`oma state patch autopilot --set <field=value> ...` so a reader never observes a
half-updated run; add `--expected-revision <n>` (from `get --json`) when a
concurrent writer must not have advanced the file first.

On EVERY session start, probe for an in-flight run — and note that `oma state get` on a missing key exits 3 by design (fail-closed), which here just means "no run yet":

```
oma state get autopilot/phase
```

- **Missing key (exit 3) or `done`** → no active run in that session. For a new authorized task, initialize atomically before any work so no reader can observe a half-initialized run — set goal and phase in one patch, then bind the worktree:
  ```
  oma state patch autopilot --set goal="<one-line goal>" --set phase=clarify
  oma state bind-worktree autopilot
  ```
- **Any other value** → resume from that phase. `autopilot/goal` must exist (read it); the run's bound worktree must match the current one — verify mechanically with `oma state check-worktree autopilot` (exit 3 = wrong worktree; re-bind with `oma state bind-worktree autopilot` only when the user authorizes continuing from another worktree); `autopilot/plan-path` may be absent until either `clarify` records a spec or `plan` produces the file, and must exist from `implement` onward. It always points at the file holding the actionable plan (the spec's plan section is fine, as long as the plan is written there).
- **A key the current phase depends on is missing** (e.g. phase `implement` but no `plan-path`) → that is recoverable corrupt workflow state: tell the user what is inconsistent and how to repair it (re-set the key or restart the phase); never restart from scratch silently.

If no explicit namespace is known on resume, discover candidates with:

```
oma state list autopilot --json
```

Resume automatically only when exactly one listed autopilot namespace has
`data.phase` other than `done`; if several are active inside the same session
scope, ask which namespace to resume rather than guessing.

Set the phase key at each transition, never retroactively.

## Phases

1. **clarify** — judge the request: if it names concrete files, behaviors and acceptance criteria, record the goal and move on. If it is vague, run the deep-interview skill as a BOUNDED subflow: entry = the ambiguous goal, exit = a crystallized spec file approved by the user; record it with `oma state set autopilot/plan-path <spec>`. Never re-enter clarify once a spec exists.
2. **plan** — write the implementation plan (ordered steps, files to touch, risks, verification per step) into a markdown file; record its path and advance to implement in one atomic patch — `oma state patch autopilot --set plan-path=<path> --set phase=implement` — so a reader never sees `implement` without a `plan-path` (the path may be the spec file's plan section). Surface the plan to the user when the task is large or destructive; otherwise proceed.
3. **implement** — work the plan top to bottom. Keep edits small and verifiable; note deviations from the plan in the plan file itself so resume sees reality, not intent.
4. **verify** — run the ralph skill as a BOUNDED subflow when verification is iterative: entry = a verifiable goal ("acceptance tests pass"), exit = ralph's terminal state. `passed` → proceed; `exhausted`/`stalled`/`plateaued` → carry ralph's stop reason to the user instead of silently shipping. One-shot checks (single test run) may skip ralph and just run the verifier directly.
5. **deliver** — summarize what changed, the verification evidence, and anything deferred; set `autopilot/phase` to `done` with `oma state set`. Delivery is a report to the user, not a merge/push decision — those remain theirs unless pre-authorized. For cross-reviewed delivery (a high-impact change, or when the user wants a second agent's sign-off), hand off to the `pair-delivery` skill as a bounded subflow instead of self-approving here — autopilot composes that primitive rather than reinventing review.

Subflows are bounded, never recursive: deep-interview only from clarify, ralph only from verify, pair-delivery only from deliver, and none of them ever starts another autopilot.

## Hard rules

1. Phase state lives in `oma state` — if you did work without updating the phase, fix the state before continuing.
2. Skipping clarify on a vague request is how "that's not what I meant" happens; when in doubt, interview.
3. Verification failures stop the pipeline at verify; never advance to deliver around a red verifier.
4. User escalations interrupt any phase; record where you stopped so resume is exact.
5. The outer loop has no counter — you are it: if `verify` reaches a terminal stop twice on the same goal, stop and report rather than bouncing implement↔verify.

> **CC acceleration (optional, Claude Code only)**: plan mode may host the plan phase, and independent implement steps may fan out to subagents. Codex and other hosts execute the same phases sequentially inline — the state keys and phase contract are identical either way.
>
> **`/goal` driver (optional, host-native)**: when `verify` runs the ralph subflow, a host-native `/goal` (Claude Code ≥2.1.139, Codex ≥0.128.0) may auto-continue its rounds — see ralph's `/goal` note. The phase contract is unchanged: oma still judges stop, and `autopilot/phase` advances past `verify` only on ralph `passed`.

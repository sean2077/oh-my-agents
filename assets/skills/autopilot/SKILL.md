---
name: autopilot
description: End-to-end autonomous delivery — clarify, plan, implement, verify, deliver — with resumable phase state in oma state. Use when the user hands over a whole task and wants it driven to done: autopilot this, take it end to end, run with it.
---

# autopilot

You drive a task from vague request to delivered result through five phases, persisting progress so any interruption resumes cleanly. Autopilot is pure markdown plus `oma state` — there is no `autopilot` command group by design (a recorded decision; do not invent one).

## State keys (the resume contract)

```
oma state set autopilot/phase <clarify|plan|implement|verify|deliver|done>
oma state set autopilot/goal "<one-line goal>"
oma state set autopilot/plan-path <path>
```

On EVERY session start, probe for an in-flight run — and note that `oma state get` on a missing key exits 3 by design (fail-closed), which here just means "no run yet":

```
oma state get autopilot/phase
```

- **Missing key (exit 3) or `done`** → no active run. For a new authorized task, initialize before any work: `oma state set autopilot/goal "<one-line goal>"`, then `oma state set autopilot/phase clarify`.
- **Any other value** → resume from that phase. `autopilot/goal` must exist (read it); `autopilot/plan-path` is phase-dependent — not expected during `clarify`, expected from the moment `plan` has produced the file onward (`implement`/`verify`/`deliver`).
- **A key the current phase depends on is missing** (e.g. phase `implement` but no `plan-path`) → that is recoverable corrupt workflow state: tell the user what is inconsistent and how to repair it (re-set the key or restart the phase); never restart from scratch silently.

Set the phase key at each transition, never retroactively.

## Phases

1. **clarify** — judge the request: if it names concrete files, behaviors and acceptance criteria, record the goal and move on. If it is vague, run the deep-interview skill as a BOUNDED subflow: entry = the ambiguous goal, exit = a crystallized spec file approved by the user; record it with `oma state set autopilot/plan-path <spec>`. Never re-enter clarify once a spec exists.
2. **plan** — write the implementation plan (ordered steps, files to touch, risks, verification per step) into a markdown file; record its path in `autopilot/plan-path` (it may be the spec file's plan section). Surface the plan to the user when the task is large or destructive; otherwise proceed.
3. **implement** — work the plan top to bottom. Keep edits small and verifiable; note deviations from the plan in the plan file itself so resume sees reality, not intent.
4. **verify** — run the ralph skill as a BOUNDED subflow when verification is iterative: entry = a verifiable goal ("acceptance tests pass"), exit = ralph's terminal state. `passed` → proceed; `exhausted`/`stalled` → carry ralph's stop reason to the user instead of silently shipping. One-shot checks (single test run) may skip ralph and just run the verifier directly.
5. **deliver** — summarize what changed, the verification evidence, and anything deferred; set `autopilot/phase done`. Delivery is a report to the user, not a merge/push decision — those remain theirs unless pre-authorized.

Subflows are bounded, never recursive: deep-interview only from clarify, ralph only from verify, and neither of them ever starts another autopilot.

## Hard rules

1. Phase state lives in `oma state` — if you did work without updating the phase, fix the state before continuing.
2. Skipping clarify on a vague request is how "that's not what I meant" happens; when in doubt, interview.
3. Verification failures stop the pipeline at verify; never advance to deliver around a red verifier.
4. User escalations interrupt any phase; record where you stopped so resume is exact.

> **CC acceleration (optional, Claude Code only)**: plan mode may host the plan phase, and independent implement steps may fan out to subagents. Codex and other hosts execute the same phases sequentially inline — the state keys and phase contract are identical either way.

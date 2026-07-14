# Autopilot Resume and Recovery

Load this reference only for unresolved session identity, an existing active
run, missing or inconsistent state, or possible concurrent state writers. A
fresh run whose phase is missing or `done` uses the initialization path in
`SKILL.md` and does not need this file.

## Resolve the session

Autopilot state uses the CLI's default current workflow session scope. The
logical namespace `autopilot` is stored as `autopilot--s-<session>` in the
shared project `.oma/state/`. If `current` cannot resolve a platform session,
set `OMA_SESSION_ID=<slug>` or pass an explicit `--session <slug>`.

If no explicit namespace is known on resume, discover candidates with:

```
oma state list autopilot --json
```

Resume automatically only when exactly one listed autopilot namespace has
`data.phase` other than `done`. If several are active inside the same session
scope, ask which namespace to resume rather than guessing.

## Resume an active run

Read `autopilot/goal`; it must exist. Verify the bound worktree with:

```
oma state check-worktree autopilot
```

Exit 3 means the worktree is wrong. Re-bind with
`oma state bind-worktree autopilot` only when the user authorizes continuing
from another worktree.

`autopilot/plan-path` may be absent until `clarify` records a spec or `plan`
produces the plan. It must exist from `implement` onward and point at the file
holding the actionable plan; a spec's plan section is valid when the plan is
actually written there.

## Repair inconsistent state

Treat a missing key required by the current phase, such as `implement` without
`plan-path`, as recoverable corrupt workflow state. Tell the user exactly what
is inconsistent and repair it by re-setting the missing key or restarting that
phase. Never silently restart the whole run.

## Protect concurrent revisions

When another writer may update the same state, read the current revision with
`get --json` and add `--expected-revision <n>` to the compound
`oma state patch`. A revision mismatch means the other writer advanced first;
reload the state and reconcile before writing again.

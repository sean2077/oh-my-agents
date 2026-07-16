<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# internal/

## Purpose

Deterministic implementation behind the `oma` command surface. Domain packages
own mechanics; `cli` stays a thin adapter.

## Key Files

| path | role |
|---|---|
| `cli/` | Cobra command tree and flag/IO adapters. |
| `asset/`, `agentdir/`, `assetaudit/` | Asset install, projection, ownership, and audit. |
| `interview/`, `ralph/`, `relay/`, `state/`, `workflowstate/` | Persisted workflow engines and relay ledger. |
| `checks/`, `budget/` | Doctor registry, conformance, and context gates. |
| `config/`, `session/`, `projectroot/` | Configuration, session, and repository scope resolution. |
| `atomicfile/`, `jsonmerge/`, `schemaver/` | Durable persistence and compatibility primitives. |
| `update/`, `version/` | Verified self-update and build/schema versions. |

## For AI Agents

- Keep `internal/cli` thin; put testable mechanics in the owning domain package.
- Preserve fail-closed validation, atomic writes, permissions, and unknown-field
  compatibility at persistence boundaries.
- Use temporary homes/project roots in tests; never touch the contributor's
  real `~/.agents`, `~/.config/oma`, or `.oma` state.
- Update the matching document under `docs/reference/` with contract changes.

## Dependencies

Consumes shipped [`../assets/`](../assets/) and fixture data under
[`../testdata/`](../testdata/); contracts are in [`../docs/reference/`](../docs/reference/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

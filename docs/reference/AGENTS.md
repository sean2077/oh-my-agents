<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# docs/reference/

## Purpose

Normative specification for the CLI surface, persisted schemas, workflows,
relay protocol, adapter behavior, configuration, and security boundaries.

## Key Files

| path | role |
|---|---|
| `command-tree.md` | Commands, flags, JSON contracts, and exit codes. |
| `workflows.md` | Interview, ralph, and delivery state machines. |
| `relay-v2-protocol.md` | Pair ledger and hook/statusline protocol. |
| `schemas.md` | Persisted data schemas and compatibility rules. |
| `adapter-conformance.md` | Asset projection, refcheck, budget, and fixture rules. |
| `security-contract.md` | Fail-closed trust and filesystem boundaries. |
| `config.md` | Configuration sources and precedence. |

## For AI Agents

- Change the spec and implementation together; do not document behavior that
  the current code or tests do not enforce.
- Preserve contractual exit codes and schema compatibility unless the change
  explicitly includes the required breaking-version work.
- Link from overview docs rather than copying normative tables elsewhere.

## Dependencies

Implemented under [`../../internal/`](../../internal/) and exercised by Go tests
and fixtures under [`../../testdata/`](../../testdata/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

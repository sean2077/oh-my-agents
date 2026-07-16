<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# cmd/

## Purpose

Executable entry points. `cmd/oma/main.go` wires build metadata and delegates to
the internal CLI package.

## Key Files

| path | role |
|---|---|
| `oma/main.go` | Minimal `oma` process bootstrap. |

## For AI Agents

- Keep entry points thin; flag parsing and behavior belong in `internal/cli` or
  the owning domain package.
- Preserve version/linker-variable wiring used by `Makefile` and release builds.

## Dependencies

Depends on [`../internal/cli/`](../internal/cli/) and
[`../internal/version/`](../internal/version/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

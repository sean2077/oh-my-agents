<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# tools/install/

## Purpose

Internal, offline regression tooling for the stable installers under `scripts/`.

## Key Files

| path | role |
|---|---|
| `test-install.sh` | Exercises local and release-download fail-closed installation paths. |

## For AI Agents

- Keep tests offline, deterministic, and pointed at `scripts/install.sh`.
- Verify refused installs leave the existing binary byte-identical and residue-free.
- Do not move the public installer into this directory.

## Dependencies

Tests [`../../scripts/install.sh`](../../scripts/install.sh) and runs from the
script-check jobs in [`../../.github/workflows/build.yml`](../../.github/workflows/build.yml).

<!-- MANUAL: notes below this line are preserved on regeneration -->

<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# tools/

## Purpose

Internal repository tooling grouped by failure domain. Stable end-user installers
remain under `scripts/`; commands here serve contributors, CI, and releases.

## Key Files

| path | role |
|---|---|
| `tools-manifest.tsv` | Machine-readable command-surface source of truth. |
| `manifest-check.sh` | Reconciles the manifest with files, syntax, and help contracts. |
| `agent/` | Vendored dual-host contributor harness. |
| `release/` | Release construction, tag validation, and changelog helpers. |
| `install/` | Offline installer regression tests. |
| `git-hooks/` | Repository-local Git hook entry points. |

## For AI Agents

- Classify every command in `tools-manifest.tsv`; run `make tooling-check` after
  adding, moving, or deleting a command surface.
- Group commands by audience, state/artifact, and hazard/verification rather
  than by file extension.
- Keep internal moves shim-free only when no external or QA caller owns the old
  path; stable installers remain under `scripts/`.
- Keep installer tests offline and fail-closed: refused installs must leave the
  existing binary byte-identical and residue-free.
- Keep Git hooks agent-neutral. Deterministic harness/tooling drift blocks;
  content-budget findings stay advisory unless explicitly configured to block.
- Treat `agent/` as externally managed and refresh it only through
  `agent-scaffold upgrade`.

## Dependencies

Invoked by [`../Makefile`](../Makefile), CI under [`../.github/`](../.github/),
and the public installers under [`../scripts/`](../scripts/) where noted.

<!-- MANUAL: notes below this line are preserved on regeneration -->

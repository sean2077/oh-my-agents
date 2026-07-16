<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# scripts/

## Purpose

Stable user-facing installers whose repository paths are part of the documented
download and security contract.

## Key Files

| path | role |
|---|---|
| `install.sh`, `install.ps1` | Checksum- and version-verified installers. |

## For AI Agents

- Keep installers fail-closed: never silently build from `main`, skip checksum
  or version verification, or clobber a working install on failure.
- Preserve `scripts/install.sh` and `scripts/install.ps1` as external paths;
  moving either requires a compatibility release and documentation migration.
- Put internal release, smoke-test, and contributor commands under `tools/`.

## Dependencies

Installer behavior follows [`../docs/reference/security-contract.md`](../docs/reference/security-contract.md);
internal verification lives under [`../tools/`](../tools/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

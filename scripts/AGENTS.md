<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# scripts/

## Purpose

User-facing installers, release construction and validation, offline smoke
tests, changelog extraction, and repository-local Git hooks.

## Key Files

| path | role |
|---|---|
| `install.sh`, `install.ps1` | Checksum- and version-verified installers. |
| `build-release.sh` | Cross-platform release asset builder. |
| `validate-release-tag.sh`, `test-release-tag.sh` | SemVer release gate and regression test. |
| `test-install.sh` | Offline verify-before-replace smoke test. |
| `extract-changelog.sh` | Fail-closed release-note extraction. |
| `hooks/pre-commit` | Contributor harness/content guard; not shipped by `oma`. |

## For AI Agents

- Keep installers fail-closed: never silently build from `main`, skip checksum
  or version verification, or clobber a working install on failure.
- Keep Bash scripts `set -euo pipefail` and shellcheck-clean; preserve native
  PowerShell support for Windows.
- Make smoke tests offline and deterministic whenever the production path can
  be exercised with local fixtures.

## Dependencies

Release behavior follows [`../docs/reference/security-contract.md`](../docs/reference/security-contract.md)
and is orchestrated by [`../.github/workflows/`](../.github/workflows/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

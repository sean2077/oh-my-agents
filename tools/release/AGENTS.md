<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# tools/release/

## Purpose

Internal release construction and validation used by `make release` and GitHub
Actions. These commands emit or validate release artifacts but are not public
download entry points.

## Key Files

| path | role |
|---|---|
| `build-release.sh` | Builds six platform binaries, assets bundle, and checksums. |
| `validate-release-tag.sh` | Validates the repository SemVer tag contract. |
| `test-release-tag.sh` | Regression suite for tag classification. |
| `extract-changelog.sh` | Extracts one fail-closed release-notes section. |

## For AI Agents

- Preserve build-once/promote-same-artifacts and fail-closed tag/changelog gates.
- Resolve the repository root two levels above this directory.
- Update Makefile, workflows, changelog guidance, and the tooling manifest with
  any path or command-contract change.

## Dependencies

Called by [`../../Makefile`](../../Makefile) and
[`../../.github/workflows/`](../../.github/workflows/); security behavior is
specified in [`../../docs/reference/security-contract.md`](../../docs/reference/security-contract.md).

<!-- MANUAL: notes below this line are preserved on regeneration -->

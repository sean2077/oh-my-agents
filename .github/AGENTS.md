<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# .github/

## Purpose

GitHub automation and issue-intake contracts for CI, release publishing, and
compatibility reports.

## Key Files

| path | role |
|---|---|
| `workflows/build.yml` | Reusable quality gate and build-once package pipeline. |
| `workflows/ci.yml` | Runs the reusable pipeline for pushes and pull requests. |
| `workflows/release.yml` | Validates tags and promotes verified artifacts to a release. |
| `ISSUE_TEMPLATE/` | Structured bug and compatibility intake. |

## For AI Agents

- Keep `build.yml` the shared gate used by both CI and releases.
- Preserve the build-once/promote-same-artifacts invariant; publishing must not
  rebuild binaries.
- Treat permissions, checksums, provenance, SBOM generation, and draft-first
  publishing as security boundaries.

## Dependencies

Calls stable installers under [`../scripts/`](../scripts/) and internal
automation under [`../tools/`](../tools/); enforces the contracts in
[`../CONTRIBUTING.md`](../CONTRIBUTING.md) and [`../docs/reference/`](../docs/reference/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

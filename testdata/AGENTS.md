<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# testdata/

## Purpose

Checked-in fixtures used to prove cross-host projection and conformance behavior
without requiring both hosts on the test machine.

## Key Files

| path | role |
|---|---|
| `conformance/claude.json` | Claude asset projection cases. |
| `conformance/codex.json` | Codex asset projection cases. |

## For AI Agents

- Keep fixtures deterministic, minimal, and aligned with
  `docs/reference/adapter-conformance.md`.
- Update fixtures and the consuming tests in the same change when target paths,
  asset kinds, or projection semantics change.
- Do not weaken a fixture merely to make a platform-specific regression pass.

## Dependencies

Consumed primarily by tests in [`../internal/asset/`](../internal/asset/) and
specified by [`../docs/reference/adapter-conformance.md`](../docs/reference/adapter-conformance.md).

<!-- MANUAL: notes below this line are preserved on regeneration -->

<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# assets/

## Purpose

Release content installed by `oma asset`; these are product assets, not the
repository-local contributor harness under `.agents/`.

## Key Files

| path | role |
|---|---|
| `skills/` | Shipped agent-neutral skill bodies and `oma-asset/1` manifests. |

## For AI Agents

- Keep every asset manifest, skill body, command reference, target list, and
  budget mutually consistent.
- Update the plugin catalog and triggering fixture when the active skill set
  changes.
- Put judgment in skills and deterministic mechanics in the Go CLI.

## Dependencies

Authoring rules live in [`../docs/skill-authoring.md`](../docs/skill-authoring.md);
projection contracts live in [`../docs/reference/adapter-conformance.md`](../docs/reference/adapter-conformance.md).

<!-- MANUAL: notes below this line are preserved on regeneration -->

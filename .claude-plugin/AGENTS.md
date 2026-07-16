<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# .claude-plugin/

## Purpose

Catalog metadata used by compatible skill installers to expose the shipped
`oh-my-agents` skill group.

## Key Files

| path | role |
|---|---|
| `plugin.json` | Plugin identity and explicit list of shipped skill paths. |

## For AI Agents

- Keep the skill list aligned with active directories under `assets/skills/`.
- Add or remove entries in the same change as the corresponding product asset.
- Do not point this manifest at repo-local `.agents/skills/` contributor assets.

## Dependencies

The listed assets follow [`../docs/skill-authoring.md`](../docs/skill-authoring.md)
and are gated by the asset-contract tests under `internal/cli`.

<!-- MANUAL: notes below this line are preserved on regeneration -->

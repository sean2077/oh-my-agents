<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# .codex/

## Purpose

Codex project configuration, hooks, and generated subagent projections for the
shared contributor harness.

## Key Files

| path | role |
|---|---|
| `config.toml` | Project-scoped Codex settings and trust reminder. |
| `hooks.json` | Managed project hook wiring. |
| `agents/` | Generated Codex subagent projections. |

## For AI Agents

- Never hand-edit generated files under `agents/`; change `.agents/subagents/`
  and regenerate both hosts.
- Preserve user-owned hook entries when reconciling `hooks.json`.
- Project settings load only after this repository is trusted by Codex.

## Dependencies

Sources live in [`../.agents/`](../.agents/); hook implementations live in
[`../tools/agent/hooks/`](../tools/agent/hooks/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# .claude/

## Purpose

Claude Code projection and project hook configuration for the shared contributor
harness.

## Key Files

| path | role |
|---|---|
| `settings.json` | Project hooks reconciled by `agent-scaffold`. |
| `agents/` | Generated Claude subagent projections. |
| `skills/` | Real symlinks to repo-local `.agents/skills/` sources. |

## For AI Agents

- Never hand-edit files under `agents/` or project-owned links under `skills/`.
- Preserve user-owned hook entries when changing `settings.json`; rerun the
  scaffold installer to reconcile managed entries.
- Put personal overrides in ignored `settings.local.json`.

## Dependencies

Sources live in [`../.agents/`](../.agents/); hook implementations live in
[`../tools/agent/hooks/`](../tools/agent/hooks/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

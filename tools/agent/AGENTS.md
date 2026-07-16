<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# tools/agent/

## Purpose

Vendored contributor-harness mechanics installed by the `agent-scaffold` skill;
these tools are not part of the shipped `oma` binary or release asset bundle.

## Key Files

| path | role |
|---|---|
| `worktree.sh` | Worktree lifecycle and trunk integration. |
| `generate-subagents.py` | Shared-source Claude/Codex subagent generator. |
| `hooks/trunk_edit_guard.sh` | Blocks tracked-file edits on trunk. |
| `hooks/format_on_edit.sh` | Advisory format dispatcher. |
| `hooks/authority_doc_budget.sh` | Advisory `AGENTS.md` budget check. |

## For AI Agents

- Refresh these files with `agent-scaffold upgrade`; do not maintain a local
  behavioral fork of the vendored templates.
- Keep shell and Python files LF-only so Git Bash and CI execute identical bytes.
- Preserve the `tools/agent/` path: hook configs and the upstream scaffold own it.

## Dependencies

Wired by [`../../.claude/settings.json`](../../.claude/settings.json) and
[`../../.codex/hooks.json`](../../.codex/hooks.json); sources and projections
live in [`../../.agents/`](../../.agents/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

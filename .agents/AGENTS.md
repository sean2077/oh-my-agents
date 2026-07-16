<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# .agents/

## Purpose

Repository-local source of truth for the dual-host contributor harness. This is
separate from the product skills shipped from [`../assets/skills/`](../assets/skills/).

## Key Files

| path | role |
|---|---|
| `relink-skills.sh` | Projects repo-local skills into Claude Code with real symlinks. |
| `symlink-manager.py` | Creates and verifies the root and skill-link contracts. |
| `skills/README.md` | Authoring contract for repo-local contributor skills. |
| `subagents/README.md` | Source schema and projection workflow for shared subagents. |

## For AI Agents

- Edit repo-local skills and subagent sources here; never edit their `.claude/`
  or `.codex/` projections directly.
- Keep shipped `oma` skills under `assets/skills/`; do not mirror them here.
- Verify links with `python .agents/symlink-manager.py verify --repo .` and
  subagents with `python tools/agent/generate-subagents.py --check`.
- Refresh vendored harness mechanics through `agent-scaffold upgrade`, not a
  local fork of `relink-skills.sh` or `symlink-manager.py`.

## Dependencies

Projects into [`../.claude/`](../.claude/) and [`../.codex/`](../.codex/) and
shares generators and hooks with [`../tools/agent/`](../tools/agent/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

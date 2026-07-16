<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# tools/git-hooks/

## Purpose

Repository-local Git hook entry points enabled explicitly with `make hooks`.

## Key Files

| path | role |
|---|---|
| `pre-commit` | Harness, tooling-manifest, and authority-document guard. |

## For AI Agents

- Keep hooks agent-neutral: they run for humans, Claude Code, and Codex alike.
- Hard-fail deterministic harness/tooling drift; keep content-budget behavior
  advisory unless `OMA_CONTENT_BUDGET_BLOCK=1` is set.
- Preserve `-h/--help` and unknown-argument exit code 2.

## Dependencies

Configured by [`../../Makefile`](../../Makefile) and delegates to
[`../manifest-check.sh`](../manifest-check.sh), [`../agent/`](../agent/), and
[`../../.agents/`](../../.agents/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

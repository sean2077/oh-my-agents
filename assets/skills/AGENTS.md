<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# assets/skills/

## Purpose

Authoritative source for the agent-neutral skills shipped in the release asset
bundle and advertised by the plugin catalog.

## Key Files

| path | role |
|---|---|
| `<name>/SKILL.md` | Trigger metadata and judgment-only workflow. |
| `<name>/manifest.json` | Asset identity, targets, lifecycle, and token budgets. |
| `<name>/references/` | On-demand detail kept out of resident skill bodies. |

## For AI Agents

- Start descriptions with `Use when`; describe the trigger, not the procedure.
- Shell out to `oma` for counted, validated, or persisted mechanics. A
  commandless skill is valid only when the workflow is judgment-only.
- Keep Claude Code and Codex on one default path; label host-specific
  acceleration explicitly.
- Add every active skill to `.claude-plugin/plugin.json` and to
  `eval/cases/triggering.jsonl`, including its nearest confusing boundary.
- Run the full Go tests because asset structure, references, conformance, and
  budgets are enforced in `internal/cli` and `internal/checks` tests.

## Dependencies

Follow [`../../docs/skill-authoring.md`](../../docs/skill-authoring.md) and
[`../../docs/reference/adapter-conformance.md`](../../docs/reference/adapter-conformance.md).

<!-- MANUAL: notes below this line are preserved on regeneration -->

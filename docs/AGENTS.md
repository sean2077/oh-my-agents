<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# docs/

## Purpose

Architecture, authoring guidance, tutorials, examples, historical rationale, and
the authoritative command/protocol specifications.

## Key Files

| path | role |
|---|---|
| `README.md` | Documentation navigation. |
| `design-philosophy.md` | Context-scarcity and mechanical-vs-judgment rationale. |
| `architecture.md` | Code and asset layout map. |
| `skill-authoring.md` | Shipped-skill authoring contract. |
| `reference/` | Authoritative behavioral specifications. |
| `history/` | Non-normative decision records. |

## For AI Agents

- Update the relevant reference document in the same change as any contract
  change; implementation follows the spec.
- Keep navigation concise and link to one authoritative location instead of
  duplicating rules.
- Treat examples and history as explanatory, never as overrides of `reference/`.

## Dependencies

Describes [`../internal/`](../internal/), [`../assets/`](../assets/), and the
release workflows under [`../.github/`](../.github/).

<!-- MANUAL: notes below this line are preserved on regeneration -->

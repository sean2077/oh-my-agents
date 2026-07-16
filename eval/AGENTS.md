<!-- Parent: ../AGENTS.md -->
<!-- Subordinate to /AGENTS.md — the authoritative agent contract; on conflict /AGENTS.md wins. -->

# eval/

## Purpose

Runnable triggering-evaluation harness for comparing resident skill surfaces
without presenting illustrative fixtures as live-agent evidence.

## Key Files

| path | role |
|---|---|
| `README.md` | Experimental contract and interpretation limits. |
| `cases/triggering.jsonl` | Closed labeled prompt fixture. |
| `run.sh` | Validates inputs and invokes the scorer. |
| `score.sh` | Deterministic Bash/AWK metrics. |
| `results/example-arm-*.tsv` | Illustrative samples, not measurements. |

## For AI Agents

- Keep every active shipped skill represented in the case fixture with a
  relevant near-miss or decoy boundary.
- Preserve provenance for real observations; never quote example TSV values as
  measured results.
- Keep scoring deterministic and shellcheck-clean; live predictions and
  workflow-adherence judgments remain explicitly manual.

## Dependencies

Tests cross-check this catalog from `internal/cli`; the claim and budget model
are documented in [`../docs/design-philosophy.md`](../docs/design-philosophy.md)
and [`../docs/reference/adapter-conformance.md`](../docs/reference/adapter-conformance.md).

<!-- MANUAL: notes below this line are preserved on regeneration -->

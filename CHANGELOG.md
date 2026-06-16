# Changelog

> Release flow: when you decide to release, update this file (the content is the GitHub Release body, written for release-page readers), commit + tag `vX.Y.Z`, and push. CI runs `scripts/extract-changelog.sh` to slice the section whose heading matches the tag into the release notes.
>
> Section heading format: `## vX.Y.Z - YYYY-MM-DD` (CI matches the tag by exact prefix; a tag with no matching section fails the release, fail-closed).

## v0.3.1 - 2026-06-16

A skills-only release: the bundled skill bodies are re-synced to the contracts the binary already ships, and a few concrete templates that earlier compression had dropped are restored. No binary or schema change. Delivered through the project's own relay pair-delivery, double-gated by a second agent.

- **Skills catch up to the binary**: `ralph` and `ultraqa` now surface the `score_improvement` keep-policy (`--score`, `--plateau-window`) and the `plateaued` terminal the binary shipped in v0.3.0; `pair-delivery` documents the fail-closed `oma-review-evidence/1` review block and the "no newer unreviewed work" approve-close rule; `autopilot` fixes a `plan-path` lifecycle inconsistency; `skillify` corrects the `oma asset catalog` invocation.
- **Restored scaffolding**: `research-mission` regains a machine-parseable evaluator-contract block and `--max-rounds`; `deep-interview` gains a compact spec skeleton and a pre-crystallize pressure-pass gate; `ai-slop-cleaner` regains masking-fallback trigger phrases and a recursion guard.
- **Cross-skill polish**: `autopilot` hands cross-reviewed delivery to `pair-delivery`; `deep-interview` adds ambiguity-stall escalation and brownfield term-conflict handling; `trace`, `best-practice-research`, and `analyze` get small correctness/boundary clarifications.

Agent-neutral markdown only; resident footprint unchanged.

## v0.3.0 - 2026-06-16

This release borrows a research/autonomy vein from oh-my-codex and adds a catalog self-audit. Every slice was delivered through the project's own relay pair-delivery, double-gated by a second agent (10 cross-reviews over one ledger).

- **Research loop**: `ralph` gains a `score_improvement` keep-policy (`oma ralph start --keep-policy score_improvement`, `oma ralph check --score`) that keeps the strict-best score and stops on a score plateau, with a falsifiable receipt over the kept best. State schema is now `oma-ralph/2` (no `/1` migration, fail-closed). The new `research-mission` skill scaffolds a falsifiable mission (deterministic evaluator contract + candidate ledger) on top of it.
- **New on-demand skills**: `analyze` (read-only ranked repository analysis with a strict evidence / inference / unknown split), `best-practice-research` (bounded external research with official/upstream sourcing and version/date context), and `research-mission`. Agent-neutral markdown, zero resident cost until installed.
- **Relay review-evidence layer**: artifact schema is now `oma-relay/4` and the completion receipt `oma-completion-receipt/2`. A ready `kind:review` must carry a structured `oma-review-evidence/1` block (findings / basis / commands / limitations with typed refs), validated by verdict and bound by content hash; `relay close --outcome approve` adds an evidence triple-check on top of the existing reviewed-head + non-lead-approve gate. **Breaking**: `/3` ledgers are not read by a `/4` binary (fail-closed, no migration layer) — install the new binary for new pairs.
- **Catalog self-audit**: `oma asset audit` is a new advisory command that flags catalog bloat (orphan / oversized / retire) from deterministic metrics (LOC, resident tokens, ref-count). It never deletes — judgment stays with you.
- **deep-interview**: a small increment — CRITICAL-axis question filtering and a research fan-out before the first user question.

## v0.2.0 - 2026-06-15

This release tightens the install and host-integration contract after the first tagged release.

- **Install path**: the default installer now resolves and downloads the latest prebuilt GitHub Release binary, verifies it against `checksums.txt`, and keeps the source-build path as a fallback or explicit override. Tagged source installs no longer fall back to a `dev` version stamp.
- **Build/release tooling**: the Makefile now has stamped `build`, `install`, `check`, `ci`, and `release` targets, so local binaries report the same version metadata shape as release assets.
- **Host integration**: removed commands that rewrote Claude Code / Codex host config files directly. Hook and statusline setup now ships as discoverable assets plus a manual wiring guide, keeping `oma` out of user-owned host configuration.
- **Project packaging**: added the MIT license to make redistribution and binary installation terms explicit.

## v0.1.0 - 2026-06-12

First tagged release: the complete `oma` CLI and the core skill set, built end-to-end through the project's own cross-reviewed pair-delivery workflow.

- **CLI (`oma`)**: asset install/projection/rollback with a fail-closed security contract; hook fragment injection with a token-exact byte contract; `doctor` diagnostics and the resident-token budget gate; the relay v2 pair ledger (atomic publish, integrity sidecars, sequence reservation, `wait` handoff) with its experience layer — `preflight`, a binding-scoped `statusline`, and auto-continue `hooks` (guarded absolute-path commands with per-event matchers, for both Claude Code and Codex); solidified `interview` and `ralph` workflow surfaces; checksum-verified `self-update`.
- **Skills**: the four core workflow skills (`deep-interview`, `ralph`, `autopilot`, `pair-delivery`), agent-neutral and projected to both Claude Code and Codex. Total resident surface: ~275 tokens. The skills require the `oma` CLI on `PATH`.
- **Tooling**: main-branch install script with Git Bash / Windows `oma.exe` defaults, cross-platform release build script (six platforms + a checksums manifest), changelog-driven release notes, and CI (test matrix + `gofmt`/`vet`/`build` + `golangci-lint`).

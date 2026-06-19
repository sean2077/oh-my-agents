# Changelog

> Release flow: when you decide to release, update this file (the content is the GitHub Release body, written for release-page readers), commit + tag `vX.Y.Z`, and push. CI runs `scripts/extract-changelog.sh` to slice the section whose heading matches the tag into the release notes.
>
> Section heading format: `## vX.Y.Z - YYYY-MM-DD` (CI matches the tag by exact prefix; a tag with no matching section fails the release, fail-closed).

## v0.6.0 - 2026-06-19

This release makes autonomous workflows safer to run side by side in one checkout, and tightens the shipped skill metadata gate that protects resident trigger text.

- **Scoped autopilot state**: the bundled `autopilot` skill now uses a scoped `autopilot-<scope>/...` state namespace for new runs, with the old `autopilot/...` namespace reserved for legacy single-run recovery. This lets the same repository host multiple independent autopilot workflows without project-global state collisions.
- **Workflow state guidance**: the workflow and schema references now spell out the scoped-state rule for concurrent workflows, including resume behavior when more than one autopilot namespace is active.
- **Skill frontmatter hardening**: the frontmatter parser now rejects duplicate top-level keys instead of allowing a later value to shadow `name` or `description`, and the real-assets release gate applies the strict parser to every shipped skill.
- **Version schema registry fix**: `oma version --json` now reports the current `oma-ralph/2` and `oma-relay/4` artifact schemas instead of stale pre-upgrade values.
- **Maintenance docs**: the contributing guide now includes the changelog-first release checklist, and the skill authoring / adapter conformance docs record the supported frontmatter subset.
- **Refcheck cleanup**: the markdown command-reference checker reuses its command separator regexp instead of recompiling it for every scanned line.

## v0.5.2 - 2026-06-19

This is a targeted skills-installer compatibility patch. It restores `autopilot` discovery through `npx skills` without changing CLI behavior or the workflow contract.

- **autopilot installer discovery**: the bundled `autopilot` skill description is now quoted as portable YAML frontmatter, so `npx skills add sean2077/oh-my-agents --skill autopilot` can discover and install it instead of silently skipping the skill.
- **Release gate coverage**: the real-assets release gate now parses every shipped `SKILL.md` frontmatter with the same YAML assumptions expected by external skill installers, preventing malformed metadata from shipping again.

## v0.5.1 - 2026-06-19

This is a skills/docs-only patch release. It tightens the authoring and delivery contracts that sit around the binary, with no CLI behavior or schema change.

- **pair-delivery review discipline**: the bundled skill now makes reviewer independence explicit, separates spec-compliance judgment from the published quality verdict, blocks pre-judged review prompts, and requires file/line-backed findings where repository content is at issue.
- **Review reception and completion gates**: leads must clarify unclear findings, independently verify each review item, record dispositions, and avoid completion claims until fresh verifier evidence and a local VCS diff check have happened in the current turn.
- **Skill authoring guide**: `docs/skill-authoring.md` captures the SP-1 guidance from the Superpowers borrowing plan: match the instruction form to the failure, write `description` as WHEN rather than WHAT, use persuasion deliberately, and close loopholes with rationalization / STOP templates.
- **skillify trigger hygiene**: `skillify` now enforces `description = WHEN, not WHAT` during authoring and models that rule in its own resident description, reducing its resident trigger text while pointing authors to the new guide.

## v0.5.0 - 2026-06-18

This release makes native Windows / Codex Desktop a first-class install target for the CLI + skills workflow. It preserves the canonical `~/.agents/` asset model while using Windows-native projection mechanics where they are available.

- **Windows Codex Desktop install path**: documentation now covers PowerShell-native self-build/install, Git Bash `make install`, and Windows hook command strings, while keeping hook wiring manual and user-owned.
- **Windows skill projection**: directory skills now project by Windows directory junction when available, matching the link-like behavior users get from `npx skills`; managed copy remains the fallback and the file-asset behavior. The install registry records the actual projection kind (`symlink`, `junction`, or `copy`) and verification/removal understands each one.
- **Projection hardening**: managed-copy projection refreshes now use a staged swap instead of remove-then-copy, rollback dry-runs disclose copy refresh paths, and Windows trusted-root checks account for junction/reparse-point targets instead of relying only on `EvalSymlinks`.
- **Cross-platform test cleanup**: POSIX-only safety fixtures are separated from Windows-compatible tests, and Windows projection coverage now includes junction escape refusal plus managed-copy integrity.

## v0.4.0 - 2026-06-17

This release hardens Codex relay self-continuation and keeps the pair-delivery instructions aligned with the real Codex Stop hook path. It also carries the latest documentation organization and Superpowers T3 guidance.

- **Codex Stop-hook continuation**: `oma relay hook Stop` now treats Codex `last_assistant_message` as part of the tolerant Stop payload union, so context, rate-limit, and auth escape valves work for real Codex Stop payloads instead of only `reason` / `stop_reason` shapes.
- **pair-delivery skill contract**: Codex now treats a trusted Stop hook as the main self-continuation path, with held/re-polled `oma relay wait` documented as the fallback when hook wiring or `/hooks` trust is unavailable. The bundled skill has a release gate to prevent regressing that wording.
- **Manual wiring docs**: README and relay protocol docs now make the Codex `/hooks` trust gate explicit for the Stop hook, while preserving the boundary that `oma` documents host config but does not write or trust it automatically.
- **Docs refresh**: project documentation was reorganized and the Superpowers T3 guidance was added.

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

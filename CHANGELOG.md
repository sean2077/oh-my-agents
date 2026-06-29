# Changelog

> Release flow: when you decide to release, update this file (the content is the GitHub Release body, written for release-page readers), commit + tag `vX.Y.Z`, and push. CI runs `scripts/extract-changelog.sh` to slice the section whose heading matches the tag into the release notes.
>
> Section heading format: `## vX.Y.Z - YYYY-MM-DD` (CI matches the tag by exact prefix; a tag with no matching section fails the release, fail-closed).

## v1.3.0 - 2026-06-29

This minor release is a deep optimization pass delivered as a Claude/Codex
paired cross-review (13 reviewed slices). It hardens security and recovery
paths, makes fail-closed errors actionable, broadens the test net, and
reconciles docs with the implementation. It does not change the relay protocol
or persisted workflow schemas, and adds no new public commands or flags.

- **Security and integrity**: projection, canonical-root, and copy-refresh
  writes now re-validate their target immediately before the write, narrowing
  the check-to-write (TOCTOU) windows; new adversarial negative tests cover
  archive extraction (now rejecting Unix-absolute entry paths the same way on
  every OS) and the ledger secret scan.
- **Relay recovery correctness**: `oma relay status` refreshes the caller's own
  heartbeat per the protocol; sequence reservations are fsynced for crash
  durability while keeping their exclusive-create guarantee; `pair join
  --rebind` now cleans the replaced session's orphan reservations (and quarantines
  any incomplete publish) instead of leaving hidden residue; a failed `approve`
  close rolls the pair back to active; an abandoned reclaim election no longer
  blocks the next reclaimer for the full lock lease.
- **Actionable errors**: CLI-authored fail-closed refusals now emit a one-line
  `hint:` naming the recovery action, while preserving exit codes.
- **Configuration and conformance**: relay commands consume the config layer
  (e.g. `relay.stale_after`, including `preflight`); deep-interview stall
  escalation is decided in the binary and surfaced as a verdict; a new
  adapter-conformance check rejects Claude-only constructs on a codex skill's
  default path.
- **Simplification and durability**: inline JSON encoders fold into the shared
  `printJSON`; the asset registry persists through the atomic
  backup-and-write path; dead code and a stale schema-dedup are removed.
- **Documentation accuracy**: the schema registry, secret-scan contract, and
  asset source labels now match the implementation.
- **Test coverage**: focused negative, boundary, concurrency, and
  interruption-matrix tests across atomicfile, relay, state, config, interview,
  budget, and the asset projection conformance harness.

## v1.2.0 - 2026-06-24

This minor release polishes the unified statusline marker and stabilizes the
relay concurrent-draft regression test on slow CI runners. It does not change
the relay protocol or persisted workflow schemas.

- **Statusline identity**: `oma statusline` now renders a colored
  `oma:<workflow>` source tag, while idle keeps the quieter `oma · idle`
  wording. The example status-line script now appends oma's own colored output
  directly instead of stripping and repainting the prefix in host shell code.
- **Relay test stability**: the goroutine-level concurrent draft test now
  retries expected fail-closed lock conflicts and asserts the protocol safety
  invariant that every successful draft receives a distinct sequence number,
  preserving race coverage without depending on slow-runner lock timing.

## v1.1.0 - 2026-06-24

This minor release adds a unified workflow statusline and applies the audited
P0-P3 hardening pass across relay, atomic file locking, state migration,
assets, ralph, interview scoring, and self-update.

- **Unified statusline**: add top-level `oma statusline`, which renders the
  most recently active core workflow (`relay`, `ralph`, `interview`, or
  `autopilot`) with an `oma` marker, JSON output, watch mode, and fail-soft
  status-bar behavior. The former relay-only statusline is superseded by this
  top-level command, and the README, command tree, relay protocol, tutorial,
  and example statusline script now point at it.
- **Relay delivery hardening**: Stop-hook escape detection no longer treats
  ordinary completion prose containing words like `auth`, `context`, or
  `quota` as an escape valve; wait and Stop-hook delivery now split
  "delivered" from "consumed" cursor marks so out-of-order peer artifacts are
  not skipped; session drift is diagnosable through `status` and recoverable
  with explicit `pair join --rebind`.
- **Locking and workflow correctness**: stale atomic-file locks are reclaimed
  by an in-place owner takeover serialized through a sibling `.reclaim`
  election, avoiding the double-holder window from rename-away reclamation;
  ralph now refuses checks before the first round and uses `BestScore == nil`
  as the plateau sentinel; interview stability scoring consumes exact-name
  matches before rename matching.
- **Asset and update security**: canonical asset installs now apply the same
  world-writable ancestor checks as projections, registry entry type is
  cross-checked against the embedded manifest, fetched asset archives extract
  with private `0600`/`0700` permissions, and self-update uses private
  same-directory temp directories while keeping release asset URL validation
  HTTPS- and repo-pinned.
- **State, receipt, and docs repairs**: relay reservation counting matches bare
  sequence reservation names, completion receipt hashing reuses the verified
  sidecar hash, close rollback errors are surfaced, session-scope migration is
  crash-idempotent, and the reference docs now record the ralph receipt schema,
  budget-counting behavior, top-level statusline, and `--rebind` command
  surface.

## v1.0.0 - 2026-06-23

First stable release: `oma` now freezes the CLI, JSON, disk-schema, release-channel, and core4 skill contracts under the compatibility rules in [`STABILITY.md`](STABILITY.md).

- **1.0 compatibility contract**: command names, flags, exit codes, JSON schema evolution, on-disk schema evolution, asset projection, relay protocol behavior, core4 behavior, artifact names, and supported platforms are now governed by semantic versioning.
- **Workflow status stability**: `interview`, `ralph`, and `relay` status JSON now expose a stable `terminal` boolean so consumers can avoid enumerating future state strings.
- **Release channels**: `oma self-update` compares versions by SemVer, refuses downgrades unless explicitly allowed, keeps prereleases out of the default stable channel, and supports `--channel stable|prerelease` plus exact `--version <tag>` pins.
- **Clean-machine asset install**: `oma asset install --ref <tag>` fetches the release asset bundle from this repository's GitHub Release, verifies it against `checksums.txt`, extracts it safely, and defaults to the running binary's own version when no local checkout is supplied.
- **Installer and updater hardening**: Unix and PowerShell installers verify artifacts before replacing a target, preserve rollback behavior on failed replacement, reject duplicate consumed checksum entries, and prepare downloaded artifacts for version probing before install.
- **Release integrity**: release tags pass a strict SemVer gate; release CI runs the same Linux, macOS, and Windows race-test/lint/govulncheck/build matrix as branch CI, verifies just-built artifacts through real installers, uploads to a draft release, downloads the draft assets back, re-verifies checksums and asset counts, then publishes.
- **State compatibility**: persistent state written by `v0.9.x` is directly supported by `v1.0.0`. Existing explicit migrations remain one-shot, fail-closed `oma doctor` flows with backups; no long-lived dual-read compatibility layer is introduced.
- **Host compatibility recorded on 2026-06-23**: Claude Code `2.1.186` and `codex-cli 0.142.0` were available in the maintainer environment used for this release. Host behavior is confined to documented projection paths, hook snippets, and statusline/Stop-hook wiring; host config remains user-owned.

## v0.9.1 - 2026-06-22

This patch release repairs the v0.9.0 publish gate after the Windows installer smoke exposed a CI-only failure. It does not change CLI behavior or shipped workflow schemas.

- **Windows installer smoke**: `scripts/install.ps1` now uses `Net.SecurityProtocolType` when enabling TLS 1.2, keeping the native Windows install smoke compatible with GitHub's PowerShell runner.
- **Release gate ordering**: release packaging now waits for the script lint and installer-smoke jobs, so a tag cannot package assets after an installer gate has failed.
- **Docs**: the reproducible install examples now pin to `v0.9.1`.

## v0.9.0 - 2026-06-22

This release hardens the release pipeline, the installers, and CI after the second external pre-1.0 review, and adds the user-facing stability and security contracts. No CLI behavior or shipped workflow schema changes.

- **Release is gated on full CI**: a reusable `build.yml` runs the 3-platform test matrix (now with `-race`), `golangci-lint`, and `govulncheck`; both `ci.yml` and the tag-triggered release call it. The release **promotes the exact artifacts CI built and verified** instead of rebuilding, and no longer uploads with `--clobber`, so a published asset can never be silently overwritten.
- **Supply chain**: release binaries now carry a build-provenance attestation (verify with `gh attestation verify`) and ship an SBOM.
- **Fail-closed installer**: `scripts/install.sh` never silently builds the unreleased `main` branch — it resolves a release, verifies the checksum, and asserts the installed binary's version, stopping with an actionable error otherwise. A source build is an explicit `OMA_INSTALL_FROM_SOURCE=1` opt-in. A native PowerShell installer (`scripts/install.ps1`) mirrors the same contract; the default install tracks the latest release, with an optional tag-pinned form documented for reproducibility.
- **Security gates in CI**: `go test -race` and `govulncheck` are required checks; the Go toolchain is bumped to 1.25.11 for upstream `net/http` / `crypto/tls` / `crypto/x509` fixes. New fuzz tests cover the relay artifact parser, the secret scanner, and the artifact-name and asset-name validators.
- **Migration coverage**: new tests prove the session-scope and relay migrations back up byte-for-byte, preserve unknown fields, and leave no residue on an interrupted or repeated run.
- **Docs**: new [`STABILITY.md`](STABILITY.md) (the 1.0 compatibility contract), [`SECURITY.md`](SECURITY.md) and issue templates, an end-to-end [`docs/tutorial.md`](docs/tutorial.md), and an [`eval/`](eval/) skill-triggering harness. The "no migration layers" wording is reconciled with the schema-migration policy, and an implemented plan doc moved to `docs/history/`.

## v0.8.2 - 2026-06-22

This is a repo-workflow patch release. It adds an opt-in content-length guard for agent instruction files and tightens the root agent onboarding notes, without changing shipped CLI behavior or workflow schemas.

- **Agent content-length guard**: add an optional repo-local `pre-commit` hook, enabled with `make hooks`, that warns when staged `SKILL.md` / `AGENTS.md` bodies exceed the default 120 non-blank-line budget.
- **CRLF-safe hook counting**: normalize CRLF input before frontmatter and blank-line checks, so blank CRLF body lines do not inflate the content budget count or trigger false block-mode failures.
- **Agent onboarding docs**: add a root `AGENTS.md` entry point for installing oma, wiring local hooks/statusline, setting relay identity, and following the repo's pair-delivery workflow.
- **Relay identity docs**: name `OMA_RELAY_SESSION_ID` explicitly and separate relay author/session identity from workflow-state session scope.

## v0.8.1 - 2026-06-22

This is a release-hygiene patch after v0.8.0 exposed CI-only failures on GitHub's current runners. It does not change the CLI behavior or shipped workflow schemas.

- **Go lint compatibility**: `jsonmerge` now uses `reflect.Pointer` instead of the old `reflect.Ptr` alias, keeping `golangci-lint v2.12.2` / `govet` green under the current Go toolchain.
- **Windows concurrency-test stability**: the real-process state, interview, and ralph concurrency tests retain cross-process lock coverage while reducing Windows worker fanout and retrying expected fail-closed lock contention on slow runners.

## v0.8.0 - 2026-06-22

This release hardens relay delivery, workflow-state migration, and worktree binding after the external P0/P1/P2 review. It also adds a read-only workflow inventory view for multi-session projects.

- **Relay delivery cursor**: `oma relay wait` and Stop-hook delivery now use a per-reader `.cursor/<author>-<session>` consumption cursor, so out-of-order concurrent publishes do not lose a peer artifact just because its sequence number is lower than the reader's latest artifact.
- **v0.7 migration repair tools**: `oma doctor state --migrate-session-scope` dry-runs/applies old `name-session` workflow-state migration into the current `name--s-session` form with backups, while `oma doctor relay --migrate` repairs old relay sessions missing `participant_sessions` so both peers can re-join fail-closed.
- **Worktree-bound operations**: relay pairs now record creator worktree/branch/commit and refuse `relay close` from another worktree unless explicitly allowed; `ralph` now refuses mutating continuation after a worktree or branch switch until `oma ralph rebind-worktree` is run.
- **Autopilot state guard**: generic `oma state bind-worktree` / `check-worktree` gives markdown workflows a mechanical worktree guard, and the bundled `autopilot` skill now initializes goal/phase and plan/phase transitions atomically with `state patch`.
- **Forward-compatible state writes**: state, interview, ralph, and relay session JSON now preserve unknown top-level fields across load/save cycles, honoring the minor-additive schema contract.
- **Workflow inventory**: `oma workflow list [--all-sessions] [--json]` gives a read-only project view over workflow instances across current or all sessions.
- **Release hygiene and coverage**: Go source checkout line endings are pinned to LF for the Windows gofmt gate, staticcheck is clean, and new tests cover crash recovery plus real-process relay publishing concurrency.

## v0.7.0 - 2026-06-20

This release moves parallel workflow isolation into the CLI itself, so agents can run several workflow sessions against one project without each skill inventing its own state layout.

- **Current-session workflow state by default**: `oma state`, `oma interview`, and `oma ralph` now default to the current host session (`OMA_SESSION_ID`, `CODEX_THREAD_ID`, or `CLAUDE_CODE_SESSION_ID`). Missing session identity fails closed instead of falling back to project-global state.
- **Shared project `.oma` across worktrees**: linked git worktrees now resolve workflow and relay state back to the primary project root, keeping one `.oma` tree per repository while still isolating sessions by CLI-managed suffixes.
- **Reusable workflow-state scope helper**: session suffixing for state keys, interview ids, ralph ids, and state listing now lives in shared internal packages instead of being repeated by individual commands or skills.
- **Autopilot state contract**: the bundled `autopilot` skill now records progress through plain `oma state` commands, relying on the CLI's default current-session scope for resumable parallel runs.
- **Pair workflow parallelism clarified**: `pair-delivery` and relay docs now make the intended model explicit: multiple pair workflows are multiple Codex/Claude session pairs, while relay continues to use author-session bindings rather than workflow `--session`.
- **Regression coverage**: tests now cover primary-root resolution from linked worktrees, current-session state isolation, state listing filters, relay ignoring workflow session flags, and two independent pair workflows sharing one ledger root.

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

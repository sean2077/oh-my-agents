# Stability and compatibility

This document is the contract for what you can depend on across `oma` releases.
A `1.0.0` release means **these surfaces are frozen under semantic versioning** —
not that the project has the most features. Until `1.0.0` the same rules are the
*intent*; this file states what becomes a hard promise at `1.0.0`.

`oma` follows [semantic versioning](https://semver.org/):

| Change | Bump |
|---|---|
| Remove/rename a command, flag, exit code, or published JSON field; bump a disk-schema major; drop a supported platform | **MAJOR** |
| Add a command/flag/JSON field/terminal state; additive disk-schema minor; new on-demand skill | **MINOR** |
| Change a shipped skill's trigger semantics, gating, role responsibility, or phase/terminal behavior | **MINOR** |
| Bug fix, doc change, non-behavioral skill wording, dependency bump with no contract change | **PATCH** |

The authoritative details live in [`docs/reference/`](docs/reference/); this file
freezes the *contract*, those files describe the *surface*.

## Frozen at 1.0 (covered by the compatibility promise)

1. **CLI surface** — command group names, flag names, and their meaning, per
   [`docs/reference/command-tree.md`](docs/reference/command-tree.md). New
   commands/flags are additive (minor); removing or renaming one is a major.
2. **Exit codes** — `0` ok, `1` warning, `2` usage error, `3` environment/state
   (fail-closed refusal), `4` gate not passed; `relay wait` adds `10/11/12`.
   These are contractual and parseable by scripts.
3. **JSON output** — every `--json` payload carries a `schema` field (e.g.
   `oma-cli/1`). Published fields are stable; new fields are additive and
   backward-compatible; removing/renaming a field is a major. Adding a
   terminal/phase state string is a **minor**, so the contract makes it *safe*:
   every workflow `status --json` document exposes a stable `terminal` boolean,
   and consumers must treat the `phase`/`status` string as opaque and tolerate
   unknown values (key done/continue logic on `terminal`, never an exhaustive
   switch). 1.x may *add* a terminal state but never repurposes an existing one.
4. **On-disk schemas** — the persisted formats and their evolution policy in
   [`docs/reference/schemas.md`](docs/reference/schemas.md). The shipped majors
   are: `oma-registry/1`, `oma-state/1`, `oma-relay/2` (artifacts `oma-relay/4`,
   binding `oma-relay-binding/1`), `oma-interview/1` (scores
   `oma-interview-scores/1`), `oma-ralph/2`, `oma-asset/1`, `oma-config/1`.
5. **Session identity & worktree binding** — how `--session` resolves and how
   workflow state / relay pairs bind to a worktree (command-tree §1, §3, §5;
   schemas §4). A linked worktree resolves to one project `.oma`.
6. **Asset model** — manifest (`oma-asset/1`), canonical placement under
   `~/.agents/`, projection (symlink / junction / copy), registry ownership,
   and `rollback` semantics ([`docs/reference/adapter-conformance.md`](docs/reference/adapter-conformance.md),
   security-contract §2–§4).
7. **Relay protocol** — sequence reservation, reader cursor, publish
   transaction, archive, and the completion-receipt gate
   ([`docs/reference/relay-v2-protocol.md`](docs/reference/relay-v2-protocol.md)).
8. **core4 behavioral contract** — for the bundled `deep-interview`, `ralph`,
   `autopilot`, and `pair-delivery` skills, the frozen surface is their **ids**,
   the **trigger semantics** of each skill's `description`, the **`oma` commands**
   they invoke (the CI `refcheck` gate enforces every `oma …` reference resolves),
   and the **phase/terminal behavior and safety stops** those commands drive. A
   skill's free-form prose is not frozen, but a change to any of these behavioral
   properties is at least a minor (see the table above and the skill-prose note
   below).
9. **Release artifact naming & channels** — release assets are
   `oma_<version>_<os>_<arch>[.exe]`, `checksums.txt`, and the content-asset
   bundle `assets-<version>.tar.gz`; `self-update` (`--channel stable|prerelease`,
   `--version <tag>`) and `oma asset install --ref` resolve releases from GitHub
   Releases of this repo only, checksum- and version-verified, SemVer-gated
   (downgrades refused without `--allow-downgrade`), fail-closed
   (security-contract §5). Prerelease tags (`vX.Y.Z-rc.N`, `vX.Y.Z-beta.N`,
   etc.) are published as GitHub prereleases and excluded from the stable
   `/releases/latest` channel; `--channel prerelease` is the opt-in path.
10. **Supported platforms** — see below.

## Not frozen (may change in any 1.x release)

- **Non-behavioral skill prose.** Examples, explanations, and phrasing in any
  `SKILL.md` body may be rewritten in a patch. But a skill body is also runtime
  policy: changes to *when it triggers, when it advances a phase, what evidence
  it treats as sufficient, which `oma` commands it calls, or when it must stop*
  are behavioral and follow the graded rule in the table above (at least a minor;
  breaking an existing workflow contract is a major) — and for core4 these
  properties are frozen per item 8.
- **The on-demand skill catalog.** Skills may be added, deprecated, merged, or
  renamed (lifecycle status is tracked in `oma asset catalog`); only the
  core4↔CLI contract above is frozen.
- **Advisory `oma doctor` checks and `oma asset audit`** — heuristics and
  thresholds may change; they are advisory, never load-bearing.
- **Optional host accelerations** — Claude Code subagents, plan mode, structured
  pickers. These are clearly-marked optional branches; the agent-neutral default
  path is the contract, the accelerations are not.
- **New backward-compatible JSON fields** and **new experimental command groups**
  (explicitly documented as experimental until promoted).
- **The approximate tokenizer / budget model** (`approx-b4/1`) — a pinned
  algorithm version that may advance; the *gate* is stable, the exact count is not.

## Data compatibility and upgrades

- **Unknown major → fail-closed.** A file whose schema major the binary does not
  recognize is refused (read and write), never guessed at. This is deliberate.
- **Additive minors.** Within a major, new fields are additive; unknown fields
  are preserved across load/save, never dropped.
- **Migrations are explicit and one-shot.** A major bump ships an `oma doctor`
  migration subcommand (e.g. `oma doctor state --migrate-session-scope`,
  `oma doctor relay --migrate`). Migrations are dry-run by default, take a backup,
  are idempotent, and fail closed on conflict. There is no long-lived dual-read
  compatibility layer (see [`docs/design-philosophy.md`](docs/design-philosophy.md) §3.4).
- **Upgrade range.** A release migrates state written by any release back to the
  oldest one still carrying a supported migration path; `oma doctor` migration
  coverage and `CHANGELOG.md` name the supported source versions for each step.
  At **1.0.0**, all `v0.9.x` persistent state — every shipped on-disk schema — is
  directly migratable to `1.0.0`. Downgrades are not supported once a major has
  migrated state.

## Supported platforms and minimum versions

- **OS / arch** — Linux, macOS, and Windows on `amd64` and `arm64` (the six
  published release binaries). Other targets require an opt-in source build.
- **Source builds** — the Go toolchain version pinned in `go.mod` (currently
  `go 1.25.12`) or newer.
- **Hosts** — Claude Code and Codex. `oma` is host-neutral by contract and
  targets no specific host version; each release records the host versions it was
  last verified against, and the date, in `CHANGELOG.md`. Future host releases are
  best-effort until verified. Host-specific behavior is confined to the optional
  acceleration branches noted above.

## Deprecation policy

A frozen surface marked deprecated keeps working for **at least two minor
releases** before removal in a major, with the deprecation noted in `CHANGELOG.md`
and (where applicable) a runtime warning on stderr. Experimental surfaces carry
no deprecation guarantee and may change or be removed in any minor.

## Reporting a compatibility break

If a release breaks something this document says is frozen, that is a bug —
please file it with the **Compatibility regression** issue template (see
[`.github/ISSUE_TEMPLATE/`](.github/ISSUE_TEMPLATE/)). Security issues follow
[`SECURITY.md`](SECURITY.md) instead.

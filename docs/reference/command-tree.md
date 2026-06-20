# Command Tree Specification (`oma` terminal-state surface)

## 1. Global conventions

- **Exit codes**: `0` success; `1` completed but with warnings (doctor check warnings, etc.); `2` usage error; `3` environment/state error (permissions, corrupt schema, fail-closed refusal); `4` gate not passed (gate/budget/refcheck verdict negative); relay wait uses a dedicated `10/11/12` (see §6).
- **`--json`**: supported by all query commands; output carries a `schema` field (e.g. `"oma-cli/1"`); fields are stable once published, added fields are backward-compatible, and removal/rename requires a major bump.
- **`--dry-run`**: a **global persistent flag**, inherited by every mutating command (the whole asset family, state set, relay draft/publish/close, self-update, etc.); it prints the exact absolute paths to be created/modified/deleted and the operation type, touching nothing on disk (zero backups, zero leftover temp files). Query commands accept it but ignore it.
- **`--session <slug|current>`**: a **global workflow-state scope** used by `state`, `interview`, and `ralph`. `current` resolves `CODEX_THREAD_ID`, `CLAUDE_CODE_SESSION_ID`, or `OMA_SESSION_ID` into a path-safe suffix. Without `--session`, commands preserve the legacy project-global behavior. Relay commands accept the global flag as part of the root command surface but ignore it; relay uses author-session bindings instead.
- **Error-message convention**: a single-line first sentence stating the reason for refusal plus a one-line suggested action (`hint:` prefix); a fail-closed refusal must name the check that triggered it.
- Single asset namespace: **no `oma skill *` alias**. `oma update` is a documented alias for `oma asset update` (noted in help).

## 2. `oma asset` — content asset management

```
oma asset install <name>... [--agent claude,codex] [--dry-run] [--force]
oma asset list [--installed] [--json]
oma asset update [<name>...] [--dry-run]        # alias: oma update
oma asset remove <name>... [--dry-run]
oma asset rollback <name> [--to <backup-id>]
oma asset catalog [--from <root>] [--json]      # catalog view generated from the manifest (status lifecycle)
oma asset audit [--from <root>] [--json]        # advisory bloat audit: LOC/resident-tokens/ref-count + classification (KEEP/ORPHAN/OVERSIZED/RETIRE), never auto-deletes
oma asset link --dev [--repo <path>]            # dogfood: symlink the local checkout
```

- `install`: asset files → canonical location `~/.agents/{skills,agents,hooks}/<name>/` → platform projection into each agent directory per the manifest's `targets` (`symlink` on Unix-like hosts; `junction` for native-Windows directory assets when available, managed `copy` fallback/file assets otherwise). By default all targets are projected; `--agent` narrows them. Hook assets are canonical-only (placed only in the canonical location, never injected into host config; the user wires them by hand — see relay-v2-protocol.md §12.4).
- Overwrite semantics: target already exists and is not oma-managed → refuse; `--force` backs up first, then overwrites (see security-contract.md §2).
- `rollback`: restores from `~/.config/oma/backups/`; when `--to` is omitted, the most recent backup is used.
- `link --dev`: rewrites the canonical entry to a symlink pointing at the repo checkout; the registry is marked `dev: true`.
- `catalog`: scans `<root>/{skills,agents,hooks,prompts}/*/manifest.json` to produce a name-sorted catalog (name/type/status/targets/canonical), defaulting to `--from ./assets`; it shares its source with install/registry and introduces no second source of truth; a duplicate name, or a name that disagrees with its directory, is fail-closed. A manifest may optionally carry `status(active|deprecated|merged|alias)` plus `canonical`.

## 3. `oma state` — general project-level state

```
oma state get <key> [--file <path>] [--json]
oma state set <key> <value> [--file <path>]
oma state list [namespace-prefix] [--json]
```

- The default file is `<project root>/.oma/state/<namespace>.json`, with keys of the form `<namespace>/<field>` (e.g. `autopilot/phase`); `--file` overrides the whole file path. A linked git worktree resolves `<project root>` back to the primary checkout, so one repository has one `.oma`.
- With global `--session`, `state get/set autopilot/phase` is stored under the session-scoped namespace (`autopilot-<session>/phase`); `oma --session current state list autopilot` lists only the current session's autopilot namespaces.
- `list` scans the project `.oma/state/*.json`, optionally filtering by namespace prefix, and validates every matching file with the same fail-closed schema/namespace checks as `get`.
- Writes: atomic via tmp+rename, mode 0600; concurrency safety is guaranteed by the atomicity of rename (last writer wins; state files take no lock — the workflow convention is a single writer).
- A value is always stored as a string; structured data is serialized by the caller (keeping state semantics minimal).

## 4. `oma doctor` — diagnostics and gates

```
oma doctor [--json]
oma doctor budget --agent claude --profile core4 --max-resident-tokens 2000 [--json]
```

- `doctor` runs every item in the check registry: install consistency (registry vs. actual projection), permission bits, refcheck (static command references), security items (POSIX world-writable targets, projection escapes), a report on legacy v1 relay ledgers, and relay v2 leftovers (residual drafts / leftovers with no `.ready`). Exit code 0 all-green / 1 has warnings / 4 has a fail-level item.
- `doctor budget`: a deterministic count against the injection-surface model in adapter-conformance.md §5; over the limit exits 4.
- relay maintenance subitems (belonging to doctor, kept out of the public relay surface): `oma doctor relay [--restore <slug>] [--clean-stale]`.

## 5. Workflow commands (implementation semantics in workflows.md)

```
oma interview start [--threshold <0-1>|--depth quick|standard|deep] [--type greenfield|brownfield] [--id <id>] [--idea <text>] [--resume]
oma interview score --input <scores.json> [--id <id>] [--json]
oma interview gate [--waive --reason <text>] [--id <id>] [--json]
oma interview crystallize --spec <path> [--id <id>]
oma interview complete [--id <id>]
oma interview abort [--id <id>]
oma interview status [--id <id>] [--json]

oma ralph start --goal <text> [--max-rounds N] [--stall-window N] [--id <id>]
oma ralph next [--id <id>] [--json]
oma ralph check --verifier-exit <code> [--note <text>] [--id <id>] [--json]
oma ralph abort [--id <id>]
oma ralph status [--id <id>] [--json]
```

- State lands in `<project root>/.oma/state/interview-<id>.json` / `.oma/state/ralph-<id>.json`. With global `--session`, the CLI scopes ids before reading or writing (`--id same --session current` becomes a session-specific id; no `--id` becomes the session suffix). Without `--session`, an omitted `--id` resolves the **single** non-terminal project-level instance of that type, and ambiguity (>1 active) is refused with the candidates listed.
- The verdict output of `gate`/`next` must contain: the verdict, the numbers it rests on, and the suggested next step (both machine-readable and human-readable forms).
- **No `oma autopilot *` surface** (autopilot is pure markdown, using general `oma state`; changing this requires reopening the spec).
- **ralph start ambiguity gate (advisory)**: if `--goal` is too vague (≤15 words and lacking a file/issue/symbol/test-runner anchor), a suggestion is printed to stderr (clarify with deep-interview first, or plan with ralplan) — it does **not** block startup.
- The migration entry points the state machine requires are present: interview carries `crystallize` (gate_passed|gate_waived → crystallized, recording the spec path), `complete`, `abort`, and `gate --waive` (an early exit recording a caution, corresponding to the gate_waived state); ralph carries `abort`. Topology lock (topology_pending → interviewing) is carried by the round-0 input of `score` (schemas.md §5) rather than a standalone command.

## 6. `oma relay` — pair ledger (protocol in relay-v2-protocol.md)

```
oma relay init [--ledger-root <path>]
oma relay preflight [--json]
oma relay statusline [--json] [--watch] [--no-color] [--preset minimal|focused|full] [--pair <slug>]
# (hidden) oma relay hook <event>   — machine-invoked dispatcher; not a public group
oma relay pair new <topic-slug> [--peer <name>] [--json]
oma relay pair ensure [--json]
oma relay pair join <slug> [--json]
oma relay pair show [--pair <slug>] [--json]
oma relay pair list [--json]
oma relay pair set-lead <participant> [--pair <slug>]
oma relay draft --kind <kind> [--in-reply-to <seq>] [--corrects <seq>] [--pair <slug>] [--json]
oma relay publish <draft> --body-file <f> --prompt-file <f> [--touched <path>]... [--status <s>] [--verdict <v>] [--review-target <seq>] [--pair <slug>]
oma relay wait [--timeout <sec>] [--pair <slug>] [--json]
oma relay status [--last N] [--pair <slug>] [--json]
oma relay close --outcome <approve|reject|abandon> --reason <text> [--pair <slug>]
```

- `preflight` exit codes = `0` all-pass / `1` has warnings / `3` fail-stop (environment/state, per the §1 convention; `2` is not used — `2` remains cobra usage error); a legacy `.shared/` at the project root is only a warning, and only an explicit `--ledger-root` pointing at a v1 tree fails. `statusline` is the render command; the hidden dispatcher `hook <event>` is machine-invoked (not counted in the public group, not used in refcheck examples).
- The host-write commands `statusline install/uninstall/doctor` and the whole `hooks install/uninstall/doctor` group (which wrote to the host's `settings.json`/`hooks.json`) are not part of the surface — the user manages host config themselves, and an install wizard would overwrite their custom statusline/hooks. What remains is `oma relay statusline` (render) and the hidden `hook <event>` (dispatcher); the manual-wiring convention is in relay-v2-protocol.md §12.4. The **public relay group is exactly 9** (init/pair/draft/publish/wait/status/close/preflight/statusline; the dispatcher is hidden).
- `pair set-lead` updates `session.json.roles.lead` — the confirmation that workflows §4.1 requires after a role swap.
- `pair new` is the entry point for creating a pair (`ensure`/`join` carry only binding semantics); the creator becomes `roles.lead` by default (protocol §4), and `--peer` defaults to the claude↔codex counterpart. `draft` carries `--corrects <seq>` for the protocol §5 `corrects` field, mandatory when kind=correction.
- The global workflow `--session` flag does not scope relay. A pair is deliberately cross-session: Codex and Claude Code each resolve their own platform author-session, then bind to the same pair under `.oma/relay/_bindings/`.
- `claim` and `heartbeat` are internal protocol operations and have no public surface: claim is internalized as the sequence-number reservation step of `draft`; the heartbeat is refreshed automatically into this party's heartbeat file whenever any relay subcommand runs; stale diagnosis goes through `oma relay status --json` (the `last_heartbeat`/`stale` fields).
- **pair resolution order** (protocol §4a): explicit `--pair` ＞ the author-session binding file in the project ledger root (`.oma/relay/_bindings/`, written by `pair join|ensure`) ＞ auto-binding when there is exactly one active pair ＞ exit 3 listing candidates, zero writes.
- **draft lifecycle**: a draft is created only just before publish (silence during the work period = a wait timeout, not stale; exit 11 = a crash after the peer signals draft intent).
- `publish` can fold draft fill-in and publication into one step (body/prompt read from files, then through the §7 publish transaction after validation); a draft still containing a `TODO:` placeholder → refused.
- **A1/A2 quality gate**: a ready `kind:review` **must carry** `--verdict` (approve|approve-with-changes|revise) plus a target seq (`--review-target`, defaulting to `--in-reply-to`, which must be ≥1; otherwise publish refuses); **R5** additionally **requires** a fenced `oma-review-evidence/1` block inside the review body (validated against the verdict + non-placeholder + structured refs), and publish computes an `evidence_hash` into the frontmatter; `kind:decision` automatically stamps a completion receipt (carrying `quality_gate_evidence_hash`) from the "non-lead approve review against the latest work (reviewed_head)". `close --outcome approve` runs through the fail-closed quality gate (protocol §9, including the three-way evidence check): gate not passed → **exit 4**; a corrupt receipt/state/evidence → **exit 3**.
- `wait` exit codes `0/10/11/12/3` (semantics in protocol §8; usage error stays the global `2`).

## 7. Other

```
oma config show [--json]       # prints the effective config + per-key source (flag/env/project/user/default); read-only
oma config path [--json]       # prints the resolved user/project config file locations; read-only
oma self-update [--check]      # --check is strictly read-only (version comparison only); --dry-run follows the global contract (discloses the paths to be downloaded/replaced); flow and security requirements in security-contract.md §5
oma version [--json]           # version, commit, schema version summary
```

`oma config` is a query command group (read-only, zero disk writes); the full config-layer semantics are in config.md: the priority chain, the per-key source mapping, and the strict boundary against schema data. Both `config show` and `config path` support `--json`, and a parse failure goes through `ExitState(3)`.

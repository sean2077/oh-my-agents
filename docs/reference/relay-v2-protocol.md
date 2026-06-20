# Relay v2 Protocol Specification (`oma relay`)

Schema marker: `oma-relay/2`. This protocol inherits the **principles** of agent-ledger v1 (not its format): an append-only ledger, sidecar integrity markers, fail-closed identity resolution, and platform signals taking priority.

## 1. Ledger Root and Legacy Coexistence

- **Default root**: `<project root>/.oma/relay/`, where linked git worktrees resolve back to the primary checkout. `--ledger-root <path>` overrides it only when a pair intentionally needs a non-project ledger.
- **Never writes** into the agent-ledger v1 `.shared/` tree. v1 is identified by the presence of a `_relay/` directory under the root, or a v1-shaped `session.json` (`schema_version` an integer 1–3).
- When a legacy `.shared/` tree is present: `oma relay status` and `oma doctor` report it as "v1 ledger: archived / for human reference, oma neither reads nor writes it," and continue using the v2 root. Coexistence requires neither deletion nor migration.
- `--ledger-root` pointing explicitly at a v1 tree → **rejected**, with the reason stated.
- **v2 sentinel**: `.oma/relay/.oma-relay-v2`, with content `{"schema":"oma-relay/2","created":"<ISO-8601>"}`. A missing sentinel over a non-empty directory → rejected; a schema major ≠ 2 → rejected (prompting an oma upgrade or a directory check).

## 2. Topology and Clock Assumptions

- The v1 release supports only the **same ledger root / same host**: both processes share one filesystem and one system clock. Linked git worktrees for one project share the same `.oma/relay/` root by default and avoid collisions through author-session bindings; use `--ledger-root` only for an intentional non-project ledger.
- Filesystem properties depended upon (verified by `oma doctor` probes): tmp+rename atomicity, O_EXCL exclusive creation; mtime need only distinguish ordering at the `OMA_RELAY_STALE_AFTER` granularity (minutes) — sub-second coarse mtime is a doctor **warning**, not a failure.
- No protocol fields are reserved for cross-machine transport (YAGNI); future topology changes are carried by `schema_version` evolution.

## 3. Directory and File Layout

```
.oma/relay/
├── .oma-relay-v2                  # sentinel
├── _bindings/<author-session>.json # pair binding (schema oma-relay-binding/1, see §4a)
├── <pair-slug>/                   # YYYYMMDD-<topic-slug>
│   ├── session.json               # pair metadata (schema in schemas.md §4)
│   ├── NNN-<author>-<kind>.md     # published artifact (append-only)
│   ├── NNN-<author>-<kind>.md.sha256
│   ├── NNN-<author>-<kind>.md.ready
│   ├── .draft/NNN-<author>-<kind>.md   # author-private draft (peer agrees not to read)
│   ├── .seq/NNN                   # sequence reservation file (O_EXCL; author recorded in the file's first token)
│   └── .heartbeat/<author>        # heartbeat file (mtime is liveness)
└── _archive/<pair-slug>/          # moved here wholesale after close
```

Permissions: directories 0700, files 0600 (checked by `oma doctor`).

## 4. Identity and Roles

- author resolution priority: **platform signal** (`CLAUDE_CODE_SESSION_ID` → `claude`; `CODEX_THREAD_ID` → `codex`) > the `OMA_RELAY_AUTHOR` environment variable > resolution failure is rejected.
- Both platform signals present with no `OMA_RELAY_AUTHOR` to arbitrate → rejected (fail-closed, zero writes).
- Relay identity is deliberately separate from the global workflow `--session` flag. `--session` scopes project workflow state (`oma state` / `interview` / `ralph`); relay pairs are cross-session ledgers where each side keeps its own author-session binding. Parallel pair workflows are represented by different Codex/Claude platform session pairs, each bound to its own pair.
- Participants are exactly 2; identical names on both sides are not allowed (claude+claude is rejected).
- **Roles**: `session.json.roles` maps `lead / planner / implementer / reviewer` to participant names (one person may hold several roles). `lead` = the primary decision-maker, **required and unique**, defaulting to the bootstrap initiator; the remaining roles may be assigned to either participant. The relay mechanism does not enforce role behavior; the role fields are read and surfaced by skills such as the pair-delivery flow (lead semantics and the swap rules are in workflows.md §4).

## 4a. Pair Binding and Resolution

- `oma relay pair join <slug>` (and the automatic-binding path of `pair ensure`) writes the binding file `.oma/relay/_bindings/<author-session>.json` (schema `oma-relay-binding/1`): `author`, the platform session-id hash, `pair`, `created`, `updated`.
- **All pair-scoped commands** (draft/publish/wait/status/close/pair show) resolve their target pair in this order: an explicit `--pair <slug>` > the current author-session's binding file > exactly one active pair, which is adopted automatically and the binding written > otherwise **exit 3, zero writes**, listing the candidate pairs.
- An unknown binding schema, a binding pointing at a nonexistent or already-terminal pair, or multiple active pairs that cannot be disambiguated → exit 3, zero writes.
- `pair show` displays the resolution result and the peer's join command; `pair list --json` lists all pairs, both active and terminal.

## 5. Artifact Model

- Filename `NNN-<author>-<kind>.md`, NNN a three-digit decimal (from 001, gaps allowed, read in filename order).
- `kind ∈ plan | review | fix | note | question | decision | correction | addendum`.
- frontmatter (YAML): `schema("oma-relay/4"), seq(int), author, peer, kind, status, created(ISO-8601), in_reply_to(int|null), prompt_for_next(string), touched_paths([string]), corrects(int|null)`. **A1/A2 fields**: a ready `kind:review` **must carry** `verdict(approve|approve-with-changes|revise)` + `review_target_seq` (≥1, the seq it adjudicates); **R5**: a ready `kind:review` must **also carry** a fenced `oma-review-evidence/1` block in its body + a frontmatter `evidence_hash` (its canonicalized sha256); `kind:decision` adds the completion receipt `receipt_id, reviewed_head_seq, reviewed_head_hash, quality_gate_seq, quality_gate_hash, quality_gate_evidence_hash, verified_at` (omitted for other kinds). The strict parser still rejects any unknown key; the session/sentinel remain `oma-relay/2`.
- `status ∈ ready | closed | cancelled | failed | timed_out`; terminal states = closed/cancelled/failed; `timed_out` is a recoverable pause (used for `@user:` escalation).
- **append-only**: a file carrying `.ready` is never modified; corrections go through a `kind: correction` + `corrects` pointing at the original seq.

## 6. Sequence Allocation and Contention (claim internalized)

- Sequence reservation: create `NNN` under `.seq/` with **O_EXCL** (author and timestamp written into the file content), NNN = max(largest published seq, largest reservation in .seq) + 1.
- Contention: O_EXCL failure → retry with NNN+1 (error after a cap of 10 attempts). Two concurrent drafts always get distinct seqs, with no possibility of overwrite.
- The exclusive object is the bare `NNN`, not `NNN.<author>` — an author-suffixed filename would make O_EXCL exclusive only on the (seq, author) axis, letting two concurrent sides each take the same NNN. Author attribution moves into the file content, and cross-author exclusion is guaranteed by the bare `NNN` filename.
- `oma relay draft` creates a draft = reserve a sequence + create a heartbeat file; the public command surface has no standalone `claim` (claim is an internal step of draft, see command-tree.md §relay). The draft and its `.seq` reservation **persist until** the publish transaction completes (the `.ready` lands) (§7).
- Reservation expiry: once the draft heartbeat backing a sequence reservation is stale (§8), `oma doctor` may clean up the reservation and the draft; the resulting sequence gaps are legal.

## 7. Publish Transaction and Interruption Recovery

A **draft is a durable publish intent**: until the `.ready` is written, the draft and its `.seq` reservation always exist; publish never consumes the draft body itself as an intermediate product.

publish steps (strict order): **render** the formal content from the draft → write `NNN-<author>-<kind>.md.tmp` → fsync → rename to the formal name → write `.sha256.tmp` → rename `.sha256` → write `.ready.tmp` → rename `.ready` → **finally** delete the draft and the `.seq` reservation.

- No `.ready` = unpublished, ignored by every reader — this is the sole publication criterion.
- Interrupted re-run: `oma relay publish <draft>` re-renders from the still-existing draft and completes the missing steps/sidecars (including a kill after the rename — the draft is still there); if an existing formal file and the draft's re-rendered content disagree → fail-closed, prompting `oma doctor relay --clean-stale` to quarantine the incomplete formal file.
- Readers verify `.sha256`; a mismatch → fail-closed (reported as corrupt, no content returned).

## 8. Heartbeat and Liveness (internal mechanism)

- The heartbeat file `.heartbeat/<author>` is touched whenever **any** `oma relay` subcommand runs for that author (draft/publish/status/wait all refresh their own side).
- stale threshold: default 15 minutes, tunable via `OMA_RELAY_STALE_AFTER` (seconds).
- `oma relay wait` exit codes: `0` a new artifact (path printed to stdout); `10` wait timeout (default 60 minutes, tunable via `--timeout`); `11` the peer has an unpublished draft / publish intent and its heartbeat is stale (crashed after declaring intent); `12` the pair is terminal; `3` an environment/protocol/fail-closed error (usage errors keep the global `2`, consistent with command-tree.md §1).
- A participant that has just published, or whose latest artifact is already its own, must not start a new relay round until either a peer artifact arrives, the pair becomes terminal, or the user explicitly interrupts. With trusted host hooks, `Stop` is the primary self-continuation path: it wakes the stopped host after a peer artifact exists. `oma relay wait` remains the fallback when hook wiring/trust is unavailable or a foreground wait is explicitly requested.
- **Draft lifecycle**: the convention is that an agent calls `oma relay draft` only **after finishing its work, just before publishing**. Long silence during the work period is expressed by the `wait` timeout (exit 10), **not** by a stale heartbeat; the strict meaning of exit 11 = the peer crashed **after** creating the draft / publish intent. Creating a draft early is an unsupported path (any future keepalive would require reopening the review of this document).
- **ready takes priority over stale**: `wait` checks for a new `.ready` artifact before it looks at stale draft state — when a publish transaction is killed after the `.ready` is written but before the draft is cleaned up, the waiter gets exit 0; a `.seq`/draft residue with a matching `.ready` already present is a cleanup warning (flagged by `status --json`, handled by `oma doctor relay --clean-stale`), **not** an exit 11 condition.
- Diagnostics: `oma relay status --json` exposes `last_heartbeat`, the stale determination, and pending draft sequences — awareness of the peer's draft comes **only from the `.seq/` reservation files**, never from reading the peer's `.draft/` content.

## 9. Terminal State and Archival

- `oma relay close --outcome <approve|reject|abandon> --reason <text>`: write the session.json terminal state → write the `CLOSED` sentinel → move the whole thing into `_archive/`.
- **approve quality gate (A2, fail-closed)**: `--outcome approve` is rejected unless the latest lead `kind:decision` carries a valid receipt and: ① the receipt's `reviewed_head` (the approved work) is still consistent by hash; ② a **non-lead** `kind:review` (verdict=approve) exists **against that reviewed_head** (review_target_seq==reviewed_head) and is consistent by hash; ③ there is **no newer unreviewed work** after reviewed_head; ④ (**R5**, additionally) the recomputed canonicalized hash of that approve review body's `oma-review-evidence/1` block must == its frontmatter `evidence_hash` == the receipt's `quality_gate_ref.evidence_hash`. `approve-with-changes`, a lead self-review, or a stale target all fail to satisfy the gate. **Exit codes (R4)**: gate not passed (no decision/review, verdict mismatch, stale target, newer unreviewed work) → **exit 4**; corrupt receipt/artifact, hash or evidence mismatch → **exit 3**. `reject`/`abandon` require no receipt.
- Archived contents are read-only; restore and cleanup are `oma doctor` subchecks (for the command surface see command-tree.md).

## 10. Security Essentials (implementation contract in security-contract.md)

- Peer artifacts are **untrusted input**: oma executes no command from within them; `touched_paths` is a hint only, to be validated before use.
- Secrets never enter the ledger: publish **mandatorily** runs a secret-pattern scan, with no skip switch in v1; false positives are handled by the narrow-scope allow patterns of security-contract.md §6; doctor includes this check too and reports the active allow list.
- Every parse path is fail-closed: an unknown schema, corrupt frontmatter, or hash mismatch is rejected with the reason reported.

## 11. Test Matrix (→ plan B8)

| # | Test | Verification point |
|---|------|--------|
| 1 | v1 fixture detection rejected | any write command against testdata/relay-v1-tree/ is rejected, zero writes |
| 2 | unknown schema rejected | sentinel major ≠ 2 → rejected |
| 3 | append-only | a `.ready` file cannot be modified by any oma path; publish does not overwrite an existing formal name |
| 4 | sidecar/hash verification | tampering with published content → reader fail-closed |
| 5 | identity ambiguity fail-closed | both platform signals with no arbitration → rejected, zero writes |
| 6 | stale draft / heartbeat recovery | a stale draft is identified and cleaned up by doctor; reads are correct over sequence gaps |
| 7 | duplicate-sequence contention | concurrent draft ×2 → distinct seqs, zero overwrites |
| 8 | interrupted publish recovery | inject a kill **between every step** (including after the formal rename) → re-running the same publish command converges from the surviving draft; fail-closed when the formal file and draft disagree; no dirty reads at any point. Subcase: kill after `.ready` is written but before draft cleanup → the peer's wait gets exit 0 (not 11), residue cleaned up by doctor |
| 9 | legacy coexistence | with a v1 `.shared/` tree present, v2 is fully functional and never touches v1 |
| 10 | --ledger-root points at v1 | rejected |
| 11 | v2 root unknown schema | rejected |
| 12 | pair binding resolution | multiple active pairs with no disambiguation → exit 3, zero writes, candidates listed; `--pair` override takes effect; a single active pair auto-binds to disk |

## 12. Experience Layer (B12–B14)

The experience layer of agent-ledger relay is one users are highly satisfied with; before switching to oma relay v2 it must be matched, with no regression. **Three** pieces are added: preflight / statusline / hooks auto-continue-turn; an issue ledger and cross-machine sync are **not** done (sync is YAGNI-deferred per §2). All three are additive, reusing internal/hookcfg (injection), the internal/checks style (checks), and the status reader (state).

### 12.1 `oma relay preflight [--json]` (B12)

A human troubleshooting gate: it diagnoses identity, ledger root/sentinel/schema, binding/peer derivation, and filesystem properties. It **never hard-fails** — each condition lands as a one-line check result, so the full table is presented no matter how broken the environment is.

- Checks: `identity.author` (+session), `ledger.root`, `ledger.v1_root` (only with an explicit `--ledger-root`; pointing at a v1 tree → fail), `legacy.shared` (project-root `.shared/_relay` present → **warn**, not fail), `ledger.sentinel` (v2 present/missing/wrong major), `pair.binding` + `identity.peer`; FS probes `fs.{tmp_rename,mtime,symlink,sha256,fsync,posix_mode}`, with coarse-grained mtime (indistinguishable within 1s) → **warn**, not fail.
- Exit codes: `0` all pass / `1` warnings present / `3` fail-stop (environment/state, the general rule of command-tree §1; **not `2`** — `2` is reserved for cobra usage errors). Behaviorally equivalent to agent-ledger's `0/1/2` (table + stop/continue semantics), with exit codes taking oma's native values.
- `--json`: schema `oma-relay-preflight/1`, with stable fields for reuse by B13/B14.
- SessionStart (B14) **does not run a full preflight** — it uses a lightweight, bounded stale/residue/status check (FS probes are expensive on shared mounts); the full preflight is human-triggered only.

### 12.2 `oma relay statusline` (B13, rendering command)

A compact "which pair / whose turn / latest seq·kind·status" line + a `--watch` dashboard. Safety properties (hard acceptance criteria): **pure read** (no GC, no last_seen/heartbeat mutation, zero ledger writes), **binding-scoped** (an unbound window does not display orphan active pairs), and self-bounded rendering/subprocesses (a hung mount does not drag down the UI). `--json` schema `oma-relay-statusline/1`, for consumption by a host status-line script.

oma does not write host configuration; the rendering command and the dispatcher below are wired in by hand (§12.4). A host that already runs a rich status-line script calls `oma relay statusline --json` from within that script and filters on `.bound`, rather than handing the whole status line to oma.

### 12.3 `oma relay hook <event>` (B14, hidden dispatcher)

A **hidden** dispatcher `hook <event>` (machine-invoked, not counted in the public group, not in refcheck). It reads the host hook payload, resolves the bound pair's state, emits the correct JSON per platform, and **never breaks the host** (any internal error → exit 0 silently, except an intentional PreToolUse deny):

- **SessionStart**: an early prompt + a lightweight stale/residue summary (`systemMessage`).
- **PreToolUse**: refuses edits to a `.ready`-published artifact (`permissionDecision:deny` + a pointer to the correction flow); does not emit fields Codex does not support.
- **Stop**: **auto-continues the turn** when the peer has published and it is addressed-to-me; guardrails (hard acceptance criteria): loop prevention (`stop_hook_active` → exit 0 silently), strict binding (equivalent to `--require-binding`, silent when unbound), bounded state (a short status read rather than a waiter; timeout/failure → exit 0 silently + a diagnostic trail), de-duplication (a stable fingerprint, silent when unchanged), and emitting `decision:"block"` to continue the turn while **never** pairing it with `continue:false`. For Codex, this is the main self-continuation path once the Stop hook is wired and trusted through `/hooks`; held `oma relay wait` is the fallback path when hook trust/wiring is absent.

oma does not write host configuration; the dispatcher itself is wired into the user's own hooks configuration by hand, using the entries below.

### 12.4 Manual Wiring Reference (user-managed host config)

> **The complete copy-paste snippet is in the README, "Wire the statusline and hooks (optional)"** (the full JSON for both the statusLine and the three-event hooks). This section is the field-level spec, for reference during precise wiring.

oma does not write host configuration; the following are the canonical shapes for the user to wire by hand into `~/.claude/settings.json` (claude) / `~/.codex/hooks.json` (codex).

- **statusLine (claude, optional)**: wire the rendering command into the top-level `statusLine` key, with an existence guard so a missing binary does not spam command-not-found:

  ```json
  { "statusLine": { "type": "command",
    "command": "[ -x '/abs/path/oma' ] || exit 0; exec '/abs/path/oma' relay statusline" } }
  ```

  A user who already has a rich status-line script instead calls `oma relay statusline --json` from within the script and filters on `.bound` (see `docs/examples/statusline-command.sh`).

- **hooks shape (real-host evidence)**: both host ends are a nested matcher group wrapped by a top-level `hooks` key (claude `settings.json`, codex `hooks.json` — codex's sibling `state` trust table is preserved byte-for-byte). One `{type:"command", command, timeout}` entry per event.
- **matcher scope / timeout**: SessionStart `startup|resume|clear` (10s); PreToolUse claude `^(Edit|Write|MultiEdit)$`, codex `^(apply_patch|Edit|Write)$` (5s); Stop with no matcher (5s). A PreToolUse with no matcher makes the dispatcher spawn once per tool call — always carry a matcher.
- **command-string guard**: use an **absolute binary path**. Unix-like hosts use an existence guard (`[ -x '<path>' ] || exit 0; exec '<path>' relay hook <event>`, POSIX single-quote escaping when the path contains spaces/quotes). Native Windows Codex Desktop runs commands through PowerShell, so use the call operator form (`& 'C:\path\oma.exe' relay hook <event>`), with no POSIX guard. This resolves two failure modes: a host that trims PATH, making the hook silently not fire; and a removed binary causing the host to spam command-not-found on every call.
- **codex trust**: after editing the codex hooks, run `/hooks` once and confirm the `oma relay hook Stop` entry is trusted. Codex self-continuation depends on that trust gate; oma documents the command shape but does not write or trust host config for the user.

**Switch gate**: agent-ledger relay is decommissioned only once the auto-continue-turn dispatcher, verified through manual wiring, is at parity or better. The public relay group is exactly 9 in its terminal shape (init/pair/draft/publish/wait/status/close/preflight/statusline; the `hook <event>` dispatcher is hidden and not counted).

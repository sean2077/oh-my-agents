# Schema Reference (registry / state / relay / workflows)

> The schema and evolution policy for all persisted data.

## 1. General evolution policy

- Every persisted file carries a `schema` field in the form `oma-<domain>/<major>` (e.g. `oma-registry/1`).
- **Unrecognized major → fail-closed**: reads and writes are refused, prompting the user to upgrade oma or check the file's origin.
- Minor evolution = additive fields only: readers tolerate unknown fields (preserved and passed through, never dropped); deletion, renaming, or any semantic change must bump the major and ship an `oma doctor` migration subcommand (versioned migration is a terminal-state mechanism, not a transitional form).
- Writes are always atomic (unique same-directory tmp+rename) and `0600`; before writing state-like JSON documents, a single-generation `.bak` backup is taken of any existing file.

## 2. Install registry `~/.config/oma/registry.json` (`oma-registry/1`)

```json
{
  "schema": "oma-registry/1",
  "assets": [{
    "name": "deep-interview",
    "type": "skill",
    "version": "v0.3.0",
    "installed_at": "ISO-8601",
    "source": "release|dev-link",
    "canonical_path": "~/.agents/skills/deep-interview",
    "projections": [{"agent": "claude", "path": "~/.claude/skills/deep-interview", "kind": "symlink"}],
    "backups": [{"id": "20260611T150000", "path": "~/.config/oma/backups/20260611T150000/..."}]
  }]
}
```
- The registry records only oma-managed entries; external sources (npx skills and the like) are neither registered nor modified, and doctor merely reports them.
- Projection `kind` is `symlink`, `junction`, or `copy`. Unix-like hosts use `symlink`; native Windows uses `junction` for directory assets when available and `copy` for managed-copy fallbacks and file assets.

## 3. Generic project state `.oma/state/<namespace>.json` (`oma-state/1`)

```json
{"schema": "oma-state/1", "namespace": "autopilot-release-20260619", "revision": 7, "data": {"<key>": "<string value>"}, "updated": "ISO-8601"}
```
- The carrier for `oma state get/set/patch`; values are always strings, leaving any structure to the caller.
- Each namespace file has a monotonic `revision`; `state set` and `state patch` run under a namespace-level cross-process lock, and callers that need compare-and-set semantics can pass an expected revision.
- Workflows that can run concurrently use CLI-level current-session scoping by default, or an explicit `--session <slug>` override instead of a shared namespace.
- `oma state list [namespace-prefix] --json` discovers validated namespaces in the shared project `.oma/state/`; state commands transform/filter namespaces by the resolved session suffix. Matching corrupt state fails closed instead of being skipped.

## 4. relay v2 `session.json` (`oma-relay/2`, see relay-v2-protocol.md)

```json
{
  "schema": "oma-relay/2",
  "pair": "20260611-topic",
  "project": "oh-my-agents",
  "participants": ["claude", "codex"],
  "participant_sessions": {"claude": "0123456789ab", "codex": "fedcba987654"},
  "roles": {"lead": "claude", "planner": "claude", "implementer": "claude", "reviewer": "codex"},
  "status": "active|closing|closed|cancelled|failed",
  "worktree_root": "/abs/path/worktree", "branch": "feature-x", "base_commit": "<sha at creation>",
  "created": "ISO-8601", "closed": null, "outcome": null, "reason": null
}
```
- Artifact frontmatter schema `oma-relay/4` (see protocol §5; base fields include `author` plus required `author_session`; a ready `kind:review` **must carry** `verdict` + `review_target_seq` (≥1), and additionally a fenced `oma-review-evidence/1` block in its body plus a frontmatter `evidence_hash`; a `kind:decision` adds completion-receipt fields including `quality_gate_evidence_hash`; the session and sentinel remain `oma-relay/2`); the `.oma-relay-v2` sentinel: `{"schema":"oma-relay/2","created":"..."}`.
- `participant_sessions` claims each author slot for one concrete session hash. The creator is claimed at pair creation; the peer is claimed by `pair join` or single-active auto-adopt. Same-author different-session reuse is refused fail-closed. This field is required: v0.7.0 pairs predate it and are unreadable (fail-closed) until repaired with `oma doctor relay --migrate`, which sets it to `{}` so both sides re-claim their seat via `pair join`.
- `worktree_root`/`branch`/`base_commit` bind the pair to the creator's checkout (optional, minor-additive, recorded at `pair new` from the CLI git context). `close` refuses an approval/abandon driven from a different worktree unless `--allow-worktree-change`, so a pair started in one worktree is not silently concluded from another.
- `closing` is a transient pair-mutation state written during `close`; draft/publish/set-lead/join refuse it. `close` is recoverable and may be re-entered from `closing` or from a `closed` pair that has not yet been archived.
- Completion receipt `oma-completion-receipt/2` (embedded in decision frontmatter): `{schema, pair, decision_seq, reviewed_head{seq,hash}, quality_gate_ref{seq,verdict,hash,evidence_hash}, verified_at}`; `reviewed_head` = the approved "work" (the latest non-review, non-decision artifact), `quality_gate_ref` = the non-lead approve review targeting it, whose `evidence_hash` = the canonicalized sha256 of that review body's `oma-review-evidence/1` block. The receipt's sha256 is stored as the frontmatter `receipt_id`, which `close --outcome approve` uses for fail-closed verification (protocol §9).
- Review evidence `oma-review-evidence/1` (a single fenced block inside the review body): `{schema, findings[{severity(critical|high|medium|low), confidence(high|medium|low), claim, refs[{type(repo|official|source_reference|supplemental), ref, version_or_date?}]}], basis_refs[…], commands_run[], limitations[]}`. publish validates by verdict (revise / approve-with-changes must carry findings; approve must carry basis_refs + commands_run + limitations), plus non-placeholder content, plus a repo ref must be `path:line[-line]` with no absolute path or `..`, and an external ref must be an http(s) URL; the canonicalized sha256 = the frontmatter `evidence_hash`, which is bound into the decision receipt (protocol §9, the close gate's additional check).
- pair binding `.oma/relay/_bindings/<author-session>.json` (`oma-relay-binding/1`): `{"schema":"oma-relay-binding/1","author":"claude","session_hash":"<hash of platform session id>","pair":"20260611-topic","created":"ISO-8601","updated":"ISO-8601"}`; the resolution order and fail-closed semantics are in protocol §4a.

## 5. interview state `.oma/state/interview-<id>.json` (`oma-interview/1`)

**The authoritative field set = workflows.md §1.2** (this section does not duplicate the field list, to avoid dual-source drift). Interview state carries a monotonic `revision` incremented on each successful write. The schema for the `score --input` input file (`oma-interview-scores/1`):

```json
{
  "schema": "oma-interview-scores/1",
  "round": 3,
  "component_scores": {"cli-core": {"goal": 0.55, "constraints": 0.3, "criteria": 0.2}},
  "question": "...", "answer": "...",
  "ontology": {"entities": [{"name": "...", "type": "...", "fields": [], "relationships": []}]},
  "challenge_mode_used": null
}
```

- **round 0 (topology lock)**: `{"schema":"oma-interview-scores/1","round":0,"topology":{"components":[{id,name,description,status,evidence?}],"deferrals":[{component_id,reason}]}}` — the `topology_pending` phase accepts only this form; once locked, it enters `interviewing`. Scoring rounds start from 1 and must be consecutive (replays or skipped rounds are rejected).

## 6. ralph state `.oma/state/ralph-<id>.json` (`oma-ralph/2`)

The field set is in workflows.md §2.1 (id/revision/session/project_root/worktree_root/branch/base_commit/phase/goal/keep_policy/max_rounds/round/checks[]/stall_window/plateau_window/best_round/best_score/created/updated). `/2` adds the keep-policy contract: `keep_policy` (pass_only|score_improvement), `plateau_window`, `best_round`, `best_score`, an optional `score` on `checks[]`, and a new terminal state `plateaued`; later minor-compatible fields add the workflow session and starting worktree identity so commands can refuse accidental cross-worktree continuation. There is **no /1→/2 migration layer** (`Load` rejects any non-2 major, fail-closed). The `receipt` (sha256 over {goal, keep_policy, checks, terminal_check}) is produced under pass_only at `passed`; under score_improvement it is produced at every terminal state (`passed` / `plateaued` / `exhausted`), with `terminal_check` taking the best-score check (it records the achieved result, not proof that the command was actually rerun).

## 7. User config `config.toml` (`oma-config/1`)

- The location and precedence chain are in config.md: `~/.config/oma/config.toml` (user) and `<project root>/.oma/config.toml` (project, private/local by default).
- The TOML root may carry `schema = "oma-config/1"`: a missing value is treated as the current major (tolerating a hand-written omission); a present value whose major ≠ 1 → fail-closed.
- It is user-intent configuration, not runtime state: it is carried exclusively through viper (the config.md §1 boundary) and does not go through the encoding/json read layer used by the rest of this document's schemas, but the major's fail-closed semantics are identical; it is registered in `version.Schemas["config"]`.

## 8. hook fragment `assets/hooks/<name>/fragment.json` (`oma-hook-fragment/1`)

- A **manual-wiring reference** shipped with a hook asset: a top-level `schema` plus per-agent sections (`claude` / `codex`), each section mapping `event → [host-native form entries…]`.
- **oma does not parse or validate it**: oma performs zero host-config rewriting, and the hook-injection command has been removed. `fragment.json` is placed at the hook asset's canonical location `~/.agents/hooks/<name>/`, and the user wires it into their own `settings.json` / `hooks.json` by hand following its contents (the wiring conventions are in relay-v2-protocol.md §12.4). This schema is therefore not registered in `version.Schemas` and takes no part in install-time fail-closed validation.

## 9. dogfood log `.oma/dogfood-log.md`

- Free-form markdown with a required header: start date, OMC disposition (the verbatim disable/blocklist command), and the **exact rollback command**.
- Each entry: date + event (which workflow was used / the problem encountered / whether a rollback was invoked).
- Phase D acceptance parsing depends on the presence of the header fields and a text assertion of "no re-enable event" (primarily a manual review, with no strict machine-readable schema).

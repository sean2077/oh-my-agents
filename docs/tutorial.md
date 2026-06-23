# oma end-to-end tutorial

This is a single walkthrough of a complete `oma` workflow — from a clean machine
to a delivered, cross-reviewed change and back to a clean machine. It stitches
the isolated snippets elsewhere into one runnable sequence.

The default path is plain `oma` commands that behave **identically on Claude
Code and Codex**. Steps marked _(optional, host-specific)_ are accelerations you
can skip without changing any outcome.

Conventions used below:

- The agent (Claude Code or Codex) drives `oma` for everything mechanical;
  **you** make the judgment calls (answering interview questions, picking among
  options, approving a close).
- Exit codes are contractual: `0` ok, `1` warn, `2` usage, `3` fail-closed,
  `4` gate failed; `oma relay wait` adds `10/11/12`. Full table:
  [reference/command-tree.md](reference/command-tree.md) §1.
- `--json` works on every query command, and `--dry-run` is a global flag that
  discloses exact paths and writes nothing — prepend it to any mutating command
  to preview it first.

The stages:

1. [Install and verify the CLI](#1-install-and-verify-the-cli)
2. [Install the core4 skills and check the budget](#2-install-the-core4-skills-and-check-the-budget)
3. [Crystallize a spec with deep-interview](#3-crystallize-a-spec-with-deep-interview)
4. [Execute with autopilot](#4-execute-with-autopilot)
5. [Verify with a ralph loop](#5-verify-with-a-ralph-loop)
6. [Cross-agent pair-delivery over the relay](#6-cross-agent-pair-delivery-over-the-relay)
7. [Simulate a failure and resume](#7-simulate-a-failure-and-resume)
8. [Inspect the on-disk state](#8-inspect-the-on-disk-state)
9. [Lifecycle: update, migrate, rollback, uninstall](#9-lifecycle-update-migrate-rollback-uninstall)

---

## 1. Install and verify the CLI

**Why:** every skill shells out to `oma` for the parts that must be counted,
validated, or persisted. A skill invoked without the binary on `PATH` stops at
its first command, so the binary comes first.

Install the latest release into `~/.local/bin` (full options — pinning a tag,
other destinations, source builds, Windows PowerShell — are in the README
["Install the CLI"](../README.md#install-the-cli) section):

```bash
curl -fsSL https://raw.githubusercontent.com/sean2077/oh-my-agents/main/scripts/install.sh | bash
```

Then confirm it resolved on `PATH`:

```bash
oma version
```

**Success looks like:** `oma version` prints a version, commit, and schema
summary (add `--json` for the machine-readable form). If the command is not
found, the installer printed a `PATH` hint — add `~/.local/bin` (or your chosen
`OMA_INSTALL_BIN_DIR`) to `PATH` and re-open the shell.

---

## 2. Install the core4 skills and check the budget

**Why:** nothing is resident in the model's context unless you put it there. The
four core workflow skills are explicit assets; you install them once into the
canonical store and `oma` projects them into both `~/.claude/` and `~/.codex/`.

```bash
oma asset install deep-interview ralph autopilot pair-delivery
```

With no source flag, `oma asset install` fetches the assets bundle published with
**your installed oma version** (verified against the release `checksums.txt`), so a
clean machine needs nothing but the binary from step 1 — and it never fetches an
unpinned ref. Pin a different release with `--ref <tag>`, or install from a local
checkout with `--from ./assets` (when developing `oma` itself). A `dev` build has
no release to match, so it asks for an explicit `--ref`/`--from`.

Confirm what landed:

```bash
oma asset list --installed
```

**Success looks like:** all four skills appear as installed. (Want to see what a
command would touch before running it? Re-run with `--dry-run` — it prints the
exact absolute paths and writes nothing.)

Now check the resident-context budget. This is a deterministic gate that counts
only the resident surface (each skill's `name` + `description`, not the
on-demand body):

```bash
oma doctor budget --agent claude --profile core4
```

**Success looks like:** exit `0` — the core4 surface is well under the 2000-token
CI threshold (internal target 1800; the budget model is
[reference/adapter-conformance.md](reference/adapter-conformance.md) §5). Over
the limit would exit `4`.

Optionally run the full diagnostic sweep (install consistency, permission bits,
command refcheck, security items, relay leftovers):

```bash
oma doctor
```

Exit `0` all-green, `1` has warnings, `4` has a fail-level item.

> _(Optional, host-specific)_ Wire the relay statusline and the auto-continue
> hooks into your own `~/.claude/settings.json` / `~/.codex/hooks.json`. `oma`
> never writes host config — the copy-paste snippets are in the README
> ["Wire the statusline and hooks"](../README.md#wire-the-statusline-and-hooks-optional)
> and the field reference is
> [reference/relay-v2-protocol.md](reference/relay-v2-protocol.md) §12.4. You can
> do the entire tutorial without this; it only removes manual "continue" nudges
> in stage 6.

---

## 3. Crystallize a spec with deep-interview

**Why:** a vague idea should become a reviewed spec before any code is written.
`oma interview` owns the math (ambiguity formula, weakest-target selection, the
threshold gate); the agent owns the questions and the scoring judgment. The
output is a spec marked `pending approval` — never direct implementation.

Tell the agent: _"deep interview me on `<your idea>`."_ Behind the skill, the
mechanical surface is:

```bash
# Start. --depth maps quick/standard/deep to thresholds 0.30/0.20/0.10.
oma interview start --depth deep --type greenfield --idea "<one-line summary>"

# Round 0 locks the component topology, then one question per round:
oma interview score --input round0.json --json
# ... agent asks a question, you answer, agent scores ...
oma interview score --input round3.json --json

# After each round, the gate decides whether ambiguity has dropped enough:
oma interview gate --json
```

**The ambiguity gate is the heart of this stage.** `oma interview gate` compares
the CLI-computed `current_ambiguity` against your threshold:

- **exit `4`** = not there yet. The JSON names the weakest component × dimension
  and the gap; the loop continues with another targeted question.
- **exit `0`** = ambiguity ≤ threshold. _Now_ the spec can be written.

When the gate passes, the agent writes the spec (goal / constraints /
**non-goals** / **decision boundaries** / acceptance criteria / topology /
ontology), records it, and closes the interview out:

```bash
oma interview crystallize --spec docs/specs/<your-feature>.md
oma interview complete    # only after you approve the spec
```

Check progress at any time (read-only) with `oma interview status --json`.

**Success looks like:** a `pending approval` spec file on disk and the interview
in `completed` state. The full state machine, scoring formula, and the
mandatory-content gate (a clean number with no stated non-goals is a "false
green") are in [reference/workflows.md](reference/workflows.md) §1.

> Stuck above threshold and want to stop anyway? That is an explicit, recorded
> decision, not a silent skip: `oma interview gate --waive --reason "<why>"`
> records the waiver as a warning, then crystallize with the gaps listed in the
> spec. Abandon entirely with `oma interview abort`.

---

## 4. Execute with autopilot

**Why:** autopilot drives the approved spec to a delivered result through five
phases — **clarify → plan → implement → verify → deliver** — persisting progress
in `oma state` so any interruption resumes cleanly. There is **no `oma
autopilot` command group** by design; it is pure markdown plus generic state.

Tell the agent: _"autopilot this spec."_ It initializes the run atomically and
binds the worktree (the mechanical guard you will exercise in stage 7):

```bash
# Initialize goal + phase in one patch so no reader sees a half-initialized run:
oma state patch autopilot --set goal="<one-line goal>" --set phase=clarify
oma state bind-worktree autopilot
```

Each phase transition records state, never retroactively:

```bash
oma state set autopilot/phase plan
oma state set autopilot/plan-path docs/specs/<your-feature>.md
oma state set autopilot/phase implement
# ... implement the plan top to bottom ...
oma state set autopilot/phase verify
oma state set autopilot/phase deliver
oma state set autopilot/phase done
```

Watch the live phase at any point:

```bash
oma state get autopilot/phase
oma state list autopilot --json     # all autopilot namespaces in this session
```

**Success looks like:** `oma state get autopilot/phase` walking forward through
the phases and ending at `done`, with a delivery summary from the agent. Note
that `oma state get` on a missing key exits `3` by design (fail-closed) — on a
fresh repo that simply means "no run yet."

Autopilot composes the other skills as **bounded subflows**: `clarify` may invoke
deep-interview (stage 3), `verify` may invoke ralph (stage 5), and `deliver` may
hand off to pair-delivery (stage 6). The phase contract and resume rules are in
[reference/workflows.md](reference/workflows.md) §3.

> _(Optional, host-specific)_ On Claude Code the `plan` phase can run in plan
> mode and independent `implement` steps can fan out to subagents. Codex runs the
> same phases inline — the state keys are identical either way.

---

## 5. Verify with a ralph loop

**Why:** when "done" means a verifier passes (and one attempt is not enough),
ralph iterates until the check is green — and stops **deterministically** when it
is exhausted or stalled, instead of looping forever. `oma` counts rounds and
judges the stop; **`oma` never runs your verifier** — that is a security
boundary. The agent runs the check and reports only the exit code.

```bash
oma ralph start --goal "go test ./... passes" --max-rounds 10 --stall-window 3
```

Each round is advance → work → check:

```bash
oma ralph next --json
# ... agent makes the smallest change toward green ...
# ... agent runs the verifier ITSELF and observes the real exit code ...
oma ralph check --verifier-exit 1 --note "TestParseLedger fails" --json
```

The `--note` is the stall detector's only input: the **same** signature for the
same failure, a **different** one when the failure changes.

**The deterministic stop is the point of this stage.** A terminal verdict from
`next` or `check` returns **exit `4`** and ends the loop — renegotiating it is
your call, not the agent's:

- **passed** (verifier exit 0): done; the agent reports how many rounds it took.
- **exhausted** (round > max-rounds): budget ran out; the agent names the
  closest-to-green state and asks you to raise the bound or change approach.
- **stalled** (`stall-window` consecutive identical `--note` signatures):
  repeating the same fix is disproven; the agent presents 2–3 genuinely
  different strategies.

```bash
oma ralph status --json     # current round, history, terminal state (read-only)
```

**Success looks like:** `oma ralph check --verifier-exit 0` flips the loop to
`passed` and `next` then reports the stop idempotently. The state machine and
the optional `score_improvement` keep-policy (stop on a score *plateau* instead
of a pass) are in [reference/workflows.md](reference/workflows.md) §2.

> _(Optional, host-specific)_ On hosts with a native `/goal` loop you can let the
> host auto-continue rounds; the round count and stop judgment still come from
> `oma ralph`, deterministic and identical across hosts (see the ralph skill's
> `/goal` note).

---

## 6. Cross-agent pair-delivery over the relay

**Why:** for a high-impact change, have a *second* agent review the work. The
`oma relay` ledger is an append-only, integrity-checked file ledger under
`<repo>/.oma/relay/`. Two agents (e.g. Claude Code as lead, Codex as auxiliary)
exchange artifacts through it. The flow has **two review gates** that the close
is fail-closed against.

This needs two sessions: open the same repo in **both** Claude Code and Codex.
Each side runs `oma relay` from its own host; the platform signal
(`CLAUDE_CODE_SESSION_ID` / `CODEX_THREAD_ID`) decides authorship automatically.

**Lead side** — initialize the ledger and create the pair (the creator becomes
`lead`):

```bash
oma relay init
oma relay pair new my-feature --json
```

`oma relay init` writes the v2 sentinel; `pair new` prints the **join command**
for the peer.

**Auxiliary side** — in the other host's session, bind to the same pair:

```bash
oma relay pair join 20260622-my-feature
oma relay pair show          # confirms roles: who is lead, who is auxiliary
```

Now the delivery moves through the gates. Each turn, a side orients on the latest
artifact (`oma relay status --json`) and publishes a reply. **Publishing is
always draft → publish:**

```bash
# Reserve the sequence and create the durable draft:
oma relay draft --kind plan --in-reply-to 0
# Write body + handoff prompt to files, then publish in one step:
oma relay publish .oma/relay/.../001-claude-plan.md \
  --body-file body.md --prompt-file next.md --touched src/foo.go
```

The gate sequence (full contract:
[reference/workflows.md](reference/workflows.md) §4):

1. **plan** — lead publishes `kind: plan` (scope, approach, acceptance criteria).
2. **plan review _(gate 1)_** — auxiliary publishes `kind: review` with a typed
   `--verdict approve|approve-with-changes|revise` and `--review-target <plan-seq>`.
   The review body **must** embed a fenced `oma-review-evidence/1` block or
   publish refuses it.
3. **implement** — lead does the work and publishes `kind: fix` listing every
   changed file via `--touched`, with fresh verification evidence.
4. **code review _(gate 2)_** — auxiliary reviews the actual changes and publishes
   `kind: review --verdict ...`. The lead records a disposition for **every**
   finding (adopt / partially adopt / reject) in the next reply.
5. **decision + close** — when both agree, the lead publishes `kind: decision`
   (the CLI auto-stamps a completion receipt) and then closes:

```bash
oma relay close --outcome approve --reason "feature delivered and cross-reviewed"
```

While waiting for the peer, the side that just published hands the turn over with
(it blocks silently and prints nothing until it exits):

```bash
oma relay wait --timeout 3600
```

`wait` exit codes: `0` new artifact (act on it), `10` peer silent past the
window, `11` peer crashed mid-turn, `12` pair terminal. _(With the optional Stop
hook from stage 2 wired, the host auto-continues the turn the moment the peer
publishes — no manual `wait` needed.)_

**Success looks like:** `oma relay close --outcome approve` succeeds (**exit
`0`**). The approve close is **fail-closed**: it refuses with **exit `4`** unless
a lead `kind: decision` with a valid receipt sits over a **non-lead** `approve`
review that targets the latest reviewed work — i.e. both gates genuinely passed
and no unreviewed work was published after the approval. A corrupt
receipt/evidence is **exit `3`**. (Dropping the pair instead? `--outcome
reject|abandon` needs no receipt.) The receipt/gate mechanics are
[reference/relay-v2-protocol.md](reference/relay-v2-protocol.md) §9.

---

## 7. Simulate a failure and resume

**Why:** workflows must survive an interrupted session, a crashed process, or
work that wandered into the wrong git worktree. Every stateful workflow is
resumable and **guards against silently running in the wrong place**.

**Resume an interrupted workflow.** State lives on disk, so a killed session
resumes by reading it back. The first move on any restart is to probe:

```bash
oma interview status --json     # or: oma ralph status --json
oma state get autopilot/phase   # exit 3 here just means "no run yet"
```

Then continue from the reported phase — for an interview,
`oma interview start --resume` shows the existing run without modifying it; for
ralph, resume the `next → work → check` cycle; for autopilot, resume from the
phase the state reports.

**The worktree guard.** Autopilot and ralph record the worktree (and branch)
they started on and **refuse to mutate from a different one** — this is what
stops a resume from corrupting state by running against the wrong checkout:

```bash
# Autopilot: this exits 3 if you are in a different worktree than the bound one.
oma state check-worktree autopilot

# Ralph: next/check/abort refuse from another worktree; status stays read-only.
oma ralph status --allow-worktree-change --json
```

**Recover deliberately.** When you genuinely moved the work (e.g. into a fresh
`.worktrees/<branch>`), re-bind explicitly rather than forcing past the guard:

```bash
oma state bind-worktree autopilot          # re-bind autopilot to here
oma ralph rebind-worktree                  # re-point the ralph loop here
```

**Re-run a phase.** Nothing is one-way. If `implement` went wrong, set the phase
back and redo it — `oma state` writes are atomic and serialize concurrent
writers, so no reader sees a half-written transition:

```bash
oma state set autopilot/phase implement
```

For the relay, a crashed peer mid-turn surfaces as `oma relay wait` **exit
`11`**, and an interrupted `publish` is recoverable: re-running the same
`oma relay publish` re-renders from the still-present draft and completes the
missing sidecars (recovery contract:
[reference/relay-v2-protocol.md](reference/relay-v2-protocol.md) §7).

**Success looks like:** after killing and restarting, the status command reports
the exact phase you left, the worktree guard exits `3` from the wrong place
(and `0` after a re-bind), and re-running a phase or an interrupted publish
converges instead of duplicating or corrupting work.

---

## 8. Inspect the on-disk state

**Why:** everything `oma` persists is plain JSON and a readable ledger tree under
the repo — auditable, greppable, and debuggable by hand. A linked git worktree
resolves back to the primary checkout, so **one repository has one `.oma/`**.

```bash
ls -R .oma
```

**Workflow state — `.oma/state/*.json`** (schemas:
[reference/schemas.md](reference/schemas.md)):

- `interview-<id>.json` — phase, threshold + its source, per-round scores,
  `current_ambiguity`, the topology, any `gate_waiver`, and the `spec_path`.
- `ralph-<id>.json` — phase, `goal`, `round`/`max_rounds`, the `checks[]` history,
  the recorded `worktree_root`/`branch`, and (under `score_improvement`)
  `best_round`/`best_score`.
- `autopilot--s-<session>.json` — the `phase`/`goal`/`plan-path` keys, plus the
  bound worktree.

What to look for: the **`revision`** field (monotonic; how concurrent writes are
serialized), the recorded **worktree/branch** (what the stage-7 guard checks
against), and a single-generation **`.bak`** sidecar written before any mutation.
List and validate every workflow instance at once with `oma workflow list --json`
(`--all-sessions` for the project-admin view).

**Relay ledger — `.oma/relay/`** (layout:
[reference/relay-v2-protocol.md](reference/relay-v2-protocol.md) §3):

- `.oma-relay-v2` — the v2 sentinel (a missing sentinel over a non-empty dir is
  fail-closed).
- `<pair-slug>/session.json` — pair metadata and `roles` (who is `lead`).
- `<pair-slug>/NNN-<author>-<kind>.md` — the published artifacts, **append-only**,
  each with a `.sha256` integrity sidecar and a `.ready` marker. **A file with no
  `.ready` is unpublished and ignored by every reader** — that is the sole
  publication criterion.
- `_archive/<pair-slug>/` — where a closed pair is moved, read-only.

What to look for: artifacts with a `.sha256` but **no `.ready`** (an interrupted
publish — stage 7), and any `.draft/` / `.seq/` residue (stale leftovers).
Diagnose, don't hand-edit: `oma relay status --json` lists residue first, and
`oma doctor relay --clean-stale` is the only thing that should clean it. Readers
verify `.sha256` on every read and **fail closed** on a mismatch (no content is
returned), so never edit a `.ready` artifact by hand — corrections go through a
new `kind: correction` artifact.

---

## 9. Lifecycle: update, migrate, rollback, uninstall

**Why:** keeping the binary, the assets, and the on-disk schemas current —
and cleanly removing everything — are first-class, reversible operations.

**Update the binary in place** (checksum-verified, atomic, with automatic
rollback on failure):

```bash
oma self-update --check     # strictly read-only: just compares versions
oma self-update             # performs the update (prepend --dry-run to preview)
```

**Migrate on-disk state after an upgrade.** A newer `oma` may carry one-time
migrations for state or relay layouts written by an older version. These are
**dry-run by default** — inspect first, then `--apply`:

```bash
oma doctor                                       # reports any legacy/leftover items
oma doctor state --migrate-session-scope         # preview state-scope migration
oma doctor state --migrate-session-scope --apply # apply it (originals backed up)
oma doctor relay --migrate                        # preview relay pair migration
oma doctor relay --migrate --apply                # apply it
```

`oma doctor relay` also restores an archived pair (`--restore <slug>`) and cleans
stale relay residue (`--clean-stale`).

**Update or roll back assets.** Refresh installed skills, and undo a bad update
from the automatic backups:

```bash
oma asset update                          # update all installed assets (alias: oma update)
oma asset rollback ralph                  # restore ralph from its most recent backup
oma asset rollback ralph --to <backup-id> # or a specific backup
```

**Uninstall.** Removal only ever touches oma-managed files (a registry tracks
ownership, so foreign files are never deleted):

```bash
oma asset remove deep-interview ralph autopilot pair-delivery
oma asset list --installed     # confirm the catalog is empty
```

**Success looks like:** `self-update` reports the new version (or "already
latest" on `--check`), each migration prints what it changed under `--apply`,
`rollback` restores the prior asset content, and after `asset remove` the
projections are gone from both `~/.claude/` and `~/.codex/` with no foreign
files touched. The full lifecycle surface is
[reference/command-tree.md](reference/command-tree.md) §2, §4, §7.

> The on-disk `.oma/` state and ledger in your repo are independent of the binary
> and assets — uninstalling the skills does not delete a repo's `.oma/`. Remove
> that yourself if you want a truly clean repo.

---

## Where to go next

- **[../README.md](../README.md)** — the project overview, the full install
  matrix (Windows, source builds, tag pinning), and the optional statusline /
  hooks wiring snippets.
- **[reference/command-tree.md](reference/command-tree.md)** — every command
  signature, `--json` contract, and exit code.
- **[reference/workflows.md](reference/workflows.md)** — the interview / ralph /
  autopilot / pair-delivery state machines and delivery gates.
- **[reference/relay-v2-protocol.md](reference/relay-v2-protocol.md)** — the relay
  ledger protocol, the fail-closed approve gate, and the experience layer.
- **[reference/security-contract.md](reference/security-contract.md)** — the
  fail-closed security model behind every refusal you saw above.
- **[design-philosophy.md](design-philosophy.md)** — the *why*: context scarcity
  and the mechanical-vs-judgment split that shapes every command.

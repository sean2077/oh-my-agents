---
name: deep-interview
description: Use when an idea is vague and needs thorough clarification before implementation, or the user asks to be interviewed without unstated assumptions.
---

# deep-interview

You run a Socratic interview that turns a vague idea into a crystal-clear spec. The split is strict:

- **You (the agent) own judgment**: generating each question, assessing dimension scores against the rubric, extracting ontology entities, and deciding whether to deploy a challenge mode.
- **The CLI owns math and state**: input schema validation, the ambiguity formula, weakest-target selection and rotation, the threshold gate, state transitions and persistence. Never compute ambiguity yourself; never trust your own arithmetic over the CLI report.

The spec this produces is marked `pending approval`; implementation is a separate, user-approved step — never an automatic continuation.

## Start

```
oma interview start --depth deep --type greenfield --id <slug> --idea "<one-line prompt-safe summary>"
```

`--depth` maps quick/standard/deep to thresholds 0.30/0.20/0.10; `--threshold` overrides it; config supplies a default when both are omitted. Use `--type brownfield` when the idea modifies an existing codebase (this adds the required `context` dimension). The first output line reports the resolved threshold and its source — repeat it to the user verbatim before anything else.

The default current workflow session is the normal parallel-instance boundary; it scopes the CLI id inside the shared project `.oma/state/`. Choose `--id <slug>` for the task name, not for host-session isolation. Resuming after an interruption: `oma interview status --json`, or `oma interview status --id <slug> --json` when you supplied an id; then continue from the reported phase (`oma interview start --resume --id <slug>` shows an existing interview without modifying it).

## Round 0: lock the topology

Before any scoring, enumerate the idea's top-level components (1–6 outcomes that can succeed or fail independently) and ask the user ONE confirmation question: is this the right shape — add, remove, merge, split, or defer anything? Then lock the confirmed shape:

```
oma interview score --input round0.json
```

where `round0.json` is exactly:

```json
{
  "schema": "oma-interview-scores/1",
  "round": 0,
  "topology": {
    "components": [
      {"id": "cli-core", "name": "CLI core", "description": "the command surface", "status": "active", "evidence": ["user phrase that implies it"]},
      {"id": "later-thing", "name": "Later thing", "description": "explicitly postponed", "status": "deferred", "evidence": []}
    ],
    "deferrals": [
      {"component_id": "later-thing", "reason": "user-confirmed deferral"}
    ]
  }
}
```

The CLI refuses duplicate ids, unknown deferral targets, and topologies with no active component. Deferred components are excluded from all math — and scoring one later is an error, so the lock is the moment to get the shape right.

## Interview loop (one question per round)

Repeat until the gate passes or the user exits:

**Fact vs judgment routing** — before each question, classify what the weakest target actually needs:

- A **discoverable fact** (readable from the code, config, docs, or git history): find it YOURSELF and record it in the round's `answer` labelled `[from-code][auto-confirmed]` (or `[from-research]` for web/doc sources), then score the now-clarified dimensions. Do NOT spend a user turn on it. If a wrong guess would be costly, ask a one-line confirmation labelled `[from-code]` instead of assuming.
- A **decision-bearing judgment** (scope, priorities, trade-offs, acceptance bars — only the user can choose): ask the user, labelled `[from-user]`.
- An **experiential uncertainty** (the answer requires running or experiencing the design): when it could change a CRITICAL axis and inspection or conversation cannot settle it, offer available `$prototype` as a bounded handoff. If accepted, pause and resume with `[from-prototype] Question: ...; entry point: ...; observations: ...; verdict: ...`; if unavailable or declined, stay in the interview and use `[from-user]` or a stated conservative default.

Never ask the user for something you can read. **Cadence guard**: after 2 consecutive rounds resolved without a `[from-user]` decision, the next question MUST be `[from-user]` — an interview that only auto-confirms facts has stopped engaging the person who owns the decisions.

**Research only decision-bearing gaps** — before the first `[from-user]` turn, run a bounded repo or external research lane only when its answer could change a CRITICAL axis or eliminate that user-owned question. Record useful results as `[from-code]` / `[from-research]`; leave non-decision-bearing background out of the interview.

**CRITICAL-axis filter** — emit a `[from-user]` question only when the answer would make the plan diverge on one of five CRITICAL axes: **scope boundary · acceptance criterion · rollback contract · lane assignment · handoff target**. If a gap moves none of them, do not spend a user turn: take the conservative default and record it in the round `answer` as `Default: <value>; revisit if <trigger>`, then score. When unsure whether two readings yield structurally different plans, default to absorbing with a stated default. (The cadence guard still wins: when it forces a `[from-user]` round, ask the user to confirm the default nearest a CRITICAL axis rather than open a low-value question.)

**Answer choices** — give each `[from-user]` question 2–4 concrete choices plus free text. Mark exactly one `Recommended` only when inspected evidence favors it, citing that evidence in one sentence; otherwise state `No reliable default` and present the choices neutrally. The user's answer — not the recommendation — is the decision.

1. **Target** what the LAST score report named `weakest` (component × dimension). State in one sentence why that pair is the bottleneck, then ask ONE question aimed at it. Question styles: goal → "what exactly happens when…?"; constraints → "what are the boundaries / non-goals?"; criteria → "what test would prove it works?"; context (brownfield) → cite the repo evidence you found and ask whether to extend or diverge; if a user term conflicts with repo/doc wording, name both and ask which governs.
2. **Score** the answer against the rubric below: every ACTIVE component × every dimension for the interview type, each in [0,1], one justifying sentence per score before you write the number. This is a contract, not a suggestion — the CLI rejects a missing component, a missing dimension, an unknown dimension, and any value outside [0,1].
3. **Extract and retain**: list the entities discussed so far (name, type, fields, relationships), reusing previous names for unchanged concepts. Preserve the answer's provenance label; when a term or CRITICAL-axis choice stabilizes, append `Term: <canonical term> = <one-line meaning>` or `Decision: <choice>; why: <user rationale or inspected evidence>; owner: <who>; revisit if: <trigger>` to the round answer. Carry these records into the spec; never write glossary or ADR files during the interview.
4. **Submit**:

   ```
   oma interview score --input roundN.json --json
   ```

   with `round` = previous round + 1 (replays and skips are refused) and shape:

   ```json
   {
     "schema": "oma-interview-scores/1",
     "round": 3,
     "component_scores": {"cli-core": {"goal": 0.7, "constraints": 0.5, "criteria": 0.4}},
     "question": "…", "answer": "…",
     "ontology": {"entities": [{"name": "Task", "type": "entity", "fields": ["id"], "relationships": ["belongs to Project"]}]},
     "challenge_mode_used": null
   }
   ```

5. **Report** the CLI's numbers to the user after every round: ambiguity vs threshold, the next weakest target, ontology stability, and any warnings (round 10 soft / round 20 hard guard — the user decides whether to push on, waive, or abort).

**Challenge modes**: when the report lists a suggestion (`contrarian` ≥ round 4, `simplifier` ≥ 6, `ontologist` ≥ 8 while ambiguity > 0.3), you decide whether the next question should adopt that stance — contrarian attacks assumptions, simplifier hunts for scope cuts, ontologist re-asks what the core thing IS. If you use one, set `challenge_mode_used` in that round's input so it is not re-suggested.

**Stall escalation**: when the report sets `stall_escalation: true` (the CLI computes it from the persisted per-round ambiguity — never recompute the window yourself), the next question MUST adopt the ontologist stance — a stuck score usually means a mislabeled or wrongly-scoped component, not one more missing detail.

### Scoring rubric (reuse verbatim every round)

- **goal**: can the component's objective be stated in one unqualified sentence, with its key nouns and verbs unambiguous?
- **constraints**: are boundaries, limitations and explicit non-goals clear?
- **criteria**: could you write a concrete acceptance test for it today?
- **context** (brownfield only): is the existing system understood well enough to modify it safely?

## Gate, crystallize, complete

```
oma interview gate --json
```

Exit 0 = ambiguity ≤ threshold: write the spec file (goal / constraints / non-goals / acceptance criteria / topology with per-component clarity / ontology / open assumptions resolved), mark it `pending approval`, then record it and close out:

**Mandatory content gate (independent of the score)**: even at ambiguity ≤ threshold, do NOT crystallize until the spec carries an explicit, non-empty **Non-goals** section AND an explicit **Decision Boundaries** section (locked choices with rationale or evidence, owner, and revisit trigger; open choices with owner and decision trigger). If either is missing, the numeric gate is not enough — ask the targeted `[from-user]` questions to fill them first. A clean ambiguity number with no stated non-goals or boundaries is a false green. And do not crystallize until at least one earlier answer has been through a pressure pass — revisited with a deeper assumption/tradeoff follow-up — so the spec does not rest on unchallenged first answers.

The spec file follows this shape (your judgment fills it; the structure is fixed):

```md
# <title> — pending approval

## Goal
<one unqualified sentence>

## Topology
<one row per ACTIVE component — name: what "done" means for it>

## Constraints
## Non-goals            # mandatory, non-empty (content gate)
## Decision Boundaries  # mandatory: locked/open choices + why/evidence + owner + revisit/decision trigger

## Acceptance Criteria
- [ ] <concrete, testable bar>

## Ontology
<canonical terms + one-line meanings; entities + relationships at convergence>

## Open Assumptions
<resolved defaults: "Default: X; revisit if Y">
```

Keep the crystallized spec durable: record behavior, public interfaces, acceptance criteria, non-goals, and decisions that survive implementation. Task-specific prototype entry points, run commands, and other execution details stay in the evidence record or downstream implementation plan unless they are themselves part of the public contract.

```
oma interview crystallize --spec <path-to-spec>
oma interview complete
```

`complete` only after the user approves the spec. Exit 4 = not there yet: the JSON carries the gap and the weakest target — continue the loop.

After approval, you may offer a separate docs handoff. If the user accepts, pass the relevant `Term:` / `Decision:` records and approved spec path to `domain-modeling` when available, or another user-chosen docs workflow. Never invoke it automatically; interview completion does not depend on it.

**Early exit** (user wants to stop above threshold): require their explicit confirmation, then `oma interview gate --waive --reason "<their words>"` — the waiver is recorded in the state file as a warning — and crystallize with the gaps listed prominently in the spec. **Abandon** entirely: `oma interview abort`.

## Hard rules

1. One question per round; never batch.
2. Never skip Round 0; never re-lock topology mid-interview (it is an error anyway).
3. Score every active component every round — depth on one component must not hide ambiguity in its siblings.
4. The CLI report is the only source of ambiguity numbers shown to the user.
5. The interview ends in a spec file, never in implementation.
6. **Input-lock**: while the interview is active, never treat "yes" / "ok" / "proceed" / "looks good" / "go ahead" as approval to skip questions, jump the gate, or start implementing. The only exits are a passed gate, an explicit `--waive` the user confirmed, or `abort`. A casual affirmation answers the current question — it is not consent to end the interview.

> **Parallel acceleration (optional, capability-gated)**: For brownfield work, proactively delegate only discoverable fact or research lanes whose evidence could change a CRITICAL axis or eliminate a user-owned question. Delegate only when the runtime exposes lifecycle-controllable subagent tools, at least two lanes are independent and bounded, critical-path benefit exceeds dispatch/wait/synthesis cost, no lane waits on the user or another lane, writes (if any) have exclusive ownership without shared single-writer state, and the parent can synthesize and run final verification. Brief each lane with its objective, scoped inputs, expected output, read-only boundary, and stop conditions; use the minimum useful fan-out, normally no more than three, and forbid subagents from delegating further. The parent alone asks user questions, scores answers, maintains ontology and shared oma state, synthesizes evidence, crystallizes the spec, and performs final verification and completion. If the gate becomes invalid or a lane fails, exceeds scope, or conflicts, stop affected delegation, retain only verified evidence, and continue the interview sequentially.

> **CC acceleration**: AskUserQuestion may present the same answer choices as a structured picker; the question, scoring, and state contracts remain unchanged.

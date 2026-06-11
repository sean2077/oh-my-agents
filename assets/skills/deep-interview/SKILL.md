---
name: deep-interview
description: Socratic requirements crystallization with deterministic ambiguity gating via oma interview. Use when an idea is vague and needs thorough clarification before any implementation — user says deep interview, interview me, ask me everything, don't assume, 我有个模糊的想法. Output is a spec file, never direct implementation.
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

Resuming after an interruption: `oma interview status --json`, then continue from the reported phase (`oma interview start --resume --id <slug>` shows an existing interview without modifying it).

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
      {"id": "cli-core", "name": "CLI core", "description": "…", "status": "active", "evidence": ["user phrase that implies it"]}
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

1. **Target** what the LAST score report named `weakest` (component × dimension). State in one sentence why that pair is the bottleneck, then ask ONE question aimed at it. Question styles: goal → "what exactly happens when…?"; constraints → "what are the boundaries / non-goals?"; criteria → "what test would prove it works?"; context (brownfield) → cite the repo evidence you found and ask whether to extend or diverge.
2. **Score** the answer against the rubric below: every ACTIVE component × every dimension for the interview type, each in [0,1], one justifying sentence per score before you write the number. This is a contract, not a suggestion — the CLI rejects a missing component, a missing dimension, an unknown dimension, and any value outside [0,1].
3. **Extract ontology**: list the entities discussed so far (name, type, fields, relationships), reusing previous names for unchanged concepts so the stability ratio means something.
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

```
oma interview crystallize --spec <path-to-spec>
oma interview complete
```

`complete` only after the user approves the spec. Exit 4 = not there yet: the JSON carries the gap and the weakest target — continue the loop.

**Early exit** (user wants to stop above threshold): require their explicit confirmation, then `oma interview gate --waive --reason "<their words>"` — the waiver is recorded in the state file as a warning — and crystallize with the gaps listed prominently in the spec. **Abandon** entirely: `oma interview abort`.

## Hard rules

1. One question per round; never batch.
2. Never skip Round 0; never re-lock topology mid-interview (it is an error anyway).
3. Score every active component every round — depth on one component must not hide ambiguity in its siblings.
4. The CLI report is the only source of ambiguity numbers shown to the user.
5. The interview ends in a spec file, never in implementation.

> **CC acceleration (optional, Claude Code only)**: questions may be presented through the structured option picker (AskUserQuestion) with 2–4 contextual options plus free text, and brownfield evidence may come from a parallel Explore subagent. Codex and other hosts ask the same questions as plain text and inspect the repo inline — the scores contract and the ledger of state are identical either way.

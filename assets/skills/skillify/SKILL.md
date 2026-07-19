---
name: skillify
description: Use when a just-performed workflow appears likely to recur across sessions or projects and may justify the resident cost of an oma skill.
---

# skillify

Turn a workflow you just performed into a reusable oma skill — a `SKILL.md` + `manifest.json` under `assets/skills/<name>/` — but only when it clears the quality gate. The goal is a small, high-value catalog, not hoarding every one-off.

Use [`docs/skill-authoring.md`](../../../docs/skill-authoring.md) when writing or reviewing the skill text. In particular, enforce `description = WHEN, not WHAT`: it begins with `Use when` followed by one space, names only the trigger, and never summarizes the workflow.

## Quality gate (all four must hold)

Before writing anything, answer honestly:

1. **Reusable?** Will this recur across sessions/projects, or was it a one-off? One-offs do not become skills.
2. **General?** Does it encode a *workflow* (judgment, sequence, stop conditions), not a single concrete task? A skill is a method, not a memory.
3. **Worth the resident cost?** Every installed skill's name+description is always-loaded context (measure it with `oma doctor budget`). Is the recurring value worth that tax? If unsure, it is not.
4. **Behavior-changing?** Can every proposed section be tied to a trigger, decision, action, artifact, stop condition, or verifier? General advice with no observable effect is documentation at best, not skill guidance.

If any answer is no, record the result as a note or memory instead, then stop without creating a skill.

## Optional efficacy gate

Use this only for discipline skills where agents have a realistic incentive to rationalize around the rule. It is not required for reference skills or simple workflow captures.

1. **RED: no-guidance control.** Run a fresh-context pressure scenario without the candidate skill. If the control does not fail, stop; there is no demonstrated behavior to correct.
2. **Capture rationalizations.** Record the exact excuses, shortcuts, or shape failures the agent used, based on the run rather than a reconstruction from memory.
3. **GREEN: add the smallest guidance that blocks those failures.** Address the observed rationalizations, not hypothetical ones.
4. **REFACTOR: pressure-test variants.** Re-run with the candidate skill, look for new loopholes, and tighten wording only where evidence shows drift.
5. **Track variance.** If repeated runs produce many different interpretations, the form is not binding yet. Prefer a clearer recipe or output contract over more prohibitions.

For expensive tests, run the tally as a `research-mission` driven by `ralph` rather than adding CLI surface.

## Extract the method first

Before writing, distill what you just did into the reusable shape: the trigger / inputs, the ordered steps, the judgment at each, the stop conditions, and how you verified success. If you cannot name those, the workflow is not abstracted enough to skillify yet.

## Write the skill

1. **Name** — lowercase-kebab, unique in the catalog. Check existing names and intents first:

   ```
   oma asset catalog
   ```

   Extend an existing skill or pick a sharper boundary when the purpose overlaps; avoid creating a duplicate trigger.
2. **SKILL.md** with frontmatter:

   ```
   ---
   name: <name>
   description: Use when <one-line trigger, within the manifest budget>
   ---
   ```

   Reject descriptions that do not begin with `Use when` followed by one space or that summarize the workflow. The rest of the description names the trigger situation, input, or boundary that should load the skill. Put the actual method in the body.

   Body = the workflow ONLY: the steps, the judgment at each, the hard rules, the stop conditions. Put the main path and invariants before exceptions, lead with the desired action or artifact, and use prohibitions only for hard boundaries that cannot be expressed positively; every prohibition must name the concrete alternative or recovery action. Prune any sentence whose removal changes no trigger, decision, action, artifact, stop, or verifier. Keep durable behavior, interfaces, acceptance criteria, non-goals, and decisions in the skill or durable spec; put task-specific paths and commands in the execution plan unless they are part of the public contract. Move optional detail to one-hop references loaded only when needed. Keep the default path agent-neutral (plain `oma` commands + markdown); mark capability-gated parallel acceleration separately from genuinely Claude-Code-only affordances. Installation/troubleshooting goes to docs, never the skill body.
3. **manifest.json**:

   ```
   {"schema":"oma-asset/1","name":"<name>","type":"skill","targets":["claude","codex"],"description_budget_tokens":80}
   ```

## Verify before declaring done

- The description begins with `Use when` followed by one space, says WHEN rather than WHAT, and fits `description_budget_tokens`. Run `oma doctor budget` after installing.
- Every `oma` command the skill names actually exists (so refcheck passes).
- The body carries workflow, not prose padding.
- The first words of headings and steps expose the controlling action or branch; mandatory behavior is not buried in rationale.
- Desired behavior appears before a prohibition; every prohibition closes a hard boundary and names the concrete alternative or recovery action.
- Main path precedes exceptions, references are one hop away, and no rule is duplicated across the body and a reference.
- Add an expected trigger case to `eval/cases/triggering.jsonl` (plus a real overlapping boundary when relevant); fixture labels are not efficacy evidence.

## Hard rules

1. The quality gate is not optional — a skill that fails it pollutes everyone's resident context.
2. `description = WHEN, not WHAT` — begin with `Use when` followed by one space and reject workflow summaries in resident trigger text.
3. Workflow only in the body; platform/install/troubleshooting → docs.
4. Agent-neutral default path; mark capability-gated parallel and genuinely CC-only paths separately and explicitly.
5. Extend an existing skill or sharpen the trigger boundary instead of duplicating its intent.

> **Parallel acceleration (optional, capability-gated)**: During the optional efficacy gate, proactively delegate independent fresh-context trials within the same RED, GREEN, or REFACTOR stage; never parallelize stages that depend on prior outcomes. Delegate only when the runtime exposes lifecycle-controllable subagent tools, at least two lanes are independent and bounded, critical-path benefit exceeds dispatch/wait/synthesis cost, no lane waits on the user or another lane, writes (if any) have exclusive ownership without shared single-writer state, and the parent can synthesize and run final verification. Brief each lane with its objective, scoped inputs, expected output, read/write boundary, and stop conditions; use the minimum useful fan-out, normally no more than three, and forbid subagents from delegating further. The parent alone asks user questions, owns the control and shared oma state, compares rationalizations and variance, writes the candidate skill, and performs final verification and completion. If the gate becomes invalid or a lane fails, exceeds scope, or conflicts, stop affected delegation, retain only verified evidence, and continue the efficacy workflow sequentially.

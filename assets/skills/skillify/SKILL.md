---
name: skillify
description: Use when a just-performed workflow may be reusable enough to become an oma skill.
---

# skillify

Turn a workflow you just performed into a reusable oma skill — a `SKILL.md` + `manifest.json` under `assets/skills/<name>/` — but only when it clears the quality gate. The goal is a small, high-value catalog, not hoarding every one-off.

Use [`docs/skill-authoring.md`](../../../docs/skill-authoring.md) when writing or reviewing the skill text. In particular, enforce `description = WHEN, not WHAT`: a skill description is resident trigger text, not a summary of the workflow.

## Quality gate (all three must hold)

Before writing anything, answer honestly:

1. **Reusable?** Will this recur across sessions/projects, or was it a one-off? One-offs do not become skills.
2. **General?** Does it encode a *workflow* (judgment, sequence, stop conditions), not a single concrete task? A skill is a method, not a memory.
3. **Worth the resident cost?** Every installed skill's name+description is always-loaded context (measure it with `oma doctor budget`). Is the recurring value worth that tax? If unsure, it is not.

If any answer is no, stop — record it as a note or memory instead, not a skill.

## Optional efficacy gate

Use this only for discipline skills where agents have a realistic incentive to rationalize around the rule. It is not required for reference skills or simple workflow captures.

1. **RED: no-guidance control.** Run a fresh-context pressure scenario without the candidate skill. If the control does not fail, stop; there is no demonstrated behavior to correct.
2. **Capture rationalizations.** Record the exact excuses, shortcuts, or shape failures the agent used. Do not generalize from memory.
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

   Don't collide or duplicate another skill's purpose — extend it or pick a sharper boundary.
2. **SKILL.md** with frontmatter:

   ```
   ---
   name: <name>
   description: <one line, ≤ ~80 tokens: WHEN to use this skill, not WHAT it does>
   ---
   ```

   Reject descriptions that summarize the workflow. The description must name the trigger situation, input, or boundary that should load the skill. Put the actual method in the body.

   Body = the workflow ONLY: the steps, the judgment at each, the hard rules, the stop conditions. Keep it agent-neutral (plain `oma` commands + markdown); mark any Claude-Code-only acceleration as a clearly-optional block. Installation/troubleshooting goes to docs, never the skill body.
3. **manifest.json**:

   ```
   {"schema":"oma-asset/1","name":"<name>","type":"skill","targets":["claude","codex"],"description_budget_tokens":80}
   ```

## Verify before declaring done

- The description says WHEN to use the skill, not WHAT the workflow does, and is genuinely ≤ ~80 tokens (it is resident on every session). Run `oma doctor budget` after installing.
- Every `oma` command the skill names actually exists (so refcheck passes).
- The body carries workflow, not prose padding.

## Hard rules

1. The quality gate is not optional — a skill that fails it pollutes everyone's resident context.
2. `description = WHEN, not WHAT` — reject workflow summaries in resident trigger text.
3. Workflow only in the body; platform/install/troubleshooting → docs.
4. Agent-neutral default path; any CC-only path clearly marked optional.
5. Never duplicate an existing skill's intent.

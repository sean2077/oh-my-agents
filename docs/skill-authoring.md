# Skill Authoring

This guide is for writing or reviewing oma skills. It is not a skill itself, so it adds no resident context. The job of a skill is to put the right judgment in front of the agent at the right time, while keeping mechanical work in `oma` and installation or troubleshooting detail in docs.

Start from `skillify` when a workflow might deserve a skill. Use this page to shape the text once the workflow has passed that gate.

## 1. Match the form to the failure

First name the failure you are trying to prevent. The fix depends on the failure shape.

| Failure shape | Use this form | Avoid |
| --- | --- | --- |
| Wrong output shape | A recipe, schema, checklist, or example contract. | A pile of "do not" rules. |
| Skipped or softened discipline | A hard rule, STOP condition, verifier, or required evidence. | A vague reminder to be careful. |
| Ambiguous trigger | A sharper description and explicit boundaries. | More body text the agent may never load. |
| Workflow drift | Ordered steps, state transitions, and stop conditions. | Narrative explanation without an observable next action. |
| Bloated skill body | Move platform setup, examples, and troubleshooting to docs. | Making resident text carry reference material. |

Prohibitions are useful for discipline failures, where the agent already knows the required shape but is tempted to skip, soften, or rationalize around it. They are weak for wrong-shape failures; use a concrete form instead.

## 2. `description = WHEN, not WHAT`

The description is resident trigger text. It must answer: **when should the agent use this skill?** It must not summarize the workflow inside the skill.

Why this matters: if the description describes the process, an agent can treat it as the whole instruction and skip the body. The body is where the actual workflow lives.

Rules:

- Start with `Use when...` or an equivalent trigger.
- Name the situation, input, or decision boundary that should load the skill.
- Do not include ordered steps, implementation detail, or a compressed workflow.
- Keep it within the manifest's `description_budget_tokens`.
- If the trigger cannot be stated crisply, the skill is probably not ready.

Examples:

| Weak: WHAT | Stronger: WHEN |
| --- | --- |
| `Run plan, review, implement, review, and decision gates through the relay ledger.` | `Use when a code change needs cross-agent delivery with explicit review gates.` |
| `Create a mission spec, evaluator contract, and candidate ledger for optimization.` | `Use when optimizing a measurable score or benchmark under a fixed budget.` |
| `Analyze files and synthesize ranked findings with evidence and confidence.` | `Use when a repo-local question needs read-only, cross-file analysis before edits.` |

## 3. Use persuasion deliberately

Skill text is behavioral design. Use only the persuasion pressure that fits the skill type, and do not use social pressure where evidence discipline is required.

| Skill type | Useful pressure | Do not use |
| --- | --- | --- |
| Reference or orientation | Authority: point to the canonical doc, schema, or local convention. | Heavy process language. |
| Workflow execution | Consistency: make each step follow from the previous artifact or verifier. | Aspirational phrasing with no next action. |
| Budget or minimalism | Scarcity: remind authors that resident context is limited and measured. | Broad "more guidance is safer" arguments. |
| Review, security, verification, or cleanup discipline | Authority + consistency: bind claims to evidence, file lines, commands, and gates. | Liking or reciprocity. Do not ask the agent to be agreeable, grateful, polite, or deferential as a substitute for verification. |
| Collaboration and review reception | Evidence disposition: verify each item, accept or reject with reason, and record the result. | Sycophantic language or automatic agreement with the reviewer. |

For discipline skills, ban liking and reciprocity outright. They reward appeasement, not correctness. The desired behavior is evidence-bound: verify, decide, and record.

## 4. Close the loopholes

Discipline skills need explicit loophole handling because agents tend to rationalize shortcuts under pressure. Add this only where the failure is real; do not pad every skill with generic warnings.

### Rationalization table

Use a table when you have seen recurring excuses or shortcuts:

```markdown
| Rationalization | Required response |
| --- | --- |
| "This is a small change, so no verifier is needed." | Run the smallest relevant verifier, or state why no verifier exists. |
| "The reviewer is probably right." | Verify the finding before changing code, then record accept / reject / partial with reason. |
| "The output has the same spirit even if the format differs." | Produce the required format exactly, then add commentary outside the contract if useful. |
```

### Red Flags / STOP block

Use a STOP block when continuing would make the result untrustworthy:

```markdown
## Red Flags / STOP

Stop and re-read this skill if:

- You are about to claim completion without fresh verifier evidence.
- You are changing the requested output shape to something more convenient.
- You are accepting another agent's finding without checking it.

When a flag fires, name the flag, return to the required step, and do not claim completion until the gate passes.
```

### Letter equals spirit

If the text can be followed literally while defeating its purpose, rewrite it. Prefer observable contracts over moral appeals:

- Weak: `Verify carefully before finishing.`
- Stronger: `Do not claim completion until a fresh verifier result from this turn is recorded.`
- Weak: `Keep the skill concise.`
- Stronger: `Move install, setup, troubleshooting, and long examples to docs; keep only trigger, workflow, judgment, and stop conditions in the skill body.`

Close loopholes with a required artifact, command result, state transition, or explicit stop condition. If no observable boundary exists, the rule is probably too vague.

## Authoring checklist

- The description says WHEN to use the skill, not WHAT the skill does.
- The body carries workflow judgment only: steps, decision points, hard rules, stop conditions, and verification.
- The form matches the failure: recipe for shape, prohibition or STOP gate for discipline.
- Discipline skills name likely rationalizations and block them.
- Agent-neutral behavior is the default; host-specific acceleration is clearly optional.
- Installation, platform setup, troubleshooting, and long examples live in docs, not resident skill text.

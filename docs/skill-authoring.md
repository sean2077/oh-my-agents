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

Lead with positive steering: state the desired action, artifact, or behavior first. Use a prohibition only for a hard boundary that cannot be expressed clearly as a positive requirement, and pair it with the concrete alternative or recovery action. Discipline failures can still require a hard rule or STOP gate; name the required behavior first, then close the shortcut the gate exists to prevent. For wrong-shape failures, use a recipe or output contract instead.

## 2. `description = WHEN, not WHAT`

The description is resident trigger text. It must answer: **when should the agent use this skill?** It must not summarize the workflow inside the skill.

Why this matters: if the description describes the process, an agent can treat it as the whole instruction and skip the body. The body is where the actual workflow lives.

Rules:

- Begin with the exact words `Use when` followed by one space; the release fixture enforces this uniform trigger shape.
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

Discipline skills need explicit loophole handling because agents tend to rationalize shortcuts under pressure. Add this only where the failure is real, and make each warning point back to a concrete required response rather than padding every skill with generic prohibitions.

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
- Stronger: `Record a fresh verifier result from this turn before claiming completion; never claim completion without it.`
- Weak: `Keep the skill concise.`
- Stronger: `Move install, setup, troubleshooting, and long examples to docs; keep only trigger, workflow, judgment, and stop conditions in the skill body.`

Close loopholes with a required artifact, command result, state transition, or explicit stop condition. If no observable boundary exists, the rule is probably too vague.

## 5. Put action-bearing words first

Agents skim headings, list prefixes, and the first clause of long instructions. Put the word that controls behavior before its rationale.

- Start procedural steps with a concrete action: **Inspect**, **Decide**, **Write**, **Verify**, or **Stop**.
- Name decision branches at the front: **If the verifier fails**, **When no peer is available**, **For a brownfield task**.
- Lead with `must`, `never`, or the required artifact when a hard boundary applies; put background explanation after the controlling action.
- Use rationale after the instruction when it helps judgment; do not make the agent infer the instruction from the rationale.

Weak: `Because stale state can be confusing, it is important to check it before continuing.`

Stronger: `Check the persisted phase before continuing; stale state can otherwise send the workflow down the wrong branch.`

## 6. Design the information hierarchy

Order content by when the agent needs it:

1. **Metadata** — the trigger and boundary in `description`.
2. **Main path** — the shortest successful workflow and its invariants.
3. **Branches and stops** — exceptions, failure handling, and host-neutral alternatives.
4. **Optional acceleration** — host-specific shortcuts after the canonical path.
5. **References** — detailed schemas, examples, and troubleshooting loaded only when needed.

This is progressive disclosure: the default task should not load detail for variants it never uses. Keep references one hop from `SKILL.md`, link each one from the exact decision that needs it, and say when to read it. Do not split a short workflow merely to look modular, and do not duplicate the same rule in both the body and a reference.

## 7. Prune no-op text and duplication

Every paragraph must change at least one observable thing: the trigger, a decision, an action, an artifact, a stop condition, or a verifier. If deleting a sentence changes none of them, delete it.

Common no-ops:

- Generic quality adjectives such as `carefully`, `robustly`, or `thoughtfully` with no concrete bar.
- Explanations of facts the model already knows that do not affect the workflow.
- The same invariant repeated in several sections instead of one canonical rule.
- Defensive warnings for failures that have not occurred and have no distinct response.

Pruning is not terseness for its own sake. Keep non-obvious rationale that changes judgment; remove prose that only sounds reassuring.

## 8. Validate proportionately

Use deterministic checks for structure, command references, budgets, and repository mechanics. A missing live-agent behavior eval does not block a bounded, locally verified judgment-layer improvement when no efficacy claim is being made. If you do claim that wording improves model behavior, name the host, scenario, comparison, and observed evidence; do not promote intuition into a benchmark result.

Use `skillify`'s optional efficacy gate when a discipline skill targets a repeatable, pressure-induced failure and a fresh-context comparison is practical. It is supporting evidence for that claim, not a universal prerequisite for local skill improvement.

## Authoring checklist

- `SKILL.md` starts with frontmatter containing exactly one `name` key and exactly one `description` key; duplicate top-level keys are invalid.
- The description says WHEN to use the skill, not WHAT the skill does.
- The description begins with `Use when` followed by one space and fits its manifest budget.
- The body carries workflow judgment only: steps, decision points, hard rules, stop conditions, and verification.
- The form matches the failure: recipe for shape, prohibition or STOP gate for discipline.
- Desired behavior comes first; every prohibition closes a hard boundary and names the concrete alternative or recovery action.
- Discipline skills name likely rationalizations and block them.
- Agent-neutral behavior is the default; host-specific acceleration is clearly optional.
- Installation, platform setup, troubleshooting, and long examples live in docs, not resident skill text.
- Headings and steps put the controlling action or branch first.
- The main path appears before exceptions; optional references are one hop away and loaded only when needed.
- Every paragraph changes an observable behavior; duplicated and no-op wording has been pruned.
- Deterministic checks cover the falsifiable mechanics; any model-behavior claim names its actual evidence, while lack of a live-agent eval does not block bounded non-efficacy work.
- Every active shipped skill has an expected case in `eval/cases/triggering.jsonl`; add its nearest confusing boundary when one exists, without presenting fixture labels as observed model behavior.

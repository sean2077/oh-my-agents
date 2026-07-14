# Companion skills for oma

> **Status: current user guidance, not an oma compatibility contract.**
> Last reviewed 2026-07-14 against oma v1.4.1 and
> [`mattpocock/skills@66898f60`](https://github.com/mattpocock/skills/tree/66898f60e8c744e269f8ce06c2b2b99ce7660d5f).

oma is deliberately a small base layer, not an attempt to own every useful
agent behavior. Add an external skill only when it supplies a distinct outcome
without introducing a second orchestration system, state machine, or competing
entry point.

## Selection rule

A companion belongs on this page only when all of these remain true:

1. Its trigger and outcome are distinct from the [skills oma ships](../README.md#what-ships).
2. Removing it would not break an oma workflow or persisted state.
3. Its resident description and the user's extra choice are justified by repeated use.
4. It can be installed selectively; adopting its source repository wholesale is unnecessary.
5. The recommendation records an upstream revision and a concrete refresh trigger.

Start with oma alone. Add one profile only after the corresponding need recurs.

## Recommended profiles

| Profile | Add | Use it for | Keep separate from |
|---|---|---|---|
| Architecture language | `domain-modeling` + `codebase-design` | Maintaining domain terminology and ADRs while designing deep modules, interfaces, seams, and caller-facing test surfaces | `deep-interview`, which produces a spec but does not mutate project glossary or ADR files; `ai-slop-cleaner`, which stops before architecture redesign |
| Cross-session continuity | `handoff` | A manually requested, disposable summary for a fresh agent or session | `autopilot` resume state and the `pair-delivery` relay ledger |

### `domain-modeling`

[`domain-modeling`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/domain-modeling/SKILL.md)
actively sharpens the project's ubiquitous language and records durable
decisions in `CONTEXT.md` and ADRs. It complements oma because
`deep-interview` deliberately keeps approved terminology and decisions inside
its spec, then offers project-doc promotion as a separate, user-authorized
handoff.

Install it when a project has a real domain vocabulary or recurring
architectural decisions. Do not install it merely so agents can read an
existing glossary.

### `codebase-design`

[`codebase-design`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/codebase-design/SKILL.md)
provides a shared vocabulary for deep modules, interfaces, seams, leverage,
locality, and interface-level testing. oma borrows a few deletion and locality
heuristics for cleanup, but it does not ship an active module-design skill.

Install it when interface design, module boundaries, testability, or
AI-navigability is recurring work. Pair it with `domain-modeling` when both the
business language and the software shape are changing: domain modeling names
the concepts; codebase design decides where their behavior belongs.

### `handoff`

[`handoff`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/productivity/handoff/SKILL.md)
writes a temporary conversation summary for another session. It is
user-invoked (`disable-model-invocation: true`), so its main cost is remembering
that the command exists rather than adding an automatic trigger.

Keep it only if cross-session transfer occurs often enough to justify that
choice. It is not review evidence, durable workflow state, or a substitute for
`pair-delivery`.

## Install selectively

Install only the reviewed companions, not the entire upstream catalog:

```bash
npx skills add mattpocock/skills -g \
  --skill domain-modeling codebase-design handoff \
  --agent claude-code codex
```

Re-run the same selective command to refresh this set, then inspect the result:

```bash
npx skills ls -g
```

Remove a companion when its need disappears or its trigger begins competing
with an oma skill:

```bash
npx skills rm -g codebase-design
```

These are npx-managed external skills under the shared `~/.agents/` canonical
root. They do not appear in `oma asset catalog`, and their resident descriptions
are not included in an oma-only command such as:

```bash
oma doctor budget --agent claude --profile core4 --max-resident-tokens 400
```

## Avoid duplicate workflow surfaces

Do not install a second skill just because its wording differs. Prefer the oma
lane when the outcome already exists:

| External skill or family | Use instead | Reason |
|---|---|---|
| `grill-me`, `grilling`, `grill-with-docs` | `deep-interview`; optionally hand approved terms to `domain-modeling` | Keeps one interview entry point and makes project-doc mutation explicit |
| `diagnosing-bugs` | `trace` | oma already owns causal investigation and the red-capable-loop gate |
| `research` | `best-practice-research` | Keeps external research cited, version-aware, terminal, and read-only |
| `code-review` | oma `code-review` | Keeps one bounded read-only review contract and avoids confusing local review with pair evidence |
| `prototype` | oma `prototype` | Keeps one on-demand disposable-evidence lane |
| `tdd` | `autopilot` with focused RED -> GREEN at a meaningful seam | Avoids a second implementation loop while preserving test-first behavior where it is valuable |
| `implement`, `to-spec`, `to-tickets`, `wayfinder`, `triage`, `setup-matt-pocock-skills` | `deep-interview`, `autopilot`, and the README workflow router | Avoids importing a competing issue-tracker and orchestration system |
| `writing-great-skills` | `skillify` and [skill-authoring.md](skill-authoring.md) | Keeps one authoring path while retaining the useful writing principles |
| `ask-matt` | the [README workflow table](../README.md#which-workflow) | A resident router would add another choice without a new outcome |

[`resolving-merge-conflicts`](https://github.com/mattpocock/skills/blob/66898f60e8c744e269f8ce06c2b2b99ce7660d5f/skills/engineering/resolving-merge-conflicts/SKILL.md)
fills a real niche, but is not recommended unchanged: its unconditional
"never abort" and final stage/commit rules are too strong when the merge or
rebase direction itself may be wrong. Borrow its intent-tracing technique when
needed instead of installing the whole contract.

## Ownership and refresh

External companions remain upstream-owned. oma does not vendor, test, or
guarantee them, and their presence must never become a prerequisite for an oma
workflow. Re-evaluate this page when any of the following occurs:

- the pinned upstream skill body or invocation metadata changes;
- oma ships a lane with the same trigger and outcome;
- a companion repeatedly activates on the wrong tasks;
- its resident context or user-choice cost is no longer justified by use.

The historical method-by-method borrowing decision is recorded separately in
[history/borrow-from-matt-pocock-skills.md](history/borrow-from-matt-pocock-skills.md).
That record explains what oma adopted; this page answers what a user may still
want to install alongside it.

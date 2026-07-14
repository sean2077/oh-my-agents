# Borrowing from OMC and OMX into oma — delivery and research veins

> **Status: historical — implemented / superseded.** This record consolidates two completed planning artifacts: the delivery-loop round and the research/autonomy round. It preserves the borrowing decisions and provenance without reviving transient branch state, obsolete line numbers, or old schema details.
>
> Decision dates: 2026-06-15–16. Sources: oh-my-claudecode (OMC) and oh-my-codex (OMX).
>
> This is **not** a normative specification. Current behavior lives in [`../reference/`](../reference/), and current judgment contracts live in [`../../assets/skills/`](../../assets/skills/). The original notes recorded upstream file paths but did not pin upstream commit IDs, so those paths are historical provenance rather than claims about either upstream's current tree.

## Ruler and core judgment

Every candidate was filtered through the principles now stated in [`../design-philosophy.md`](../design-philosophy.md): keep resident context small, put countable and persistable mechanics in the Go binary, keep judgment in agent-neutral skills, avoid host-config mutation, and make terminal states and gates fail closed.

The original comparison found that neither OMC nor OMX supplied oma's cross-agent, append-only relay ledger. That ledger remained oma's architecture. The transferable value was narrower: mechanisms that make autonomous work falsifiable, plus a small number of judgment-only skill methods. OMA adopted or adapted those mechanisms instead of copying either upstream's host-specific orchestration model.

## Round 1 — trustworthy delivery loop

| Candidate | Historical source signal | Decision | Current oma landing |
|---|---|---|---|
| Completion receipts that make a `done` claim falsifiable | OMX ultragoal / completion audit | Adopt | Relay decisions carry a hash-bound completion receipt; ralph produces a result receipt for passed and score-optimization terminal outcomes. See [`../reference/schemas.md`](../reference/schemas.md) and [`../reference/relay-v2-protocol.md`](../reference/relay-v2-protocol.md). |
| Typed review verdicts plus a fail-closed close gate | OMC verdict artifacts; OMX `QualityGate` | Adopt | `oma-relay/4`, `oma-review-evidence/1`, and `oma-completion-receipt/2` bind the reviewed head, non-lead approval, and structured evidence. |
| Stop-hook escape valves for context, rate-limit, and authentication stops | OMC persistent mode | Adapt narrowly | [`internal/relay/hook.go`](../../internal/relay/hook.go) recognizes anchored stop reasons and stays silent instead of forcing continuation. |
| Fact-versus-judgment routing and content gates in interviewing | OMC / OMX deep-interview methods | Adapt | [`deep-interview`](../../assets/skills/deep-interview/SKILL.md) keeps inspectable facts out of user questions and requires non-goals and decision boundaries independently of the numeric gate. |
| Compact statusline presets | OMC HUD | Adapt narrowly | `oma statusline --preset minimal|focused|full` formats bounded oma state without parsing host transcripts; see [`../reference/command-tree.md`](../reference/command-tree.md). |
| Catalog as a generated view, not a second source of truth | OMX catalog manifest | Adapt and deepen | `oma asset catalog` and `oma asset audit` derive from asset manifests; lifecycle and token diagnostics remain inspectable and advisory. |
| Adversarial root-cause investigation | OMC `trace` | Adopt as agent-neutral judgment | [`trace`](../../assets/skills/trace/SKILL.md) ranks competing causes, self-falsifies the leader, and stops at a discriminating probe. |
| Regression-safe deletion of AI-style code bloat | OMC `ai-slop-cleaner` | Adopt with a verifier gate | [`ai-slop-cleaner`](../../assets/skills/ai-slop-cleaner/SKILL.md) locks behavior, simplifies in bounded passes, and requires fresh verification. |
| Adversarial end-to-end QA | OMX ultraqa | Adapt as a ralph profile | [`ultraqa`](../../assets/skills/ultraqa/SKILL.md) owns the hostile-scenario judgment while ralph owns loop state and stopping. No separate mechanical loop was added. |
| Refuse to start a loop from an unanchored goal | OMC ralplan | Adapt narrowly | A small advisory on `ralph start` flags short goals with no file, issue, symbol, or test anchor. No standalone ralplan skill or generic gate command landed. |
| Turn a proven workflow into a reusable skill | OMC `skillify` | Adopt | [`skillify`](../../assets/skills/skillify/SKILL.md) remains a judgment-only authoring workflow backed by oma's catalog and budget checks. |
| Remote notifications and reply injection | OMX reply-listener / Hermes | Defer, user-gated | Not added: a daemon or remote session-control plane would cross oma's zero-host-mutation and security boundary. |

## Round 2 — falsifiable research and autonomy

| Candidate | Historical source signal | Decision | Current oma landing |
|---|---|---|---|
| Score-based keep policy and score-plateau stopping | OMX missions / autoresearch evaluator | Adopt in the deterministic kernel | Ralph's `score_improvement` policy records the strict best score, distinguishes `plateaued` from repeated-failure `stalled`, and emits a receipt over the kept best. See [`../reference/workflows.md`](../reference/workflows.md) and [`../reference/schemas.md`](../reference/schemas.md). |
| Re-runnable research mission scaffold | OMX `missions/` | Adopt as judgment plus templates | [`research-mission`](../../assets/skills/research-mission/SKILL.md) defines the mission, deterministic evaluator contract, and candidate ledger, then delegates counting and stop judgment to ralph. |
| Evidence / inference / unknown split with ranked synthesis | OMC `analyze` | Adopt as agent-neutral judgment | [`analyze`](../../assets/skills/analyze/SKILL.md) is a read-only repository-analysis lane. |
| Source hierarchy, URL and version anchors, and a read-only research boundary | OMC / OMX best-practice research | Adopt as agent-neutral judgment | [`best-practice-research`](../../assets/skills/best-practice-research/SKILL.md) researches current external guidance and hands decisions or execution to another workflow. |
| Structured review evidence, placeholder rejection, and confidence independent of severity | OMX validation / research prompts | Adopt after the first relay gate | `oma-review-evidence/1` is validated, hashed, and bound into the approve-close receipt; see [`../reference/relay-v2-protocol.md`](../reference/relay-v2-protocol.md). |
| Ask only on decision-critical axes and research before asking | OMX strict interview prompts | Adapt narrowly | [`deep-interview`](../../assets/skills/deep-interview/SKILL.md) researches brownfield facts first and reserves user questions for choices the user owns. |

## Explicit exclusions

- Host-heavy orchestration surfaces — team, swarm, worker, pipeline, ultrawork, session-manager, setup, full host-integrated HUD, and similar control planes — were not carried over as package-level features because they were redundant, host-coupled, or too mechanically heavy for oma's small neutral base. Remote notification and reply injection remained separately deferred and user-gated.
- Mission-board, teleport, and issue commands were excluded by user decision; oma did not need another issue-management surface.
- `prometheus-strict`, self-improve, deep-dive, remember, visual-verdict, and similar broad personas were not copied wholesale. Only bounded methods with a clear oma landing were retained.
- OMX `deepsearch` and `ralph-init` were recorded as deprecated shells and were not borrowed.
- `design`, `wiki`, and visual ralph/verdict surfaces were outside the delivery-and-research scope; a wiki would also compete with the relay ledger as a source of truth.
- The OMC/OMX code-review verdict schema was skipped because the relay gate already owned that mechanic. The repository's current on-demand [`code-review`](../../assets/skills/code-review/SKILL.md) skill came from a later, separate decision recorded in [`borrow-from-matt-pocock-skills.md`](borrow-from-matt-pocock-skills.md).
- The autoresearch professor/critic ledger was not duplicated. Its evaluator and rubric semantics were enough; relay receipts and the research candidate ledger already covered the useful accountability boundary.

## Historical source index

The original notes referenced these upstream paths:

- **OMX:** `src/ultragoal/artifacts.ts`, `src/ralph/completion-audit.ts`, `src/state/workflow-transition.ts`, `missions/*/{mission,sandbox}.md`, `skills/autoresearch-goal/SKILL.md`, `skills/deep-interview/SKILL.md`, `templates/catalog-manifest.json`, `src/notifications/reply-listener.ts`, `src/mcp/hermes-server.ts`, and `prompts/{researcher,ux-researcher,prometheus-strict-metis}.md`.
- **OMC:** `scripts/persistent-mode.mjs`, `src/hooks/persistent-mode/index.ts`, `src/ultragoal/artifacts.ts`, `src/shared/artifact-descriptor.ts`, `src/hud/*`, and the historical `trace`, `ai-slop-cleaner`, `ralplan`, `skillify`, `analyze`, and `best-practice-research` skill bodies.

For exact historical wording, use the repository snapshots rather than current upstream files:

- [Round 1 original plan at `a8fe3cb`](https://github.com/sean2077/oh-my-agents/blob/a8fe3cbba01b9e46760c521a11900b906fd10bad/docs/borrow-from-omx-omc.md)
- [Round 2 original plan at `92832ed`](https://github.com/sean2077/oh-my-agents/blob/92832ed4c9decbf6ae996bf5e10f308fc09ea404/docs/borrow-from-omx-omc-2.md)

## Implementation and archival timeline

- [`a8fe3cb`](https://github.com/sean2077/oh-my-agents/commit/a8fe3cbba01b9e46760c521a11900b906fd10bad) implemented the first borrow set; [`3742e16`](https://github.com/sean2077/oh-my-agents/commit/3742e16c6d75166c38d899b9a564643cabebf21b) and [`8b739fa`](https://github.com/sean2077/oh-my-agents/commit/8b739facc9b6ed45ed24056a6393e8c2a7ddc4b9) tightened the reviewed-head and future-target close gate.
- [`92832ed`](https://github.com/sean2077/oh-my-agents/commit/92832ed4c9decbf6ae996bf5e10f308fc09ea404) implemented the research/autonomy set; [`4f70da1`](https://github.com/sean2077/oh-my-agents/commit/4f70da111da4ab9c1a843009db7117c97771795a) merged the double-gated result.
- [`6573a70`](https://github.com/sean2077/oh-my-agents/commit/6573a70d9f873839665edca83d457981553be6c4) later removed both completed plans while reorganizing the documentation. This consolidated record restores their design provenance under the history zone without making the obsolete plans normative again.
- The release-level account remains in [`../../CHANGELOG.md`](../../CHANGELOG.md), especially v0.3.0; current contracts in `docs/reference/` always take precedence over this history.

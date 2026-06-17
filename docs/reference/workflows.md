# Workflow terminal-state spec (interview / ralph / autopilot / pair-delivery)

## 1. `oma interview` — the fixed surface of Socratic requirements clarification

Fixing principle: **the math and the state live in the CLI; the questions and the judgment stay with the agent.** The CLI handles score computation, threshold gating, and round/state persistence; question generation, per-dimension scoring, and ontology extraction are performed by the agent per the skill text and then fed to the CLI.

### 1.1 State machine

```
created ──start──▶ topology_pending ──(lock topology)──▶ interviewing
   interviewing ──(score: ambiguity ≤ threshold)──▶ gate_passed ──▶ crystallized(spec_path) ──▶ completed
   interviewing ──(early exit / hard cap)──▶ gate_waived(warning recorded) ──▶ crystallized ──▶ completed
   any state ──abort──▶ aborted
```

### 1.2 Persistence (`.oma/state/interview-<id>.json`, schema `oma-interview/1`)

Fields: `id, phase, type(greenfield|brownfield), threshold, threshold_source, initial_idea, topology{status, components[{id,name,description,status,evidence[],clarity_scores{goal,constraints,criteria,context?}}], deferrals[], last_targeted_component_id}, rounds[{round, component, dimension, question, answer, scores, ambiguity}], ontology_snapshots[{round, entities[], stability_ratio, matching_reasoning}], challenge_modes_used[], current_ambiguity, gate_waiver?, spec_path, created, updated`. `gate_waiver` carries the early-exit warning record — the landing field for the state machine's `gate_waived(warning recorded)` transition.

### 1.3 Command semantics

- `start`: threshold resolution priority = `--threshold` > `--depth` (quick 0.30 / standard 0.20 / deep 0.10) > config > default 0.20; writes the initial state and reports the threshold and its source (mandatory first-line report, following established practice).
- `score --input scores.json`: input is the agent's per-component × per-dimension scores (0–1) plus the ontology snapshot; the CLI computes **deterministically**:
  - dimension total = the minimum across active components for that dimension (min-across-components)
  - ambiguity: greenfield `1-(goal×0.40+constraints×0.30+criteria×0.30)`; brownfield `1-(goal×0.35+constraints×0.25+criteria×0.25+context×0.15)`
  - ontology stability = (stable+changed)/total (N/A on the first round; changed = a rename with the same type and >50% field overlap)
  - selects the next target `weakest(component,dimension)` and applies the rotation rule (when N>1 are equally weak, avoid `last_targeted`)
  - appends the round record, updates state, and returns the full report (--json)
- `gate`: `current_ambiguity ≤ threshold` → exit code 0; otherwise 4, emitting the weakest component × dimension and the gap. Round guardrails are surfaced through the gate output: a soft warning at round ≥ 10, a hard cap at ≥ 20 (the gate still decides by the numeric value; the override decision is left to the user).
- Challenge modes: at round ≥ 4/6/8 (and where each mode is unused, and at round 8 only when ambiguity > 0.3) `score` prompts contrarian/simplifier/ontologist; once the agent adopts one it marks `challenge_mode_used` in the next round's `score` input.

### 1.4 Errors and recovery

- Corrupt state file / unknown schema → fail-closed refusal with the backup location (a single-generation `.bak` is written automatically before any write).
- All commands are idempotent and re-enterable: `start` refuses an existing id (unless `--resume` explicitly resumes and shows the current state).

## 2. `oma ralph` — the fixed surface of the persistent loop

Fixing principle: **counting, stop judgment, and history live in the CLI; doing the work and running the verification stay with the agent.** oma **never executes** the verifier command (security contract); the agent runs the verification itself and reports the exit code to the CLI.

### 2.1 State machine and persistence (`.oma/state/ralph-<id>.json`, schema `oma-ralph/2`)

```
created ──start──▶ running ──next──▶ running (round+1)
running ──check(verifier-exit=0)──▶ passed (terminal)
running ──next while round>max_rounds──▶ exhausted (terminal)
running ──stall_window consecutive identical failure signatures (pass_only)──▶ stalled (terminal, change strategy)
running ──plateau_window consecutive rounds with no strict score gain (score_improvement)──▶ plateaued (terminal, change strategy)
any state ──abort──▶ aborted
```

Fields: `id, phase, goal, keep_policy(pass_only|score_improvement, default pass_only), max_rounds(default 10), round, checks[{round, verifier_exit, score?, note, at}], stall_window(default 3), plateau_window(default 3), best_round, best_score, created, updated`. Under score_improvement, `checks[].score` is required and finite, and `best_round`/`best_score` record the strict best; for the `receipt`, see schemas.md §6.

### 2.2 Command semantics

- `start --goal <text> [--keep-policy pass_only|score_improvement] [--max-rounds N] [--stall-window N] [--plateau-window N]`: initializes; `goal` is required (the anchor for stop-judgment semantics). keep-policy defaults to pass_only.
- `next`: round+1; emits continue|stop with the reason (stop on passed/exhausted/stalled/plateaued, exit code 4).
- `check --verifier-exit <code> [--note] [--score <float>]`: records one verification result; exit 0 → passed. `--note` should carry the failure signature (e.g. the test name); the CLI judges `stalled` from the note string (stall_window consecutive identical notes, pass_only). `--score` is **required and finite** under score_improvement (omitting it is refused; passing `--score` under pass_only is also refused); plateau_window consecutive rounds with no strict gain → plateaued.
- `status`: current round, history, and stop state.

## 3. autopilot — a pure-markdown workflow (no dedicated command surface)

- There is no `oma autopilot *` command and none may be added (a change requires reopening the spec and re-reviewing this document).
- Persistent state uses the generic `oma state` under the `autopilot/` namespace (e.g. `autopilot/phase`, `autopilot/plan-path`).
- Skill-text skeleton: clarify (may invoke interview) → plan → implement → verify (may invoke ralph) → deliver; each step records state so an interrupted session can resume.
- CC acceleration branch (explicitly marked): Plan mode / subagent parallel exploration is available; the Codex default path runs the pure-text flow plus `oma state`.

## 4. pair-delivery — the paired delivery flow (built on relay v2)

- Roles come from `session.json.roles` (lead/planner/implementer/reviewer can each be assigned to any participant, and one person may hold several; lead is required and unique, defaulting to the initiator).
- Process gates (identical to this project's own delivery flow): plan (kind: plan) → review (kind: review, verdict approve/approve-with-changes/revise) → implement (recorded in touched_paths) → code review (kind: review) → kind: decision to close out.
- Skill responsibility: translate the gates above into the sequence of relay command calls and the `prompt_for_next` writing conventions; the revise-loop cap and the @user escalation rule (line-leading `@user:` + `--status timed_out`).
- Continuation responsibility: after publishing, or whenever the latest artifact is your own, do not start another relay round until the peer publishes, the pair becomes terminal, or the user explicitly tells you to stop. Trusted Stop hooks are the main Codex self-continuation path; held `oma relay wait` is the fallback when hook wiring/trust is unavailable, and any Codex harness wake-ups during that fallback are not completion.
- Both ends consistent: the flow is driven entirely by `oma relay` plus text, with no CC-specific dependency.

### 4.1 Lead (primary decision-maker) model

The relative strengths of different model families shift with scenario and version (today the claude family is generally stronger at planning; tomorrow may differ), so the mechanism **does not hardcode who is stronger** — it fixes the decision structure instead:

- **Authority order**: user decision ＞ lead's technical judgment ＞ the assisting party's suggestions. A decision the user has already made may not be overturned by anyone; a conflicting review point may only be escalated line-leading with `@user:`, never silently adopted.
- **lead = primary decision-maker** (the initiator by default): responsible for each gate's go/no-go and for choosing among options. **The assisting party (a non-lead participant) exists to cover blind spots**: review, counterexamples, risk and omission prompts — its conclusions are not binding on the lead.
- **Duty of independent verification**: the lead must independently verify each of the assisting party's findings before acting on it (adopt / partially adopt / reject), recording the disposition and the reason in the reply artifact — never accepting wholesale, never discarding without reason. The skill text must write "verify each finding + record the disposition" into the lead's turn conventions.
- **Role-swap triggers (rule-based, a prompt rather than automatic)**: on any of the following signals, the lead's skill must prompt the user line-leading with `@user:` to consider swapping the lead — ① within a single gate, the lead's output is reviewed to a blocker-level revise for ≥2 consecutive rounds; ② the lead's fix is rejected again for the same reason; ③ the assisting party finds a substantive defect the lead missed across two consecutive gates with no corresponding output from the lead. The swap itself = recorded with `kind: decision` after the user confirms + updating `session.json.roles.lead` (via `oma relay pair set-lead`); the process gates do not reset.
- After a swap the role semantics are symmetric: the former lead becomes the assisting party, taking on the same blind-spot-covering duty.

# oma skill-triggering eval

A runnable harness that substantiates oma's headline claim
([`docs/design-philosophy.md`](../docs/design-philosophy.md) §1.1): a **minimal,
on-demand resident skill surface improves accuracy**, not just token cost. The
thesis is that a large always-resident skill set "dilutes attention, supplies
distractors, and erode[s] instruction-following" so the model "occasionally
latches onto the wrong one." This eval makes that failure mode *measurable*.

A full live-agent study cannot run unattended here, so this directory ships the
**harness + fixtures + a deterministic scorer**. A human or CI produces the
per-arm observations; the scorer turns them into numbers. What is automated and
what still needs a human is stated plainly in
[Automated vs. manual](#automated-vs-manual) — nothing here silently claims a
result it did not measure.

## The three arms

Each arm answers the same fixture of user prompts; the only thing that varies is
the resident skill surface the agent reasons amid.

| Arm | Resident surface | What it models |
|---|---|---|
| **A — plain agent (no oma)** | none | Baseline: no skills installed. Can never *trigger* a skill, so it pins the floor for triggering recall and isolates the "does a bare agent follow the workflow anyway?" question. |
| **B — always-resident (OMC-style)** | ~40 skills permanently in context (~15–20k tokens) | The status quo oma argues against: every skill's trigger text is resident every turn, with no per-skill disable. |
| **C — oma on-demand** | only the skills relevant to the task (core4 ≈ 275 tokens resident; everything else installed on demand, zero when absent) | oma's bet: a small, precise resident surface. |

A and C use the same underlying agent and the same skill *definitions*; the
difference under test is **surface size**, exactly the variable
[`docs/design-philosophy.md`](../docs/design-philosophy.md) §1.1 names.

## Metrics

The scorer ([`score.sh`](score.sh)) computes the first four deterministically
from a predictions file. The last three are **observational** — recorded by the
operator during the run, not derived by the scorer.

1. **Skill-triggering precision** — of the cases where the arm triggered *some*
   skill, the fraction where it was the *correct* skill.
   `precision = correct-picks / all-non-"none"-picks`.
2. **Skill-triggering recall** — of the cases that *should* trigger a skill
   (`expected != none`), the fraction the arm got right.
   `recall = correct-picks / should-trigger`.
3. **Triggering F1** — harmonic mean of the two.
4. **False-trigger rate** — of the **decoy** cases (`expected == none`, where no
   skill should fire), the fraction where the arm fired one anyway.
   `false-trigger = fires-on-decoy / decoys`. This is the number expected to
   *separate B from C*: a noisy resident surface false-triggers more.
5. **Resident-token footprint** — measured per arm with
   `oma doctor budget --agent claude --profile core4 --json` (the enforcement
   gate from [`docs/reference/adapter-conformance.md`](../docs/reference/adapter-conformance.md)
   §5). Arm C's core4 target is < 2000; arm B's surface is ~15–20k by
   construction. Record the number, do not re-derive it.
6. **Workflow-adherence checklist** — once a skill *is* triggered, did the run
   follow the skill's contract? A small yes/no checklist per skill (e.g.
   deep-interview must end at a `pending approval` spec, never straight code;
   ralph must report a verifier exit code and honor the stop verdict; analyze
   must stay read-only). Human-judged; one row per (arm, case).
7. **Resume success** — for the resumable workflows (autopilot phase state,
   ralph rounds, the relay ledger), kill the session mid-run and resume: did
   state reload correctly? Pass/fail, human-observed.

> Precision/recall here are **multiclass over a closed skill set** (the labels in
> the fixture), plus the explicit `none` class for decoys. A wrong-skill pick is
> *not* counted as a true positive — it lowers precision (a bad pick was made)
> and lowers recall (the right skill was not picked). The scorer labels these
> `WRONG-SKILL` so they are visible in the per-case table.

## Run protocol

```bash
# Score one arm's predictions out of the box (uses the shipped sample):
bash eval/run.sh eval/results/example-arm-c.tsv

# Or score against an explicit fixture / with a nicer arm label:
bash eval/run.sh eval/results/example-arm-c.tsv --arm "C: oma on-demand"

# score.sh can be called directly too:
bash eval/score.sh --cases eval/cases/triggering.jsonl \
                   --pred  eval/results/example-arm-c.tsv --arm C
```

To produce **real** numbers (replacing the shipped samples):

1. **Set up each arm.**
   - Arm A: an agent with **no** oma skills installed.
   - Arm B: an agent with a large always-resident skill set (the OMC profile, or
     any ~40-skill always-on set). The point is surface *size*, so the exact set
     need only be large and always-on.
   - Arm C: `oma asset install` only the core4
     (`deep-interview autopilot ralph pair-delivery`) plus whichever on-demand
     skill the task at hand needs; install nothing else. See the README at the
     repo root for the install commands.
2. **Record the resident footprint** of each arm with `oma doctor budget`
   (metric 5) before running prompts.
3. **Run every prompt** in [`cases/triggering.jsonl`](cases/triggering.jsonl)
   against each arm, in a *fresh* session per case (so prior cases don't prime
   the next), and note **which skill the arm actually triggered** — or `none`.
   For metrics 6–7, also note whether the workflow contract was honored and
   whether a mid-run resume succeeded.
4. **Write a predictions file** per arm: a two-column TSV,
   `<case-id><TAB><skill>` (`none` when no skill fired), `#`-comments allowed.
   See [`results/example-arm-c.tsv`](results/example-arm-c.tsv) for the format.
   Record the run **provenance** in the file's `#` header — without it a number
   is not reproducible and must not be quoted as a result:
   - model + version and host + version (e.g. `claude-opus-4-8` on Claude Code
     `x.y.z`, or a GPT model on Codex `a.b.c`);
   - `oma version` (commit/build) and the case-manifest revision;
   - the run date and the sampling settings (temperature / reasoning effort);
   - the repetition index `i` of `k` (one file per repeat).
5. **Score** each arm with `bash eval/run.sh <your-arm>.tsv --arm <label>` and
   compare the tables.

## How to interpret results

The §1.1 claim is supported if, across enough cases and repetitions:

- **C ≥ B on precision and F1**, and notably **C < B on false-trigger rate** —
  the large resident surface fires the wrong skill / fires on decoys more often.
  This is the *accuracy* leg, the primary one.
- **C ≪ B on resident-token footprint** (metric 5) — the *cost* leg, secondary
  and already deterministic via `oma doctor budget`.
- **A** anchors both ends: recall 0 (no skills to trigger) and false-trigger 0
  (cannot fire what it lacks). A's value is the workflow-adherence comparison —
  whether the bare agent improvises the workflow acceptably without the skill.

A single run is **anecdote, not evidence**: triggering is stochastic. Treat one
predictions file as one sample; repeat each arm *k* times (fresh sessions),
score each, and report the **mean and a confidence interval** per metric. A
predictions file with missing, duplicate, or extra case-ids is invalid — fix it
rather than scoring it. Never publish an accuracy claim without the per-run
provenance (above) and the A/B/C comparison attached. The fixture is small by
design — it is a *probe*, sized for repeated manual runs, not a benchmark with
statistical power on its own. Widen [`cases/triggering.jsonl`](cases/triggering.jsonl)
before drawing strong conclusions.

What a passing result does **not** prove: that oma's specific skills are good, or
that on-demand installation is frictionless. It probes one mechanism —
*resident-surface size vs. triggering accuracy* — and nothing more.

## The fixture

[`cases/triggering.jsonl`](cases/triggering.jsonl) is NDJSON, one flat object per
line: `id`, `prompt`, `expected` (the skill that should trigger, or `none`),
`category` (`clear` / `near-miss` / `decoy`), and a `note`. It is intentionally
flat so the jq-free scorer can read it. Categories:

- **clear** — an unambiguous trigger for exactly one skill.
- **near-miss** — a prompt that *should* trigger a skill but lacks the literal
  trigger words, or sits on the boundary between two skills (e.g. causal
  "why did it break" → `trace`, not `analyze`). These stress precision.
- **decoy** — irrelevant or trivial prompts where **no** skill should fire
  (a one-line edit, a factual Q&A, a single command run). These measure the
  false-trigger rate, the core of the §1.1 argument.

## Automated vs. manual

| Step | Automated here? |
|---|---|
| Joining predictions to labels, precision/recall/F1, false-trigger rate, the per-case table | **Yes** — `score.sh`, deterministic, pure bash + awk. |
| Resident-token footprint (metric 5) | **No** (but deterministic elsewhere): one `oma doctor budget` call. The scorer does not shell out to oma. |
| Producing the predictions (which skill each arm actually picked) | **No** — needs live agent runs (metric 1–4 inputs). |
| Workflow-adherence checklist (metric 6) | **No** — human judgment per run. |
| Resume success (metric 7) | **No** — human-observed kill/resume. |

The shipped `results/example-arm-*.tsv` files are **illustrative samples, not
measurements** — each says so in its header. They exist so `bash eval/run.sh
eval/results/example-arm-c.tsv` works out of the box and so the
expected-direction contrast (C beats B on precision and false-trigger rate; A
floors recall) is visible without a live agent. Replace them with real
observations before quoting any number as a result.

## Files

- [`README.md`](README.md) — this document.
- [`cases/triggering.jsonl`](cases/triggering.jsonl) — labeled prompt fixture.
- [`run.sh`](run.sh) — front-end: resolve fixture, sanity-check, invoke scorer.
- [`score.sh`](score.sh) — deterministic scorer (bash + awk; no jq, no python).
- [`results/example-arm-a.tsv`](results/example-arm-a.tsv),
  [`results/example-arm-b.tsv`](results/example-arm-b.tsv),
  [`results/example-arm-c.tsv`](results/example-arm-c.tsv) — sample predictions
  per arm.

All scripts: `set -euo pipefail`, `-h/--help`, contractual exit codes
(`0` ok, `2` usage, `3` input/state), shellcheck-clean. Targeted at `mawk`
(the strictest common awk), so any awk works.

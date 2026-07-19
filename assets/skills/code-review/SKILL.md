---
name: code-review
description: Use when the user requests a read-only review of a working tree, branch, pull request, commit range, patch, or other bounded change set without implementation or pair delivery.
---

# code-review

Review a bounded change set and return evidence-backed findings. This is a **read-only and terminal** review, not an implementation or delivery workflow.

## Establish the target

Pin the exact review surface before judging it:

- **Working tree:** inspect unstaged, staged, and untracked changes against `HEAD`.
- **Branch:** compare the branch head with the merge base of the stated base branch.
- **Pull request:** use read-only provider metadata/diff or available local refs, and record the reviewed head SHA.
- **Fixed point:** inspect the supplied commit, range, patch, or file set exactly as named.

Record the base, head, and requested behavior or governing spec. Do not switch branches, fetch, or mutate refs merely to construct the target; use existing refs or read-only provider access, otherwise state the blocker.

At the start, record a lightweight review baseline: the `HEAD` SHA (or other fixed point), the exact file set in scope, and a diff/snapshot identity when the available tooling can produce one without mutation. For a working tree, include staged, unstaged, and untracked material in that baseline. The baseline makes later target drift visible; it is not a requirement to build a heavyweight integrity system.

## Review method

1. Read the user request, applicable `AGENTS.md`, repository contracts, and acceptance criteria.
2. Inspect the complete diff plus enough surrounding code, callers, tests, configuration, and documentation to judge its effects.
3. Review **Spec compliance** and **Standards & quality** separately. Within each axis, order findings by severity: critical, high, medium, then low.
4. For every suspected issue, try to falsify it. Report it only when a concrete affected behavior and precise `file:line` evidence remain.
5. Run safe, focused verification when it materially tests a claim. Do not install, update, or mutate the project to make a check available.
6. Immediately before the final report, recheck the mutable target against the recorded baseline. If it drifted, re-review the changed material and update the baseline, or identify the drift in **Limitations** and do not claim that material was reviewed.

Severity reflects impact, not effort:

- **critical:** credible data loss, security compromise, or unusable release.
- **high:** likely material correctness failure or broad regression.
- **medium:** bounded correctness, compatibility, or maintainability defect.
- **low:** real but limited risk that is still worth changing.

## Output contract

```md
# Code review: <target>

## Review target
<base, head/fixed point, exact file set, available diff/snapshot identity, and governing request/spec>

## Spec compliance
<findings ordered by severity, or "No findings in this axis.">

## Standards & quality
<findings ordered by severity, or "No findings in this axis.">

## Verification
<commands or checks actually run and their observed results; state why none ran>

## Limitations
<unreviewed surfaces, unavailable context/checks, assumptions, and residual uncertainty>
```

Format every finding as:

```md
### [<severity>] <short title>
- **Location:** `<path>:<line>`
- **Impact:** <observable failure or concrete maintenance risk>
- **Evidence:** <code path, contract, test, or command result that establishes the claim>
- **Confidence:** <high|medium|low> — <brief reason>
```

Keep line ranges tight; cite multiple exact `file:line` locations when the claim crosses files. Do not report style preference as a finding unless a repository rule or material risk makes it actionable.

If both axes are empty, state **No findings identified in the reviewed scope.** Still provide Verification and Limitations, and state that absence of findings does not prove correctness.

## Hard rules

1. **Read-only and terminal:** do not edit files, apply patches, stage, commit, push, or post review comments externally.
2. Do not start or publish to an `oma relay`; this review does not satisfy a `pair-delivery` gate or create a completion receipt.
3. When reviewing changes produced by this same model or session, label it **self-review, never independent review**.
4. Never pre-judge the outcome or suppress a supported finding because the author says the change is intentional.
5. If the user also asks for fixes, finish and return the review, then hand off to an implementation workflow; do not fix inside this skill.

> **Parallel acceleration (optional, capability-gated)**: Proactively delegate non-overlapping read-only review axes or subsystems against the same pinned baseline. Delegate only when the runtime exposes lifecycle-controllable subagent tools, at least two lanes are independent and bounded, critical-path benefit exceeds dispatch/wait/synthesis cost, no lane waits on the user or another lane, writes (if any) have exclusive ownership without shared single-writer state, and the parent can synthesize and run final verification. Brief each lane with its objective, scoped inputs, expected output, read-only boundary, and stop conditions; use the minimum useful fan-out, normally no more than three, and forbid subagents from delegating further. The parent alone asks user questions, writes shared oma state when applicable, rechecks the baseline, falsifies and deduplicates findings, labels same-session output self-review, and performs final verification and completion. If the gate becomes invalid or a lane fails, exceeds scope, or conflicts, stop affected delegation, retain only verified evidence, and finish the review sequentially.

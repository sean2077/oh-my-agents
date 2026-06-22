# oma Design Philosophy

> This document states oma's worldview — *why* each tradeoff is the way it is. The rest of `docs/` describes *what* — see [`architecture.md`](architecture.md) for the system map and [`reference/`](reference/) for the authoritative spec (protocol, schemas, security model, command tree). Read this to get the single ruler used when designing, borrowing, or reviewing.

## 0. In one line

oma bets that the scarcest resource in an agentic workflow is **clean context**, and that the way to protect it is to keep the model's resident surface **small, precise, and free of mechanical noise** — everything countable sinks into a deterministic binary; only judgment stays in the prompt.

## 1. The thesis: context is scarce — for two reasons, accuracy first

### 1.1 Accuracy (the primary reason)

An LLM attends over its **entire context** every turn. Tokens that are resident but irrelevant to the task at hand are not neutral: they **dilute attention, supply distractors, and erode instruction-following**. A model reasoning amid ~40 always-on skills must read past instructions that mostly don't apply to the task in front of it — and occasionally latches onto the wrong one. **Precise context is a precondition for accurate output.** In oma, minimalism is first a *correctness* strategy and only second an *economy*.

### 1.2 Cost (the secondary reason)

Resident tokens are also a tax paid on **every single turn**. OMC keeps ~40 skills permanently in context (~15–20k tokens, with no per-skill disable); oma's core4 together cost ≈275 tokens (about 2%), and everything else is installed on demand — zero when not installed.

Both legs point the same way: **the default state of the system should be empty.** You pay — in money and in attention — only for what you explicitly install, and ideally only for what the current task actually needs.

## 2. The load-bearing cut: mechanical vs. judgment

This is oma's spine. It decides **where every piece of logic lives.**

> **Probabilistic intelligence on the outside, a deterministic kernel on the inside.** The model supplies judgment; the binary supplies every invariant that can be counted, hashed, or verified.

- **Mechanical** (countable / verifiable / persistable): sequence numbers, ambiguity math, threshold gates, stall / score-plateau detection, atomic writes, integrity checks, receipt hashes … → the **Go binary**. Deterministic, tested, fail-closed, computed once, and **never resident in context.**
- **Judgment** (the non-mechanical reasoning): which question to ask, which implementation to change, how to design an evaluator command, whether a piece of evidence actually holds … → **markdown skills.** The only thing the model genuinely needs to reason about.

Two payoffs:

1. **Trust.** The binary is deterministic, tested, fail-closed — you never ask the model to count sequence numbers, hash content, or judge a stall.
2. **Clean context** (back to §1). Mechanical facts reach the model as a **verdict** ("ambiguity 0.7 — gate failed"), not as raw material it must re-derive amid everything else. The model reasons about judgment alone; less noise → better judgment.

> Where to put the cut is oma's recurring core design question: **how deep can logic move into the binary before *judgment itself* gets mechanized and a skill degrades into a rigid script that can't adapt?** Every new feature is negotiated on this line. Mechanical-vs-judgment is oma's philosophical fault line.

## 3. What falls out (consequences of §1–§2, not independent axioms)

### 3.1 A minimal, on-demand resident footprint

Don't make anything resident if it doesn't have to be; an on-demand skill costs **zero** when not installed. `oma doctor budget` turns "resident footprint" into a **measured number** rather than a feeling — the enforcement gate for this principle. The target isn't merely "few skills," but "exactly the skills relevant to the current task": **relevance** is the real quantity to minimize.

### 3.2 Agent-neutral by default; host acceleration is an optional branch

The default path is plain `oma` commands + markdown, running under the **same contract** on Claude Code and Codex. Host-only features (CC subagents, plan mode, structured pickers) are **clearly-marked optional branches**, never the default. This is also a *cleanliness* argument: a skill thick with per-host conditionals is noisier. Corollary: oma is a **CLI + skills, deliberately not a Claude Code plugin** — a plugin is a CC-only concept, at odds with neutrality.

### 3.3 Zero host-config mutation; document, don't command

oma places assets canonically under `~/.agents/` and projects them by symlink on Unix-like hosts, or by junction/copy on native Windows; it **never rewrites your host config** — `settings.json` / `hooks.json` stay yours, and hook wiring (the one unreliable, host-mutating step) is **documented for you to do by hand**, not performed by oma. When installing an asset would overwrite a file oma doesn't manage, it stays **fail-closed**: it refuses by default, and only an explicit `--force` replaces that file — after taking a restorable backup. The one thing oma updates in place is **itself** — `self-update` mutates only the `oma` binary, from a pinned, checksum-verified release, atomically, with automatic rollback.

### 3.4 Terminal-state design + fail-closed

Build each feature to its final shape in one step — no "thin version first," and **no long-lived migration layers**: the binary does not carry dual-read/dual-write code to interpret old and new formats at once. An unknown schema major is rejected, not guessed at. Crossing a major instead ships a single, explicit, audited `oma doctor` migration — dry-run by default, backed up, idempotent, and fail-closed on conflict (a *terminal-state* mechanism, not a permanent compatibility shim; see [`reference/schemas.md`](reference/schemas.md) §1).

Fail-closed only earns its keep if every refusal is *actionable*. A fail-closed stop names the check that triggered it and pairs a one-line reason with a one-line suggested fix (`hint:`), so "refuse" never decays into "obscure" (the error convention is in [`reference/command-tree.md`](reference/command-tree.md) §1). Anything repairable mechanically is offered as a plan you run — never a host change oma makes for you.

## 4. How the philosophy stays honest

- **Self-hosting**: oma is built with its own `pair-delivery` (Claude drafts + implements, Codex double-gates the review — the two gates are not skippable).
- **Spec-first**: `docs/` is authoritative; the code follows it, not the reverse.
- **The budget gate**: `oma doctor budget` makes resident footprint a number that is guarded, not a slogan.

## 5. The tensions we accept (a philosophy is defined by what it will pay)

1. **Minimalism vs. capability** — the mechanical/judgment line, and the risk of mechanizing judgment itself.
2. **Neutral default vs. host-native UX** — the neutral path **must not be worse than** what a host could do natively (the "relay must match the agent-ledger experience" worry comes from here).
3. **Doc-not-command vs. automation friction** — refusing to wire hooks for the user means the user must do it; some won't, and the auto-continue loop won't run for them.

These costs are paid deliberately. The fastest way to see them is by contrast: **superpowers** — a sibling methodology for the same job — took the opposite pole on nearly every axis: an **always-resident bootstrap, SessionStart hooks auto-wired through the plugin installer, host-specific subagent defaults.** It optimizes for *automatic, zero-friction, turnkey*; oma optimizes for *minimal, neutral, non-intrusive*. Neither is free. **oma's wager is that clean context plus a deterministic trust anchor matter more than turnkey automation.**

## 6. Non-goals (what oma is deliberately not)

- Not an always-on framework.
- Not a Claude Code plugin (a plugin is a host-specific concept, at odds with neutrality).
- Not a host-config manager.
- Not a place for mechanical logic to live in prompts.
- Not a model router, an MCP server, or a cloud agent-orchestration platform.
- Not a prompt marketplace or a generic multi-agent chat / middleware layer.
- Not an autonomy framework that lets an agent do anything — it is the narrow, recoverable, auditable *local work protocol* a coding agent runs against.

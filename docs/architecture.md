# oma Architecture

A developer-facing map of how oma is put together. For *why* it is shaped this way — the context-scarcity thesis and the mechanical-vs-judgment split — read [`design-philosophy.md`](design-philosophy.md); this document describes the *what* and *where*.

## Two artifacts, one contract

oma is a single Go binary (`oma`) plus a set of agent-neutral markdown skills. The binary owns every mechanical step (counting, scoring, atomic writes, integrity checks); the skills carry only judgment and shell out to `oma` for anything counted, validated, or persisted. This split is not a convention — it is the organizing principle, and it decides which package or file a given piece of logic lives in.

## The four layers

The two artifacts resolve into four layers, from the one closest to the model down to the host:

| Layer | Responsibility | Where it lives |
|---|---|---|
| **Skill policy** | Judgment: when to ask, when to review, how to weigh evidence, what to change | `assets/skills/<name>/SKILL.md` (markdown) |
| **Deterministic CLI** | Counting, scoring, thresholds, round / stall / plateau judgment, atomic writes, integrity hashes | `internal/{interview,ralph,relay,asset,state,budget,checks}` |
| **Protocol & state** | Session identity, worktree binding, the relay ledger, schemas, revisions, one-shot migrations | `internal/{relay,state,session}` + `~/.config/oma`, `<repo>/.oma` |
| **Host adapter** | Projecting one canonical asset set into Claude Code / Codex; optional host accelerations | `internal/{agentdir,asset}`; `~/.agents/` → `~/.claude/`, `~/.codex/` |

The line between the top layer and the three below it is the [mechanical-vs-judgment cut](design-philosophy.md): everything countable sinks below it; only judgment stays above.

## The asset model: one canonical store, projected to two hosts

Assets — skills, subagents, hooks, prompts — live once in a canonical store and are projected into each host's directory (symlink on Unix-like hosts; junction/copy on native Windows):

```
~/.agents/                  canonical asset store (shared with the npx-skills ecosystem)
  skills/<name>/            skill body
  agents/<name>.md          subagent
  hooks/<name>/             hook (manifest + fragment; canonical-only)
  prompts/<name>.md         prompt
        |  projection (symlink on Unix, junction/copy on native Windows)
        v
~/.claude/   ~/.codex/      per-agent directories
```

Each asset is described by a `manifest.json` (`oma-asset/1`) declaring its type and target agents. Installation places the body under `~/.agents/` and projects it to each target; a registry under `~/.config/oma/` tracks ownership so removal and rollback never touch files oma does not manage. Hooks are placed canonically only — oma never writes a host's `settings.json` / `hooks.json`; hook wiring is documented for the user to do by hand. See [`reference/adapter-conformance.md`](reference/adapter-conformance.md) for projection and conformance rules.

## The relay ledger

Cross-agent pair delivery runs over an append-only file ledger at `<repo>/.oma/relay/`. Each turn one agent reads the peer's latest artifact and publishes a reply; the binary owns sequence numbers, atomic publish, integrity sidecars, heartbeats, and the fail-closed completion-receipt gate. The protocol is v2 — a fresh design that never reads or writes the legacy agent-ledger `.shared/` tree. See [`reference/relay-v2-protocol.md`](reference/relay-v2-protocol.md) and [`reference/workflows.md`](reference/workflows.md).

## Repository layout

```
oh-my-agents/
├── cmd/oma/main.go          entry point (wiring only)
├── internal/
│   ├── cli/                 cobra command layer — thin shells; logic lives below
│   ├── asset/               manifest / install / projection / backup / registry
│   ├── assetaudit/          catalog bloat audit (orphan / oversized / retire), advisory
│   ├── agentdir/            per-agent directory resolution (claude / codex paths, projection kind)
│   ├── state/               project-level .oma/state/*.json (atomic 0600 writes)
│   ├── relay/               relay v2: ledger, sentinel, sidecars, heartbeat, publish, receipt, evidence, hook dispatcher, wait, statusline
│   ├── interview/           deep-interview scoring, threshold gate, round state machine
│   ├── ralph/               ralph loop counting, stall / score-plateau detection, receipt
│   ├── budget/              pinned approximate tokenizer + per-agent resident-injection model
│   ├── checks/              doctor registry: refcheck / security / conformance / budget
│   ├── config/              viper config layer (precedence: flag > env > project > user > default)
│   ├── update/              self-update: release query, checksum-verified atomic replace, rollback
│   └── version/             version stamping
├── assets/skills/<name>/    the shipped skills (each: SKILL.md + manifest.json [+ references/])
├── docs/
│   ├── design-philosophy.md  the why
│   ├── architecture.md       this file
│   ├── reference/            the authoritative spec set
│   └── examples/
├── scripts/                 install / release-build helpers
└── testdata/                fixtures: conformance golden files, traversal-attack paths
```

The CLI layer (`internal/cli`) is deliberately thin: each command parses flags and delegates to a domain package, so the mechanical logic stays testable in isolation and is never re-derived in a prompt.

## The five quality gates

Every change passes the gates wired into CI (see [`../CONTRIBUTING.md`](../CONTRIBUTING.md)):

1. unit / integration (`go test ./...`, against a temp `$HOME`);
2. static command-reference check (`refcheck`) — every `oma …` reference in a shipped skill must resolve to a real command;
3. resident-token budget (`oma doctor budget`);
4. per-agent offline conformance fixtures;
5. security tests (dry-run, backup / `--force`, symlink-escape, permissions) and the relay protocol suite.

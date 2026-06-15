# oh-my-agents

A single Go binary (`oma`) plus a small set of curated, agent-neutral skills that form a lightweight base layer for everyday AI coding workflows across both Claude Code and Codex.

`oma` solidifies the *mechanical* parts of a workflow (asset install/projection, state, scoring gates, loop stop-judgment, a cross-review pair ledger) into a deterministic, fail-closed CLI. The skills describe only the *non-mechanical* parts: the judgment. They call `oma` commands for everything that should be counted, validated, or persisted.

## Why

The trigger for this project was a concrete pain point in oh-my-claudecode (OMC): it ships ~40 skills that are **always resident** in the model's context (roughly 15-20k tokens) with **no per-skill disable**. You pay that context tax on every turn whether or not the work needs any of those skills.

`oma` is the opposite bet:

- **You decide what is installed.** Skills are explicit assets you install and remove. Nothing is resident unless you put it there. The four core skills together cost **~275 tokens** of resident surface (name + description), versus OMC's 15-20k, about 2%, and every asset is independently installable.
- **Mechanical logic belongs in a binary, not a prompt.** Sequence numbers, ambiguity math, threshold gates, stall detection, atomic file writes, and integrity checks are deterministic. They live in `oma`, where they are testable and fail-closed, not re-derived by the model each turn.
- **Skills stay agent-neutral.** A skill's default path is plain `oma` commands plus markdown, so Claude Code and Codex follow the *same* contract. Host-only accelerations (Claude Code's structured option picker, subagents, plan mode) are clearly-marked optional branches, never the default.
- **One asset model, two agents.** Assets live in canonical `~/.agents/` and are projected by symlink, or by atomic fragment injection for hooks, into both `~/.claude/` and `~/.codex/`. Install once, available to both.

This is CLI + skills, deliberately **not** a Claude Code plugin: a plugin is a Claude-Code-only concept, and the whole point is to stay neutral and lightweight.

## What ships

Four core workflow skills (the "core4"):

| Skill | What it does |
|---|---|
| **deep-interview** | Socratic requirements crystallization with deterministic ambiguity gating. Turns a vague idea into a `pending approval` spec, never straight to code. |
| **ralph** | A persistent improvement loop that iterates until a verifier passes. `oma` counts rounds and judges the stop; the agent runs the verifier itself and reports the exit code. |
| **autopilot** | End-to-end autonomous delivery (clarify -> plan -> implement -> verify -> deliver) with resumable phase state in `oma state`. Pure markdown; no dedicated command group. |
| **pair-delivery** | Cross-agent delivery over the `oma relay` ledger (plan -> review -> implement -> review -> decision) with an explicit lead and rule-based role-swap escalation. |

> **The skills require the `oma` CLI.** They are deliberately *not* standalone prompts: every mechanical step (state, scoring gates, sequence-numbered ledger operations) shells out to `oma`, and a skill invoked without the binary on `PATH` stops at its first command. Install the CLI first, then the skills. By the same principle, skill bodies carry only the core workflow — installation and platform guidance live here in the README, never in a `SKILL.md`.

## Install the CLI

Install the latest `oma` from the main branch into `~/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/sean2077/oh-my-agents/main/scripts/install.sh | bash
```

The installer builds from source, writes `oma` to `${OMA_INSTALL_BIN_DIR:-$HOME/.local/bin}`, and prints a PATH hint if that directory is not already on `PATH`. Override the destination with `OMA_INSTALL_BIN_DIR=/some/bin`. On Windows, run the same command from Git Bash; it installs `oma.exe` to `$HOME/.local/bin`, and Git Bash can invoke it as `oma` once that directory is on `PATH`. Release builds also include Windows `amd64` and `arm64` binaries once versioned releases are published.

You can also build from a checkout:

```bash
git clone https://github.com/sean2077/oh-my-agents
cd oh-my-agents
go build -o oma ./cmd/oma
./oma version
```

Once releases are published, `oma self-update` updates the binary in place from the pinned GitHub Releases (checksum-verified, atomic, with automatic rollback).

## Install the skills

**Prerequisite: the `oma` CLI must be installed and on `PATH`** (see above) — the skills drive it for every mechanical step and do not work without it.

```bash
# With oma, from a checkout of this repository
./oma asset install --from assets deep-interview ralph autopilot pair-delivery

# Or directly through the npx skills installer
npx skills add sean2077/oh-my-agents -g --agent claude-code codex
```

## Quickstart

```bash
# See what is installed and healthy
oma asset list --installed

# Check the resident-context budget gate
oma doctor budget --agent claude --profile core4
```

## Command surface

`oma` is organized into a few command groups (full reference: [`docs/command-tree.md`](docs/command-tree.md)):

- **`oma asset`**: install / list / remove / rollback assets; canonical placement plus per-agent projection.
- **`oma doctor`**: installation diagnostics and the resident-token budget gate.
- **`oma interview`**: the solidified surface of deep-interview: scoring math, threshold gate, and state. Math in the CLI; judgment in the agent.
- **`oma ralph`**: the solidified surface of the loop: round counting, stall detection, terminal-state judgment. `oma` never runs your verifier.
- **`oma relay`**: the v2 pair ledger: append-only artifacts, atomic publish with integrity sidecars, sequence reservation, and `wait`-based handoff.
- **`oma state`**: generic project-level key/value state for workflows.
- **`oma config`**, **`oma self-update`**, **`oma version`**.

Conventions: `--json` on every query command; `--dry-run` is a global flag that discloses exact paths and writes nothing; exit codes are contractual (`0` ok, `1` warn, `2` usage, `3` fail-closed, `4` gate failed; relay `wait` adds `10/11/12`).

## Architecture

```text
~/.agents/                      canonical asset store (shared with the npx-skills ecosystem)
  skills/<name>/                skill body
  agents/<name>.md              subagent
  hooks/<name>/                 hook (manifest + fragment)
  prompts/<name>.md             prompt
        |  projection (symlink, or atomic fragment injection for hooks)
        v
~/.claude/   ~/.codex/          per-agent directories
```

- **Assets** are described by a `manifest.json` (`oma-asset/1`) declaring type and target agents. Install places the body in `~/.agents/` and projects it to each target; a registry under `~/.config/oma/` tracks ownership so removal and rollback never touch foreign files.
- **relay v2** is a fresh protocol (it does not read or write the legacy agent-ledger `.shared/` tree) that keeps the proven principles: append-only ledger, sidecar integrity markers, fail-closed identity, platform-signal authorship. The ledger lives at `<repo>/.oma/relay/`.
- **Security is fail-closed throughout**: unmanaged-target refusal with backup on `--force`, trusted-root symlink-escape checks, world-writable refusal, duplicate-JSON-key rejection on host configs, mandatory secret scanning before publish, and a checksum-verified self-update trust chain. Details in [`docs/security-contract.md`](docs/security-contract.md).

The design documents under [`docs/`](docs/) (protocol, command tree, workflows, adapter conformance, schemas, security, config) are the authoritative specification; the implementation follows them rather than the reverse.

## Development

```bash
go build ./...
go test ./...           # full suite
gofmt -l .              # must be empty
go vet ./...
```

CI runs the test matrix, `gofmt`/`vet`/`build`, and `golangci-lint` on every push and PR. The release workflow cross-compiles six platforms with a checksums manifest and a tag-version gate.

This project is itself built through its own `pair-delivery` workflow: every slice is cross-reviewed by a second agent over the `oma relay` ledger before it lands. The skill that describes that process is the one we used to build it.

## License

[MIT](LICENSE) © 2026 sean2077

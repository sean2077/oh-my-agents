# oh-my-agents

A single Go binary (`oma`) plus a small set of curated, agent-neutral skills that form a lightweight base layer for everyday AI coding workflows across both Claude Code and Codex.

`oma` solidifies the *mechanical* parts of a workflow (asset install/projection, state, scoring gates, loop stop-judgment, a cross-review pair ledger) into a deterministic, fail-closed CLI. The skills describe only the *non-mechanical* parts: the judgment. They call `oma` commands for everything that should be counted, validated, or persisted.

## Why

The trigger for this project was a concrete pain point in oh-my-claudecode (OMC): it ships ~40 skills that are **always resident** in the model's context (roughly 15-20k tokens) with **no per-skill disable**. You pay that context tax on every turn whether or not the work needs any of those skills.

`oma` is the opposite bet:

- **You decide what is installed.** Skills are explicit assets you install and remove. Nothing is resident unless you put it there. The four core skills together cost **~275 tokens** of resident surface (name + description), versus OMC's 15-20k, about 2%, and every asset is independently installable.
- **Mechanical logic belongs in a binary, not a prompt.** Sequence numbers, ambiguity math, threshold gates, stall detection, atomic file writes, and integrity checks are deterministic. They live in `oma`, where they are testable and fail-closed, not re-derived by the model each turn.
- **Skills stay agent-neutral.** A skill's default path is plain `oma` commands plus markdown, so Claude Code and Codex follow the *same* contract. Host-only accelerations (Claude Code's structured option picker, subagents, plan mode) are clearly-marked optional branches, never the default.
- **One asset model, two agents.** Assets live in canonical `~/.agents/` and are projected into both `~/.claude/` and `~/.codex/` (symlink on Unix-like hosts; directory junction for skills on native Windows, with managed copy fallback). Install once, available to both. (Hook assets are placed canonically only — oma never writes your host config; you wire hooks into `settings.json`/`hooks.json` by hand, see [`docs/reference/relay-v2-protocol.md`](docs/reference/relay-v2-protocol.md) §12.4.)

This is CLI + skills, deliberately **not** a Claude Code plugin: a plugin is a Claude-Code-only concept, and the whole point is to stay neutral and lightweight.

## What ships

Four core workflow skills (the "core4"):

| Skill | What it does |
|---|---|
| **deep-interview** | Socratic requirements crystallization with deterministic ambiguity gating. Turns a vague idea into a `pending approval` spec, never straight to code. |
| **ralph** | A persistent improvement loop that iterates until a verifier passes — or, under the `score_improvement` keep-policy, until a scored evaluator plateaus. `oma` counts rounds and judges the stop; the agent runs the verifier itself and reports the exit code (and score). |
| **autopilot** | End-to-end autonomous delivery (clarify -> plan -> implement -> verify -> deliver) with resumable phase state in `oma state`. Pure markdown; no dedicated command group. |
| **pair-delivery** | Cross-agent delivery over the `oma relay` ledger (plan -> review -> implement -> review -> decision) with an explicit lead and rule-based role-swap escalation. |

On-demand skills (install only when a task needs them — zero resident cost otherwise):

| Skill | What it does |
|---|---|
| **trace** | Adversarial root-cause investigation: competing hypotheses, evidence-strength ranking, self-falsification, ending at the single highest-value discriminating probe. |
| **analyze** | Read-only deep repository analysis: a ranked, confidence-tagged synthesis with `file:line` evidence and a strict evidence / inference / unknown split. |
| **best-practice-research** | Bounded external best-practice research with official/upstream sourcing, version/date context, and a terminal read-only handoff. |
| **research-mission** | Scaffold a falsifiable research/optimization mission (deterministic evaluator contract + candidate ledger) and drive it with ralph's `score_improvement` keep-policy. |
| **ai-slop-cleaner** | Regression-safe, deletion-first cleanup of AI code slop, gated by a green verifier. |
| **ultraqa** | Adversarial end-to-end QA as a ralph profile (hostile-scenario matrix). |
| **skillify** | Capture a repeatable workflow into a new oma skill, gated by a 3-question quality test. |

The full installed catalog (with lifecycle status) is `oma asset catalog`; `oma asset audit` flags catalog bloat (orphan / oversized / retire) as advisory-only signals.

> **The skills require the `oma` CLI.** They are deliberately *not* standalone prompts: every mechanical step (state, scoring gates, sequence-numbered ledger operations) shells out to `oma`, and a skill invoked without the binary on `PATH` stops at its first command. Install the CLI first, then the skills. By the same principle, skill bodies carry only the core workflow — installation and platform guidance live here in the README, never in a `SKILL.md`.

## Install the CLI

Install the latest released `oma` into `~/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/sean2077/oh-my-agents/main/scripts/install.sh | bash
```

This downloads the prebuilt binary for the **latest GitHub release**, verifies its SHA-256 against the release `checksums.txt`, atomically installs `oma` into `${OMA_INSTALL_BIN_DIR:-$HOME/.local/bin}`, and asserts the installed binary's version — the same fail-closed contract `self-update` uses, no Go toolchain required. It is **fail-closed**: if it cannot resolve a release, match a prebuilt asset, verify the checksum, or confirm the version, it stops with an actionable error — it never silently builds from source or from the unreleased `main` branch.

Useful overrides: `OMA_INSTALL_VERSION=vX.Y.Z` pins a specific release, `OMA_INSTALL_BIN_DIR=/some/bin` changes the destination, and `OMA_INSTALL_FROM_SOURCE=1` opts into a source build (needs `git` + `go`). On Windows, run the same command from Git Bash (it installs `oma.exe`, callable as `oma` once the directory is on `PATH`), or use the native PowerShell installer:

```powershell
irm https://raw.githubusercontent.com/sean2077/oh-my-agents/main/scripts/install.ps1 | iex
oma version
```

For a reproducible, supply-chain-pinned install, fetch the installer **at a release tag** (so the script itself is immutable, not the moving `main`) and pin the version to match:

```bash
OMA_VERSION=v0.9.1   # a tag from the releases page
curl -fsSL "https://raw.githubusercontent.com/sean2077/oh-my-agents/${OMA_VERSION}/scripts/install.sh" | OMA_INSTALL_VERSION="$OMA_VERSION" bash
```

You can also install from a checkout. This is the preferred self-build path:
it stamps `oma version` with `git describe`, the short git commit, and
`-dirty` when the checkout has uncommitted changes.

```bash
git clone https://github.com/sean2077/oh-my-agents
cd oh-my-agents
make install
oma version
```

`make install` uses Go's normal install location (`GOBIN`, or `GOPATH/bin` when
`GOBIN` is unset), so make sure that directory is on `PATH`. Use `make build`
instead when you want a stamped local `./oma` binary in the checkout.
On native Windows / Codex Desktop, PowerShell users can skip `make` and run:

```powershell
go install -trimpath ./cmd/oma
oma version
```

Ensure Go's install directory (usually `%USERPROFILE%\go\bin`) is on `PATH`.

Once releases are published, `oma self-update` updates the binary in place from the pinned GitHub Releases (checksum-verified, atomic, with automatic rollback).

## Install the skills

**Prerequisite: the `oma` CLI must be installed and on `PATH`** (see above) — the skills drive it for every mechanical step and do not work without it.

```bash
# With oma, from a checkout of this repository
./oma asset install --from assets deep-interview ralph autopilot pair-delivery

# Or directly through the npx skills installer
npx skills add sean2077/oh-my-agents -g --agent claude-code codex
```

## Wire the statusline and hooks (optional)

`oma` never writes your host config. The relay statusline and the auto-continue
hooks are **opt-in**: you add them to your own `~/.claude/settings.json` (or
`~/.codex/hooks.json`) by hand. Use the absolute path to your binary (`which oma`)
behind an existence guard so a missing binary degrades silently instead of
spamming command-not-found.

**Statusline** (a compact "which pair / whose turn" line) — add to
`~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "[ -x '/ABS/PATH/oma' ] || exit 0; exec '/ABS/PATH/oma' relay statusline"
  }
}
```

Already have a custom statusline script? Don't replace it — call
`oma relay statusline --json` from inside it and gate on `.bound` (so non-pair
windows stay clean). See [`docs/examples/statusline-command.sh`](docs/examples/statusline-command.sh)
for a complete working example — the relay segment is the last block.

**Auto-continue hooks** (drive the pair-delivery loop without manual nudging) —
add to the top-level `hooks` key in `~/.claude/settings.json`. For Codex, the
Stop hook is the main self-continuation path; without a trusted Stop hook it
falls back to foreground `oma relay wait`. The dispatcher `oma relay hook
<event>` is pure-read and always exits 0, so it can never break your session:

```json
{
  "hooks": {
    "SessionStart": [{ "matcher": "startup|resume|clear",
      "hooks": [{ "type": "command", "timeout": 10,
        "command": "[ -x '/ABS/PATH/oma' ] || exit 0; exec '/ABS/PATH/oma' relay hook SessionStart" }] }],
    "PreToolUse": [{ "matcher": "^(Edit|Write|MultiEdit)$",
      "hooks": [{ "type": "command", "timeout": 5,
        "command": "[ -x '/ABS/PATH/oma' ] || exit 0; exec '/ABS/PATH/oma' relay hook PreToolUse" }] }],
    "Stop": [{
      "hooks": [{ "type": "command", "timeout": 5,
        "command": "[ -x '/ABS/PATH/oma' ] || exit 0; exec '/ABS/PATH/oma' relay hook Stop" }] }]
  }
}
```

For **Codex**, put the same structure in `~/.codex/hooks.json`, change the
`PreToolUse` matcher to `^(apply_patch|Edit|Write)$`, then run `/hooks` and
confirm the `oma relay hook Stop` entry is trusted. Full field reference
(matchers, timeouts, guard rationale) is in
[`docs/reference/relay-v2-protocol.md`](docs/reference/relay-v2-protocol.md) §12.4.
For native Windows Codex Desktop, use a PowerShell command string such as:

```json
{
  "hooks": {
    "Stop": [{
      "hooks": [{ "type": "command", "timeout": 5,
        "command": "& 'C:\\Users\\YOU\\go\\bin\\oma.exe' relay hook Stop" }]
    }]
  }
}
```

Use the same PowerShell form for `SessionStart` and `PreToolUse`, changing only
the final event name.

## Quickstart

```bash
# See what is installed and healthy
oma asset list --installed

# Check the resident-context budget gate
oma doctor budget --agent claude --profile core4
```

For a full walkthrough — install → deep-interview → autopilot → ralph → pair-delivery → failure/resume → upgrade, migrate, and cleanup — see **[docs/tutorial.md](docs/tutorial.md)**.

## Command surface

`oma` is organized into a few command groups (full reference: [`docs/reference/command-tree.md`](docs/reference/command-tree.md)):

- **`oma asset`**: install / list / remove / rollback assets; canonical placement plus per-agent projection. `catalog` derives a status-lifecycle view from manifests; `audit` flags catalog bloat (orphan / oversized / retire), advisory-only.
- **`oma doctor`**: installation diagnostics and the resident-token budget gate.
- **`oma interview`**: the solidified surface of deep-interview: scoring math, threshold gate, and state. Math in the CLI; judgment in the agent.
- **`oma ralph`**: the solidified surface of the loop: round counting, stall detection (`pass_only`) or score-plateau detection (`score_improvement`), terminal-state judgment with a falsifiable receipt. `oma` never runs your verifier.
- **`oma relay`**: the v2 pair ledger: append-only artifacts, atomic publish with integrity sidecars, sequence reservation, `wait`-based handoff, and a fail-closed approve-close gate (completion receipt binding the reviewed work + a non-lead approve review + its structured review-evidence by content hash).
- **`oma state`**: generic project-level key/value state for workflows.
- **`oma config`**, **`oma self-update`**, **`oma version`**.

Conventions: `--json` on every query command; `--dry-run` is a global flag that discloses exact paths and writes nothing; exit codes are contractual (`0` ok, `1` warn, `2` usage, `3` fail-closed, `4` gate failed; relay `wait` adds `10/11/12`).

## Architecture

```text
~/.agents/                      canonical asset store (shared with the npx-skills ecosystem)
  skills/<name>/                skill body
  agents/<name>.md              subagent
  hooks/<name>/                 hook (manifest + fragment; canonical-only, wire by hand)
  prompts/<name>.md             prompt
        |  projection (symlink on Unix; junction/copy on native Windows; hooks are canonical-only)
        v
~/.claude/   ~/.codex/          per-agent directories
```

- **Assets** are described by a `manifest.json` (`oma-asset/1`) declaring type and target agents. Install places the body in `~/.agents/` and projects it to each target; a registry under `~/.config/oma/` tracks ownership so removal and rollback never touch foreign files.
- **relay v2** is a fresh protocol (it does not read or write the legacy agent-ledger `.shared/` tree) that keeps the proven principles: append-only ledger, sidecar integrity markers, fail-closed identity, platform-signal authorship. The ledger lives at `<repo>/.oma/relay/`.
- **Security is fail-closed throughout**: unmanaged-target refusal with backup on `--force`, trusted-root escape checks, POSIX world-writable refusal, duplicate-JSON-key rejection on host configs, mandatory secret scanning before publish, and a checksum-verified self-update trust chain. Details in [`docs/reference/security-contract.md`](docs/reference/security-contract.md).

The design documents under [`docs/`](docs/) (protocol, command tree, workflows, adapter conformance, schemas, security, config) are the authoritative specification; the implementation follows them rather than the reverse.

## Development

```bash
go build ./...
go test ./...           # full suite
gofmt -l .              # must be empty
go vet ./...
```

CI runs the 3-platform test matrix with `-race`, `gofmt`/`vet`/`build`, `golangci-lint`, and `govulncheck` on every push and PR. Releases call the **same** pipeline as a hard gate, then promote the exact built-and-verified artifacts (no rebuild) with a checksums manifest, a tag-version gate, a build-provenance attestation, and an SBOM. The compatibility contract is [`STABILITY.md`](STABILITY.md).

This project is itself built through its own `pair-delivery` workflow: every slice is cross-reviewed by a second agent over the `oma relay` ledger before it lands. The skill that describes that process is the one we used to build it.

## Project policies

- **[STABILITY.md](STABILITY.md)** — what is frozen across releases (the compatibility contract).
- **[SECURITY.md](SECURITY.md)** — supported versions and private vulnerability reporting.
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — the cross-reviewed delivery process oma is built with.

## License

[MIT](LICENSE) © 2026 sean2077

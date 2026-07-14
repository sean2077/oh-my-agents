# AGENTS.md — oh-my-agents (`oma`)

`oma` is a single Go binary plus a small set of agent-neutral skills. It solidifies
the *mechanical* parts of AI coding workflows (asset install/projection, state,
scoring gates, loop stop-judgment, a cross-review pair ledger) into a deterministic,
fail-closed CLI; the skills carry only the *judgment* and shell out to `oma` for
anything counted, validated, or persisted. Judgment-only skills with no mechanical
step remain commandless. Same contract for Claude Code and Codex by design.

This file is the agent entry point. The detailed specs under [`docs/`](docs/) are
authoritative, and [`README.md`](README.md) is the human-facing version of the setup
below — link to them rather than re-deriving anything here.

## Set `oma` up (to USE it in any repo)

Do these in order; each step points at the README for options and detail.

1. **Install the CLI first for the core workflows** — a skill that names an `oma`
   command stops at that command without the binary on `PATH`:
   ```bash
   curl -fsSL https://raw.githubusercontent.com/sean2077/oh-my-agents/main/scripts/install.sh | bash
   ```
   From a checkout instead: `make install` (stamps `oma version`) or
   `go install -trimpath ./cmd/oma`. Confirm with `oma version`.

2. **Install the skills** (after the CLI):
   ```bash
   oma asset install --from assets deep-interview ralph autopilot pair-delivery
   # or: npx skills add sean2077/oh-my-agents -g --agent claude-code codex
   ```
   Assets live in canonical `~/.agents/` and project into `~/.claude/` and `~/.codex/`.
   Inspect with `oma asset list --installed`.

3. **Wire hooks + statusline (optional, host-owned)** — `oma` never writes your host
   config. Copy the snippets from the README section **"Wire the statusline and hooks"**
   into `~/.claude/settings.json` (or `~/.codex/hooks.json`), using your absolute
   binary path behind an existence guard. The Stop hook drives the `pair-delivery`
   auto-continue; without it, foreground `oma relay wait` is the fallback. Field-level
   reference: [`docs/reference/relay-v2-protocol.md`](docs/reference/relay-v2-protocol.md) §12.4.

4. **Identity / session env** — two *separate* concerns, each fail-closed when
   unresolved (no silent project-global fallback):
   - *Workflow-state scope* (interview/ralph/autopilot/state): `OMA_SESSION_ID`
     (explicit slug, wins) else `CLAUDE_CODE_SESSION_ID` (Claude) / `CODEX_THREAD_ID` (Codex).
   - *Relay identity* (independent of the above): `OMA_RELAY_AUTHOR` (`claude`|`codex`),
     paired with `OMA_RELAY_SESSION_ID` (or `OMA_SESSION_ID` as fallback) — a bare
     `OMA_RELAY_AUTHOR` with no session is refused.

5. **Verify**: `oma doctor` (install diagnostics), and `oma doctor budget --agent claude
   --profile core4 --max-resident-tokens 400` (core4 release gate).

## Work IN this repo (contributing to `oma`)

- **Local gate — green before any handoff:**
  ```bash
  go build ./...
  go test ./...      # full suite
  gofmt -l .         # must print nothing
  go vet ./...
  ```
  CI additionally runs `golangci-lint` and a Windows matrix; `.gitattributes` pins
  `*.go` to LF so `gofmt` is byte-identical across platforms.
- **Specs are authoritative.** [`docs/reference/`](docs/reference/) holds command-tree,
  relay-v2-protocol, schemas, adapter-conformance, config, security-contract, and
  workflows. Implementation follows the docs — change the doc together with the code.
- **Cross-review is optional and risk-based.** Use `pair-delivery` when the user asks
  for it or an available independent reviewer materially improves a change. A missing
  peer host must not block ordinary local work: use the four-axis local fallback review
  in [`CONTRIBUTING.md`](CONTRIBUTING.md), clear the local gate, and report the
  verification instead. Never present same-host assistance as independent cross-host
  evidence.
- **Runtime state lives under `<repo>/.oma/`** (gitignored): workflow state in
  `.oma/state/`, the pair ledger in `.oma/relay/`.

## Command surface

`oma asset | doctor | interview | ralph | relay | state | workflow | config |
self-update | version`. Full tree: [`docs/reference/command-tree.md`](docs/reference/command-tree.md).
Conventions: `--json` on query commands, a global `--dry-run` that writes nothing, and
contractual exit codes (`0` ok / `1` warn / `2` usage / `3` fail-closed / `4` gate
failed; relay `wait` adds `10/11/12`).

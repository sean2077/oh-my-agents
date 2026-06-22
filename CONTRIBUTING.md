# Contributing to oma

oma is built through its own delivery process — the same `pair-delivery` workflow it ships. Every change is cross-reviewed by a second agent over the `oma relay` ledger before it lands.

## The delivery process

A change moves through two non-skippable review gates:

1. **Plan.** The implementing agent (the *lead*) writes a plan: scope, approach, acceptance criteria.
2. **Gate 1 — plan review.** A second agent reviews the plan and returns a typed verdict. Only an `approve` clears the gate.
3. **Implement.** The lead does the work on a feature branch / worktree, clears a concrete verification bar, and records the evidence.
4. **Gate 2 — code review.** The second agent reviews the actual diff and returns a verdict; the lead dispositions every finding (adopt / partially adopt / reject, with reasoning).
5. **Decision.** When both sides agree, the lead publishes a decision and the binary stamps a completion receipt binding the approved plan, the non-lead `approve` review, and the reviewed head by content hash.

In practice the lead is Claude and the reviewer is Codex, but the roles are configurable. The two gates are not optional. See the `pair-delivery` skill and [`docs/reference/workflows.md`](docs/reference/workflows.md) for gate semantics, and [`docs/reference/relay-v2-protocol.md`](docs/reference/relay-v2-protocol.md) for the ledger.

## Spec-first

The documents under [`docs/`](docs/) are authoritative; the implementation follows them, not the reverse. A change that alters a contract updates the relevant [`docs/reference/`](docs/reference/) document in the same delivery. Build everything to its terminal shape — no migration layers, no "thin version first."

## Building and testing

```bash
go build ./...
go test ./...          # full suite — installation/projection tests use a temp $HOME
gofmt -l .             # must print nothing
go vet ./...
```

CI runs the test matrix, `gofmt` / `vet` / `build`, and `golangci-lint` on every push and PR. The release workflow cross-compiles six platforms with a checksums manifest and a tag-version gate.

Optionally enable the repo git hooks once with `make hooks` (`git config core.hooksPath scripts/hooks`). The `pre-commit` hook is a dev aid — not part of shipped `oma` — that warns when a staged `SKILL.md`/`AGENTS.md` body exceeds the content budget (default 120 non-blank lines after the frontmatter), to keep agent content lean. It rides git, so it fires identically for Codex, Claude Code, and humans. Tune with `OMA_CONTENT_BUDGET_LINES=<n>`, or set `OMA_CONTENT_BUDGET_BLOCK=1` to fail the commit instead of warning.

## Releasing

Releases are changelog-first and tag-triggered:

1. Update [`CHANGELOG.md`](CHANGELOG.md) with a section headed `## vX.Y.Z - YYYY-MM-DD`; the GitHub Release body is extracted from that section.
2. Run the local gate: `gofmt -l .`, `go vet ./...`, `go test ./...`, and `make release VERSION=vX.Y.Z`.
3. Commit the changelog and related release changes, then create an annotated `vX.Y.Z` tag.
4. Push the branch and tag. The release workflow verifies the changelog section, rebuilds assets, checks the version stamp and checksums, then creates or updates the GitHub Release.

### The quality gates

Beyond `go test`, CI enforces:

- **refcheck** — every `oma …` reference in a shipped skill resolves to a real command;
- **`oma doctor budget`** — the resident-token footprint of the core skill set stays under threshold;
- **conformance fixtures** — projected assets carry no host-unsupported references on their default path;
- **security + relay protocol suites** — dry-run discloses but never writes, unmanaged targets are refused without `--force`, symlink escapes are rejected, and the relay protocol invariants hold.

## Conventions

- **Agent-neutral by default.** A skill's default path is plain `oma` commands plus markdown, identical for Claude Code and Codex. Host-only accelerations (CC subagents, plan mode) are clearly-marked optional branches, never the default.
- **Mechanical logic belongs in the binary, judgment in the skill.** If something can be counted, validated, or persisted, it goes in Go — testable and fail-closed — not in a prompt.
- **Minimal resident footprint.** Skills cost context on every turn; keep them small and installable on demand.

See [`docs/design-philosophy.md`](docs/design-philosophy.md) for the reasoning behind these.

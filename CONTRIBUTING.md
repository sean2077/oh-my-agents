# Contributing to oma

Every change must clear the repository's local and CI quality gates. `oma` also ships
the `pair-delivery` workflow for changes that benefit from an independent second
agent, but that workflow is optional rather than a prerequisite for contributing.

## Choosing the delivery process

Use `pair-delivery` when the user requests cross-review or when an available
independent reviewer materially improves confidence in a risky change. If no
independent peer host is available, continue with a focused diff review and the
normal verification gates; do not block ordinary local work or relabel same-host
assistance as independent cross-host review.

For that local fallback, review the final diff under four explicit headings:

- **Spec compliance** — does the change satisfy the requested behavior and the
  authoritative repository contract?
- **Standards & quality** — does it follow repository conventions without introducing
  avoidable complexity, regressions, or maintenance risk?
- **Verification** — which focused and repository-wide checks ran, and what were their
  observed results?
- **Limitations** — what was not checked or remains uncertain?

The on-demand `code-review` skill may perform this review-only pass over an existing
diff when installed. Whether performed inline or through that skill, this is a
self-review, not an independent review. Report it as local verification.
Never describe same-host assistance as cross-reviewed or use it to satisfy a
`pair-delivery` gate.

When `pair-delivery` is selected, its two review gates remain part of that workflow:

1. **Plan.** The implementing agent (the *lead*) writes a plan: scope, approach, acceptance criteria.
2. **Gate 1 — plan review.** A second agent reviews the plan and returns a typed verdict. Only an `approve` clears the gate.
3. **Implement.** The lead does the work on a feature branch / worktree, clears a concrete verification bar, and records the evidence.
4. **Gate 2 — code review.** The second agent reviews the actual diff and returns a verdict; the lead dispositions every finding (adopt / partially adopt / reject, with reasoning).
5. **Decision.** When both sides agree, the lead publishes a decision and the binary stamps a completion receipt binding the approved plan, the non-lead `approve` review, and the reviewed head by content hash.

The roles are configurable and agent-neutral. See the `pair-delivery` skill and
[`docs/reference/workflows.md`](docs/reference/workflows.md) for gate semantics, and
[`docs/reference/relay-v2-protocol.md`](docs/reference/relay-v2-protocol.md) for the
ledger. Once a pair run is chosen, follow its gates rather than claiming a partial run
was cross-reviewed.

## Spec-first

The documents under [`docs/`](docs/) are authoritative; the implementation follows them, not the reverse. A change that alters a contract updates the relevant [`docs/reference/`](docs/reference/) document in the same delivery. Build everything to its terminal shape — no migration layers, no "thin version first."

## Building and testing

```bash
go build ./...
go test ./...          # full suite — installation/projection tests use a temp $HOME
gofmt -l .             # must print nothing
go vet ./...
```

CI runs the 3-platform test matrix with `-race`, `gofmt` / `vet` / `build`, `golangci-lint`, and `govulncheck` on every push and PR, as a reusable pipeline. A release calls that **same** pipeline as a hard gate before publishing.

Optionally enable the repo git hooks once with `make hooks` (`git config core.hooksPath scripts/hooks`). The `pre-commit` hook is a dev aid — not part of shipped `oma` — that warns when a staged `SKILL.md`/`AGENTS.md` body exceeds the content budget (default 120 non-blank lines after the frontmatter), to keep agent content lean. It rides git, so it fires identically for Codex, Claude Code, and humans. Tune with `OMA_CONTENT_BUDGET_LINES=<n>`, or set `OMA_CONTENT_BUDGET_BLOCK=1` to fail the commit instead of warning.

## Releasing

Releases are changelog-first and tag-triggered:

1. Update [`CHANGELOG.md`](CHANGELOG.md) with a section headed `## vX.Y.Z - YYYY-MM-DD`; the GitHub Release body is extracted from that section.
2. Run the local gate: `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `govulncheck ./...`, and `make release VERSION=vX.Y.Z`.
3. Commit the changelog and related release changes, then create an annotated `vX.Y.Z` tag.
4. Push the branch and tag. The release workflow runs the full gated pipeline, then **promotes the exact artifacts it built and verified** (no rebuild) — re-checking the version stamp and checksums, attaching a build-provenance attestation and an SBOM, and uploading without `--clobber` — before creating or updating the GitHub Release.

### The quality gates

Beyond `go test`, CI enforces:

- **`-race` + `govulncheck`** — the 3-platform test matrix runs under the race detector, and the module is scanned for known vulnerabilities;
- **refcheck** — every `oma …` reference in a shipped skill resolves to a real command;
- **context budgets** — `oma doctor budget --profile core4 --max-resident-tokens 400` keeps the core resident surface under the release ceiling, and the asset fixture rejects any active description over its manifest budget;
- **conformance fixtures** — projected assets carry no host-unsupported references on their default path;
- **security + relay protocol suites** — dry-run discloses but never writes, unmanaged targets are refused without `--force`, symlink escapes are rejected, and the relay protocol invariants hold.

## Conventions

- **Agent-neutral by default.** A skill's default path is plain `oma` commands plus markdown, identical for Claude Code and Codex. Host-only accelerations (CC subagents, plan mode) are clearly-marked optional branches, never the default.
- **Mechanical logic belongs in the binary, judgment in the skill.** If something can be counted, validated, or persisted, it goes in Go — testable and fail-closed — not in a prompt.
- **Minimal resident footprint.** Skills cost context on every turn; keep them small and installable on demand.

See [`docs/design-philosophy.md`](docs/design-philosophy.md) for the reasoning behind these.

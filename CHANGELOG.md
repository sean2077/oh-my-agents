# Changelog

> Release flow: when you decide to release, update this file (the content is the GitHub Release body, written for release-page readers), commit + tag `vX.Y.Z`, and push. CI runs `scripts/extract-changelog.sh` to slice the section whose heading matches the tag into the release notes.
>
> Section heading format: `## vX.Y.Z - YYYY-MM-DD` (CI matches the tag by exact prefix; a tag with no matching section fails the release, fail-closed).

## Unreleased

First end-to-end implementation of `oma` and the core skill set. No versioned release has been cut yet; the publish point is a user decision.

- **CLI (`oma`)**: asset install/projection/rollback with a fail-closed security contract; hook fragment injection with a token-exact byte contract; `doctor` diagnostics and the resident-token budget gate; the relay v2 pair ledger (atomic publish, integrity sidecars, sequence reservation, `wait` handoff); solidified `interview` and `ralph` workflow surfaces; checksum-verified `self-update`.
- **Skills**: the four core workflow skills (`deep-interview`, `ralph`, `autopilot`, `pair-delivery`), agent-neutral and projected to both Claude Code and Codex. Total resident surface: ~275 tokens.
- **Tooling**: main-branch install script with Git Bash / Windows `oma.exe` defaults, cross-platform release build script, changelog-driven release notes, and CI (test matrix + `gofmt`/`vet`/`build` + `golangci-lint`).

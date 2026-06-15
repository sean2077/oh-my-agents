# Changelog

> Release flow: when you decide to release, update this file (the content is the GitHub Release body, written for release-page readers), commit + tag `vX.Y.Z`, and push. CI runs `scripts/extract-changelog.sh` to slice the section whose heading matches the tag into the release notes.
>
> Section heading format: `## vX.Y.Z - YYYY-MM-DD` (CI matches the tag by exact prefix; a tag with no matching section fails the release, fail-closed).

## v0.2.0 - 2026-06-15

This release tightens the install and host-integration contract after the first tagged release.

- **Install path**: the default installer now resolves and downloads the latest prebuilt GitHub Release binary, verifies it against `checksums.txt`, and keeps the source-build path as a fallback or explicit override. Tagged source installs no longer fall back to a `dev` version stamp.
- **Build/release tooling**: the Makefile now has stamped `build`, `install`, `check`, `ci`, and `release` targets, so local binaries report the same version metadata shape as release assets.
- **Host integration**: removed commands that rewrote Claude Code / Codex host config files directly. Hook and statusline setup now ships as discoverable assets plus a manual wiring guide, keeping `oma` out of user-owned host configuration.
- **Project packaging**: added the MIT license to make redistribution and binary installation terms explicit.

## v0.1.0 - 2026-06-12

First tagged release: the complete `oma` CLI and the core skill set, built end-to-end through the project's own cross-reviewed pair-delivery workflow.

- **CLI (`oma`)**: asset install/projection/rollback with a fail-closed security contract; hook fragment injection with a token-exact byte contract; `doctor` diagnostics and the resident-token budget gate; the relay v2 pair ledger (atomic publish, integrity sidecars, sequence reservation, `wait` handoff) with its experience layer — `preflight`, a binding-scoped `statusline`, and auto-continue `hooks` (guarded absolute-path commands with per-event matchers, for both Claude Code and Codex); solidified `interview` and `ralph` workflow surfaces; checksum-verified `self-update`.
- **Skills**: the four core workflow skills (`deep-interview`, `ralph`, `autopilot`, `pair-delivery`), agent-neutral and projected to both Claude Code and Codex. Total resident surface: ~275 tokens. The skills require the `oma` CLI on `PATH`.
- **Tooling**: main-branch install script with Git Bash / Windows `oma.exe` defaults, cross-platform release build script (six platforms + a checksums manifest), changelog-driven release notes, and CI (test matrix + `gofmt`/`vet`/`build` + `golangci-lint`).

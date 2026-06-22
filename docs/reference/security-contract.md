# Security Contract Implementation Spec

> Implementation-level spec for the seven clauses of the spec's "Security and permission requirements"; each clause maps to a Phase B test.

## 1. --dry-run and write-time disclosure

- `--dry-run` is a global persistent flag, inherited by all mutating commands (the full asset family, `state set`, `relay draft`/`publish`/`close`/`init`, `self-update`): it runs the complete compute-and-validate path, prints the **exact absolute paths** to be created/modified/deleted along with the operation type, and guarantees zero disk writes (including zero backups and zero leftover temp files).
- Test: after a dry-run, snapshot-compare the target directory tree and assert no change.

## 2. Overwrite protection and backup/rollback

- The write target already exists and is **not oma-managed** (no registry record, or the content hash does not match) → refuse, prompt for `--force`.
- `--force`: first copy the current state in full to `~/.config/oma/backups/<UTC timestamp>/<original relative path>`, write a backup manifest into the registry, then overwrite.
- `oma asset rollback <name> [--to <id>]`: restore in reverse from the manifest; the restore itself also obeys this section (it will not silently overwrite non-oma content newer than the backup — on conflict it refuses and prompts).
- Test: overwrite refused, `--force` produces a restorable backup, rollback restores identically, rollback conflict refused.

## 3. Projection and path constraints

- Any projection/resolution path is first normalized with `EvalSymlinks` plus Windows reparse-point target checks where needed; the result must lie within a **trusted root**: the canonical location `~/.agents/`, each agent's known directories, `.oma/`, and the repo checkout (dev link). Out of bounds → refuse.
- Reject path-traversal input (asset names, `--ledger-root`, etc. that escape the bounds once `..` is normalized).
- On POSIX, a target parent directory that is world-writable (writable by other users) → refuse projection. On native Windows, Go's mode bits are only an ACL approximation, so this specific POSIX bit check is skipped and trusted-root checks remain the enforcement point.
- Test: three classes of refusal on POSIX — traversal fixture, out-of-bounds symlink, world-writable directory. Windows test coverage asserts trusted-root behavior, junction escape refusal, and managed-copy projection integrity.

## 4. Permission bits

- New directories are 0700, files 0600 (applies to `.oma/`, `~/.config/oma/`, and the entire relay ledger); doctor validates and can report drift.
- **oma never writes any host config file.** It places hook assets canonically only (canonical-only) and projects ordinary assets by symlink on Unix-like hosts, or by junction/copy on native Windows; host config stays the user's to manage. The unreliable, host-mutating step — hook wiring — is documented for the user to do by hand (the wiring spec lives in relay-v2-protocol.md §12.4) rather than performed by oma.
- Projection removal preserves the "leave foreign obstacles in place + warn" semantics. Symlink and junction projections must still point at the canonical path; copy projections must digest-match the recorded managed content. `--dry-run` runs the same validation for remove/rollback as the real path does.

## 5. self-update trust chain

- The update source is **restricted** to GitHub Releases of the compile-time constant `github.com/sean2077/oh-my-agents`; it does not follow cross-repo/cross-domain redirects; the asset name must match the `oma_<version>_<os>_<arch>` pattern, and an unexpected name is fail-closed.
- After download, verify the SHA-256 from the `checksums.txt` shipped with the release; a mismatch → refuse and keep the current binary (when the release pipeline supports signing, this upgrades to signature verification — a minor-evolution slot is left open).
- Replacement: write a tmp file in the same directory → verify it is executable → atomic rename to swap it in; the old binary is first backed up as `<path>.old`; if the post-replacement self-check (a `--version` subprocess) fails → automatically roll back to `.old`.
- Target path not writable → degrade to printing manual-update guidance (no privilege escalation, no sudo).
- `--check` is strictly read-only: it only queries and compares versions, with zero disk writes.
- **Installer parity** (`scripts/install.sh`, `scripts/install.ps1`): first install consumes the same release/asset/checksum contract and is equally fail-closed — it resolves a release, downloads only the `oma_<version>_<os>_<arch>` asset, verifies its SHA-256 against the release `checksums.txt`, then asserts the freshly installed binary reports exactly the requested version. It never silently degrades to a source build or to the unreleased `main` branch: a source build is an explicit opt-in (`OMA_INSTALL_FROM_SOURCE=1`) that still prefers the newest released tag. The default install tracks the latest release; for a reproducible install the README documents fetching the installer script pinned to a release tag (immutable) instead of the mutable `main` ref.
- Test: checksum mismatch, metadata unavailable, target not writable, replacement interrupted (kill injection), self-check failure rollback, `--check` zero disk writes.

## 6. relay peer-input handling

- All fields of a peer artifact are treated as untrusted: the frontmatter passes strict schema validation (unknown kind/status refused); the body is rendered as text only, and oma **never** parses or executes any command within it; `touched_paths` is passed through for display only.
- `.sha256` verification fails / `.ready` missing → the content is not returned to the caller (fail-closed).
- Secret-leak prevention: before publishing, run a pattern scan over the body, `prompt_for_next`, and any frontmatter fields that may contain user text (a regex set for common token/key shapes); this is **enforced, with no bypass switch in v1**; a hit → refuse to publish and point at the line number. Handling a false positive = edit the artifact, or register a **narrow-scope allow pattern** in the appendix of this contract (`oma doctor` reports the effective allow list). doctor includes a ledger sweep using the same rule.
- Test: tampered ledger refused-read, unknown schema refused, secret pattern refused-publish.

## 7. Threat matrix

| Threat | Defense | Test |
|---|---|---|
| Malicious/corrupt asset overwrites user config | §2 overwrite protection + backup | force/rollback suite |
| Symlink escape writes an arbitrary path | §3 trusted-root constraint | traversal/out-of-bounds suite |
| oma mis-edits/corrupts host config | §4 oma zero host writes (hook injection removed, documented for manual wiring) | conformance (hook→skip) suite |
| Supply chain: forged update package | §5 source restriction + checksum + rollback | self-update suite |
| Pair peer injects instructions / poisons input | §6 untrusted input + fail-closed | relay security suite |
| Secrets spread into the ledger | §6 pre-publish scan | secret refused-publish |

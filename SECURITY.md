# Security Policy

## Supported versions

Security fixes are issued for the **latest released minor** of `oma`. Once `1.0`
ships, the current and previous minor (`1.N` and `1.N-1`) receive fixes; older
versions should upgrade. The installers and `oma self-update` are checksum- and
version-verified (see [Trust chain](#trust-chain)).

| Version | Supported |
|---|---|
| latest minor | ✅ |
| previous minor (post-1.0) | ✅ |
| older | ❌ — please upgrade |

## Reporting a vulnerability

**Do not open a public issue for a security vulnerability.**

Report it privately through GitHub's advisory form:
**https://github.com/sean2077/oh-my-agents/security/advisories/new**
(repository → **Security** → **Report a vulnerability**).

Please include the affected version (`oma version`), platform, a description, and
ideally a minimal reproduction. You will get an acknowledgement within **5
business days** and a remediation plan or assessment within **30 days**. Please
allow coordinated disclosure before any public write-up.

## Scope

In scope: the `oma` binary, the installers (`scripts/install.sh`,
`scripts/install.ps1`), the release / `self-update` trust chain, and the relay
ledger's handling of untrusted peer input. The security model and its tests are
documented in [`docs/reference/security-contract.md`](docs/reference/security-contract.md).

Out of scope: vulnerabilities in a host (Claude Code, Codex) or in your own
`settings.json` / `hooks.json` — `oma` never writes host config. Issues that
require an already-compromised machine, or a malicious local user who already has
write access to your `~/.agents` / `~/.config/oma`, are generally out of scope.

## Trust chain

Releases ship a `checksums.txt`; `oma self-update` and the installers verify both
the SHA-256 and the installed binary's reported version, fail closed, and never
follow cross-repo / cross-domain redirects (security-contract §5). Release
binaries, the `assets-<version>.tar.gz` bundle, and the SBOM also carry
build-provenance attestations — verify one with:

```bash
gh attestation verify oma_<version>_<os>_<arch> --repo sean2077/oh-my-agents
```

## Release withdrawal

A release found to ship a vulnerability is superseded by a patched release;
`CHANGELOG.md` records the issue and the fixed version. The release workflow
publishes through a draft that is verified before going public and never passes
`--clobber`, so the **pipeline itself** does not overwrite a published asset.
This is workflow discipline, not a platform guarantee: until GitHub's
immutable-releases setting is enabled for this repository, a maintainer (or a
stolen token) with write access could still alter or delete assets. Treat the
build-provenance attestation and `checksums.txt` as the authoritative integrity
signal, and trust a new version over any in-place change.

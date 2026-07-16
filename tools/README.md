# Repository tooling

`tools/` contains internal contributor and CI commands. Stable end-user
installers remain under [`../scripts/`](../scripts/) because those paths are
part of the public download and security contract.

| domain | path | entry point |
|---|---|---|
| Tool inventory | `tools-manifest.tsv` | `bash tools/manifest-check.sh` |
| Release | `release/` | `make release VERSION=vX.Y.Z` |
| Installer regression | `install/` | `bash tools/install/test-install.sh` |
| Git hooks | `git-hooks/` | `make hooks` |

The manifest classifies every committed command surface by audience, hazard,
and verifier. Run `make tooling-check` after adding, moving, or deleting any
script or Python CLI.

The contributor harness is vendored separately under [`.agents/tools/`](../.agents/tools/)
by `agent-scaffold`; it remains registered in the manifest but is not a `tools/`
subdirectory. Every directory listed above is owned by this repository.

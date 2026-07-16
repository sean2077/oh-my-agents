# Repository tooling

`tools/` contains internal contributor and CI commands. Stable end-user
installers remain under [`../scripts/`](../scripts/) because those paths are
part of the public download and security contract.

| domain | path | entry point |
|---|---|---|
| Tool inventory | `tools-manifest.tsv` | `bash tools/manifest-check.sh` |
| Agent harness | `agent/` | `bash tools/agent/worktree.sh --help` |
| Release | `release/` | `make release VERSION=vX.Y.Z` |
| Installer regression | `install/` | `bash tools/install/test-install.sh` |
| Git hooks | `git-hooks/` | `make hooks` |

The manifest classifies every committed command surface by audience, hazard,
and verifier. Run `make tooling-check` after adding, moving, or deleting any
script or Python CLI.

`tools/agent/` is vendored by the `agent-scaffold` skill and intentionally keeps
its upstream-owned path. Other subdirectories are owned by this repository.

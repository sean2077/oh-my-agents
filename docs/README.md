# oma documentation

Start here:

- **[design-philosophy.md](design-philosophy.md)** — the *why*: context scarcity (accuracy first, cost second), the mechanical-vs-judgment split, and the constraints that follow.
- **[architecture.md](architecture.md)** — the *how*: the four layers, the asset / projection model, the relay ledger, and the repository layout.
- **[tutorial.md](tutorial.md)** — an end-to-end walkthrough: install → core4 → deep-interview → autopilot → ralph → pair-delivery → resume → upgrade / migrate / uninstall.
- **[skill-authoring.md](skill-authoring.md)** — how to write small, trigger-accurate skills that agents actually follow.
- **[companion-skills.md](companion-skills.md)** — a deliberately small, versioned list of external skills that complement oma without duplicating its workflow surfaces.
- **[porting-to-new-harness.md](porting-to-new-harness.md)** — how to map oma skills to another harness without changing skill bodies or user-owned files.

Authoritative reference — the spec the implementation follows:

- **[reference/relay-v2-protocol.md](reference/relay-v2-protocol.md)** — the pair-delivery relay protocol and the experience layer.
- **[reference/command-tree.md](reference/command-tree.md)** — the full CLI surface: signatures, `--json` contracts, exit codes.
- **[reference/workflows.md](reference/workflows.md)** — the interview / ralph / pair-delivery state machines and delivery gates.
- **[reference/adapter-conformance.md](reference/adapter-conformance.md)** — projection, refcheck, and budget conformance.
- **[reference/schemas.md](reference/schemas.md)** — registry / state / ledger schemas and the versioning policy.
- **[reference/security-contract.md](reference/security-contract.md)** — the fail-closed security model.
- **[reference/config.md](reference/config.md)** — the configuration layer.

Project policies (repo root): **[../STABILITY.md](../STABILITY.md)** (compatibility contract), **[../SECURITY.md](../SECURITY.md)** (vulnerability reporting), **[../CONTRIBUTING.md](../CONTRIBUTING.md)** (delivery process).

See also **[examples/](examples/)** for working snippets and **[history/](history/)** for historical decision records, including the implemented borrowing notes for [`superpowers`](history/borrow-from-superpowers.md) and [`mattpocock/skills`](history/borrow-from-matt-pocock-skills.md).

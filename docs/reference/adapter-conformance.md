# Adapter Capability Matrix and Conformance Spec

> Defines manifest `targets` semantics, per-agent projection rules, the Codex fallback, the budget injection surface, the conformance fixture format, and the refcheck extraction rules.

## 1. Asset manifest (`assets/<type>/<name>/manifest.json`, schema `oma-asset/1`)

```json
{
  "schema": "oma-asset/1",
  "name": "deep-interview",
  "type": "skill",                          // skill | subagent | hook | prompt
  "version": "tied to the repo tag, recorded at install time",
  "targets": ["claude", "codex"],           // or ["claude"] (CC-only); "shared" = canonical placement only, no projection
  "description_budget_tokens": 80,
  "fallback": "Codex-side degradation note (required for CC-only assets)"
}
```

## 2. Projection rule matrix

| Asset type | Canonical location (the file itself) | claude projection | codex projection |
|---|---|---|---|
| skill | `~/.agents/skills/<name>/` | symlink `~/.claude/skills/<name>` | symlink per the codex skills convention; not projected where unsupported, plus a doctor warning |
| subagent | `~/.agents/agents/<name>.md` | symlink `~/.claude/agents/<name>.md` | **not projected** (Codex has no subagent) plus a `manifest.fallback` note |
| hook | `~/.agents/hooks/<name>/` | **not projected** (canonical placement only) plus a skip note | **not projected** (canonical placement only) plus a skip note |
| prompt | `~/.agents/prompts/<name>.md` | (on demand) `~/.claude/commands/` | `~/.codex/prompts/<name>.md` |

- Projection is always by symlink. **oma never rewrites any host config file**: the earlier approach of injecting a hook fragment into `~/.claude/settings.json` / `~/.codex/hooks.json` has been removed — behavior that is uncertain or that mutates host state is documented as guidance rather than performed by a command (see the same-source decision in relay-v2-protocol.md §12.4).
- **A hook asset is canonical-only**: `manifest.json` + `fragment.json` still land alongside the asset under `~/.agents/hooks/<name>/`, but oma neither parses nor injects them; `agentdir.For(hook)` returns skip for both targets. The user wires the entry into their own `settings.json`/`hooks.json` **by hand**, following the contents of `fragment.json` (the wiring spec and guards are in relay-v2-protocol.md §12.4).
- Uninstall = remove the symlink projection + remove the canonical entry + de-register from the registry (a hook has no projection, so only the canonical entry is removed and de-registered).
- The concrete codex-side paths are maintained in a constant table (`internal/agentdir`; on a machine without codex they are verified by file assertion, see §6).

## 3. Dual-target consistency contract

- Every core-workflow skill must have a **default path that does not depend on CC-native capabilities** (oma commands + text-driven).
- Skill text retains only the core workflow steps, decision rules, state contract, and safety boundaries; installation, PATH, platform onboarding, and other general product instructions belong in the README / docs, not in `SKILL.md`, to avoid bloating the resident prompt.
- A CC acceleration branch must be marked explicitly (the recommended uniform marker is `> **CC acceleration**: …`), and **must not produce workflow state that Codex cannot inspect or resume via oma commands** (state lands only in `.oma/state/` and the relay ledger).
- CC-only mechanisms (AskUserQuestion, subagent invocation, etc.) must give a corresponding fallback form in the skill text (free-text questioning, single-threaded sequential execution).

## 4. refcheck extraction rules (static command-reference check)

- Scan scope: code blocks and inline code in `assets/**/SKILL.md` and `assets/**/references/**/*.md`.
- Extraction: tokenize in shell style, stopping at a flag (leading `-`), a redirection, a pipe, a semicolon, or end of line; for the token sequence, find the **longest registered cobra command prefix** match (supporting arbitrary nesting depth: `oma relay pair ensure`, `oma doctor budget`).
- Verdict: the longest non-flag prefix must **exactly equal** a registered runnable command (or a command group declared in the docs for use in prose examples) to count as a valid reference; a case like `oma relay pair typo` — a "valid prefix + invalid leaf" — fails.
- Reference table: reflected from the cobra command tree (including full nested paths).
- Failure condition: any invalid reference → fail (exit code 4). Exemptions: none (terminal-state principle).
- Required fixtures: `oma relay pair ensure` (valid three-level), `oma relay pair typo` (invalid leaf), `oma doctor budget` (valid two-level + flags), `oma asset link --dev` (flag truncation), and a multi-line shell snippet.

## 5. Budget injection surface model (`oma doctor budget`)

- **Counted objects (claude profile)**: for each installed skill projected to claude, the frontmatter `name` + `description` fields; for a subagent, `name` + `description` + `whenToUse`; modeled on Claude Code's actual resident loading behavior (only the resident surface is counted, not the on-demand `SKILL.md` body or `references/`). **A hook asset is canonical-only (not injected into the host, not resident) and counts as zero** — a manually wired hook is a user-managed surface and is not counted against the oma budget gate.
- tokenizer: a pinned approximation algorithm `tok ≈ ceil(utf8_bytes/4)`, with the constant `BudgetAlgoVersion = "approx-b4/1"` stored and written into the `--json` output; calibrated once against a `/context` measurement before each release, with the deviation recorded in the dogfood log.
- profile: `core4` = deep-interview, autopilot, ralph, pair-delivery (fully metered from Phase C onward); threshold 2000 (CI gate), internal target 1800.

## 6. conformance fixtures (offline dual-target verification)

- Location: case files at `testdata/conformance/{claude,codex}.json`. Each case carries `manifest` (an inline oma-asset/1 document), `payload_file` (+ optional `payload_content`), and `want_rel_home` (the expected symlink projection location; empty = skip expected — the case for both hook and shared assets). oma projects only symlinks, with no injection assertion.
- Test flow: fake HOME (t.TempDir) → engine Install (narrowed to a single agent) → assert per `want_kind`: the symlink target points at the canonical location, or the injected command can be retrieved from the host config by `_oma_asset`.
- Default-path check: for each skill, assert that the default-path text contains no reference unsupported by the target side (for example, an `AskUserQuestion` or subagent invocation appearing in a codex fixture fails; it is allowed inside an explicit CC-acceleration marker block).
- The real-world constraint of having no codex on the machine: codex-side acceptance is judged by fixture-file assertion; a real-machine smoke test is a non-blocking Phase D follow-up.

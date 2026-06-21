# Configuration Layer (viper)

This document defines oma's user-configurable settings, the precedence chain, the config file format, and the strict boundary between viper and schema-persisted data.

## 1. Scope and boundary (most important)

viper **governs only the "user-configurable layer"** — the settings that have sensible defaults, can be overridden from multiple sources, and express *user intent*.

viper **never touches schema-persisted data**: `registry.json`, `.oma/state/*.json`, the relay ledger, `manifest.json`. These are read and written precisely by `encoding/json` + `schemaMajor()` and fail-closed (schemas.md), because they must reject on an unrecognized major and reject on corruption — viper's loose multi-source merge and silent defaulting would break that semantics.

Discriminating principle:

| | Config layer (viper) | Schema data (encoding/json) |
|---|---|---|
| Nature | User intent, defaultable, overridable | Persistent system state, precise contract |
| Missing | Use built-in default | Context-dependent (missing registry = empty table; everything else fail-closed) |
| Corrupt | Parse error → fail-closed error | Fail-closed rejection |
| Multi-source merge | Yes (precedence chain §3) | No (single authoritative file) |

## 2. Config file location and format

- Format: **TOML** (consistent with the user's existing `~/.codex/config.toml` — readable and comment-friendly).
- User config: `~/.config/oma/config.toml` (XDG; changes accordingly when `OMA_HOME` overrides the home anchor).
- Project config: `<project root>/.oma/config.toml`, where linked git worktrees resolve back to the primary checkout's project root. **Private/local by default**: `.oma/` is gitignored, so project config is not committed by default; a team that wants to share it must explicitly `git add -f` (sharing is the user's explicit decision, not the default).
- Both files are **optional**: absent → fall through to lower-precedence sources. Present but with a TOML syntax error / type mismatch / unknown schema major → **fail-closed** (exit 3), never silently ignored.

## 3. Precedence chain

Highest to lowest (higher overrides lower, merged per-key):

```
1. Command-line flag (cobra/pflag)
2. Environment variable (OMA_ prefix, explicit BindEnv)
3. Project config (<project root>/.oma/config.toml)
4. User config (~/.config/oma/config.toml)
5. Built-in default (SetDefault)
```

This is isomorphic to the resolution chain for the deep-interview threshold in the spec (flag > project > user > default), now unified under the config layer.

## 4. List of configurable settings

| Config key | Type | Default | Flag | Environment variable |
|--------|------|------|------|----------|
| `relay.ledger_root` | path | `.oma/relay/` | `--ledger-root` | `OMA_RELAY_LEDGER_ROOT` |
| `relay.stale_after` | duration | `15m` | — | `OMA_RELAY_STALE_AFTER` |
| `relay.wait_timeout` | duration | `60m` | `--timeout` | `OMA_RELAY_WAIT_TIMEOUT` |
| `budget.max_resident_tokens` | int | `2000` | `--max-resident-tokens` | `OMA_BUDGET_MAX_TOKENS` |
| `interview.threshold` | float | (no standalone default, see §4a) | `--threshold` | `OMA_INTERVIEW_THRESHOLD` |
| `interview.depth` | enum(quick/standard/deep) | `standard` | `--depth` | — |
| `asset.default_agents` | list | `[claude, codex]` (narrowing request, see §4b) | `--agent` | `OMA_ASSET_AGENTS` |

**Bootstrap-level (not via viper, pure env):**
- `OMA_HOME` — determines the location of the config file itself, a chicken-and-egg problem, so it is the lowest-level env, read directly at CLI startup.
- `OMA_RELAY_AUTHOR` — explicit operator override for relay identity. Manual use must also provide `OMA_RELAY_SESSION_ID` or `OMA_SESSION_ID`, so two same-author shells do not share one relay session. **Identity is not user-preference data**: it is derived by default from the protocol's platform signals (relay-v2-protocol §4), with env serving only as an explicit override; it **never enters the config file** — in particular, project-local `.oma/config.toml` must not change the author, or it could break or forge pair bindings.

### 4a. interview effective-threshold resolution (aligned with A3 workflows.md)

threshold is the single underlying value; depth is a convenient alias that sets threshold (quick=0.30 / standard=0.20 / deep=0.10). **The built-in default has only one semantics**: standard (0.20) — threshold has no standalone default value, to avoid `interview.depth` being permanently masked by a default `interview.threshold=0.20`.

The effective threshold takes the **first explicit source** by precedence:

```
1. --threshold flag
2. --depth flag (mapped to a threshold value)
3. OMA_INTERVIEW_THRESHOLD env
4. Project config interview.threshold or interview.depth
5. User config interview.threshold or interview.depth
6. Built-in default 0.20 (standard)
```

When threshold and depth are both set explicitly within the same layer → threshold wins (more precise), and `threshold_source` records that source and notes that depth was ignored. `threshold_source` records the final effective source string (such as `--depth=deep`, `project config interview.threshold`, `default(standard)`). This chain is the expansion of the "config" layer in A3's "`--threshold` > `--depth` > config > default"; the two do not conflict.

### 4b. asset projection agents are a narrowing request, not a manifest override

`asset.default_agents` and `--agent` supply *requested_agents*. The final projection target = **`manifest.targets ∩ requested_agents`**:

- The manifest `targets` is always authoritative for the projectable range (A4); config/flag can only narrow within it, **never extend**.
- An asset with `targets:["claude"]` still projects only to claude under requested=[claude,codex]; `targets:["shared"]` never projects.
- Requesting a target the manifest doesn't support (such as `--agent codex` on a claude-only asset) → **reported** (command prompt / doctor warning), neither silently extended nor silently swallowed.
- The default `[claude, codex]` means "no additional narrowing" and is safe to intersect with any targets.

## 5. viper usage constraints (tightening "loose" into "predictable")

The terminal-state principle requires configuration behavior to be deterministic and testable, so viper's usage is constrained:

- **Explicit binding**: bind each setting with `SetDefault` + `BindPFlag` + `BindEnv(key, "OMA_...")`; **do not use `AutomaticEnv`** (its implicit env mapping is unpredictable and may collide with unrelated environment variables).
- **env prefix**: `OMA_`, with `.` in the key mapped to `_` (such as `relay.stale_after` → `OMA_RELAY_STALE_AFTER`), but the mapping is declared via explicit `BindEnv`, not relying on automatic conversion.
- **Strongly-typed exit**: after `Unmarshal` into the strongly-typed `Config` struct, do range validation (such as `interview.threshold` ∈ [0,1], `*_after`/`timeout` being positive durations, `depth` within the enum, `default_agents` ⊆ {claude,codex}); out of range → fail-closed.
- **Config file error grading**: `ReadInConfig`'s `ConfigFileNotFoundError` is treated as "no such source" and continues; everything else (syntax/permissions) → fail-closed.
- **Config schema**: the config file may contain `schema = "oma-config/1"`; absent → treated as the current major (tolerating a hand-written omission); present but major ≠ 1 → fail-closed (consistent with the rest of schema). `oma-config/1` is registered in the schema registry and emitted in the `schemas` field of `oma version --json` (it is user config rather than runtime state, but it is still a schema string with fail-closed major handling).

## 6. Interface with the command tree / exit codes

- Config parsing failures uniformly go through `ExitState(3)` (environment/state error, command-tree.md §1).
- A new `oma config` command group (already registered in command-tree.md §7):
  - `oma config show [--json]` prints the effective configuration and **the source of each key** (flag/env/project/user/default); read-only, zero disk writes. The source annotation relies on a controlled load + **per-key source map** (see §7 implementation requirements), not inferred back from viper's `AllSettings()`.
  - `oma config path [--json]` prints the resolved location of the user/project config files; read-only.

## 7. Implementation and testing requirements (Phase B config-layer tasks)

- New `internal/config` package: `Load(layout) (*Config, error)` assembles the precedence chain and returns the strongly-typed config; the viper instance does not leak out (the rest of the packages see only `*Config` and don't depend on viper directly — isolating a heavy dependency to ease future replacement).
- **per-key source map**: load source by source in controlled order (default→user→project→env→flag), recording which keys each layer overrode as it is applied, building a `map[key]source` for `config show` to use; not inferred after the fact from viper's `AllSettings()` (which exposes only the merged value, losing the source).
- The command layer consumes `*Config` at construction points such as `newEngine()`, replacing the currently scattered `os.Getenv` reads.
- Tests (fake HOME + temporary config files):
  - Per-layer override of the precedence chain (default → user → project → env → flag, adding one layer at a time and asserting the final value);
  - Syntax-error config fails closed; unknown schema major fails closed; out-of-range values (threshold=1.5, negative duration, illegal agent) fail closed;
  - `config show` source annotation is correct;
  - Missing config files fall back to defaults without error.
- Dependencies: only add `github.com/spf13/viper` (the config layer's sole new heavy dependency; the schema-data path introduces no viper).

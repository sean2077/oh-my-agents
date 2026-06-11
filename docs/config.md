# 配置层规范（viper）

> Phase A 设计文档（A7，新增）。状态：待设计评审。定义 oma 的用户可配置项、优先级链、配置文件格式，以及 viper 与 schema 持久数据之间的严格边界。

## 1. 范围与边界（最重要）

viper **只管「用户可配置项层」**——那些有合理默认、可被多源覆盖、表达*用户意图*的设置。

viper **绝不触碰 schema 持久数据**：`registry.json`、`.oma/state/*.json`、relay 账本、`manifest.json`。这些由 `encoding/json` + `schemaMajor()` 精确读写并 fail-closed（docs/schemas.md），原因是它们要求 major 不识别即拒绝、损坏即拒绝——viper 的多源宽松合并与静默缺省会破坏这一语义。

判别原则：

| | 配置层（viper） | schema 数据（encoding/json） |
|---|---|---|
| 性质 | 用户意图、可缺省、可覆盖 | 系统持久状态、精确契约 |
| 缺失 | 用内置默认 | 视上下文（registry 缺失=空表；其余 fail-closed） |
| 损坏 | 解析错 → fail-closed 报错 | fail-closed 拒绝 |
| 多源合并 | 是（优先级链 §3） | 否（单一权威文件） |

## 2. 配置文件位置与格式

- 格式：**TOML**（与用户环境既有的 `~/.codex/config.toml` 一致，可读、注释友好）。
- 用户配置：`~/.config/oma/config.toml`（XDG；`OMA_HOME` 覆盖 home 锚点时随之改变）。
- 项目配置：`<git 主 worktree>/.oma/config.toml`（项目级覆盖）。**默认 private/local**：`.oma/` 被 .gitignore，故项目配置默认不入库；团队要共享须显式 `git add -f`（共享是用户的显式决定，不是默认）。
- 两个文件都**可选**：不存在 → 用更低优先级来源。存在但 TOML 语法错 / 类型不符 / 未知 schema major → **fail-closed**（exit 3），绝不静默忽略。

## 3. 优先级链

从高到低（高者覆盖低者，逐键合并）：

```
1. 命令行 flag （cobra/pflag）
2. 环境变量    （OMA_ 前缀，显式 BindEnv）
3. 项目配置    （<worktree>/.oma/config.toml）
4. 用户配置    （~/.config/oma/config.toml）
5. 内置默认    （SetDefault）
```

这与 spec 中 deep-interview 阈值的解析链同构（flag > 项目 > 用户 > 默认），现在统一由配置层承载。

## 4. 可配置项清单

| 配置键 | 类型 | 默认 | flag | 环境变量 |
|--------|------|------|------|----------|
| `relay.ledger_root` | path | `.oma/relay/` | `--ledger-root` | `OMA_RELAY_LEDGER_ROOT` |
| `relay.stale_after` | duration | `15m` | — | `OMA_RELAY_STALE_AFTER` |
| `relay.wait_timeout` | duration | `60m` | `--timeout` | `OMA_RELAY_WAIT_TIMEOUT` |
| `budget.max_resident_tokens` | int | `2000` | `--max-resident-tokens` | `OMA_BUDGET_MAX_TOKENS` |
| `interview.threshold` | float | （无独立默认，见 §4a） | `--threshold` | `OMA_INTERVIEW_THRESHOLD` |
| `interview.depth` | enum(quick/standard/deep) | `standard` | `--depth` | — |
| `asset.default_agents` | list | `[claude, codex]`（收窄请求，见 §4b） | `--agent` | `OMA_ASSET_AGENTS` |

**Bootstrap-level（不走 viper，纯 env）**：
- `OMA_HOME` —— 决定配置文件本身的位置，存在鸡生蛋问题，故为最底层 env，CLI 启动时直接读取。
- `OMA_RELAY_AUTHOR` —— relay 身份的显式操作员覆盖。**身份不是用户偏好数据**：默认由协议（relay-v2-protocol §4）的平台信号推导，env 仅作显式覆盖；**绝不进配置文件**——尤其项目本地 `.oma/config.toml` 不得改 author，否则可破坏或伪造 pair 绑定。

### 4a. interview 有效阈值解析（与 A3 docs/workflows.md 对齐）

threshold 是唯一底层值；depth 是设定 threshold 的便捷别名（quick=0.30 / standard=0.20 / deep=0.10）。**内置默认只有一个语义**：standard（0.20）——threshold 不设独立默认值，避免 `interview.depth` 被一个默认 `interview.threshold=0.20` 永久遮蔽。

有效阈值按优先级取**第一个显式来源**：

```
1. --threshold flag
2. --depth flag（映射到 threshold 值）
3. OMA_INTERVIEW_THRESHOLD env
4. 项目配置 interview.threshold 或 interview.depth
5. 用户配置 interview.threshold 或 interview.depth
6. 内置默认 0.20（standard）
```

同层内 threshold 与 depth 同时显式 → threshold 优先（更精确），`threshold_source` 记该来源并标注忽略了 depth。`threshold_source` 记录最终生效来源串（如 `--depth=deep`、`project config interview.threshold`、`default(standard)`）。本链是 A3「`--threshold` > `--depth` > 配置 > 默认」中「配置」层的展开，二者不矛盾。

### 4b. asset 投影 agents 是收窄请求，不是 manifest 覆盖

`asset.default_agents` 与 `--agent` 提供 *requested_agents*。最终投影目标 = **`manifest.targets ∩ requested_agents`**：

- manifest `targets` 始终是可投影范围的权威（A4）；配置/flag 只能在其内收窄，**绝不扩展**。
- `targets:["claude"]` 的资产在 requested=[claude,codex] 下仍只投 claude；`targets:["shared"]` 始终不投影。
- 请求了 manifest 不支持的 target（如对 claude-only 资产 `--agent codex`）→ **报告**（命令提示 / doctor warning），既不静默扩展也不静默吞掉。
- 默认 `[claude, codex]` 表示「不额外收窄」，与任意 targets 取交集都安全。

## 5. viper 使用约束（把「宽松」收紧成可预测）

终态原则要求配置行为确定、可测试，因此对 viper 的用法加约束：

- **显式绑定**：用 `SetDefault` + `BindPFlag` + `BindEnv(key, "OMA_...")` 逐项绑定；**不使用 `AutomaticEnv`**（其隐式 env 映射不可预测，且可能撞上无关环境变量）。
- **env 前缀**：`OMA_`，键中的 `.` 映射为 `_`（如 `relay.stale_after` → `OMA_RELAY_STALE_AFTER`），但映射经显式 `BindEnv` 声明，不依赖自动转换。
- **强类型出口**：`Unmarshal` 到强类型 `Config` 结构体后做范围校验（如 `interview.threshold` ∈ [0,1]、`*_after`/`timeout` 为正 duration、`depth` 在枚举内、`default_agents` ⊆ {claude,codex}）；越界 → fail-closed。
- **配置文件错误分级**：`ReadInConfig` 的 `ConfigFileNotFoundError` 视为「无此来源」继续；其余（语法/权限）→ fail-closed。
- **配置 schema**：配置文件可含 `schema = "oma-config/1"`；缺失 → 视为当前 major（容忍用户手写遗漏）；存在但 major ≠ 1 → fail-closed（与其余 schema 一致）。`oma-config/1` 登记进 schema 注册表并在 `oma version --json` 的 `schemas` 中输出（它是用户配置而非运行时状态，但仍是带 fail-closed major 处理的 schema 串）。

## 6. 与命令树/退出码的衔接

- 配置解析失败统一走 `ExitState(3)`（环境/状态错误，docs/command-tree.md §1）。
- 新增 `oma config` 命令组（已同步登记入 command-tree.md §7）：
  - `oma config show [--json]` 打印生效配置及**每个键的来源**（flag/env/项目/用户/默认）；纯读、零写盘。来源标注依赖受控加载 + **per-key 来源映射**（见 §7 实现要求），不从 viper 的 `AllSettings()` 反推。
  - `oma config path [--json]` 打印用户/项目配置文件的解析位置；纯读。

## 7. 实现与测试要求（Phase B 配置层任务）

- 新增 `internal/config` 包：`Load(layout) (*Config, error)` 组装优先级链并返回强类型配置；viper 实例不外泄（其余包只见 `*Config`，不直接依赖 viper —— 隔离重依赖，便于将来替换）。
- **per-key 来源映射**：按受控顺序逐源加载（default→用户→项目→env→flag），每应用一层即记录该层覆盖了哪些键，构建 `map[key]source` 供 `config show` 使用；不依赖 viper `AllSettings()` 事后反推（其只暴露合并值，丢失来源）。
- 命令层在 `newEngine()` 等构造点消费 `*Config`，取代当前散落的 `os.Getenv` 读取。
- 测试（假 HOME + 临时配置文件）：
  - 优先级链逐级覆盖（default → 用户 → 项目 → env → flag 各加一层断言最终值）；
  - 语法错配置 fail-closed；未知 schema major fail-closed；越界值（threshold=1.5、负 duration、非法 agent）fail-closed；
  - `config show` 来源标注正确；
  - 缺失配置文件回落默认无报错。
- 依赖：仅新增 `github.com/spf13/viper`（配置层唯一新重依赖；schema 数据路径不引入 viper）。

# 命令树规范（`oma` 终态全集）

> Phase A 设计文档（A2）。状态：待设计评审。本文档冻结后，任何命令面变更需重开本文档评审。

## 1. 全局约定

- **退出码**：`0` 成功；`1` 完成但有警告（doctor 检查警告等）；`2` 用法错误；`3` 环境/状态错误（权限、损坏的 schema、fail-closed 拒绝）；`4` 门禁不通过（gate/budget/refcheck 判负）；relay wait 专用 `10/11/12`（见 §6）。
- **`--json`**：所有查询类命令支持；输出含 `schema` 字段（如 `"oma-cli/1"`）；字段一旦发布即稳定，新增字段向后兼容，删除/改名需 major bump。
- **`--dry-run`**：**全局持久 flag**，被所有变更类命令继承（asset 全系、state set、relay draft/publish/close、self-update 等）；打印将创建/修改/删除的确切绝对路径与操作类型，零落盘（含零备份、零临时残留）。查询类命令接受但忽略。
- **错误信息规范**：单行首句说明拒绝原因 + 一行建议动作（`hint:` 前缀）；fail-closed 拒绝必须指明触发的校验项。
- 单一 asset 命名空间：**无 `oma skill *` 别名**。`oma update` 是 `oma asset update` 的文档化别名（help 中注明）。

## 2. `oma asset` —— 内容资产管理

```
oma asset install <name>... [--agent claude,codex] [--dry-run] [--force]
oma asset list [--installed] [--json]
oma asset update [<name>...] [--dry-run]        # 别名：oma update
oma asset remove <name>... [--dry-run]
oma asset rollback <name> [--to <backup-id>]
oma asset link --dev [--repo <path>]            # dogfood：软链本地 checkout
```

- `install`：资产文件 → `~/.agents/{skills,agents,hooks}/<name>/` 规范位 → 按 manifest `targets` 投影到各 agent 目录（软链/fragment 注入）。默认 targets 全投影，`--agent` 收窄。
- 覆盖语义：目标已存在且非 oma 管理 → 拒绝；`--force` 先备份再覆盖（见 security-contract.md §2）。
- `rollback`：从 `~/.config/oma/backups/` 恢复；`--to` 省略时取最近备份。
- `link --dev`：规范位条目改为指向仓库 checkout 的软链；registry 标记 `dev: true`。

## 3. `oma state` —— 通用项目级状态

```
oma state get <key> [--file <path>] [--json]
oma state set <key> <value> [--file <path>]
```

- 默认文件 `.oma/state/<namespace>.json`，key 形如 `<namespace>/<field>`（如 `autopilot/phase`）；`--file` 覆盖整个文件路径。
- 写入：tmp+rename 原子、0600；并发安全由 rename 原子性保证（最后写入者胜，状态文件不做锁——工作流约定单写者）。
- value 一律存字符串；结构化数据由调用方序列化（保持 state 语义最小）。

## 4. `oma doctor` —— 诊断与门禁

```
oma doctor [--json]
oma doctor budget --agent claude --profile core4 --max-resident-tokens 2000 [--json]
```

- `doctor` 运行检查注册表全部项：安装一致性（registry vs 实际投影）、权限位、refcheck（静态命令引用）、安全项（world-writable 目标、软链逃逸）、遗留 v1 relay 账本报告、relay v2 残留草稿/无 .ready 残留。退出码 0 全绿 / 1 有警告 / 4 有 fail 级。
- `doctor budget`：按 docs/adapter-conformance.md §5 的注入面模型确定性计数；超限退出码 4。
- relay 维护子项（归 doctor，不进 relay 公开面）：`oma doctor relay [--restore <slug>] [--clean-stale]`。

## 5. 工作流命令（实现语义见 docs/workflows.md）

```
oma interview start [--threshold <0-1>|--depth quick|standard|deep] [--type greenfield|brownfield] [--id <id>] [--idea <text>] [--resume]
oma interview score --input <scores.json> [--id <id>] [--json]
oma interview gate [--waive --reason <text>] [--id <id>] [--json]
oma interview crystallize --spec <path> [--id <id>]
oma interview complete [--id <id>]
oma interview abort [--id <id>]
oma interview status [--id <id>] [--json]

oma ralph start --goal <text> [--max-rounds N] [--stall-window N] [--id <id>]
oma ralph next [--id <id>] [--json]
oma ralph check --verifier-exit <code> [--note <text>] [--id <id>] [--json]
oma ralph abort [--id <id>]
oma ralph status [--id <id>] [--json]
```

- 状态落 `.oma/state/interview-<id>.json` / `.oma/state/ralph-<id>.json`；`--id` 省略时取该类型**唯一**非终态实例，歧义（>1 活跃）则拒绝并列候选。
- `gate`/`next` 的判定输出必须包含：判定结果、依据数值、下一步建议（机器可读 + 人类可读双形态）。
- **无 `oma autopilot *` 命令面**（autopilot 纯 markdown，用通用 `oma state`；重开 spec 才可改变）。
- **B9/B10 修订记录**：状态机要求的迁移入口在原命令面缺失，补齐——interview 增 `crystallize`（gate_passed|gate_waived → crystallized，记录 spec 路径）、`complete`、`abort`、`gate --waive`（早退记录警示，对应 gate_waived 态）；ralph 增 `abort`。拓扑锁定（topology_pending → interviewing）经 `score` 的 round-0 输入承载（schemas.md §5），不增独立命令。

## 6. `oma relay` —— 结对账本（协议见 docs/relay-v2-protocol.md）

```
oma relay init [--ledger-root <path>]
oma relay preflight [--json]
oma relay statusline [--json] [--watch] [--no-color] [--pair <slug>]
oma relay statusline install [--force] | uninstall | doctor [--json]
oma relay hooks install [--target claude|codex|both] | uninstall | doctor [--json]
# (hidden) oma relay hook <event>   — machine-invoked dispatcher; not a public group
oma relay pair new <topic-slug> [--peer <name>] [--json]
oma relay pair ensure [--json]
oma relay pair join <slug> [--json]
oma relay pair show [--pair <slug>] [--json]
oma relay pair list [--json]
oma relay pair set-lead <participant> [--pair <slug>]
oma relay draft --kind <kind> [--in-reply-to <seq>] [--corrects <seq>] [--pair <slug>] [--json]
oma relay publish <draft> --body-file <f> --prompt-file <f> [--touched <path>]... [--status <s>] [--pair <slug>]
oma relay wait [--timeout <sec>] [--pair <slug>] [--json]
oma relay status [--last N] [--pair <slug>] [--json]
oma relay close --outcome <approve|reject|abandon> --reason <text> [--pair <slug>]
```

- **体验层修订记录（B12-B14，用户决定 2026-06-12 补齐 agent-ledger 体验，加 preflight/statusline/hooks 三组，不做 issue/sync）**：`preflight` 退出码 = `0` 全过 / `1` 有警告 / `3` fail-stop（环境/状态，§1 通则；不用 `2`——`2` 仍归 cobra 用法错误）；legacy `.shared/` 在项目根仅警告，仅显式 `--ledger-root` 指 v1 树才 fail。后续 B13 `statusline`、B14 `hooks` + 隐藏派发器 `hook <event>`（机器调用、不计入公开组、不入 refcheck 示例），公开 relay 组终态恰 10。
- **C1 修订记录**：增 `pair set-lead`——workflows §4.1 要求确认对调后「更新 session.json.roles.lead」，原命令面无任何 roles 变更入口（评审 068）。
- **B8 修订记录**：①新增 `pair new`——原命令面只有绑定语义（ensure/join），不存在任何创建 pair 的入口，实现期补此缺口；创建者默认成为 `roles.lead`（协议 §4），`--peer` 默认 claude↔codex 对端。②`draft` 增 `--corrects <seq>`——协议 §5 的 `corrects` 字段原无 CLI 入口，kind=correction 强制要求。

- 公开子命令 7 组（≤10 目标达成）。**决策记录**：`claim` 与 `heartbeat` 为内部协议操作——claim 内化为 `draft` 的序号保留步骤；heartbeat 由任意 relay 子命令执行时自动刷新本方心跳文件；stale 诊断经 `oma relay status --json`（`last_heartbeat`/`stale` 字段）。
- **pair 解析顺序**（协议 §4a）：显式 `--pair` ＞ author-session 绑定文件（`.oma/relay/_bindings/`，`pair join|ensure` 写入）＞ 恰一 active pair 自动绑定 ＞ exit 3 列候选、零写入。
- **草稿生命周期**：临发布前才建 draft（工作期静默 = wait 超时而非 stale；exit 11 = 对端建意图后崩溃）。
- `publish` 支持把 draft 填充与发布合为一步（body/prompt 从文件读入，校验后走 §7 发布事务）；草稿仍含 `TODO:` 占位 → 拒绝。
- `wait` 退出码 `0/10/11/12/3`（语义见协议 §8；用法错误维持全局 `2`）。

## 7. 其他

```
oma config show [--json]       # 打印生效配置 + 每键来源（flag/env/项目/用户/默认）；纯读
oma config path [--json]       # 打印用户/项目配置文件解析位置；纯读
oma self-update [--check]      # --check 严格只读（仅版本比对）；--dry-run 遵循全局契约（披露将下载/替换的路径）；流程与安全要求见 security-contract.md §5
oma version [--json]           # 版本、commit、schema 版本汇总
```

`oma config` 为查询命令组（纯读、零写盘）；配置层完整语义见 docs/config.md（A7）：优先级链、per-key 来源映射、与 schema 数据的严格边界。`config show`/`config path` 均支持 `--json`，解析失败走 `ExitState(3)`。

# Relay v2 协议规范（`oma relay`）

> Phase A 设计文档（A1）。状态：待设计评审。Schema 标记：`oma-relay/2`。
> 继承自 agent-ledger v1 的**原则**（不继承格式）：append-only 账本、sidecar 完整性标记、fail-closed 身份判定、平台信号优先。

## 1. 账本根与遗留共存

- **默认根**：`<git 主 worktree 顶层>/.oma/relay/`；`--ledger-root <path>` 可覆盖（高级用法）。
- **绝不写入** agent-ledger v1 的 `.shared/` 树。v1 识别特征：根下存在 `_relay/` 目录或 v1 形态的 `session.json`（`schema_version` 为整数 1-3）。
- 遗留 `.shared/` 存在时：`oma relay status` 与 `oma doctor` 将其报告为「v1 账本：归档/人工参考，oma 不读不写」，并继续使用 v2 根。共存不要求删除或迁移。
- `--ledger-root` 显式指向 v1 树 → **拒绝**并说明原因。
- **v2 sentinel**：`.oma/relay/.oma-relay-v2`，内容 `{"schema":"oma-relay/2","created":"<ISO-8601>"}`。缺失 sentinel 且目录非空 → 拒绝；schema major ≠ 2 → 拒绝（提示升级 oma 或检查目录）。

## 2. 拓扑与时钟假设

- v1 release 仅支持**同 checkout/同主机**：双方进程共享同一文件系统与系统时钟。
- 依赖的文件系统性质（由 `oma doctor` 探针验证）：tmp+rename 原子性、O_EXCL 排他创建；mtime 只需在 `OMA_RELAY_STALE_AFTER` 量级（分钟）上可区分先后——亚秒级粗粒度 mtime 是 doctor **警告**而非失败（rev A.2，评审 012 minor 4）。
- 不为跨机传输预留协议字段（YAGNI）；未来拓扑变化经 `schema_version` 演进承担。

## 3. 目录与文件布局

```
.oma/relay/
├── .oma-relay-v2                  # sentinel
├── _bindings/<author-session>.json # pair 绑定（schema oma-relay-binding/1，见 §4a）
├── <pair-slug>/                   # YYYYMMDD-<topic-slug>
│   ├── session.json               # pair 元数据（schema 见 docs/schemas.md §4）
│   ├── NNN-<author>-<kind>.md     # 已发布 artifact（append-only）
│   ├── NNN-<author>-<kind>.md.sha256
│   ├── NNN-<author>-<kind>.md.ready
│   ├── .draft/NNN-<author>-<kind>.md   # 作者私有草稿（对端约定不读）
│   ├── .seq/NNN                   # 序号保留文件（O_EXCL；作者记录在文件内容首 token）
│   └── .heartbeat/<author>        # 心跳文件（mtime 即活性）
└── _archive/<pair-slug>/          # close 后整体移入
```

权限：目录 0700，文件 0600（`oma doctor` 检查）。

## 4. 身份与角色

- author 解析优先级：**平台信号**（`CLAUDE_CODE_SESSION_ID` → `claude`；`CODEX_THREAD_ID` → `codex`）＞ `OMA_RELAY_AUTHOR` 环境变量 ＞ 解析失败即拒绝。
- 双平台信号并存且无 `OMA_RELAY_AUTHOR` 仲裁 → 拒绝（fail-closed，零写入）。
- 参与者恰为 2，不允许同名双方（claude+claude 拒绝）。
- **角色**：`session.json.roles` 将 `lead / planner / implementer / reviewer` 映射到参与者名（一人可任多角色）。`lead` = 主决策者，**必填且唯一**，默认 = bootstrap 发起方；其余角色可配置到任一参与者。relay 机制不强制角色行为；角色字段供结对交付流等 skill 读取与提示（lead 语义与对调规则见 workflows.md §4）。

## 4a. Pair 绑定与解析（rev A.1，采纳设计评审 010 finding 2）

- `oma relay pair join <slug>`（及 `pair ensure` 的自动绑定路径）写绑定文件 `.oma/relay/_bindings/<author-session>.json`（schema `oma-relay-binding/1`）：`author`、平台会话 id 哈希、`pair`、`created`、`updated`。
- **所有 pair 作用域命令**（draft/publish/wait/status/close/pair show）按此顺序解析目标 pair：显式 `--pair <slug>` ＞ 当前 author-session 的绑定文件 ＞ 恰有一个 active pair 时自动采用并写绑定 ＞ 否则 **exit 3、零写入**，并列出候选 pair。
- 未知绑定 schema、绑定指向不存在/已终态的 pair、多 active pair 无法消歧 → exit 3、零写入。
- `pair show` 显示解析结果与对端 join 命令；`pair list --json` 列出 active/terminal 全部 pair。

## 5. Artifact 模型

- 文件名 `NNN-<author>-<kind>.md`，NNN 三位十进制（001 起，允许空洞，读取按文件名排序）。
- `kind ∈ plan | review | fix | note | question | decision | correction | addendum`。
- frontmatter（YAML）：`schema("oma-relay/2"), seq(int), author, peer, kind, status, created(ISO-8601), in_reply_to(int|null), prompt_for_next(string), touched_paths([string]), corrects(int|null)`。
- `status ∈ ready | closed | cancelled | failed | timed_out`；终态 = closed/cancelled/failed；`timed_out` 为可恢复暂停（用于 `@user:` 上抛）。
- **append-only**：带 `.ready` 的文件永不修改；更正经 `kind: correction` + `corrects` 指向原 seq。

## 6. 序号分配与竞争（claim 内部化）

- 序号保留：在 `.seq/` 下以 **O_EXCL** 创建 `NNN`（作者与时间戳写入文件内容），NNN = max(已发布最大 seq, .seq 中最大保留) + 1。
- 竞争：O_EXCL 失败 → 取 NNN+1 重试（上限 10 次后报错）。两个并发 draft 永远得到不同 seq，无覆盖可能。
- **B8 修订记录**：排他对象从 `NNN.<author>` 改为 `NNN`——带作者后缀的文件名使 O_EXCL 只在 (seq, author) 维度排他，双方并发可各自拿到同一 NNN（协议测试 7 实测复现）；作者归属移入文件内容，跨作者排他由裸 NNN 文件名保证。
- `oma relay draft` 创建草稿 = 保留序号 + 建心跳文件；公开命令面无独立 `claim`（决策记录：claim 是 draft 的内部步骤，见 command-tree.md §relay）。草稿与 `.seq` 保留在发布事务完成（`.ready` 落盘）之前**一直存在**（§7）。
- 保留过期：序号保留对应的草稿心跳 stale（§8）后，`oma doctor` 可清理保留与草稿；产生的序号空洞合法。

## 7. 发布事务与中断恢复

**草稿 = 持久发布意图**（rev A.1，解决评审 010 blocker）：在 `.ready` 写成之前，草稿与 `.seq` 保留始终存在；publish 从不消耗草稿本体作为中间产物。

publish 步骤（严格顺序）：从草稿**渲染**正式内容 → 写 `NNN-<author>-<kind>.md.tmp` → fsync → rename 为正式名 → 写 `.sha256.tmp` → rename `.sha256` → 写 `.ready.tmp` → rename `.ready` → **最后**删除草稿与 `.seq` 保留。

- 无 `.ready` = 未发布，读取方一律忽略——这是唯一的发布判据。
- 中断重跑：`oma relay publish <draft>` 从仍然存在的草稿重新渲染并补齐缺失步骤/sidecar（含 rename 之后的 kill——草稿仍在）；若既有正式文件与草稿重渲染内容不一致 → fail-closed，提示 `oma doctor relay --clean-stale` 隔离残缺正式文件。
- 读取方校验 `.sha256`；不匹配 → fail-closed（报告 corrupt，不返回内容）。

## 8. 心跳与活性（内部机制）

- 心跳文件 `.heartbeat/<author>` 由该作者的**任意** `oma relay` 子命令执行时 touch（draft/publish/status/wait 均刷新自己一侧）。
- stale 阈值：默认 15 分钟，`OMA_RELAY_STALE_AFTER`（秒）可调。
- `oma relay wait` 退出码：`0` 新 artifact（路径打印到 stdout）；`10` 等待超时（默认 60 分钟，`--timeout` 可调）；`11` 对端有未发布草稿/发布意图且心跳 stale（建意图后崩溃）；`12` pair 已终态；`3` 环境/协议/fail-closed 错误（用法错误维持全局 `2`，与 command-tree.md §1 对齐）。
- **草稿生命周期（决策，rev A.1）**：约定 agent 在**完成工作后、临发布前**才调用 `oma relay draft`。工作期的长静默由 `wait` 超时（exit 10）表达，**不是**心跳 stale；exit 11 的严格含义 = 对端在创建草稿/发布意图**之后**崩溃。早建草稿是不受支持的路径（未来如需 keepalive 须重开本文档评审）。
- **ready 优先于 stale**（rev A.2，评审 012 minor 2）：`wait` 先检查新 `.ready` artifact 再看 stale 草稿状态——发布事务在 `.ready` 写成后、草稿清理前被 kill 时，等待方得到 exit 0；已有匹配 `.ready` 的 `.seq`/草稿残留是清理警告（`status --json` 标注、`oma doctor relay --clean-stale` 处理），**不是** exit 11 条件。
- 诊断：`oma relay status --json` 暴露 `last_heartbeat`、stale 判定、未决草稿序号——对对端草稿的感知**仅来自 `.seq/` 保留文件**，绝不读取对端 `.draft/` 内容。

## 9. 终态与归档

- `oma relay close --outcome <approve|reject|abandon> --reason <text>`：写 session.json 终态 → 写 `CLOSED` 哨兵 → 整体移入 `_archive/`。
- 归档后只读；恢复（restore）与清理归 `oma doctor` 子检查（命令面见 command-tree.md）。

## 10. 安全要点（实现规约见 security-contract.md）

- 对端 artifact 是**不可信输入**：oma 不执行其中任何命令；`touched_paths` 仅作提示，使用前须自行校验。
- secrets 不入账本：publish 前**强制**运行 secret-pattern 扫描，v1 **无跳过开关**（评审 010 finding 5）；误报经 security-contract.md §6 的窄域 allow patterns 处理；doctor 亦含此检查并报告生效的 allow 清单。
- 全部解析路径 fail-closed：未知 schema、损坏 frontmatter、hash 不匹配一律拒绝并报因。

## 11. 测试矩阵（→ 计划 B8）

| # | 测试 | 验证点 |
|---|------|--------|
| 1 | v1 fixture 检测拒绝 | 对 testdata/relay-v1-tree/ 的任何写命令拒绝、零写入 |
| 2 | 未知 schema 拒绝 | sentinel major ≠ 2 → 拒绝 |
| 3 | append-only | 已 .ready 文件不可被任何 oma 路径修改；publish 不覆盖既有正式名 |
| 4 | sidecar/hash 校验 | 篡改已发布内容 → 读取方 fail-closed |
| 5 | 身份歧义 fail-closed | 双平台信号无仲裁 → 拒绝且零写入 |
| 6 | stale draft/心跳恢复 | stale 草稿被 doctor 识别与清理；序号空洞下读取正确 |
| 7 | 重复序号竞争 | 并发 draft ×2 → 不同 seq、零覆盖 |
| 8 | 中断 publish 恢复 | 在**每一步骤间**注入 kill（含正式 rename 之后）→ 重跑同一 publish 命令从存活草稿收敛；正式文件与草稿不一致时 fail-closed；读取方全程无脏读。子用例：`.ready` 写成后、草稿清理前 kill → 对端 wait 得 exit 0（非 11），残留由 doctor 清理 |
| 9 | 遗留共存 | `.shared/` v1 树并存时 v2 全功能正常且不触碰 v1 |
| 10 | --ledger-root 指 v1 | 拒绝 |
| 11 | v2 根未知 schema | 拒绝 |
| 12 | pair 绑定解析 | 多 active pair 无消歧 → exit 3 零写入并列候选；`--pair` 覆盖生效；单 active pair 自动绑定落盘 |

## 12. 体验层（B12-B14，2026-06-12 用户决定补齐 agent-ledger 体验）

用户对 agent-ledger relay 体验极满意；切换到 oma relay v2 前须补齐体验层、不得回归。补**三项**：preflight / statusline / hooks 自动续turn；**不做** issue 账本与 sync 跨机（sync 本就 §2 YAGNI 延后）。三者全为增量，复用 internal/hookcfg（注入）、internal/checks 风格（检查）、status 读取器（状态）。

### 12.1 `oma relay preflight [--json]`（B12）

人工排障门：诊断身份、账本根/sentinel/schema、绑定/对端推导、文件系统性质。**永不硬失败**——每个条件落为一行检查结果，环境再坏也整表呈现。

- 检查项：`identity.author`（+session）、`ledger.root`、`ledger.v1_root`（仅显式 `--ledger-root` 时；指 v1 树 → fail）、`legacy.shared`（项目根 `.shared/_relay` 存在 → **warn**，非 fail）、`ledger.sentinel`（v2 在/缺/异 major）、`pair.binding` + `identity.peer`；FS 探针 `fs.{tmp_rename,mtime,symlink,sha256,fsync,posix_mode}`，mtime 粗粒度（1s 内不可分）→ **warn** 不 fail。
- 退出码：`0` 全过 / `1` 有警告 / `3` fail-stop（环境/状态，command-tree §1 通则；**不用 `2`**——`2` 归 cobra 用法错误）。与 agent-ledger 的 `0/1/2` 行为等价（表 + 停/继续语义），退出码取 oma 原生值。
- `--json`：schema `oma-relay-preflight/1`，字段稳定供 B13/B14 复用。
- SessionStart（B14）**不跑全量 preflight**——用轻量有界 stale/residue/status 检查（FS 探针在共享挂载上偏贵）；全量 preflight 仅人工触发。

### 12.2 `oma relay statusline`（B13，已实现）

紧凑「哪个 pair / 轮到谁 / 最新 seq·kind·status」行 + `--watch` 看板 + `install/uninstall/doctor`。安全属性（硬验收）：**纯读**（不 GC、不改 last_seen/心跳、零账本写）、**绑定作用域**（未绑定窗口不显示孤 active pair）、**仅 claude 装**（Codex 无命令式 statusline）、单 `statusLine` 槽不覆盖（除非 `--force`）、渲染/子进程**自限时**（挂载卡死不拖死 UI）。

### 12.3 `oma relay hooks`（B14，已实现）

`hooks install|uninstall|status|doctor --target claude|codex|both` + **隐藏**派发器 `hook <event>`（机器调用，不计入公开组、不入 refcheck）。派发器读宿主 hook 载荷、解析绑定 pair 状态、按平台输出正确 JSON、**绝不弄坏宿主**（内部任何错 → exit 0 静默，除 PreToolUse 的有意 deny）：

- **SessionStart**：早期提示 + 轻量 stale/residue 摘要（`systemMessage`）。
- **PreToolUse**：拒改 `.ready` 已发布 artifact（`permissionDecision:deny` + 指向 correction 流程）；不发 Codex 不支持的字段。
- **Stop**：对端发布且 addressed-to-me 时**自动续turn**；护栏（硬验收）：防循环（`stop_hook_active` → exit 0 静默）、严格绑定（等价 `--require-binding`，未绑定静默）、有界状态（短状态面非 waiter，超时/失败 → exit 0 静默 + 诊断 trail）、去重（稳定指纹，不变则静默）、输出 `decision:"block"` 续turn 且**绝不**配 `continue:false`。
- 安装复用 hookcfg 注入双端、relay 专属 marker、幂等、Codex `/hooks` 一次性信任提示。

**切换门**：停用 agent-ledger relay 须等 B14 落地——届时体验持平或更好。公开 relay 组终态恰 10（init/pair/draft/publish/wait/status/close/preflight/statusline/hooks；派发器隐藏）。

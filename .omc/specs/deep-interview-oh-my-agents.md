# Deep Interview Spec: oh-my-agents — 轻量双 Agent CLI+Skill 工具集

> 状态：**pending approval**（待用户批准后进入「Claude 规划 → Codex review → Claude 实现 → Codex review」流程）

## Metadata
- Interview ID: 6c0b56e8-6b9e-4a1d-8126-50b6a9029f6e
- Rounds: 10（+ Round 0 拓扑门）
- Final Ambiguity Score: 10%
- Type: greenfield（参照系：agent-ledger、OMC 4.14.6，均已实地摸底）
- Generated: 2026-06-11T14:54:45+08:00
- Revised: 2026-06-11（rev 2——采纳 Codex 评审 `002-codex-review` 全部 8 条发现；rev 2.1——按复核 `004` 收敛 asset 单一命名空间；**rev 2.2——按用户指令撤销「M1 委托旧 relay 过渡」，产品零旧 relay 依赖**；**rev 3——用户终态设计指令：一切按终态设计实现、不留计划性过渡形态，里程碑改为依赖序构建阶段，不急上线、打磨优先、规划定上限**；rev 3.1——采纳计划评审 `006`：v2 账本根迁至 `.oma/relay/` 与遗留 .shared/ 安全共存、Phase A 增补 workflows/adapter-conformance 设计文档、self-update 安全契约、活跃术语 Phase 化）
- Threshold: 0.10
- Threshold Source: --deep
- Initial Context Summarized: no
- Status: **PASSED**

## Clarity Breakdown
| Dimension | Score | Weight | Weighted |
|-----------|-------|--------|----------|
| Goal Clarity | 0.90 | 0.40 | 0.360 |
| Constraint Clarity | 0.90 | 0.30 | 0.270 |
| Success Criteria | 0.90 | 0.30 | 0.270 |
| **Total Clarity** | | | **0.900** |
| **Ambiguity** | | | **0.10 (10%)** |

（整体分 = 5 个组件中各维度最弱值；逐组件分见状态文件 `.omc/state/deep-interview-state.json`）

## Topology
| Component | Status | Description | Coverage / Deferral Note |
|-----------|--------|-------------|--------------------------|
| CLI 核心 (cli-core) | active | Go 单二进制（暂名 `oma`，可在评审改名）；分级固化：通用机械层 + 按工作流深化层 | R1 固化边界、R7 语言、R10 命令范围；验收见 Phase A-D（rev 3 取代 M1/M2） |
| 内容资产集 (skill-set) | active | 核心 4 工作流 skill + subagents/hooks 资产；agent 中立写法 | R4 清单、R9 重写约束、R10 预算与候选池归期 |
| 多 Agent 适配层 (agent-adapters) | active | ~/.agents 规范位 + per-agent 投影（CC + Codex） | R5 安装规范、R9 骨架/肌肉策略、R10 Codex 验收方式 |
| 结对编程 Relay (pair-relay) | active | relay 在 oma 中原生重实现，协议 v2 全新设计 | R3 合入语义、R8 协议取舍、R6 历史验收输入（rev 3 后由 Phase A-D 承接） |
| 分发与安装方案 (packaging) | active | v1 纯 CLI+Skill；GitHub release；plugin 薄壳 post-v1 | R2 载体、R10 更新机制与 dogfood |

无组件级 deferral；范围内分期项（plugin 薄壳、候选池、Codex 真机验收）记入 Non-Goals / 里程碑。

## Goal
用 **Go** 实现单二进制 CLI（暂名 **`oma`**），配合一套**精选、agent 中立**的 markdown skill，替代 oh-my-claudecode（OMC）成为日常 AI 编码工作流的基座：

1. **CLI 固化机械层**：资产安装/更新/诊断、状态读写，以及按工作流逐个评估的深化命令（评分门禁、循环判停等「半机械」逻辑）；
2. **Skill 描述非固化层**：deep-interview、autopilot、ralph、结对交付流四个核心工作流的精炼说明书，调用 oma 命令为骨架；
3. **资产模型** `{skill, subagent, hook, command/prompt}` 经 `~/.agents/{skills,agents,hooks}/` 规范位投影（软链/注入）到 **Claude Code 与 Codex** 两端；
4. **合入结对编程**：agent-ledger 的 cross-review 结对功能以**全新协议 v2** 在 oma 中原生重实现，命令面大幅简化，角色（planner/implementer/reviewer）可配置；
5. **解决原始痛点**：彻底摆脱 OMC「40 个 skill 关不掉、15-20k tokens 常驻」的问题——装几个 skill 由用户决定，常驻注入 ≤ 2k tokens。

## Constraints
- **语言/形态**：Go；单一静态二进制；不依赖 Python/Node 运行时；CLI 生态用 cobra + goreleaser（建议项）
- **载体**：v1 纯 CLI+Skill，不做 Claude Code plugin（plugin 薄壳为 post-v1 可选附加渠道）
- **安装规范**：资产文件落 `~/.agents/{skills,agents,hooks}/` 规范位；`~/.claude/skills` 等 agent 目录放软链；hooks 以 fragment 注入双端配置（沿用 npx-skills / agent-ledger 约定，兼容现有链路）
- **skill 写作规范**：agent 中立——工作流骨架（状态/轮次/门禁）必须走 `oma` 命令；CC 原生能力（subagent、Plan mode 等）只能作为显式标注的 per-agent 加速分支；CC-only 机制必须给降级路径
- **token 预算**：每 skill frontmatter description ≤ 2 行；SKILL.md 正文 ≤ 500 行；详细协议放 references/ 按需加载；全集安装后 CC 常驻注入 ≤ 2k tokens
- **relay v2 协议**：磁盘格式与命令面全新设计，**不兼容**旧 .shared/ 账本、无互操作期（用户明确决策）；但必须继承旧协议已验证的设计原则：append-only 账本、sidecar 完整性标记、fail-closed 身份判定、平台信号优先的身份识别
- **工作流固化分级**（R1）：通用机械件打底；deep-interview 评分/门禁、ralph 循环判停固化进 CLI（Phase B）；autopilot 编排保持纯 md
- **交付过程**（用户修正版）：Claude 规划 → Codex review 计划 → 通过后 Claude 实现 → 完成后 Codex review 代码；实现走 feature 分支/worktree 纪律

## Non-Goals
- ❌ OMC 40-skill 全量搬运；不收 setup/omc-doctor/hud 等一次性 skill（此类功能归 `oma` 命令，如 `oma doctor`）
- ❌ 候选池工作流进 v1：ultragoal/ultrawork/ralplan（计划-执行簇）、autoresearch/best-practice-research（研究簇，来源待查）、ai-slop-cleaner、performance-goal —— post-v1 按簇合并评估
- ❌ 旧 .shared/ 账本兼容、读取或迁移工具
- ❌ Claude Code plugin 载体（post-v1 薄壳另议）
- ❌ CC/Codex 之外的 agent 适配（资产模型设计留口，不实现）
- ❌ MCP server、GUI/HUD 类组件
- ❌ v1 发版不被 Codex 真机冒烟验收阻塞（文件投影断言 + 离线 conformance fixtures 为准，真机验收后补）
- ❌ rsync/多机 relay 拓扑：v1 仅支持同 checkout/同主机账本，传输层 post-v1（见「Relay v2 拓扑」）
- ❌ 上线/开放时间承诺：发版能力就绪即可，对外 marketplace/推广不设期限（用户指令：不急上线，打磨优先）

## 边界与契约（Rev 2：采纳 Codex 评审新增）

### 终态设计原则与构建顺序（rev 3，取代原 M1/M2 里程碑框架；同时结构性解决原 blocker）
**终态设计原则（用户指令）**：协议、命令树、schema、skill 一律按终态设计并一次实现到终态，不引入计划性过渡形态——无「先薄版后固化」的重写计划、无过渡依赖。本工具不急于上线开放，打磨优先；但规划必须按终态，这决定工具上限。

**构建按依赖顺序分四阶段**（取代 M1/M2；文中残留的 M1/M2 字样按此映射：M1 资产管理内容 → Phase B 基础层；M2 工作流/relay 内容 → Phase B 工作流层与 Phase C）：
- **Phase A 设计定稿**：relay v2 协议规范、命令树全集、资产/状态 schema、安全契约——文档先行并过独立评审（上限在此确定）
- **Phase B CLI 终态**：基础层（asset/state/doctor/budget/self-update）→ relay v2 核心环 → interview/ralph 固化命令，全部按 Phase A 设计一次到位
- **Phase C 资产终态**：核心 4 skill（deep-interview、autopilot、ralph、结对交付流）一次写到终态，直接引用最终命令；产品任何阶段零旧 relay 依赖（用户决策，rev 2.2）
- **Phase D Dogfood 打磨**：link --dev 自用、替换 OMC、周志、持续优化；对外开放不设期限

单一规范命名空间：不设 `oma skill *` 别名（rev 2.1）。**静态命令引用检查**保留为 CI 防回归门禁：已交付 skill 不得引用未实现命令——终态构建顺序（先命令后 skill）使其自然满足，原 blocker（skill 引用未实现命令）由此结构性消除。`oma doctor budget` 的 core4 profile 自 Phase C 起按全部 4 skill 计量，阈值 2k。

### Token 预算的可执行判据
`oma doctor budget --agent claude --profile core4 --max-resident-tokens 2000` 必须**确定性**统计投影后的常驻注入面：包含每个已装 skill 被 Claude Code 加载的 frontmatter 字段，以及注入会话的 command/prompt/hook 元数据。tokenizer（或近似算法）在测试中 pin 定；core4 投影超 2k tokens 时 CI fail。

每 skill `description` 目标 ≤ 80 tokens；「SKILL.md ≤ 500 行」保留为可读性上限，**不作为**常驻预算的证明手段。deep-interview 的 frontmatter 必须是极简路由器而非压缩版手册，详细流程全部走 references/ 按需加载。

### Relay v2 切换契约（fail-closed）
**v2 账本根**：`oma relay` 使用 v2 专属账本根，默认 **`.oma/relay/`**（高级用法 `--ledger-root` 覆盖）；**绝不写入** agent-ledger v1 的 `.shared/` 树。检测到遗留 `.shared/` 时，`oma relay status`/`oma doctor` 将其报告为「归档/人工参考」并继续使用 v2 根——共存无需删除或迁移旧账本（rev 3.1，解决评审 006 blocker）。

`oma relay` 使用 **v2 专属 ledger sentinel 与 schema 标记**。对 v1 树（含显式 `--ledger-root` 指向 v1 树）或任何未知 schema **拒绝操作**，并明确提示：旧账本仅作归档/人工参考，不迁移。共存测试：遗留 `.shared/` 与空 `.oma/relay/` 并存正常工作；`--ledger-root` 指向 v1 树拒绝；v2 根内未知 schema 拒绝。

Phase B 协议测试必须覆盖：v1 fixture 检测并拒绝；未知 schema 拒绝；append-only publish；sidecar/hash 校验；身份歧义 fail-closed；stale draft/heartbeat 恢复；重复序号竞争；中断 publish 恢复。

### Relay v2 拓扑（Phase A 定稿 / Phase B 实现）
v1 **仅支持一种拓扑：同 checkout/同主机**（共享文件系统账本）。〔带标记默认值：依评审建议采纳；若需将多机 rsync 纳入 v1 release 范围，需重开 spec 决策〕其他拓扑 post-v1。

v2 协议规范（Phase A）必须为该拓扑定义：序号分配、claim 所有权、心跳 stale 阈值、stale draft 清理、时钟偏差假设、中断 publish 恢复、冲突行为。Phase D「一次真实结对交付」的验收即在该拓扑下执行。

### 安全与权限要求
- 所有变更类资产/配置命令支持 `--dry-run`，写入前报告确切路径
- 既有用户文件**绝不**在无备份或无显式 `--force` 时被覆盖；备份可经 `oma asset rollback`（或 `oma doctor repair`）恢复
- 软链解析后必须约束在可信资产根目录内；拒绝路径穿越与 world-writable 目标目录
- agent 配置与 relay 状态在平台支持处使用限制性权限（0700/0600）
- hook fragment 原子化安装、可干净移除
- 对端 relay artifact 是**不可信输入**：其中的命令绝不隐式执行、secrets 绝不复制进账本、schema/hash 校验失败即 fail-closed
- `oma self-update`（rev 3.1）：发布物 **checksum 校验**通过才替换（release 流支持则优先签名校验）；更新源限定配置的 GitHub repo/release 渠道，重定向与异常资产名 fail-closed；原子替换 + 失败自动回退旧二进制；`--check` 绝不写盘

### 适配层能力矩阵与工作流一致性
每个资产声明支持目标：`claude` | `codex` | `shared`。CC-only subagent 作为 shared 资产安装但只投影到支持端；Codex 端获得文档化的 fallback prompt/skill 路径，或不投影并由 `oma doctor` 给出警告。

每个核心工作流必须有**不依赖 CC 原生能力的默认路径**（由 `oma` 状态驱动）。CI 包含双端离线 conformance fixtures：证明投影文件的默认路径不含目标端不支持的引用。CC 加速分支只能加速执行，**不得产生 Codex 无法检视或接续的工作流状态**。

### 更新命令语义
`oma self-update` 只更新二进制；`oma asset update` 只更新已装内容资产；`oma update` 是 `oma asset update` 的**文档化别名**（避免双更新路径语义漂移）。

## Acceptance Criteria

### Phase A 设计定稿
- [ ] `docs/relay-v2-protocol.md`：v2 账本根（`.oma/relay/`）、sentinel/schema 标记、与遗留 `.shared/` 共存行为；同 checkout/同主机拓扑下的序号分配、claim 所有权、心跳 stale 阈值、stale draft 清理、时钟偏差假设、中断 publish 恢复、冲突行为全部定义
- [ ] 命令树全集文档（含 asset/state/doctor 与 relay/interview/ralph 终态命令面；显式决定 relay heartbeat/claim 为公开子命令还是内部协议操作——若内部，定义归属的公开命令与经 `oma relay status --json` 诊断 stale claim 的方式）+ 资产/状态 schema 文档
- [ ] `docs/workflows.md`：`oma interview *` 与 `oma ralph *` 的终态状态机（持久化字段、评分/门禁公式、停止条件、恢复/错误态）；autopilot 保持纯 markdown——可用通用 `oma state` 存续状态，但**不引入 `oma autopilot *` 命令面**（重开 spec 才可改）
- [ ] `docs/adapter-conformance.md`：manifest targets 语义、per-agent 投影规则、Codex fallback 行为、预算计量的常驻注入面定义、conformance fixture 格式、refcheck 提取规则
- [ ] 安全契约文档化（见「安全与权限要求」）
- [ ] 以上设计文档通过独立评审（Codex）

### Phase B CLI 终态
- [ ] 基础层：`oma asset install/list/update/remove/link/rollback` + `oma state get/set` + `oma doctor`（含 budget）+ `oma self-update` 可用且有 Go 测试（无 `oma skill *` 别名；`oma update` 为 `oma asset update` 文档化别名）
- [ ] 安全与权限要求落地：--dry-run、备份/--force 覆盖策略、软链可信根约束、限制性权限、hook 原子安装可移除
- [ ] relay v2 核心环按协议规范实现；协议测试集全绿：v1 fixture 检测拒绝、未知 schema 拒绝、append-only publish、sidecar/hash 校验、身份歧义 fail-closed、stale draft/heartbeat 恢复、重复序号竞争、中断 publish 恢复
- [ ] `oma interview *` 与 `oma ralph *` 按 Phase A `docs/workflows.md` 实现（不得从旧 OMC 或现有 .omc/state 文件推断行为）
- [ ] 门禁引擎范围（rev 3.1）：refcheck/budget/conformance 引擎在 Phase B 以 `testdata/` 合成 fixtures 验证，不声称 core4 真实资产预算
- [ ] self-update 安全测试：checksum 不匹配、release 元数据不可用、目标不可写、替换中断、回退全覆盖
- [ ] GitHub release/goreleaser 链路就绪（tag 版本门禁；发版能力就绪 ≠ 对外开放）

### Phase C 资产终态
- [ ] 核心 4 skill 一次写到终态：直接引用最终命令、frontmatter ≤80 tokens、references/ 拆分、agent 中立默认路径 + CC 加速显式分支
- [ ] 三门对**真实资产**复跑并 release-blocking（rev 3.1）：静态命令引用检查、`oma doctor budget --agent claude --profile core4 --max-resident-tokens 2000`（pinned tokenizer）、双端离线 conformance
- [ ] 安装后双端投影断言通过；结对角色（planner/implementer/reviewer）可配置
- [ ] 结对交付流 skill 接线 `oma relay` v2——产品全程零旧 relay 依赖

### Phase D Dogfood 打磨（验收随打磨期持续）
- [ ] `oma asset link --dev` 自用；disable/blocklist OMC 并记录确切回退命令
- [ ] 带日期一周周志无 OMC re-enable 事件，所需工作流均使用 oma 资产
- [ ] 用固化版 deep-interview 跑通一次真实需求澄清；一次真实结对交付（plan → cross-review → implement → cross-review）在指定拓扑下全程跑通
- [ ] /context 实测校准 budget 近似算法并记录偏差

## Assumptions Exposed & Resolved
| Assumption | Challenge | Resolution |
|------------|-----------|------------|
| 「高频清单」= OMC 的目录 | R4 Contrarian：CC 原生已覆盖部分；实际提取过的只有 2 个 | 核心 4 确认进 v1；其余进候选池 post-v1 按簇评估 |
| CLI+Skill 比 plugin 轻（待确认） | R2 证据：plugin 是 CC 专属概念、OMC 关不掉根因即 plugin 全量加载 | 确认：CLI+Skill 是唯一满足「双 agent + per-skill 选装」的方案 |
| 「合入 relay」= 移植现有代码 | R3：现有工具膨胀（35 命令） | 重实现 + 简化整合命令面 |
| 新实现应兼容旧文件协议 | R8 Ontologist：磁盘协议与命令面可分开处置（我推荐兼容） | **用户决策：全新 v2，不兼容**；继承设计原则作为缓解 |
| 工作流重写可依赖 CC 原生编排 | R9：Codex 端无原生编排 | oma 状态机为中立骨架，CC 原生只作加速分支 |
| 交付流 = Claude 计划、Codex 实现 | 用户中途修正（质量考虑） | Claude 规划+实现，Codex 把双道评审门 |
| v1 一次交付全部价值 | R6 Simplifier：换掉 OMC 与证明固化是两个不同的最小切片 | 双里程碑 M1/M2 顺序交付（后被 rev 3 终态阶段框架取代） |
| 可引入过渡依赖先跑通 | Codex blocker 修复曾引入「M1 委托旧 relay」 | **用户否决（rev 2.2）**：产品零旧 relay 依赖，结对交付流随 relay v2 交付 |
| 尽快换掉 OMC 优先（切片求快） | 用户终态指令：不急上线、规划按终态定上限 | **rev 3**：里程碑改为依赖序四阶段（设计→CLI→资产→dogfood），交付件一次到终态 |

## Technical Context
**参照系一：agent-ledger（用户自有，将被部分继承）**
- 单文件 Python relay（6539 行 stdlib）、35 命令、35 个 pytest 文件；file-protocol.md 冻结 1.0
- 可继承资产：`npx skills add → ~/.agents/skills/ + 软链` 安装链路（本机已在用）、hooks fragment 双端注入先例、SKILL.md+references/ 按需加载结构、tag 驱动 release 版本门禁、设计原则（append-only/sidecar/fail-closed/平台信号身份）
- 处置：旧仓库进入维护模式；协议资产不直接复用（v2 决策），原则性继承

**参照系二：OMC 4.14.6（痛点来源）**
- 40 skills ≈ 11,232 行 SKILL.md，估 15-20k tokens 常驻；`loadSkillsFromDirectory()` 无条件加载、无 per-skill 禁用（痛点根因，已源码确认）；另有 19 hooks/19 subagents/1 MCP/28 commands
- 借鉴对象：ralph/autopilot/deep-interview 等工作流的**概念**，不搬实现（深度依赖 OMC 自家基建）

**环境事实**
- 本机无 codex 二进制（Codex 验收策略由此而来）；Codex 无 plugin/subagent 机制，有 hooks 与 custom prompts；npx skills 支持 codex 投递目标
- CC 原生：Plan mode、Explore、Workflow/ultracode、/loop、teams——与 OMC 部分工作流重叠（重写时避免重复造轮子的依据）

## Ontology (Key Entities)
| Entity | Type | Fields | Relationships |
|--------|------|--------|---------------|
| CLI 二进制 (oma) | core domain | 通用机械层；深化层（按工作流）；Go | 提供命令给 Skill 引用；安装/管理内容资产 |
| Skill 内容单元 | core domain | SKILL.md ≤500行；references/ 按需；agent 中立 | 调用 oma 命令；被 Agent 加载 |
| 工作流 | core domain | 核心4 + 候选池；固化分级 | = oma 命令骨架 + Skill 说明 + 可选原生加速 |
| Agent | external system | Claude Code；Codex | 加载资产；调用 oma |
| Relay 结对 | supporting | v2 重实现；命令面收敛；角色可配置 | 合入 oma 为子命令；支撑结对交付流 |
| 通用机械层 | supporting | asset/state/doctor | 属于 oma |
| 分发机制 | supporting | GitHub release + oma 安装/更新 | 分发 oma 与资产 |
| Subagent 定义 | supporting | CC-only | 属于内容资产 |
| Hook 片段 | supporting | 双端 fragment 注入 | 属于内容资产 |
| Plugin 薄壳 | supporting | post-v1，仅 CC 发现性 | 转发到 oma |
| Relay 协议 v2 | supporting | 全新磁盘格式；继承 1.0 设计原则 | 约束 Relay 重实现 |
| 规范资产目录 | supporting | ~/.agents/*；per-agent 投影 | 承载资产；oma 维护 |
| 构建阶段 | supporting | Phase A-D（原 M1/M2，rev 3 取代） | 依赖序交付与验收线 |

## Ontology Convergence
| Round | Entity Count | New | Changed | Stable | Stability Ratio |
|-------|-------------|-----|---------|--------|----------------|
| 1 | 7 | 7 | - | - | N/A |
| 2 | 10 | 3 | 0 | 7 | 70% |
| 3 | 11 | 1 | 0 | 10 | 91% |
| 4 | 11 | 0 | 0 | 11 | 100% |
| 5 | 12 | 1 | 0 | 11 | 92% |
| 6 | 13 | 1 | 0 | 12 | 92% |
| 7 | 13 | 0 | 0 | 13 | 100% |
| 8 | 13 | 0 | 1（文件协议1.0→Relay协议v2，概念延续） | 12 | 100% |
| 9 | 13 | 0 | 0 | 13 | 100% |
| 10 | 13 | 0 | 0 | 13 | 100% |

连续 4 轮 100% 稳定——域模型完全收敛。

## 风险与建议（顾问区，非约束）
1. **relay v2 协议风险**（最大单项风险）：旧协议 35 个测试文件的验证成果被放弃。缓解：协议规范文档先行（写 spec 再写码）、把旧协议的不变量改写为 v2 测试向量、核心环先窄后宽。
2. **autopilot/ralph 终态设计谨防复刻 OMC 复杂度**：终态 ≠ 大而全——Phase A 以最小完备的判停/编排接口为终态目标，宁可终态精简，也不留「以后再加码」的半成品接口。
3. **skill 中立性可被 `oma doctor` 守护**：doctor 可静态检查 SKILL.md 引用的 oma 命令是否存在、frontmatter 是否超预算——把写作规范变成可执行检查。
4. **OMC 退场建议保留回退**：Phase D dogfood 周内先 disable/blocklist OMC 而非删除安装，一周达标后再卸载。
5. **deep-interview 固化的现成参照**：本次访谈使用的状态文件（`.omc/state/deep-interview-state.json`）即固化版状态 schema 的雏形，评分公式/轮换规则/挑战模式触发都已在实践中验证。
6. **二进制名 `oma` 为带标记默认值**：评审时确认或更名（影响所有 skill 文本，宜早定）。
7. **`best-practice-research`、`performance-goal` 来源待查**：不在 OMC 4.14.6 清单中，候选池评估时先定位出处。


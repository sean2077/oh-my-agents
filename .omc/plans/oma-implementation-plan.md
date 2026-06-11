# oma 实现计划 v2.1（终态版）

> 依据：`.omc/specs/deep-interview-oh-my-agents.md` **rev 3.1**（v2.1 采纳计划评审 `006` 全部 7 条发现）。
> 状态：pending review（第二道 Codex 评审门）；通过 + 用户确认后开始实现。
> 流程：Claude 编写本计划 → Codex 评审 → 用户确认 → Claude 实现 → Codex 代码评审。

## 0. 规划原则（rev 3 用户指令，全计划遵循）

1. **终态设计**：协议、命令树、schema、skill 全部按最终形态设计并一次实现到位；无「先薄版后固化」的重写计划，无任何过渡依赖（含旧 relay——产品任何阶段零依赖，用户决策）。
2. **构建按依赖顺序**：设计文档 → CLI（基础层 → relay v2 → 工作流命令）→ skill（引用已存在的最终命令）→ dogfood。顺序保证任何已交付 skill 永不引用未实现命令（原评审 blocker 结构性消除）。
3. **不急上线**：发版**能力**（goreleaser/self-update）按终态建好，对外开放不设期限；质量门禁全部进 CI，打磨期无限长。

## 1. 技术栈与 Go module

- Go ≥ 1.22，module `github.com/sean2077/oh-my-agents`；CLI 框架 cobra；零 cgo；标准库优先
- 目标平台：linux/amd64、linux/arm64、darwin/arm64（goreleaser 矩阵）

## 2. 仓库目录布局（终态）

```
oh-my-agents/
├── go.mod / go.sum
├── cmd/oma/main.go              # 入口（只做 wire）
├── internal/
│   ├── cli/                     # cobra 命令层（薄壳，逻辑下沉）
│   │   ├── root.go  asset.go  state.go  doctor.go  relay.go  interview.go  ralph.go  selfupdate.go  version.go
│   ├── asset/                   # manifest(targets 声明)/install/project/backup/verify
│   ├── agentdir/                # claude.go / codex.go：路径、软链、fragment 原子注入
│   ├── state/                   # 项目级 .oma/state/*.json，tmp+rename 原子写 0600
│   ├── relay/                   # v2 协议实现：ledger/sentinel/sidecar/heartbeat/claim
│   ├── workflow/
│   │   ├── interview/           # 评分公式、阈值门禁、轮次状态机
│   │   └── ralph/               # 判停、轮次计数
│   ├── budget/                  # pinned 近似 tokenizer + per-agent 注入面模型
│   ├── checks/                  # doctor 注册表：refcheck/security/conformance/budget
│   ├── update/                  # self-update（release 查询+原子替换+回退）
│   └── version/
├── assets/                      # 内容资产终态（随仓库 tag 版本化）
│   ├── skills/{deep-interview,autopilot,ralph,pair-delivery}/SKILL.md (+references/)
│   ├── agents/                  # CC-only subagents（targets: claude）
│   └── hooks/                   # 双端 fragment
├── docs/                        # Phase A 设计文档（上限所在）
│   ├── relay-v2-protocol.md  command-tree.md  schemas.md  security-contract.md
│   ├── workflows.md  adapter-conformance.md
├── testdata/                    # v1 旧账本 fixture、conformance 黄金文件、穿越攻击路径
└── .github/workflows/{ci.yml,release.yml}
```

## 3. 命令树（终态全集草案；Phase A 文档定稿后冻结）

```
oma asset install <name>... [--agent claude,codex] [--dry-run] [--force]
oma asset list|update|remove|rollback|link --dev          # `oma update` = asset update 别名
oma state get <key> / set <key> <value> [--file <path>]
oma doctor [--json]；oma doctor budget --agent claude --profile core4 --max-resident-tokens 2000
oma relay init|pair|publish|wait|status [--json]          # 目标 ≤10 子命令；heartbeat/claim 公开 vs 内部由 A2 显式决定
oma interview start|score|gate|status                     # deep-interview 固化面
oma ralph start|next|check|status                         # 循环判停/轮次
oma config show|path [--json]                             # 配置查询（纯读，详见 docs/config.md A7）
oma self-update [--check]；oma version
```
单一 asset 命名空间（无 `oma skill *`）。错误码、--json 输出契约、退出码语义在 `docs/command-tree.md` 统一定义。

## 4. 状态与 schema 文件（终态）

| 文件 | 用途 | 要点 |
|---|---|---|
| `~/.config/oma/registry.json` | 安装注册表 | XDG 位；schema_version，未知版本 fail-closed |
| `~/.agents/{skills,agents,hooks}/<name>/` | 资产规范位 | 兼容 npx-skills 生态；只管理自家注册条目 |
| `~/.claude/*`、codex 侧对应位 | per-agent 投影 | 投影前 verify 目标目录权限 |
| `.oma/state/*.json` | 项目级工作流状态 | 原子写 0600；interview/ralph 状态机数据 |
| `.oma/relay/` v2 账本（默认根，`--ledger-root` 可覆盖） | relay v2：v2 专属 sentinel + schema 标记 | **绝不写入 v1 `.shared/`**；遗留 .shared/ 报告为归档并共存；v1 树/未知 schema 拒绝（fail-closed） |
| `~/.config/oma/backups/<ts>/` | 覆盖前备份 | rollback 来源 |

schema 演进策略：schema_version 字段 + 版本化迁移命令——这是终态机制的一部分，不是过渡形态。

## 5. 测试策略与五道门禁

1. **单元/集成**：`go test ./...`；安装/投影跑 `t.TempDir()` 假 HOME，不碰真实用户目录
2. **静态命令引用检查**：checks/refcheck 解析 assets/**/SKILL.md 提取 `oma ...` 引用，对照 cobra 命令树反射注册表；CI + doctor 双跑（防回归门禁）
3. **token 预算**：budget 包对投影后 CC 常驻注入面建模；pinned 近似 tokenizer（算法版本入常量）；阈值 2000 进 CI；发版前 /context 实测校准一次
4. **conformance fixtures**：testdata/conformance/{claude,codex}/ 黄金文件断言投影产物；默认路径含不支持引用即 fail。**门禁分级（v2.1）**：Phase B 引擎以合成 fixtures 验证；Phase C 对真实 assets/ 复跑并 release-blocking
5. **安全测试**：--dry-run 不落盘、备份生成、--force 语义、软链穿越拒绝、权限位、fragment 原子注入与干净移除
6. **relay v2 协议测试**：spec 8 项清单逐项一个测试文件
7. **CI**：golangci-lint + 全部门禁 + goreleaser snapshot

## 6. Phase A：设计定稿（先行，过评审门）

| # | 任务 | 验收 |
|---|------|------|
| A1 | `docs/relay-v2-protocol.md`：**v2 账本根 `.oma/relay/` 与遗留 .shared/ 共存行为**；同机同 checkout 拓扑下的序号分配、claim 所有权、心跳 stale 阈值、stale 清理、时钟假设、中断 publish 恢复、冲突行为；v2 sentinel/schema 标记；继承的设计原则落为不变量清单 | Codex 设计评审通过 |
| A2 | `docs/command-tree.md`：全集签名、--json 契约、退出码语义、错误信息规范；**relay heartbeat/claim 公开 vs 内部的显式决定**（若内部：归属命令 + status --json 诊断方式） | 同上 |
| A3 | `docs/workflows.md`：interview/ralph 终态状态机（持久化字段、公式、停止条件、恢复/错误态）；autopilot 纯 md、不引入 `oma autopilot *` 命令面 | 同上 |
| A4 | `docs/adapter-conformance.md`：targets 语义、投影规则、Codex fallback、预算注入面定义、fixture 格式、refcheck 提取规则 | 同上 |
| A5 | `docs/schemas.md`：registry/state/ledger schema + schema_version 演进策略 | 同上 |
| A6 | `docs/security-contract.md`：spec 安全六条的实现规约 | 同上 |
| A7 | `docs/config.md`：viper 配置层——可配置项清单、优先级链（flag>env>项目>用户>默认）、与 schema 数据的严格边界、viper 使用约束（用户 2026-06-11 拍板引入 viper 限配置层） | 同上 |

## 7. Phase B：CLI 终态（依赖序内分三层）

| # | 任务 | 验证 |
|---|------|------|
| B1 | 脚手架：go.mod、cobra root/version、CI 骨架 | `go build ./... && go test ./...` |
| Bcfg | 配置层 `internal/config`（viper：优先级链 flag>env>项目>用户>默认，显式绑定+强类型校验，viper 实例不外泄）；命令层改消费 `*Config` 取代散落 `os.Getenv`；`oma config show/path` | 优先级链逐级覆盖 + fail-closed（语法/schema/越界）测试绿 |
| B2 | asset manifest + targets 解析 | unit 绿 |
| B3 | install/remove/update + 备份/rollback + --dry-run/--force | 安全测试集绿；`oma asset install deep-interview --dry-run` |
| B4 | agentdir 投影（软链 + fragment 原子注入）+ verify | conformance fixtures 绿 |
| B5 | state get/set | 并发原子写测试绿 |
| B6 | doctor 框架 + refcheck + security 检查 | `oma doctor --json` 干净环境全绿 |
| B7 | doctor budget | budget gate 接入 CI |
| B8 | relay v2 核心环（按 A1 实现，账本根 `.oma/relay/`） | 8 项协议测试 + 共存测试（遗留 .shared/ 并存 / --ledger-root 指 v1 拒绝 / 未知 schema 拒绝）全绿 |
| B9 | interview 固化命令（**按 A3 docs/workflows.md 实现**，不从旧 OMC/现有 state 文件推断行为） | unit + A3 一致性 |
| B10 | ralph 判停命令（按 A3 实现） | unit 绿 |
| B11 | self-update（checksum 校验、限定更新源、原子替换+回退、--check 只读）+ goreleaser + tag 版本门禁 | 安全测试：checksum 不匹配/元数据不可用/不可写/中断/回退；snapshot 构建 |

## 8. Phase C：资产终态（命令已全部存在，skill 一次写到位）

| # | 任务 | 验证 |
|---|------|------|
| C1 | deep-interview skill（frontmatter 极简路由器 + references/） | 三门对真实资产全绿（refcheck/budget core4/conformance），release-blocking |
| C2 | autopilot skill（纯 md：通用 `oma state` 存续状态，无专属命令面；CC 加速显式分支） | 三门全绿 |
| C3 | ralph skill | 三门全绿 |
| C4 | pair-delivery skill（接线 oma relay v2；planner/implementer/reviewer 角色可配置） | 三门 + 投影断言 |
| C5 | CC-only subagents 与双端 hooks 资产（targets 矩阵落地） | conformance + doctor 警告路径测试 |

## 9. Phase D：Dogfood 打磨（无限期，验收持续）

| # | 任务 | 验证 |
|---|------|------|
| D1 | `oma asset link --dev` 自用；disable/blocklist OMC（记录确切回退命令） | 周志启动 |
| D2 | 带日期一周周志（无 re-enable 事件） | 周志归档 `.oma/dogfood-log.md` |
| D3 | 固化版 deep-interview 跑通一次真实需求 | 状态文件 + spec 产物 |
| D4 | 一次真实结对交付（plan→cross-review→implement→cross-review，指定拓扑） | v2 账本归档 |
| D5 | /context 实测校准 budget 算法；持续打磨（候选池评估、plugin 薄壳等 post-v1 议题从这里再立项） | 校准记录 |
| D6 | 结对工作流案例文档（用户决定 2026-06-11）：以 oma 自身从设计到实现的全程 relay 结对过程（50+ turn，spec 评审→Phase A 设计评审→Phase B 逐片实现评审）为素材，写入 docs/ 作为 pair-delivery 的价值佐证与真实样例。**原始素材不入仓**（用户决定 2026-06-11）：relay 账本（.shared/，已 gitignore）与访谈记录（本机 .omc/specs/\*-transcript.md，已从仓内 spec 剥离）仅留本机；案例文档为提炼叙述，不嵌原文、不依赖仓内可解引用 | 案例文档落 docs/，仓内不含原始账本/访谈记录 |

## 10. 计划级风险

1. **Phase A 设计错误成本高**（终态设计的代价）：缓解——设计文档独立过 Codex 评审门；schema_version 机制允许版本化演进（演进 ≠ 过渡形态）
2. **tokenizer 近似偏差**：/context 实测校准；内部目标 1800（10% 余量）
3. **与 npx skills 共存**：registry 只管自家条目；doctor 报告不修改外部条目
4. **self-update 写自身**：路径不可写降级为提示；替换失败自动回退
5. **deep-interview skill 预算压力**：frontmatter 极简路由器是硬约束，写不进就继续往 references/ 拆

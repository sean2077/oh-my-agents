# 借鉴 omx / omc 到 oma —— 终稿 + 分阶段实现规划

> 状态：**设计已收敛**。Claude(lead)规划 + Codex(reviewer)评审，经 relay pair `20260615-borrow-from-omx-omc` 交叉评审，verdict = `approve-with-changes`，全部 change 已采纳。
> 日期：2026-06-15　·　来源：oh-my-codex(omx)、oh-my-claudecode(omc)　·　基线：oma v0.2.0
> 本文是权威设计笔记 + 实现路线。每个借鉴点的实现作为**独立交付**，各自再走 Codex 双门评审（实现前 plan review、实现后 code review）。

## 0. 背景与核心判断
- 深度调研 omx / omc，对照 oma 真实代码。
- **核心判断：omx / omc 都没有 oma 这种跨 agent(Claude↔Codex)relay 结对账本 —— relay 是 oma 原创。** 值钱的是它们打磨出来的「让自治循环可信」的机制 + 少量 agent-neutral skill。
- 取舍以 oma 原则为尺：零宿主改写、文档而非命令、终态设计、agent-neutral 默认、机械逻辑入二进制(可测/fail-closed)、判断留在 skill、精简常驻足迹。

## 1. 决策摘要（借鉴集终稿）

| 项 | 借鉴 | 源 | verdict | oma 落地形态 | 优先级 |
|---|---|---|---|---|---|
| A1 | 完成验证回执(hash 回执，让「done」可证伪) | omx ultragoal | adopt | `oma-relay/3` artifact schema；ralph 终态 receipt | **P0** |
| A2 | 质量门即数据(类型化 verdict + close 门) | omc verdict.json / omx QualityGate | adopt | review/decision frontmatter + close fail-closed 门 | **P0** |
| A3 | Stop-hook 逃生阀(context/rate/auth) | omc persistent-mode | adopt(缩小) | HookPayload + hookStop 前置短路 | P1 |
| A4 | deep-interview 增强(事实/判断路由等) | omx/omc deep-interview | adopt | deep-interview SKILL.md(+ 可选 interview 前置) | P1 |
| A5 | statusline turn/stale 预设 | omc HUD | adopt(缩小) | statusline 渲染，**不**解析 transcript | P2 |
| A6 | 通知 + 回复注入(远程操控) | omx reply-listener/hermes | **延后·user-gated** | 候选，不实现(需用户批准) | 延后 |
| A7 | catalog 单一真源(生成视图) | omx catalog-manifest | adopt(改进) | 从现有 manifest **生成**的 catalog/check | P2 |
| B1 | `trace`(对抗式根因调查) | omc | adopt | 新 skill(纯判断) | **wave1** |
| B1 | `ai-slop-cleaner`(回归安全清理) | omc | adopt | 新 skill + 完成绑 verifier | **wave1** |
| B2 | `ultraqa` = ralph profile | omx | adopt | ralph 预设，非独立 skill | wave2 |
| B2 | `ralplan` 模糊门 | omc | adopt(缩小) | 挂 start 边界的 advisory 门 | wave2 |
| B2 | `skillify`(工作流→skill) | omc | adopt | 元 skill，依赖 A7 | wave2 |

- **已剔除**：原 issue 命令(mission-board/teleport/issue) —— 用户明确不需要。
- **明确 skip**：prometheus-strict / self-improve / ccg / deep-dive / team·swarm·worker·pipeline / ultrawork / verify / remember / visual-verdict / 所有 *-setup·doctor·hud·notifications·session-manager(冗余 / 宿主强耦合 / 机制过重 / 属安装排障内容)。
- **具名候选池**(非 wave1，日后或作中立 skill)：`release` / `best-practice-research` / `deepinit`。

## 2. 关键架构约束（已对照代码核实，驱动设计）
- 严格 frontmatter parser，未知 key / 重复 key / schema major≠2 一律 fail-closed：`internal/relay/artifact.go:51-80`(Validate)、`:149-235`(Parse，`:227-229` 未知 key)、`:174-176`(重复 key)。→ **A1/A2 必须升 schema，不能贴字段。**
- `.sha256` 是对 rendered bytes 的传输完整性：`internal/relay/artifact.go:248-269`。→ 回执是字节内的**语义哈希**，不替代 sidecar。
- `Render` 固定 key 顺序、`Parse` 只回读该子集：`:105-144`。→ 加字段须同改 Render+Parse+Validate。
- `close` 仅校验 outcome∈{approve,reject,abandon}+reason 即置终态归档，无 verdict 依赖：`internal/relay/session.go:248-287`。→ **A2 的 close 门是新机制。**
- hookStop 只在「有新 peer artifact」时 block，`HookPayload` 无 stop_reason：`internal/relay/hook.go:24-32`、`:87-124`。→ A3 需扩 payload + 前置短路。
- statusline 有界纯读(2s deadline)：`internal/relay/statusline.go:29-47`。→ A5 **不得**解析 transcript 取 context%。
- ralph 只持有计数/check/停判，无证据回执：`internal/ralph/ralph.go:44-56`、`:207-237`。→ A1 在 ralph 仅 PhasePassed 加 receipt。

## 3. 分阶段实现规划

### Phase 1（P0）—— A1 + A2：`oma-relay/3` 回执 + 质量门 + close 门
作为**同一 schema/门切片**一次做到终态（最先、最复杂）。

> 注：本节初版规划经 Codex code-review（R1–R4 + 未来-target 残留）收敛为下述**终态**——`reviewed_head` 模型，无 `ledger_head`/`plan_ref`/`evidence_hash`。

**Schema 变更（终态）**
- artifact schema 升 `oma-relay/3`（session/sentinel 仍 `oma-relay/2`）。
- ready `kind:review` **必带**：`verdict`(approve|approve-with-changes|revise) + `review_target_seq`（≥1，且须指向一条**已发布、且早于本 review**的 artifact）。纯散文走 `note`/`addendum`。
- `kind:decision` 增完成回执字段：`receipt_id`、`reviewed_head_seq`、`reviewed_head_hash`、`quality_gate_seq`、`quality_gate_hash`、`verified_at`。
- 回执 canonical JSON `oma-completion-receipt/1` = `{schema, pair, decision_seq, reviewed_head{seq,hash}, quality_gate_ref{seq,verdict,hash}, verified_at}`；`reviewed_head` = 被批准的工作（最新非 review/非 decision artifact）。

**代码变更**
- `internal/relay/artifact.go`：`Frontmatter` 增上述字段；`Render`/`Parse`/`Validate` 扩展；artifact major=3；**未知 key 仍 fail-closed**。
- `internal/relay/receipt.go`：`Receipt`、`buildDecisionReceipt`（取 reviewed_head + 针对它且更新的非-lead approve review）、`verifyApproveClose`、`ErrGate`。
- `internal/relay/publish.go`：review 必带 verdict+target，且 target 须已发布且 `< 本 review seq`（防未来-target）；decision 自动盖回执。
- `internal/relay/session.go` `Close`：approve 走 `verifyApproveClose`；`internal/cli/relay.go`：`ErrGate` → exit 4。
- `internal/ralph/ralph.go`：`PhasePassed` 写 `receipt = hash{goal, checks, terminal_check}`（只证明记录的 exit code）。

**命令面**
- `oma relay publish` 对 `kind:review` 增 `--verdict`、`--review-target <seq>`；`kind:decision` 自动盖回执；`oma relay close --outcome approve` 走门。

**close approve 门（终态，fail-closed）**
1. 载入最新 ready + 校验 sidecar；找最新 lead `kind:decision`，须含合法回执（否则 exit 4）。
2. 回执 `reviewed_head` 须存在且 hash 一致（损坏 → exit 3）；reviewed_head 之后**无更新的未评审工作**（否则 exit 4）。
3. 回执引用的 `kind:review` 须存在、hash 一致（损坏 → exit 3），且：非-lead、`verdict=approve`、`review_target_seq == reviewed_head`、`review.seq > reviewed_head.seq`（任一不满足 → exit 4）。
4. `approve-with-changes` / lead 自评 / 未来或陈旧 target 均不满足。

**测试**（`internal/relay/receipt_test.go` + `internal/cli/relay_test.go`）
- 设计路径（plan 被评）与实现路径（fix 被评）均出回执并过 close。
- R1：未评审 fix → close 拒，直到 fix 被 approve；未来-target review → publish 拒。
- R3：review 缺 verdict / 缺 target → publish 拒。
- R4：gate miss → exit 4（CLI 回归），损坏 → exit 3。
- ralph PhasePassed receipt 哈希稳定、可复算。

### Phase 2（P1）—— A3 逃生阀 + A4 deep-interview + wave1 skills（可并行）
**A3 Stop-hook 逃生阀**
- `internal/relay/hook.go`：`HookPayload` 增 `StopReason`(及必要的 transcript 字段)；`hookStop` 前置短路：`context_limit` / `rate_limit` / `auth_error` → 静默(`return nil`)；宿主命名容错别名表(Claude/Codex 不同串)。
- scheduled-wakeup 暂缓，待确认双宿主 payload 证据。
- 测试：三类 stop_reason → 即使有新 peer artifact 也静默；普通 stop + 新 artifact → 仍 block。

**A4 deep-interview 增强**（判断层，仅改 SKILL.md；机械门已在 `oma interview`）
- 事实 vs 判断路由：`[from-code][auto-confirmed]` / `[from-user]`，可自查的事实不问人。
- 节奏守卫：连续 **2** 轮非用户/纯证据 → 强制一轮 `[from-user]`。
- input-lock：interview 进行中阻止「yes/ok/proceed」提前批准(先作 skill 纪律；仅当能表达为确定性 transition/score 前置才入二进制)。
- 强制 Non-goals + Decision-Boundaries 门(与分数无关)。

**wave1 `trace`**（新 skill，纯判断）
- `assets/skills/trace/{SKILL.md,manifest.json}`(targets=[claude,codex])。
- 保留 7 段契约 + 6 级证据强度 + 自我证伪 + 反驳轮 + 最高价值判别探针；**默认单 agent 顺序**(并行 lane 作 CC 可选加速)；去掉 trace_* MCP 引用。

**wave1 `ai-slop-cleaner`**（新 skill + 完成门）
- `assets/skills/ai-slop-cleaner/{SKILL.md,manifest.json}`。
- 先用测试锁行为 → 分类气味(重复/死代码/多余抽象/边界破坏/缺测试) → 逐遍清理 + 遍间复验。
- **完成必须有观测到的 verifier 结果**或**用户显式批准的 no-test 理由**(绑 `oma ralph check` 或 autopilot verify)，不得沦为通用清理许可证。

### Phase 3（P2，不阻塞）—— A5 statusline + A7 catalog
**A5**：`internal/relay/statusline.go` 增 turn/stale 渲染预设；保持有界纯读，**不解析 transcript**。
**A7**：从现有 `assets/*/*/manifest.json` **生成** catalog/check（接入 `internal/checks/` 的 refcheck/budget/conformance），带 status 生命周期(active/deprecated/merged/alias)；**不引入会与 install manifest/registry 分叉的第二真源**。待核心资产 >4 触发。

### Phase 4（wave2）—— ultraqa / ralplan 门 / skillify
- **ultraqa = ralph profile**：对抗 e2e 场景矩阵 + 安全边界(判断层，取 omx 正文)骑在 `oma ralph` 现有【周期计数 + 3x 同因停滞 + 终态】上，**无新二进制**。
- **ralplan 模糊门**：≤15 有效词 + 无 文件/issue/符号 锚点 → 转回规划；仅挂 CLI 已有 start 边界(`ralph start` / `interview start` / 可能 `autopilot start`)作 advisory/fail-closed；**不**新建泛化 `oma gate`。
- **skillify**：把完成的工作流固化成新 skill + 质量门；**依赖 A7**(catalog)，置于其后。

### 延后 / 候选池
- **A6**(远程通知 + 回复注入)：**user-gated 延后**。引入守护进程/会话控制面，触及零宿主改写与安全边界；纳入路线图前需用户显式批准 + 设计轻量化形态。
- 候选池：`release` / `best-practice-research` / `deepinit`(干净中立，但偏离双 agent 交付循环主旨)。

## 4. 全局原则与每阶段验收
- **终态设计**：schema/命令/skill 一次到终态，不引入过渡形态(oma-relay/3 一步到位，不做 /2→/3 迁移层)。
- **fail-closed**：所有新解析/门未知即拒。
- **agent-neutral**：skill 默认路径纯 `oma` 命令 + markdown；CC 加速作可选分支。
- **零宿主改写**：A3/A5 只改 oma 自身派发器/渲染；hook wiring 仍由用户手动(文档)。
- **每项独立交付**：实现前 Codex plan review、实现后 Codex code review(双门不可跳过)。
- **budget 门**：新增 wave1 skill 后跑 `oma doctor budget`，核心集常驻足迹不破阈。

## 5. 来源索引（关键文件，便于实现时回查）
- omx：`src/ultragoal/artifacts.ts`(回执/质量门/steering)、`src/ralph/completion-audit.ts`、`src/state/workflow-transition.ts`、`skills/deep-interview/SKILL.md`、`templates/catalog-manifest.json`、`src/notifications/reply-listener.ts`、`src/mcp/hermes-server.ts`。
- omc：`scripts/persistent-mode.mjs`(逃生阀)、`src/hooks/persistent-mode/index.ts`、`src/ultragoal/artifacts.ts`、`src/shared/artifact-descriptor.ts`、`src/hud/*`、`skills/trace/SKILL.md`、`skills/ai-slop-cleaner/SKILL.md`、`skills/ralplan/SKILL.md`、`skills/skillify/SKILL.md`。
- relay 交叉评审记录：`.oma/relay/_archive/20260615-borrow-from-omx-omc/`(plan 001 / review 002 / addendum 003 / decision 004)。

## 6. 实现状态（分支 `feat/borrow-omx-omc`，2026-06-15）

> 实现进行中。已完成项均 build + vet + 全测试 green（`make check`）。每项仍待 Codex code-review 门（codex 未装在本机，需用户窗口接力驱动）。

**已完成（green）**
- **wave1 skill `trace`** — `assets/skills/trace/`（纯判断、agent-neutral；CC 并行 lane 作可选加速）。
- **wave1 skill `ai-slop-cleaner`** — `assets/skills/ai-slop-cleaner/`（含「green verifier 或用户批准 no-test 理由」完成门）。
- **A4 deep-interview** — 事实/判断路由（`[from-code][auto-confirmed]`/`[from-user]`）、节奏守卫(连续 2 轮非用户→强制 [from-user])、input-lock、强制 Non-goals/Decision-Boundaries 内容门。
- **A3 stop-hook 逃生阀** — `internal/relay/hook.go`：`HookPayload` 解析 stop_reason，context/rate/auth 静默(容错别名)；`TestHookStopEscapeValves` 覆盖。
- **A1+A2** — `oma-relay/3` artifact schema（session/sentinel 仍 /2）；ready review `verdict` **必填** + decision 完成回执（`internal/relay/receipt.go`：build+verify，hash 绑定 **reviewed_head(最新工作) + 针对它的非-lead approve review**，且校验无更新的未评审工作）；`close --outcome approve` fail-closed 门（`session.go`，门未过 exit 4 / 损坏 exit 3）；CLI `--verdict`/`--review-target`；ralph PhasePassed 回执（`ralph.go`）；pair-delivery skill 已对齐新门。**Codex code-review R1–R4 已全部修复**（R1 未评审改动不得 close、R2 reviewed_head 强校验、R3 verdict 必填、R4 exit 4/3 分流）。

**已完成（续）**
- **A5** statusline 预设 — `oma relay statusline --preset minimal|focused|full`（纯格式化，不解析 transcript；`relay/statusline.go` `RenderPreset`）。
- **A7** catalog 生成视图 — `oma asset catalog`（从 `assets/*/manifest.json` 派生，无第二真源；manifest 增 `status`/`canonical` 生命周期；`asset/catalog.go` + 测试）。
- **wave2** — `assets/skills/ultraqa/`(ralph profile) + `assets/skills/skillify/`(质量门元 skill) + ralplan 模糊门（`cli/gate.go`，挂 `ralph start` 的 advisory，不阻断；+ 测试）。
- **文档同步** — `docs/{schemas,relay-v2-protocol,command-tree}.md` 已更新到 oma-relay/3 + verdict/receipt + close 门 + 新 flags + catalog。

**待办（非代码）**
- **A6** 通知/远程注入 — 用户已决定延后、不实现（候选）。
- **Codex code-review 门** — 对整批发起 relay 评审（用户驱动 Codex，codex 未在本机）。
- dogfood：安装到 `~/.agents` 后跑 `oma doctor budget` 复核常驻足迹（catalog 现 8 skill）。

## 7. 外部信号：宿主原生 `/goal`（= Ralph loop）验证 ralph 押注（2026-06-16）

> 非 omx/omc 借鉴项，而是来自 Codex / Claude Code 的外部信号；因直接关乎 ralph（+ A1 receipt / B2 ultraqa）暂记于此。`docs/borrow-from-omx-omc-2.md` 现于 `feat/borrow-omx-omc-2` worktree、尚未并入 main；其并入后本节可迁移过去。

- **发生了什么**：Codex 0.128.0、Claude Code 2.1.139 各自把 Ralph loop 做成**宿主原生** `/goal` —— 声明可验证终态 → 自治续 turn → 小模型（默认 Haiku）读 transcript 判停；两家都仍挂 experimental 门。
- **判断：验证而非冗余。** 两家同时原生化此模式，反证 ralph 押注正确；但 `/goal` = 宿主耦合 + 版本门禁 + LLM 概率判停，oma 的**确定性计数 + stall 检测 + 终态持久化 + 跨宿主一致 + 可恢复**才是护城河 —— **不 deprecate ralph**。
- **落地：组合而非取代。** `/goal` 作外层 driver（自治续 turn），判停**委托** `oma ralph`：goal condition 写成「每 turn 推进一轮 ralph，直到 `oma ralph status --json` 报 terminal（passed/exhausted/stalled），每轮打印该 JSON」，宿主 evaluator 退化为只确认 JSON。额外收益：契合**零宿主改写** —— 用 `/goal` 自治续 ralph 轮次**免装 Stop-hook**。
- **已落地**：`assets/skills/{ralph,autopilot}/SKILL.md` 的 `/goal` driver 注脚（同 “CC acceleration (optional)” 约定：宿主能力是可选加速器，oma 契约跨宿主不变）。
- **来源**：Claude Code docs `code.claude.com/docs/en/goal`、Codex `developers.openai.com/codex/use-cases/follow-goals`、Simon Willison「Codex 0.128.0 adds /goal」（2026-04-30）。

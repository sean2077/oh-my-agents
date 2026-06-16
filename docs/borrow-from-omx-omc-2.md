# 借鉴 omx / omc 到 oma（第二批）—— 研究脉络 plan

> 状态：**Claude(lead) 起草，待 Codex 门1(plan review)**。本文是 `kind:plan` 的权威载体；门1 approve 后进 wave R1 实现，再走门2(code review) → decision/close。双门不可跳过。
> 日期：2026-06-16　·　来源：oh-my-codex(omx)、oh-my-claudecode(omc)　·　基线：oma 第一批 borrow 已合入 `main`（`bca2dd5`，含 A1–A7 + wave1/2 + R1–R4 close 门收敛）。
> 与第一份的关系：`docs/borrow-from-omx-omc.md` 收的是**交付环**（relay 回执 / 质量门 / 逃生阀 / trace / ai-slop-cleaner 等）；本文补的是它**低配的一整条「研究 / 自治探索」脉络**。

## 0. 背景与核心判断
- 第一批深调研聚焦"让双 agent 交付可信"。复盘时发现 omx 还有一条**研究性质**的脉络（`missions/` + `autoresearch-goal` + `analyze` + 一组研究人格 prompt），第一份只在候选池轻点了 `best-practice-research`，未系统借鉴。
- **核心判断**：omx 的研究机制值钱在「**可证伪的自治探索**」——假设 → 实验 → 确定性度量(evaluator) → keep-policy 保留 → 候选账本 → 可证伪停判。oma 已有 ralph 的**停判骨架**（计数 / 同因停滞 / 终态 / receipt），但缺两块：① score-based keep-policy（按分保留最优 + score-plateau 停）；② 把研究问题立项成可复跑契约的 scaffold。
- 取舍仍以 oma 原则为尺：零宿主改写、文档而非命令、终态设计、agent-neutral 默认、机械逻辑入二进制(可测 / fail-closed)、判断留 skill、精简常驻足迹。
- **已核实**：omx evaluator 契约真实存在（`missions/*/sandbox.md` 的 `keep_policy: pass_only|score_improvement`，11 个 mission 在用；evaluator 输出 `{"pass":bool,"score":float}`）。

## 1. 决策摘要（研究脉络借鉴集，**提议**待评审）

| 项 | 借鉴 | 源 | 提议 verdict | oma 落地形态 | wave |
|---|---|---|---|---|---|
| R1 | 自治研究环：score-based keep-policy + score-plateau 停判 | omx missions / autoresearch evaluator | adopt | `ralph.Check.Score` + keep-policy 入二进制；`research` = ralph 预设 | **R1** |
| R2 | research-mission scaffold（mission + evaluator 契约 + 候选账本） | omx `missions/` | adopt | 新 skill `research-mission`（markdown+模板），handoff 给 R1 | **R1** |
| R3 | `analyze`（证据/推断/未知三分 + 强度阶梯 + ranked synthesis） | omc `analyze` | adopt | 纯 markdown skill，agent-neutral | R2 |
| R4 | `best-practice-research`（源质量阶梯 + URL/版本锚 + 只调研强制 handoff） | omc/omx | adopt（候选池→提议采纳） | 纯 markdown skill | R2 |
| R5 | 证据层硬化（evidence_type 分层 + 非占位证据门 + artifactPath 必填 + 置信×严重度正交） | omx goal-workflows/validation + researcher/ux-researcher prompt | adopt（**降序**） | 扩 review evidence JSON + receipt 校验 | **R3（blocked-on 第一批 A1/A2）** |
| R6 | deep-interview 再增强（CRITICAL-轴问题过滤 + search-before-asking research fan-out） | omx prometheus-strict-metis | adopt-缩小 | 在已落地 A4 基础上**增量**改 SKILL | R2（可选） |

**已剔除 / 明确 skip**（承袭第一份判断 + 本轮复核）：
- `deepsearch`、`ralph-init` —— omx 内已 hard-deprecated 空壳，无内容。
- `design`、`wiki`、`visual-ralph`/`visual-verdict` —— 前端/知识库专用，偏离双 agent 交付主旨；`wiki` 还会与 relay 账本争"单一真源"。
- `code-review` verdict schema —— 与第一批 A2（review verdict + evidence JSON + close 门）基本同形，不重借。
- `prometheus-strict` 整体 —— 已 skip；仅取方法学碎片并入 R6。
- `autoresearch-goal` 的 professor-critic 账本 / 只读对账 —— 大部分 ≈ 第一批 A1/A2 receipt；仅取其 **evaluator + rubric** 的研究语义并入 R1/R2。

## 2. 关键架构约束（已对照 oma 现状代码核实，驱动设计）
1. **ralph 安全契约**：oma 从不执行 verifier，agent 自己跑并通过 `oma ralph check --verifier-exit` 报告（`internal/ralph/ralph.go:1-5`、`RecordCheck` `:216-245`）。→ **R1 的 evaluator 仍由 agent 执行**，oma 只记录 score + 判 keep/plateau；receipt 只证明"记录的结果"，不证明真跑了命令。**不得破此契约。**
2. **`Check` 无 score**：现为 `{Round, VerifierExit int, Note string, At}`（`:38-44`），`oma ralph check` 只有 `--verifier-exit/--note`（无 `--score`）。→ R1 升 `Check` 加 `Score *float64` + `oma ralph check --score`。**state contract 定为 `oma-ralph/2`（终态，无迁移层）**：新增持久字段 `KeepPolicy`/`PlateauWindow`/`BestRound`/`BestScore` + 新终态改变了 `/1` 契约；`Load` 本就拒 foreign major（`:130-132`），干净 `/2` 符合仓库 fail-closed 风格（**codex 门1 R1-#1**）。
3. **停判三态**：exit0 → `PhasePassed`+receipt；`StallWindow` 个同 `note` 连续失败 → `PhaseStalled`；round>max → `PhaseExhausted`（`:225-231`、`stalled` `:248-264`、`Next` `:198-205`）。→ R1 在 `score_improvement` 下加 **独立终态 `PhasePlateaued`**（**不复用 `PhaseStalled`** —— 后者是同失败签名重复，plateau 是 score 无提升，语义不同，混用会让 next/status 解释失真；**codex 门1 R1-#2**），连续 `PlateauWindow` 轮 score 无提升即停，并记 best_round/best_score 实现 keep-best。
4. **ralph receipt** = sha256{goal, checks, terminal_check}（`ralphReceipt:268-283`，现仅 exit0 设 receipt `:225-229`）。→ `score_improvement` 下 **plateau/exhaustion 也是有意义终态**，receipt 须在这些终态也产出（不止 `PhasePassed`），hash 全部 checks 且 `terminal_check` 指向 **best-score 轮**；否则「keep-best」无 receipt 支撑（**codex 门1 R1-#3**）。
5. **keep-policy 二态**：`pass_only`（= 现状 exit0=passed，几乎零改动）；`score_improvement`（新增机械逻辑）。两者皆**确定性、可测、fail-closed**，入二进制；假设生成 / 改动实现 / evaluator 命令设计留 skill。
6. **skill 足迹**：现 8 个（`assets/skills/`：ai-slop-cleaner, autopilot, deep-interview, pair-delivery, ralph, skillify, trace, ultraqa）。R2/R3/R4 新增 `analyze`+`best-practice-research`+`research-mission` → 11。→ 跑 `oma doctor budget` 核常驻足迹；三者多为**按需调用非常驻**，评估是否破阈。
7. **evidence schema（R5 前置）**：`docs/schemas.md` 未直接命中 `oma-review-evidence` 串，现状落点需实现时对照核实。R5 终态扩字段，**改 `receipt.go`/`artifact.go`/`publish.go` ——与第一批 A1/A2 同一批文件**。→ R5 必须排在第一批 pair `20260616-borrow-impl-review` close 之后，禁止两个 pair 并发改 relay 内核。

## 3. 分阶段实现规划

### wave R1（最大能力缺口；与第一批**零文件耦合**）—— 自治研究环
**R1 score keep-policy + R2 mission scaffold 作同一切片一次做到终态。**

**二进制变更（机械、可测、fail-closed）**
- `internal/ralph/ralph.go`：`Check` 增 `Score *float64`；`State` 增 `KeepPolicy`（`pass_only|score_improvement`）、`PlateauWindow`、`BestRound`、`BestScore`；schema `oma-ralph/1`→**`/2`**（无迁移层）。
- keep-policy 逻辑：`score_improvement` 下 `RecordCheck` 比较新 score 与 `BestScore`，更新 best/best_round；连续 `PlateauWindow` 轮无提升 → **`PhasePlateaued`**（独立终态）。
- **score 校验/比较规则（fail-closed，实现前定稿；codex 门1 R1-#4）**：`score_improvement` 下每次 `check` 必带**有限数值** `--score`，缺失即拒；改进判定为**严格 `score > BestScore`**（不引入 epsilon）；`pass_only` 下传 `--score` **拒**（避免无意 inert 记录）；**不强制 `[0,1]` 区间**（omx 示例隐含但不全声明 —— 取通用有限实数、越大越好）。
- `ralphReceipt`：`score_improvement` 下 plateau/exhaustion/passed 终态均产 receipt，`terminal_check` 取 best-score 轮。
- CLI：`oma ralph start --keep-policy --plateau-window`、`oma ralph check --score <float>`。

**skill 变更（判断、纯 markdown）**
- 新 skill `research-mission`：产出 `mission.md`（目标 / 主目标文件 / 成功判据 / 允许·禁止改动）+ `sandbox` evaluator 契约（`command` + `format:json` + `keep_policy` + scope）+ **候选账本**（每轮 `{hypothesis, change_summary, evaluator_output, decision: keep|discard}`）。handoff 给 ralph `research` profile。
- `research` profile = ralph 预设（类比已落地 ultraqa），**非独立二进制**：用 research-mission 的 evaluator 契约驱动 `oma ralph`，hypothesis 生成 + 改动实现是 agent 判断。

**测试**（`internal/ralph/*_test.go` 扩矩阵）
- `score_improvement`：score 上升→keep+继续；连续 plateau-window 轮无提升→终态；best_round/best_score 正确。
- `pass_only`：行为与现状 exit0=passed 等价（回归）。
- receipt：score 进 checks 后哈希稳定可复算；terminal_check 指向 best 轮。
- fail-closed：score 缺失但 policy=score_improvement、非法 keep-policy 值 → 拒。

### wave R2（纯 markdown / 增量；零耦合）—— 研究类 skill
- **R3 `analyze`**：`assets/skills/analyze/{SKILL.md,manifest.json}`（targets=[claude,codex]）。只读调查；证据/推断/未知三分 + 证据强度阶梯 + ranked synthesis（rank/解释/置信/依据 + file:line）。与 `trace` 互补（trace 查根因，analyze 做有据问答），去掉 omx MCP 引用。
- **R4 `best-practice-research`**：`assets/skills/best-practice-research/{SKILL.md,manifest.json}`。源质量阶梯（官方/upstream/release-notes > 第三方）+ 强制 URL + 版本/日期锚 + 固定输出契约 + **只调研不实现**的强制 handoff（接 ralplan / pair-delivery / research-mission）。
- **R6（可选）deep-interview 增量**：在已落地 A4（事实/判断路由、节奏守卫、input-lock、Non-goals/Decision-Boundaries 门）**之上**加 CRITICAL-轴问题过滤（仅在 范围/验收/回滚/lane/handoff 5 轴分叉才发问）+ search-before-asking research fan-out。**先对照 A4 现状核实避免重复**。

### wave R3（**blocked-on 第一批 A1/A2 close**）—— 证据层硬化
- 扩 review 证据 JSON：`evidence_type`(official|source_reference|supplemental) + URL + 版本/日期；置信(High/Med/Low) × 严重度正交。
- receipt/completion 校验：非占位证据门（拒 `todo|tbd|stub|fake pass|not implemented`）+ `artifactPath` 必填（不只散文）。
- **改 `receipt.go`/`artifact.go`/`publish.go` —— 与第一批同文件**：第一批 pair `20260616-borrow-impl-review` 已 **archived + abandon-closed**（2026-06-16T04:45:27Z；codex 曾 approve seq005/006 但无 lead decision，故 abandon，**非 approve-closed**）。并发编辑风险已消除，故 R5 可排后；但 R5 实现**须以当前 `main`/分支代码为准重新核对**，不得拿那个 abandon close 当 approve receipt 依赖（**codex 门1 R5-#5**）。终态扩字段，无迁移层。

## 4. 全局原则与每阶段验收（承袭第一份）
- **终态设计**：schema / 命令 / skill 一次到终态（`oma-ralph/2` 若需则一步到位，不做 /1→/2 迁移层）。
- **fail-closed**：所有新解析 / keep-policy / 证据门未知即拒。
- **agent-neutral**：skill 默认纯 `oma` 命令 + markdown；CC 加速作可选分支。
- **零宿主改写**：R1 只改 oma 自身；evaluator 仍由 agent 执行。
- **每项独立交付**：实现前 Codex plan review、实现后 Codex code review，双门不可跳过。
- **budget 门**：wave R2 后跑 `oma doctor budget`，核心集常驻足迹不破阈。
- **不与第一批抢方向盘**：R5 显式 blocked-on，不并发改 relay 内核。

## 5. 来源索引（便于实现 / 评审回查）
- **omx**：`missions/*/sandbox.md`（evaluator `keep_policy`）、`missions/*/mission.md`、`skills/autoresearch-goal/SKILL.md`（professor-critic + 只读对账，仅取研究语义）、`skills/analyze/SKILL.md`、`skills/best-practice-research/SKILL.md`、`prompts/{researcher,ux-researcher,prometheus-strict-metis}.md`。
- **oma 现状核实点（file:line）**：`internal/ralph/ralph.go:1-5`(安全契约)、`:38-44`(Check 无 score)、`:216-264`(停判)、`:268-283`(receipt)；`oma ralph check`(无 `--score`)；`assets/skills/`(8 个)；第一批 `docs/borrow-from-omx-omc.md`。
- **本轮调研**：四路并行深读 omx 研究 skill / prompt / goal-workflow / missions（结论见本表）。

## 6. 状态
- Claude(lead) 起草 → **Codex 门1(plan review) = approve-with-changes（seq 2）**，5 点收紧已全数 adopt 纳入本文（schema `/2` 定稿、独立 `PhasePlateaued`、`score_improvement` 终态 receipt、score 校验/比较规则、R5 措辞）。
- 下一步：门1 二轮确认 → wave R1 实现 → 门2(code review) → decision。**close 留用户回来确认**（不自主 close）。
- R5 前置：第一批 pair 已 abandon-closed（非 approve-closed），R5 实现以当前代码为准。

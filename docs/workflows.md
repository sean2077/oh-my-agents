# 工作流终态规范（interview / ralph / autopilot / pair-delivery）

> Phase A 设计文档（A3）。状态：待设计评审。Phase B 的 `oma interview|ralph` 实现与 Phase C 的 skill 文本**必须引用本文档**，不得从旧 OMC 实现或历史 state 文件推断行为。

## 1. `oma interview` —— Socratic 需求澄清的固化面

固化原则：**数学与状态进 CLI，提问与判断留给 agent**。CLI 负责评分计算、阈值门禁、轮次/状态持久化；问题生成、维度评估打分、本体抽取由 agent 按 skill 文本执行后喂给 CLI。

### 1.1 状态机

```
created ──start──▶ topology_pending ──(锁定拓扑)──▶ interviewing
   interviewing ──(score: ambiguity ≤ threshold)──▶ gate_passed ──▶ crystallized(spec_path) ──▶ completed
   interviewing ──(早退/硬上限)──▶ gate_waived(警示记录) ──▶ crystallized ──▶ completed
   任意态 ──abort──▶ aborted
```

### 1.2 持久化（`.oma/state/interview-<id>.json`，schema `oma-interview/1`）

字段：`id, phase, type(greenfield|brownfield), threshold, threshold_source, initial_idea, topology{status, components[{id,name,description,status,evidence[],clarity_scores{goal,constraints,criteria,context?}}], deferrals[], last_targeted_component_id}, rounds[{round, component, dimension, question, answer, scores, ambiguity}], ontology_snapshots[{round, entities[], stability_ratio, matching_reasoning}], challenge_modes_used[], current_ambiguity, spec_path, created, updated`。

### 1.3 命令语义

- `start`：阈值解析优先级 = `--threshold` > `--depth`（quick 0.30 / standard 0.20 / deep 0.10）> 配置 > 默认 0.20；写初始状态，输出阈值与来源（强制首行报告，沿袭既有实践）。
- `score --input scores.json`：输入为 agent 评估的逐组件×维度分（0-1）与本体快照；CLI **确定性计算**：
  - 维度总分 = active 组件该维度的最小值（min-across-components）
  - ambiguity：greenfield `1-(goal×0.40+constraints×0.30+criteria×0.30)`；brownfield `1-(goal×0.35+constraints×0.25+criteria×0.25+context×0.15)`
  - 本体稳定度 = (stable+changed)/total（首轮 N/A；changed=同 type 且字段重叠>50% 的改名）
  - 选出下一目标 `weakest(component,dimension)` 并执行轮换规则（N>1 等弱时避开 last_targeted）
  - 追加 round 记录、更新状态、返回完整报告（--json）
- `gate`：`current_ambiguity ≤ threshold` → 退出码 0；否则 4，并输出最弱组件×维度与差距。轮次护栏由 gate 输出提示：round ≥ 10 软警告、≥ 20 硬上限（gate 仍按数值判定，越权决定留给用户）。
- 挑战模式：score 输出在 round ≥ 4/6/8（且各模式未用过、8 轮时 ambiguity>0.3）时提示 contrarian/simplifier/ontologist；agent 采用后在下轮 score 输入中标记 `challenge_mode_used`。

### 1.4 错误与恢复

- 状态文件损坏/未知 schema → fail-closed 拒绝并提示备份位置（写前自动 `.bak` 单代备份）。
- 所有命令幂等可重入：`start` 对已存在 id 拒绝（除非 `--resume` 显式恢复展示当前状态）。

## 2. `oma ralph` —— 持久循环的固化面

固化原则：**计数、判停、历史进 CLI；做事与验证执行留给 agent**。oma **绝不执行** verifier 命令（安全契约）；agent 自己跑验证，把退出码报给 CLI。

### 2.1 状态机与持久化（`.oma/state/ralph-<id>.json`，schema `oma-ralph/1`）

```
created ──start──▶ running ──next──▶ running（round+1）
running ──check(verifier-exit=0)──▶ passed（终态）
running ──next 且 round>max_rounds──▶ exhausted（终态）
running ──连续 N 次相同失败签名──▶ stalled（终态，建议换策略）
任意态 ──abort──▶ aborted
```

字段：`id, phase, goal, max_rounds(默认 10), round, checks[{round, verifier_exit, note, at}], stall_window(默认 3), created, updated`。

### 2.2 命令语义

- `start --goal <text>`：初始化；goal 必填（判停语义锚点）。
- `next`：round+1；输出 continue|stop 与原因（passed/exhausted/stalled 时 stop，退出码 4）。
- `check --verifier-exit <code> [--note]`：记录一次验证结果；exit 0 → passed。`--note` 建议填失败签名（如测试名），CLI 据 note 串判断 stalled（连续 stall_window 次相同 note）。
- `status`：当前轮次、历史、判停态。

## 3. autopilot —— 纯 markdown 工作流（无专属命令面）

- **决策记录**：不存在也不得新增 `oma autopilot *` 命令（变更需重开 spec + 本文档评审）。
- 持久状态用通用 `oma state`：namespace `autopilot/`（如 `autopilot/phase`、`autopilot/plan-path`）。
- skill 文本骨架：澄清（可调 interview）→ 计划 → 实现 → 验证（可调 ralph）→ 交付；每步落 state，使会话中断可恢复。
- CC 加速分支（显式标注）：可用 Plan mode / subagent 并行探索；Codex 默认路径走纯文本流程 + oma state。

## 4. pair-delivery —— 结对交付流（基于 relay v2）

- 角色取自 `session.json.roles`（planner/implementer/reviewer 可配置到任一参与者，一人可兼）。
- 流程门（与本项目自身交付流一致）：plan（kind: plan）→ 评审（kind: review，verdict approve/approve-with-changes/revise）→ 实现（touched_paths 记录）→ 代码评审（kind: review）→ kind: decision 收口。
- skill 职责：把上述门翻译为 relay 命令调用序列与 prompt_for_next 写作规范；revise 循环上限与 @user 上抛规则（行首 `@user:` + `--status timed_out`）。
- 双端一致：流程完全由 oma relay + 文本驱动，无 CC 专属依赖。

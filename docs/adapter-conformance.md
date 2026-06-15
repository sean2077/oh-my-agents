# 适配层能力矩阵与一致性规范

> Phase A 设计文档（A4）。状态：待设计评审。定义 manifest targets 语义、per-agent 投影规则、Codex fallback、预算注入面、conformance fixture 格式与 refcheck 提取规则。

## 1. 资产 manifest（`assets/<type>/<name>/manifest.json`，schema `oma-asset/1`）

```json
{
  "schema": "oma-asset/1",
  "name": "deep-interview",
  "type": "skill",                          // skill | subagent | hook | prompt
  "version": "随仓库 tag，安装时记录",
  "targets": ["claude", "codex"],           // 或 ["claude"]（CC-only）；"shared" 表示仅入规范位不投影
  "description_budget_tokens": 80,
  "fallback": "codex 端降级说明（CC-only 资产必填）"
}
```

## 2. 投影规则矩阵

| 资产类型 | 规范位（文件本体） | claude 投影 | codex 投影 |
|---|---|---|---|
| skill | `~/.agents/skills/<name>/` | 软链 `~/.claude/skills/<name>` | 按 codex skills 约定位软链；不支持时不投影 + doctor 警告 |
| subagent | `~/.agents/agents/<name>.md` | 软链 `~/.claude/agents/<name>.md` | **不投影**（Codex 无 subagent）+ manifest.fallback 说明 |
| hook | `~/.agents/hooks/<name>/` | **不投影**（仅规范位放置）+ skip 说明 | **不投影**（仅规范位放置）+ skip 说明 |
| prompt | `~/.agents/prompts/<name>.md` | （按需）`~/.claude/commands/` | `~/.codex/prompts/<name>.md` |

- 投影一律软链。**oma 不改写任何宿主配置文件**：早先 hook fragment 注入 `~/.claude/settings.json` / `~/.codex/hooks.json` 的做法已移除（用户决定 2026-06-15：不确定/改宿主状态的行为用文档指导而非命令，详见 relay-v2-protocol.md §12.4 同源决策）。
- **hook 资产 = 规范位放置（canonical-only）**：`manifest.json` + `fragment.json` 仍随资产落到 `~/.agents/hooks/<name>/`，但 oma 不解析、不注入；`agentdir.For(hook)` 对两端返回 skip。用户照 `fragment.json` 内容**手动**把条目接进自己的 `settings.json`/`hooks.json`（接线规范与守卫见 relay-v2-protocol.md §12.4）。
- 卸载 = 移除软链投影 + 移除规范位条目 + registry 去账（hook 无投影，仅删规范位+去账）。
- codex 侧具体路径以常量表维护（`internal/agentdir`；本机无 codex 时以文件断言验证，见 §6）。

## 3. 双端一致性契约

- 每个核心工作流 skill 必须有**不依赖 CC 原生能力的默认路径**（oma 命令 + 文本驱动）。
- Skill 文本只保留核心工作流步骤、判断规则、状态契约和安全边界；安装、PATH、平台引导等通用产品说明属于 README / docs，不进入 `SKILL.md`，避免常驻 prompt 膨胀。
- CC 加速分支必须显式标注（建议统一标记 `> **CC 加速**：…`），且**不得产生 Codex 无法经 oma 命令检视/接续的工作流状态**（状态只落 `.oma/state/` 与 relay 账本）。
- CC-only 机制（AskUserQuestion、subagent 调用等）在 skill 文本中必须给出对应降级写法（自由文本提问、单线程顺序执行）。

## 4. refcheck 提取规则（静态命令引用检查）

- 扫描范围：`assets/**/SKILL.md` 与 `assets/**/references/**/*.md` 的代码块与行内代码。
- 提取（rev A.1，采纳评审 010 finding 4）：按 shell 风格 token 化，遇 flag（`-` 开头）、重定向、管道、分号、行尾即停；对 token 序列求**最长已注册 cobra 命令前缀**匹配（支持任意层级嵌套：`oma relay pair ensure`、`oma doctor budget`）。
- 判定：最长非 flag 前缀必须**精确等于**一个已注册可运行命令（或文档声明的命令组用于行文示例）才算有效引用；`oma relay pair typo` 这类「合法前缀+非法叶子」判 fail。
- 对照表：cobra 命令树反射导出（含完整嵌套路径）。
- 失败条件：任何无效引用 → fail（退出码 4）。豁免：无（终态原则）。
- 必备 fixtures：`oma relay pair ensure`（合法三级）、`oma relay pair typo`（非法叶子）、`oma doctor budget`（合法二级+flags）、`oma asset link --dev`（flag 截断）、多行 shell 片段。

## 5. 预算注入面模型（`oma doctor budget`）

- **计数对象（claude profile）**：每个已安装且投影到 claude 的 skill 的 frontmatter `name` + `description` 字段；subagent 的 `name`+`description`+`whenToUse`；按 Claude Code 实际常驻加载行为建模（只算常驻面，不算按需加载的 SKILL.md 正文与 references/）。**hook 资产为 canonical-only（不注入宿主、不常驻），计零**——手动接入的 hook 是用户自管面，不计入 oma 预算门禁。
- tokenizer：pinned 近似算法 `tok ≈ ceil(utf8_bytes/4) `，常量 `BudgetAlgoVersion = "approx-b4/1"` 入库并写入 --json 输出；发版前与 `/context` 实测校准一次，偏差记录在 dogfood 日志。
- profile：`core4` = deep-interview, autopilot, ralph, pair-delivery（自 Phase C 起完整计量）；阈值 2000（CI 门禁），内部目标 1800。

## 6. conformance fixtures（双端离线验证）

- 位置：`testdata/conformance/{claude,codex}.json` 用例文件：每例含 `manifest`（内联 oma-asset/1 文档）、`payload_file`（+ 可选 `payload_content`）、`want_rel_home`（期望软链投影位，空 = 期望 skip——hook 与 shared 资产均为此情形）。oma 只投影软链，无注入断言。
- 测试流程：假 HOME（t.TempDir）→ 引擎 Install（单 agent 窄化）→ 按 `want_kind` 断言：软链目标指向规范位，或宿主配置中可按 `_oma_asset` 取回注入命令。
- 默认路径检查：对每个 skill 的默认路径文本断言不含目标端不支持的引用（如 codex fixture 中出现 `AskUserQuestion`、subagent 调用即 fail；允许出现在显式 CC 加速标记块内）。
- 本机无 codex 的现实约束：codex 侧验收以 fixtures 文件断言为准；真机冒烟为 Phase D 非阻塞补做项。

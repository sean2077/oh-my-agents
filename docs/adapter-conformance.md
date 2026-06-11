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
| hook | `~/.agents/hooks/<name>/` | fragment 原子合并入 `~/.claude/settings.json` | fragment 合并入 codex hooks 配置（一次性信任由用户完成） |
| prompt | `~/.agents/prompts/<name>.md` | （按需）`~/.claude/commands/` | `~/.codex/prompts/<name>.md` |

- 投影一律软链优先；无法软链的注入型（hook fragment）走「读-合并-写 tmp-rename + .bak」原子流程。
- 卸载 = 移除投影 + 移除规范位条目 + registry 去账；fragment 注入可干净反向移除（标记块包裹）。
- codex 侧具体路径在实现期以常量表维护（本机无 codex 时以文件断言验证，见 §6）。

## 3. 双端一致性契约

- 每个核心工作流 skill 必须有**不依赖 CC 原生能力的默认路径**（oma 命令 + 文本驱动）。
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

- **计数对象（claude profile）**：每个已安装且投影到 claude 的 skill 的 frontmatter `name` + `description` 字段；hooks 注入 settings.json 的 command 字符串；subagent 的 `name`+`description`+`whenToUse`；按 Claude Code 实际常驻加载行为建模（只算常驻面，不算按需加载的 SKILL.md 正文与 references/）。
- tokenizer：pinned 近似算法 `tok ≈ ceil(utf8_bytes/4) `，常量 `BudgetAlgoVersion = "approx-b4/1"` 入库并写入 --json 输出；发版前与 `/context` 实测校准一次，偏差记录在 dogfood 日志。
- profile：`core4` = deep-interview, autopilot, ralph, pair-delivery（自 Phase C 起完整计量）；阈值 2000（CI 门禁），内部目标 1800。

## 6. conformance fixtures（双端离线验证）

- 位置：`testdata/conformance/{claude,codex}/<asset-name>/` 黄金目录树（期望的投影产物：软链目标、注入后的配置片段）。
- 测试流程：假 HOME（t.TempDir）→ `oma asset install <name>` → 实际产物与黄金树逐项比对（路径、链接目标、文件内容、权限位）。
- 默认路径检查：对每个 skill 的默认路径文本断言不含目标端不支持的引用（如 codex fixture 中出现 `AskUserQuestion`、subagent 调用即 fail；允许出现在显式 CC 加速标记块内）。
- 本机无 codex 的现实约束：codex 侧验收以 fixtures 文件断言为准；真机冒烟为 Phase D 非阻塞补做项。

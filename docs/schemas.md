# Schema 规范（registry / state / relay / 工作流）

> Phase A 设计文档（A5）。状态：待设计评审。所有持久化数据的 schema 与演进策略。

## 1. 通用演进策略

- 每个持久化文件含 `schema` 字段，格式 `oma-<domain>/<major>`（如 `oma-registry/1`）。
- **major 不识别 → fail-closed**：拒绝读写，提示升级 oma 或检查文件来源。
- minor 演进 = 纯增字段：读取方容忍未知字段（保留透传，不丢弃）；删除/改名/语义变更必须 bump major 并提供 `oma doctor` 迁移子命令（版本化迁移是终态机制，非过渡形态）。
- 写入一律 tmp+rename 原子 + 0600；写前对既有文件做单代 `.bak` 备份。

## 2. 安装注册表 `~/.config/oma/registry.json`（`oma-registry/1`）

```json
{
  "schema": "oma-registry/1",
  "assets": [{
    "name": "deep-interview",
    "type": "skill",
    "version": "v0.3.0",
    "installed_at": "ISO-8601",
    "source": "release|dev-link",
    "canonical_path": "~/.agents/skills/deep-interview",
    "projections": [{"agent": "claude", "path": "~/.claude/skills/deep-interview", "kind": "symlink"}],
    "backups": [{"id": "20260611T150000", "path": "~/.config/oma/backups/20260611T150000/..."}]
  }]
}
```
- registry 只记录 oma 管理的条目；外部来源（npx skills 等）不入账、不修改，doctor 仅报告。

## 3. 通用项目状态 `.oma/state/<namespace>.json`（`oma-state/1`）

```json
{"schema": "oma-state/1", "namespace": "autopilot", "data": {"<key>": "<string value>"}, "updated": "ISO-8601"}
```
- `oma state get/set` 的载体；value 一律字符串，结构化交调用方。

## 4. relay v2 `session.json`（`oma-relay/2`，详见 relay-v2-protocol.md）

```json
{
  "schema": "oma-relay/2",
  "pair": "20260611-topic",
  "project": "oh-my-agents",
  "participants": ["claude", "codex"],
  "roles": {"planner": "claude", "implementer": "claude", "reviewer": "codex"},
  "status": "active|closed|cancelled|failed",
  "created": "ISO-8601", "closed": null, "outcome": null, "reason": null
}
```
- artifact frontmatter schema 同协议文档 §5；`.oma-relay-v2` sentinel：`{"schema":"oma-relay/2","created":"..."}`。
- pair 绑定 `.oma/relay/_bindings/<author-session>.json`（`oma-relay-binding/1`）：`{"schema":"oma-relay-binding/1","author":"claude","session_hash":"<平台会话id哈希>","pair":"20260611-topic","created":"ISO-8601","updated":"ISO-8601"}`；解析顺序与 fail-closed 语义见协议 §4a。

## 5. interview 状态 `.oma/state/interview-<id>.json`（`oma-interview/1`）

**权威字段集 = workflows.md §1.2**（本节不复制字段清单，避免双源漂移）。`score --input` 的输入文件 schema（`oma-interview-scores/1`）：

```json
{
  "schema": "oma-interview-scores/1",
  "round": 3,
  "component_scores": {"cli-core": {"goal": 0.55, "constraints": 0.3, "criteria": 0.2}},
  "question": "...", "answer": "...",
  "ontology": {"entities": [{"name": "...", "type": "...", "fields": [], "relationships": []}]},
  "challenge_mode_used": null
}
```

## 6. ralph 状态 `.oma/state/ralph-<id>.json`（`oma-ralph/1`）

字段集见 workflows.md §2.1（id/phase/goal/max_rounds/round/checks[]/stall_window/created/updated）。

## 7. 用户配置 `config.toml`（`oma-config/1`）

- 位置与优先级链见 docs/config.md（A7）：`~/.config/oma/config.toml`（用户）与 `<worktree>/.oma/config.toml`（项目，默认 private/local）。
- TOML 根级可含 `schema = "oma-config/1"`：缺失视为当前 major（容忍手写遗漏）；存在但 major ≠ 1 → fail-closed。
- 它是用户意图配置而非运行时状态：由 viper 限定承载（config.md §1 边界），不走本档其余 schema 的 encoding/json 读取层，但 schema 串的 major fail-closed 语义一致；登记入 `version.Schemas["config"]`。

## 8. dogfood 日志 `.oma/dogfood-log.md`

- 自由 markdown + 必填头部：开始日期、OMC 处置方式（disable/blocklist 命令原文）、**确切回退命令**。
- 每条记录：日期 + 事件（使用了哪个工作流 / 遇到的问题 / 是否动用回退）。
- Phase D 验收解析依赖头部字段存在性与「无 re-enable 事件」的文本断言（人工复核为主，不做严格机读 schema）。

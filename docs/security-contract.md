# 安全契约实现规约

> Phase A 设计文档（A6）。状态：待设计评审。spec「安全与权限要求」七条的实现级规约；每条映射到 Phase B 测试。

## 1. --dry-run 与写前披露

- `--dry-run` 为全局持久 flag，所有变更类命令继承（asset 全系、state set、relay draft/publish/close/init、self-update）：完整执行计算与校验路径，打印将创建/修改/删除的**确切绝对路径**与操作类型，保证零落盘（含零备份、零临时文件残留）。
- 测试：dry-run 后对目标目录树做快照比对，断言零变化。

## 2. 覆盖保护与备份/回滚

- 写入目标已存在且**非 oma 管理**（registry 无记录或内容哈希不符）→ 拒绝，提示 `--force`。
- `--force`：先把现状完整复制到 `~/.config/oma/backups/<UTC 时间戳>/<原相对路径>`，写 backup 清单入 registry，再覆盖。
- `oma asset rollback <name> [--to <id>]`：按清单逆向恢复；恢复本身也遵守本节（不会静默覆盖比备份更新的非 oma 内容——冲突时拒绝并提示）。
- 测试：覆盖被拒、force 产生可恢复备份、rollback 还原一致、rollback 冲突拒绝。

## 3. 软链与路径约束

- 任何投影/解析路径先 `EvalSymlinks` 归一化，结果必须位于**可信根**内：规范位 `~/.agents/`、各 agent 已知目录、`.oma/`、仓库 checkout（dev link）。出界 → 拒绝。
- 拒绝路径穿越输入（资产名、--ledger-root 等含 `..` 归一化后出界）。
- 目标父目录 world-writable（其他用户可写）→ 拒绝投影。
- 测试：穿越 fixture、出界软链、world-writable 目录三类拒绝。

## 4. 权限位

- 新建目录 0700、文件 0600（`.oma/`、`~/.config/oma/`、relay 账本全部适用）；doctor 校验并可报告漂移。
- **oma 不改写任何宿主配置文件**（用户决定 2026-06-15）。早先的 hook fragment 注入（向 `~/.claude/settings.json` / `~/.codex/hooks.json` 原子合并条目级 `_oma_asset` 标记、字节契约、重复键拒绝、移除/回滚 fail-closed 等整套机制）已移除：不确定/改宿主状态的行为改由文档指导用户手动接线（接线规范见 relay-v2-protocol.md §12.4）。hook 资产仅做规范位放置（canonical-only），oma 投影一律软链，宿主配置由用户自管。
- 软链投影移除维持「外来障碍物留置 + 警告」语义（无 oma 残留风险，删 canonical 即真移除）；`--dry-run` 的 remove/rollback 跑与真实路径相同的校验。

## 5. self-update 信任链

- 更新源**限定**编译期常量 `github.com/sean2077/oh-my-agents` 的 GitHub Releases；不跟随跨仓库/跨域重定向；资产名必须匹配 `oma_<version>_<os>_<arch>` 模式，意外名称 fail-closed。
- 下载后校验 release 附带 `checksums.txt` 中的 SHA-256；不匹配 → 拒绝并保留现二进制（release 流支持签名后升级为签名校验，留 minor 演进位）。
- 替换：写同目录 tmp → 校验可执行 → rename 原子替换；旧二进制先备份为 `<path>.old`；替换后自检（`--version` 子进程）失败 → 自动回滚 `.old`。
- 目标路径不可写 → 降级为打印手动更新指引（不提权、不 sudo）。
- `--check` 严格只读：仅查询与比对版本，零写盘。
- 测试：checksum 不匹配、元数据不可用、目标不可写、替换中断（kill 注入）、自检失败回滚、--check 零写盘。

## 6. relay 对端输入处理

- 对端 artifact 全字段视为不可信：frontmatter 经严格 schema 校验（未知 kind/status 拒绝）；body 仅作文本呈现，oma **永不**解析执行其中的命令；`touched_paths` 仅透传展示。
- `.sha256` 校验失败 / `.ready` 缺失 → 内容不返回给调用方（fail-closed）。
- secrets 防泄漏（rev A.1，评审 010 finding 5）：publish 前对 body、prompt_for_next 及可含用户文本的 frontmatter 字段运行模式扫描（常见 token/key 形态正则集），**强制执行，v1 无绕过开关**；命中 → 拒绝发布并指出行号。误报处理 = 修改 artifact，或在本契约附录登记**窄域 allow pattern**（`oma doctor` 报告生效的 allow 清单）。doctor 含同规则的账本巡检。
- 测试：篡改账本拒读、未知 schema 拒绝、secret 模式拒发。

## 7. 威胁对照表

| 威胁 | 防线 | 测试 |
|---|---|---|
| 恶意/损坏资产覆盖用户配置 | §2 覆盖保护 + 备份 | force/rollback 套件 |
| 软链逃逸写任意路径 | §3 可信根约束 | 穿越/出界套件 |
| oma 误改/损坏宿主配置 | §4 oma 零宿主写入（hook 注入已移除，改文档手接） | conformance（hook→skip）套件 |
| 供应链：伪造更新包 | §5 限源 + checksum + 回滚 | self-update 套件 |
| 结对对端注入指令/投毒 | §6 不可信输入 + fail-closed | relay 安全套件 |
| secrets 入账本扩散 | §6 发布前扫描 | secret 拒发 |

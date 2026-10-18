# CST 工程 Agent 入口（2026-10-12 起）

> **当前任务不在 GitHub Issue、旧 Mac 迁移文档或此 AGENTS 正文中派发。** 新 AI 必须从 Obsidian Vault 的
> `03-项目与工程/机场/Clash SpeedTest Pro/README.md`
> → `03-项目与工程/机场/Clash SpeedTest Pro/Handoff.md` 接续；
> 只有要核对长期决定时再读同目录 `Decisions.md`，验证范围见 `状态与证据.md`，附件索引见 `资料与附件.md`。
>
> **2026-10-13 用户决定：开发围绕 GitHub，每次源码修改验证后同步仓库。** 代码以 GitHub 明确开发分支与提交为准；Obsidian 保留任务与交接，Drive 保留脱敏附件。同步前保留本地改动、核对远端并检查凭据，不以旧文档或远端版本覆盖 dirty。同步源码不自动授权 main 合并、发布或实网扩测。
>
> **不要从已关闭 [Issue #32](https://github.com/suifracti/clash-speedtest/issues/32)、[历史 Mac 交接](MAC_HANDOFF.md) 或 [2026-10-07 暂停记录](docs/development-paused-20261007.md) 重新执行旧“下一步”。** 无法读取 Obsidian 时标明当前交接不可核验，不把旧计划自动顶替为最新授权。

## 接手时的最少核验

1. 对照本次用户消息与 Obsidian Handoff，确认**当前仅要做的一项**。原方案/竞品调研仅供参考，不默认启用旧 19 章路线。
2. 在**现有 Mac/Windows 工作树**核对 `git status --short`、`git rev-parse HEAD`、当前运行 App 或构建产物、数据根及未提交改动。远端 main 不自动等于本机最新成果；禁止 reset、覆盖 dirty、删除 worktree 或替换正在使用的 App。
3. 复用已有 Go/Mihomo/Wails/Vue/SQLite 模块，围绕实际故障做最小修复，执行受影响的最少编译/测试；必要时做已获授权的有限原生验证。**完成一项、验证一项、回写 Handoff 再进下一项**。
4. 清晰记录本次改动文件、确切测试结果、最新 HEAD/dirty、已验和未验。长日志、脱敏截图、构建、ZIP 放既有 Google Drive 项目文件夹，Obsidian 只放短结论和准确链接；不要反复复制完整证据到 GitHub。
5. 交付前按 [版本与功能保留检查](docs/version-consistency.md) 核对远端、main 完整性、已接受能力和 App 构建来源。此前确认旧未提交 IPOK 后端、VLESS pcs/vcn、Mac 订阅系统代理识别漏带；2026-10-13 已最小迁回并离线验证。以后仍须逐项核对接线、行为和 App 来源，不能只凭已合并 PR 完整或文件存在就宣称功能都在。`scripts/candidate_audit.py` 的源码/manifest 检查不等于运行验收；不得删除能力条目或给旧未知二进制猜写 SHA 来取得通过。

## 子代理（2026-10-18 用户指定）

- 需要并行时可自行使用子代理，只允许 `gpt-6-luna`，推理强度 `max`（Luna max）。创建时显式指定，不继承主代理模型。
- 无法使用这个组合就由主代理完成，不换其他模型，不唤起旧模型子代理。
- 简单任务直接完成，不为形式拆分；结束后自检遗漏。

## 安全与版本规则

- 保留订阅账号隔离、完整节点 identity/revision、历史 SQLite/WAL、取消和保存状态。未知不伪装成功/失败，Web HTTP 可达不等于服务业务成功。
- 未经对应许可不运行全机场大测、持续监测、额外付费模型请求或更改 VPN/TUN 设置；不擅自迁移或覆写真实生产数据库及凭据。不要向公开 GitHub 上传任何私有配置、订阅 URL、token、截图或生产历史。
- 默认不新建 PR、分支、工作树或 Issue；commit、push、合并、发布按当前用户授权执行。完成源码修改不等于 Mac/Windows 客户端已经真实验收。
- 本仓库的 [README.md](README.md) 只做公开导航和已知状态摘要，不是第二套交接主源。此文件的代码事实按实际工作树重新检查，不让文档里的旧 SHA 决定实现。

**CST 当前角色分工：Obsidian = 决策/执行交接；Google Drive = 文件附件；GitHub = 源码/提交与历史证据。**

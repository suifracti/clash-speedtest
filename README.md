# Clash SpeedTest Pro

基于 Go / Mihomo / Wails / Vue 3 / SQLite 的多机场代理节点测速与筛选工具。支持管理独立订阅、选节点、六站 HTTP 延迟、限量下载、服务入口检查、比较导出与历史回看；**网页可达不代表登录、播放或模型业务可用**。

> [!IMPORTANT]
> **现行 Agent 接手与工程任务只看 Obsidian（2026-10-12 起）：**
>
> `03-项目与工程/机场/Clash SpeedTest Pro/README.md` → `03-项目与工程/机场/Clash SpeedTest Pro/Handoff.md`
>
> 决定：同目录 `Decisions.md`；真实证据：`状态与证据.md`；文件（ZIP、截图、构建和长日志）：既有 Google Drive 项目目录。
>
> **不要用本仓库的历史迁移说明、已关闭 [Issue #32](https://github.com/suifracti/clash-speedtest/issues/32)、旧 README、旧 PR 评论或截图作为现在的执行授权。** GitHub 管源码、提交与历史证据；Obsidian 管 CST 的当前任务和交接。本仓库的其他说明若与 Obsidian Handoff 冲突，以后者及本机真实工作树为准。

## 当前 Mac 开发代码（2026-10-13）

**版本核对发现的三项旧本地遗漏现已最小迁回并同步原开发分支：IPOK 后端/缓存预算、VLESS pcs/vcn 特殊 TLS、Mac 订阅系统代理识别。** 原 82 项服务规则内容与顺序保留；新增 IPOK 两项按需执行。迁回经过离线回归，不升级为上游实网已通过。详见 [版本与功能保留检查](docs/version-consistency.md)。

用户决定每次源码修改验证后同步 GitHub，以仓库中的明确分支与提交为准。当前 Mac 候选在 [原开发分支](https://github.com/suifracti/clash-speedtest/tree/codex/cst-w12i-integration-20261011-6aa2079)：完整节点配置作用域、队列接纳与保存、Grok 挑战分类、首页质量快照刷新，以及按需延迟/服务的私有物理出口保护。保留现有历史与功能；Windows 本轮不做。

物理绑定成功与真实无 TUN 转发分别记录；没有独立包流向证据时仍标为未验证。HTTP 响应时延不等于业务可用，超时不自动认定永久节点故障。源码同步不代表正式发布或全部原生验收完成，长日志、截图、构建和私有数据不提交仓库。

## 2026-10-12 已知进展（历史截点，不是今日运行确认）

- 17 个原有 PR 已合入 `main`，整合业务代码基线 `89d7b248`；[Linux/macOS/Windows 集成 CI](https://github.com/suifracti/clash-speedtest/actions/runs/38228748839) 已通过。**本文件以后的文档提交会改变 `main` HEAD，实际版本以 Git 与本机现场核对为准。**
- 原有 Mac Wails 单节点报告：六站、5 MiB 下载、公共服务探测、历史保存及重开曾通过；这不证明所有节点/协议可用。
- 2026-10-12 全节点报告：41 个节点执行六站检测、246 样本中 226 成功，另 3 个配置解析歧义未执行；下载 12 完整、1 有效部分、3 连接失败，后续 28 未执行且存在“启动尚待确认”。**全节点闭环尚未通过**。报告来自执行方，原始 ZIP 未由本次仓库文档更新独立核验。
- 目前待修主线：配置别名与完整 revision → Wails/Web 启动拒绝分类及队列继续 → Grok 非 2xx 挑战页判断 → 紧凑 UI / 同一候选验收。**以 Obsidian Handoff 的最新顺序和完成状态为准，不能从本列表机械重开任务。**
- Windows 旧 v11 库的安全升级与真实跨机场测试仍有独立验证边界；CI 绿色并不等于所有原生平台与历史数据库已验收。

## 开发与运行范围

- **先确认哪个工程和哪个 App 真正在用。** Sol A / 本机执行者先核对已有工作树、`git status`、`HEAD`、未提交差异、运行程序和隔离数据根；不能因为 GitHub 最新就覆盖本地源码或生产 SQLite/WAL。
- 日常使用目标是**现有 Wails 原生桌面 App**；仓库还保留本地 Web 辅助入口、原 CLI 与历史 GUI/TUI。旧版 [Mac 迁移文档](MAC_HANDOFF.md) 的“先 Web、无需桌面壳”是 2026-10-05/07 期间的安排，**不是当前日用路线**。
- 技术依赖及锁定版本以 `go.mod`、`go.sum`、`frontend/package-lock.json` 为准。需要调试或重建时只在明确的工作树和**隔离**数据目录操作；前端产物重新构建后才能运行匹配的嵌入式 Go App。不要自行恢复旧定时任务或全机场网络检测。
- 测速会消耗机场流量；默认沿用现有有界下载、取消和状态/保存分离。**新的实网节点、流量、付费模型、VPN/TUN 修改和生产数据库迁移必须有相应明确许可**，不因为可运行或 CI 通过而自动执行。
- 禁止将订阅 URL、口令、Token、生产数据库/WAL、未脱敏节点日志或私有快照上传公开 GitHub。衍生项目保留上游 [faceair/clash-speedtest](https://github.com/faceair/clash-speedtest) 的 GPL-3.0 许可要求。

## 历史和资料

- [Issue #32｜已关闭的合并、缺陷与旧计划归档](https://github.com/suifracti/clash-speedtest/issues/32)（仅历史）。
- [MAC_HANDOFF.md](MAC_HANDOFF.md) / [2026-10-07 暂停记录](docs/development-paused-20261007.md) / [Mac 迁移来源记录](docs/migration-source-map.md)（仅按其日期查证，不指导当前 Agent 接管）。
- 2026-10-07 脱敏历史截图：[节点与下载](docs/images/node-download-history.jpg) · [六站结果](docs/images/latency-six-sites.jpg) · [暂停的周期采样配置](docs/images/periodic-paused.jpg)。旧截图不代表当前原生窗口或节点状态。

**当前唯一操作入口仍是 Obsidian Handoff；本 README 只定位项目与提醒历史边界。**

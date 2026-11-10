# Clash SpeedTest Pro 项目补充

继承：先读用户已授权的 Obsidian Vault 根 `AGENTS.md`（通过现有连接器或客户端入口定位），按其中的层级章节接手。本文件只维护项目身份、主源和补充要求；总规则不可达时说明缺口，依据当前用户指令与已读项目资料继续可做部分。

## 1 项目身份与主源

仓库 `suifracti/clash-speedtest`，协作分支 main。技术栈沿用 Go/Mihomo/Wails/Vue/SQLite。

- 目标与顺序：[roadmap-round3](docs/roadmap-round3.md)。
- 当前进度与完成证据：[backend-round3-progress](docs/backend-round3-progress.md)。
- 技术经验与覆盖：[PROJECT_MEMORY](docs/PROJECT_MEMORY.md)。
- Vault README 导航，《项目总览》提供规划、经验与接续摘要；Handoff/Decisions/状态与证据保留阶段材料。
- 版本和功能接线核对：[version-consistency](docs/version-consistency.md)。

## 2 产品与数据补充

保留订阅账户隔离、完整节点 identity/revision、SQLite/WAL 历史、取消与保存状态。未知保持未知；HTTP 可达不代表服务业务成功。

公开材料使用代号/泛称，真实机场、节点、入口、配额、账号状态、订阅 URL、token、生产历史和私人截图留既有私人位置。全机场大测、持续监测、付费模型调用、VPN/TUN 修改、生产库迁移和凭据覆盖按对应授权。

## 3 Mac 构建与附件补充

真实 Mac 候选统一通过 `scripts/build-mac-candidate.sh`：前端构建 → production core（macOS 12）→ `scripts/package-mac-candidate.py`，任一步失败即停。单独 packaging 只包装已有核心，不能替代新源码的 core 构建。

`scripts/candidate_audit.py` 检查源码及 manifest；保留能力条目，未知旧二进制不猜 SHA。新截图、构建与长日志留本地，Drive 只保留既有历史附件；退出旧版、切换入口与回滚沿原授权方式。

## 4 分工与同步补充

现行项目分工沿 GPT 实施前后端、Claude 负责方案/UI 决定与把关，用户本次明确安排可覆盖。子代理设置继承总规则，旧 Luna-only 不再适用。

普通源码和文档在已有授权范围完成相关检查后同步；main 合并、发版、实网扩测及正式运行版切换分别按相应授权。关闭的 Issue #32、MAC_HANDOFF 和旧暂停/监测安排仅作历史。

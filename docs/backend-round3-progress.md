# 第三轮进度

执行方式与范围以 [roadmap-round3.md](roadmap-round3.md) 顶部两节为准；前后端由 GPT 实施，Claude 负责方案与把关。测试使用本地服务和临时数据，不代表真实节点检测。

## 第 0 步：CI
- 修正 IPPure 本地证书测试，加入 `go vet`。
- 本地检查及三平台 CI 已通过。

## B1.1：SQLite
- 每个连接通过 DSN 设置 pragma；修正 Windows 文件 URI。
- 只读孤儿报告：`go run ./cmd/history-orphans -db PATH`，不自动修复。
- 本地测试和三平台 CI 已通过。

## B1.2：历史清理
- 清理加入事务并上报错误，处理终止记录的空 `finished_at`。
- 保留进行中记录；本地测试和 CI 已通过，未对用户历史执行清理。

## B1.3：每日用量采集
- 网络抓取移到 profile 锁外；取消后收尾为 `refresh.state="cancelled"`。
- 本地测试和 CI 已通过。

## B2.1：用量头
- 接受小数、科学计数和重复字段；缺失计数保留未知。
- 字段：`usage.upload/download` 可为 null，`usage.raw_header` 保留原头；历史部分数据标为 `partial`。
- Go 检查、357 项前端测试和构建已通过。

## B2.2：备用 UA
- 缺少有效用量时补抓一次响应头，保留主请求节点内容。
- 既有字段：`usage.fallback`、`fetch.usage_fallback`；本地检查和 CI 已通过，此链路不再加保护。

## B2.3：订阅 UA
- 字段 `user_agent`；Web `PUT /api/airports/{id}/subscriptions/{subId}/user-agent`，Wails `SetSubscriptionUserAgent`。
- 空字符串恢复默认；本地检查和 CI 已通过。

## B2.4：单账户用量
- 复用现有抓取，只保存账户用量；台账分别提供“刷新用量”和“更新节点”。
- Web `POST /api/airports/{id}/subscriptions/{subId}/usage/refresh`，Wails `RefreshSubscriptionUsage`，返回 `{subscription, observation}`。
- 按新工作方式移除本项新增的复核、冷却检查、回退及空正文分支；节点缓存与更新时间不变。
- 验证：主路径与抓取失败；`go vet`、`go test ./...`、358 项前端测试及构建通过。

## B3.1：下次监测
- 字段 `MonitorJob.next_run_at`；监测卡片原参数行显示下次运行，暂停或停止显示“—”。
- 验证：启动与暂停主路径；`go vet`、`go test ./...`、359 项前端测试及构建通过。

## B3.2：库版本与备份时间
- `ProfileSetupDTO.schema_version/last_backup_at` 接入数据页；成功导出后刷新显示，失败保留上次时间。
- 最近备份时间保存在数据目录 `last-backup-at`；界面沿用原存储信息列表。
- 验证：备份成功与写入失败；`go vet`、`go test ./...`、359 项前端测试及构建通过。

## B3.3：上次同条件记录
- Web `GET /api/workbench/{download|public-service}-tests/{attempt_id}/previous`；Wails `PreviousWorkbenchDownloadAttempt/PreviousWorkbenchPublicServiceAttempt`。
- 匹配账户、节点、配置、来源及规则快照关键字段；无匹配返回 null，节点记录详情展示“和上次比”。
- 验证：下载/服务匹配与无匹配、前端数值/结论和读取失败；`go vet`、`go test ./...`、361 项前端测试及构建通过。

## F3.1：订阅节点变化
- 成功更新保留 `node_versions.previous/current`，`node_changes.added/removed/renamed` 接入台账和来源带；点击查看名单。
- Web `GET /api/airports/{id}/subscriptions/{subId}/changes`，Wails `GetSubscriptionNodeChanges`；公告排除，首次无旧列表只建立基线。
- 验证：新增/下线/改名及刷新失败保留上一版；`go vet`、`go test ./...`、362 项前端测试及构建通过。

## F2：体验细节
- 侧栏支持搜索/折叠；健康点按最近已保存延迟批次：至少半数已检测节点无响应为红，部分为黄，无记录灰。
- 空状态区分无节点/筛选隐藏，清除搜索、地区、账户、协议和质量筛选；检查器其他项目显示最近测量时间。
- UI 沿用现有按钮和摘要行；验证主路径与历史读取失败，`go vet`、`go test ./...`、364 项前端测试及构建通过。

## F3.3：推荐页
- “现在用哪个”按流媒体/AI/下载/日常展示最多 3 个候选，列出具体规则/站点、检测次数和时间；不足则不推荐。
- 现有 policy 输出是监测任务单一切换建议，不能直接用于四用途；本页复用 `readQuality` 与现有可比条件、服务结论。
- 复用 Clash 导出接口及 `group_name`，只读近 7 天本地历史（本轮复核调整）；验证候选/缺失及新失败证据，初版 365 项前端测试及构建通过。

## F3.2：机场体检
- 机场页增加“体检”：当前节点近 30 天的节点轮次可用率、失败类型、每日平均响应延迟。
- 本机设置 `airport_monthly_prices` 保存月费，月费÷近 30 日已观测 GB 估算成本；未知/缺失不补零。
- 复用历史和用量接口，界面明确统计口径；验证正常轮次与缺失历史，`go vet`、`go test ./...`、366 项前端测试及构建通过。

## F3.4：本机通知
- 设置 `local_notifications.{enabled,failure_percent,remaining_percent,expiry_days}`，默认关闭；桌面绑定 `EnableLocalNotifications/SendLocalNotification` 复用 Wails 原生通知。
- 应用运行时读取已保存批次/已结束监测轮次与套餐，按阈值提醒失败、低余量和到期；相同告警本机去重，恢复后可再提醒。
- 验证判断/去重与投递失败；`go vet`、`go test ./...`、368 项前端测试及构建通过。未触发系统授权或实机通知，未启用用户设置。

## B4：缩减版稳定性
- 检测队列/下载/服务、订阅刷新、监测调度与探测工作器捕获内部异常并记录失败；用量采集沿用已有异常收尾，不扩展。
- Web OAuth 仅加去重；`ReadHeaderTimeout=5s`，测速配置 HTTP 客户端超时 `60s`。
- 验证异常后终态与再次检测、监测未执行记录；`go vet`、`go test ./...` 通过，既有主路径测试保留。

## CI 实际报错修正
- macOS race 作业的旧 20ms 预算测试偶发在准备阶段前超时；改为在指定阶段主动取消，保留不发探测和阶段断言。
- 删除该测试中的睡眠与机器速度依赖；`go vet`、`go test ./...` 通过。
- Linux 另报旧恢复测试在调度槽释放前手动触发；测试只对该正常忙状态短暂重试，保留原断言，本地 Go 检查通过。

## B5：路径 token 遮罩
- `RedactURL` 遮住 link/sub/subscribe/token 路径中的凭据，以及长不透明路径片段；不增加鉴权或限速。
- 验证常见 token、编码斜线及普通路径；`go vet`、`go test ./...` 通过。

## B6：删除旧 GUI 实现
- 删除 `gui/server.go`、`gui/manager.go` 及只覆盖旧服务器/广播器的测试；CLI 启动器和现有导出功能保留。
- 验证引用与 `go vet`、`go test ./...` 通过；未拆文件、合并测试或加自动清理。

## 收尾自检
- 补上 `completed_with_issues/completed_with_save_failures` 批次的健康点与通知，避免失败批次被过滤。
- `go vet`、`go test ./...`、368 项前端测试及构建通过；合成页面实看：首页、推荐、体检、监测、数据，12 个推荐候选，页面错误 0。
- 截图留本机 `.ui-preview/round3-*.png`；未读取真实订阅、未运行真实检测、未替换当前 App；后续 CI 不阻塞推进。

## 复核：请求量与推荐时间范围
- 体检只读取当前机场的 `nodes.value`，切换机场重新读取；两页共用 `readQualityBatch`，同时最多读取 6 个节点。
- 通知只在设置变化（含初次加载）及每 5 分钟检查，移除机场和批次变化监听；推荐范围与页头改为近 7 天，候选最近时间保留。
- 验证：四项各一个新增主路径测试；`go vet`、`go test ./...`、372 项前端测试及构建通过。未换 App，等用户看过提交。

## 修复：历史配置记录被漏读
- 原因：两页只查当前 revision；记录页按批次/历史配置可见，工作台当前配置仍为空。SG04 同身份旧配置可读 7 条延迟、至少 100 条服务，时间与 complete 非主因。
- 批量读取复用 `ListNodeHistoryRevisions` 和原分页接口，保留账户/节点身份范围及 6 并发；页面明确包含历史配置，查看记录打开对应 revision。
- SG04 真实副本：读取 70 条延迟、267 条服务；流媒体/AI 各 1 候选、日常 3 个；良心云 56/70 轮、80%，4 天曲线。副本实页已看到，原库只读未改。
- 验证：`go vet`、`go test ./...`、376 项前端测试与构建通过；其余历史节点身份已变化，未按名字合并。

## 修复：通知授权和提示
- 勾选通知时先调用现有 `requestAuthorization`，拒绝/失败保持关闭，提示“系统未允许 CST 发送通知”；保存不重复授权。
- 增加桌面 `OpenSystemNotificationSettings` 打开 macOS 通知设置；不使用 osascript；复选框改为 16px 与文字同行。
- 验证授权成功/拒绝两条路径、设置入口及上述全量检查；原通知设置保持关闭。

## UI：推荐和体检空状态
- 推荐全空仅一张卡及“去工作台检测”；部分用途缺失缩成一行并说明原因，隐藏该项导出按钮。
- 体检无轮次仅提示和已观测用量；有记录保留统计、失败类型和曲线，月费输入放到成本估算下。
- 真实副本已看到良心云有数据、飞鸟云无对应记录；节点变化与下次运行按用户决定暂不处理。

## UI：侧栏和顶部来源条联动
- 单机场保留订阅选择，按钮按“当前订阅 / 此机场 / 选择范围”；全部机场按机场合计额度、今日用量及刷新状态，“只看”同步侧栏。
- 全部模式仅“全部刷新”，近三日合计保留“部分记录”；失败原因集中在展开名单，行内悬停可看原因。无新增接口或字段。
- 验证：只新增汇总行与单机场刷新范围两个测试；`go vet`、`go test ./...`、378 项前端测试与构建通过，SG04 副本实页查看两种模式及缓存失败。

## 待定
- 暂无新增保护或扩展项。

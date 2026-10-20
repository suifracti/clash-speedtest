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
- 复用 Clash 导出接口及 `group_name`，只读近 24 小时本地历史；验证候选/缺失及新失败证据，`go vet`、`go test ./...`、365 项前端测试及构建通过。

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

## 待定
- 暂无新增保护或扩展项。

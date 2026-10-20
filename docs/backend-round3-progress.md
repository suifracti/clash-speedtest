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

## 待定
- 暂无新增保护或扩展项。

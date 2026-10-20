# CST 优化路线（第三轮）

这份路线由 Claude 自查整个仓库后整理，用户已批准。**2026-10-20 起分工调整：前端和后端的实施全部由 GPT 做；Claude 只负责方案、UI 决定和把关（防止做歪、做复杂）。** 下面 F1–F3 原写“Claude 做”的部分，现在也由 GPT 实施。

## 工作方式（每次开工必读）

这是个人用的本机工具，不是对外服务。目标是“功能对、用户看得见”，不是“任何边角都防住”。

1. **只做清单里写的事。** 清单没写的保护、诊断字段、冷却、并发上限、兼容分支，一律不加。觉得有必要，先在进度文档“待定”里写一句，等用户或 Claude 拍板。
2. **测试只覆盖主路径和一个关键失败路径。** 每项新增测试代码不超过实现代码的量。不为假想的竞态、时序、跨平台边角写专门测试（Windows 路径这种真实 CI 报错除外）。
3. **一项做完就推，不用等完整 CI 矩阵跑完再开下一项。** 本地 `go vet`、`go test ./...`（改到前端就加 `npm test`）过了就推。CI 红了再回头修。
4. **同一个问题修两次还不通，就停下。** 把这段流程从头按最简单的方式重写，不要继续打补丁。
5. **进度文档每项最多 5 行：** 改了什么、新接口/字段名、怎么验证。不写推理过程。
6. **UI 改动先按 Claude 给的方案做。** 方案没覆盖的界面细节，选最简单的一种，做完在进度里写一句，由 Claude 看图再定。

## 剩余事项的取舍（2026-10-20 Claude 复核）

已完成：第 0 步、B1.1–B1.3、B2.1–B2.3。B2 这几项做得对，但已经偏重（同源冷却、来源指纹复核、UA 过期复核等），**以后不要再往用量链路上加防护**。

接下来按这个顺序：
1. **B2.4** 单账户只刷新用量：复用现有抓取，不新增保护逻辑。
2. **B3** 三个接口：`next_run_at`、`schema_version`/`last_backup_at`、上次同条件的下载/服务记录。做完直接接前端（监测卡片“下次运行”、数据页“库版本/最近备份”、记录页“和上次比”）。
3. **F3.1 订阅变化提醒**（后端存上一版节点列表 + diff 接口，前端台账显示“新增/下线/改名”）。
4. **F2 体验项**（见下文）。
5. **F3.3 推荐页 → F3.2 体检报告 → F3.4 本机通知。**

降级或删除：
- **B4**：只给检测队列、订阅刷新、用量采集、监测调度这几个主 worker 加 recover，其余不动。`context.Background()` 不做全面替换。Antigravity 只加去重。`ReadHeaderTimeout` 和 `speedtester.go:86` 的超时照做（各几行）。
- **B5**：只做 `RedactURL` 遮路径 token。其余（常量时间比较、限速、`/url` 访问控制）先不做。
- **B6**：只删 `gui/server.go`、`gui/manager.go`。拆文件、注入时钟、合并测试都不做。自动清理旧数据放到 F3 之后再议。
- **F1.2–F1.5**（CSS 收尾、拆 `workspace.ts`、懒加载、虚拟滚动）：先不做。哪页真的卡了再做那一页。


## 背景
第二轮界面焕新已全部完成（`8cdb5fa` 及后续修正 `202b411`、`5e3ab62`、`87e9141`、`cd80897`）。用户要求自查整个项目，给出一套优化路线，包括用户自己没想到的方向；前后端分开写。主要分工：Claude 负责前端，GPT（6.1 sol）负责后端，两边都可以改前端，以 GitHub main 为准。

我这次自查发现了几件该先处理的事：
- **GitHub CI 一直是红的。** 前端已有的 6 个失败让每次 push 的 `npm test` 都失败。今天上午 GPT 推了 12 个前端 UI 提交（订阅卡片、搜索框），又多弄坏 2 个测试，现在共 8 个失败。
- **前端两边都会改。** 用户已确认 GPT 和 Claude 都可以改前端。协作规则：开工前 `git fetch` 并看对方最近的提交；每次只改一小块就推；推之前本地测试必须全绿；不回退对方的改动，有冲突时在提交说明里写清楚。
- **用量读不到，多半不是 UA 的问题。** 后端默认 UA 已经是 `clash-verge/v2.5.8`（`core/profiles/fetch.go:30`）。更可能是 `Subscription-Userinfo` 解析太严：`core/profiles/usage.go:37-48` 遇到浮点数（如 `1.07E+11`）、重复字段或缺 upload，就把整条头丢掉；另外也没有换 UA 重试。

---

## 第 0 步（先做，前后端各一半）：把 CI 修绿
- **前端（Claude）**：修好 8 个失败测试。
  - `plan-queue-interactions` 5 个：测试目录补上信誉规则，或把计划改成没有信誉规则时不强制。
  - `legacy-target-visibility` 1 个：断言改成按现行 4 站。
  - `home-first-screen`：GPT 改成镜像订阅只算一次后，节点数 64→32，按新口径改断言。
  - `home-subscription-overview`：GPT 改了额度文案，跟着更新。
  - 只改测试夹具和断言，不改统计口径。如果某个失败是真 bug，就先修代码。
- **后端（GPT）**：`go vet` 是干净的。加 CI 门槛，main 红了不再继续推。

## 后端路线（交给 GPT）
按“影响真实数据 → 用户能感知 → 内部整洁”的顺序排。

### B1 数据正确性（最优先）
1. **SQLite 连接池 pragma 只作用于一个连接。** `core/history/db.go:274-285` 用 `db.Exec` 设置 `foreign_keys/busy_timeout/synchronous`，其它连接上 `ON DELETE CASCADE` 不生效，删除会留下孤儿行。改成 DSN 里写 `_pragma=`（modernc 支持），或 `SetMaxOpenConns(1)` 写连接加读池。另做一次孤儿行体检（只读报告，不自动修）。
2. **`CleanupWorkbenchHistory` 有缺陷。** 在 `db.go:2853`：没有事务、吞掉错误、`finished_at IS NULL` 的中断记录永远清不掉。改成事务加错误上报。
3. **用量采集 `collectSubscriptionUsage` 两个问题。** 在 `application/subscription_usage.go:165`：取消时状态卡在 running；持有 `profileWriteMu` 时串行做网络请求（每个最长 60s）。改成先并发抓取，再短锁写入；取消也要收尾。

### B2 用量检测（用户今天提的）
1. `usage.go` 宽松解析：接受浮点和科学计数，upload/download 缺一个就记为未知，不再整条丢弃，并保留原始头用于诊断。
2. 拿不到用量头时，用 `clash.meta` / `mihomo` UA 补抓一次，只取响应头，不覆盖节点缓存。
3. 每个订阅可单独设 UA（订阅表单加一个字段，前端配合）。
4. 新增“只刷新某账户用量”接口，前端台账的“刷新用量”和“更新节点”就能分开。

### B3 前端等着的接口
- `MonitorJob.next_run_at`：调度器记录下一次 tick，见 `core/monitor/scheduler.go:514`。
- `ProfileSetupDTO` 增加 `schema_version`（现为 12）和 `last_backup_at`。导出备份时记录时间，见 `application/data_backup.go`。
- 按条件找“上一次同条件”的下载/服务记录：给 `rule_snapshot_json` 的关键字段建可筛选列，或做一个专门查询，前端“和上次比”就能覆盖下载和服务。

### B4 稳定性
- 42 处后台 goroutine 只有 1 处 recover。给 worker 统一加 recover，记录日志并把任务标为失败，避免一个 panic 让整个 App 退出。
- Antigravity 登录流程没有 context 和去重（`adapter/web/server.go:891`）。
- `http.Server` 补 `ReadHeaderTimeout`。`speedtester.go:86` 的 `http.DefaultClient` 没有超时，也没有正文上限。
- 126 处 `context.Background()`，先改用户能取消的路径（刷新订阅、抓取）。

### B5 隐私与安全
- `RedactURL` 不遮路径里的 token（如 `/link/<token>`），要补上。
- `/api/airports/{id}/url` 在本机模式下任何本地进程都能读到原始订阅地址。改成桌面端走绑定、Web 模式要求令牌。
- 远程模式的密码比较改成常量时间，并加失败限速。

### B6 整理（不急）
- 删除未使用的 `gui/server.go` 和 `gui/manager.go`（约 1.7k 行，与 `application/` 重复）。
- 拆分 god-file：`application/service.go`（3k 行）、`adapter/web/server.go`（114 个路由）。
- 自动保留策略：定期清理 `subscription_usage_snapshots`、`measurement_rounds`、`ip_reputation_cache`，加 WAL checkpoint。
- 合并审计式命名的测试；减少 36 处 `time.Sleep`，改用注入时钟。

## 前端路线（Claude）
### F1 工程健康
1. 第 0 步：CI 修绿。
2. **样式收尾**：`app.css`(34K)、`closeout-ui.css`、`observatory.css`、`native-ui.css`、`secondary-observatory.css` 里剩下的旧规则逐页迁到 `design/*`。每次删除都先出待删清单，再用元素位置指纹核对。目标是只剩 tokens、base、shell 加各页自己的样式。
3. **`workspace.ts`（43K，单行巨函数）按职责拆分**：节点/范围、检测队列与恢复、历史缓存、排序快照。对外接口 `useWorkspace()` 不变。
4. **按页懒加载**：机场、记录、监测、设置、数据页改用 `defineAsyncComponent`，首屏 JS 375K 可以先分出几块。
5. **表格虚拟滚动**：每页选“全部”、节点很多时，只渲染可见行。

### F2 体验（本次自查发现的细节）
- 机场多了以后，侧栏要有搜索和折叠；每个机场显示健康小点（最近一轮有无大面积失败）。
- 工作台空状态区分“被筛选挡住”和“这个机场没节点”，并给一键清除。
- 检查器“其他项目”摘要加最近测量时间，免得把旧结论当成新的。
- 记录页把延迟以外的“和上次比”接上（等 B3）。
- 监测卡片显示下次运行（等 B3）。
- 数据页显示库版本和最近备份（等 B3）。

### F3 新功能（用户已选的四项，按依赖顺序）
1. **订阅变化提醒**：每次刷新后对比节点的增删和改名，在机场台账和来源带显示“新增 3 / 下线 2 / 改名 1”，点开能看到名单。后端（GPT）要保留上一版节点列表并提供 diff 接口；前端负责展示。
2. **机场体检报告**：机场页新增“体检”标签，展示每个机场近 30 天的可用率（按轮次）、失败类型分布、平均延迟走势，以及每 GB 成本（用量 + 用户填写的价格，价格只存在本机设置里）。先用现有的历史和用量接口；如果读取太慢，再请 GPT 加汇总接口。
3. **“现在用哪个”推荐页**：按用途（流媒体 / AI / 下载 / 日常）各给 3 个候选，每条附依据（哪些站点、几次、什么时间）。证据不够就明确写“证据不足”，不硬推。能一键把候选导出成 Clash 配置组，复用 `exportSelection`。后端已有 `core/policy/recommend.go`，需要 GPT 先确认它的输出可以直接给前端用。
4. **macOS 本机通知**：节点大面积失败、套餐剩余低于阈值、7 天内到期时通知。阈值在偏好设置里，默认关闭。发送走桌面端绑定（GPT 负责 Wails 侧），前端负责设置界面和去重显示。

## 执行顺序
第 0 步（CI 修绿）→ 前端 F1.1–F1.2 与后端 B1、B2 同时进行 → B3 接口到位后做 F2 → F3 依次做 1→2→3→4（3 和 4 要等 GPT 那边的接口）。

## 执行方式
- 每一项都是“改 → vitest / vue-tsc / build（后端 go test）→ 内置浏览器用 SG04 副本实看 → fetch → push main”。改完前端再打 Mac 包，换包前先问用户。
- 不发起真实检测，不碰原 SG04 数据根；缺后端接口的地方先标“暂无数据来源”。
- 交接：后端清单整理成一份给 GPT 的说明，写到仓库 `docs/roadmap-round3.md`。Obsidian 写入需要另外授权。

## 验证
- CI：GitHub Actions 的 `CI & Desktop Build Matrix` 变绿。这是第 0 步的完成标志。
- 前端每步：`npm test` 全绿（不再有已知失败）、`npx vue-tsc -b`、`npm run build`；内置浏览器 1440/800/390、浅色和深色各看一次。
- 后端由 GPT 自测：`go vet ./...`、`go test -race ./...`。用量解析补上浮点、缺字段、重复字段的单元测试。

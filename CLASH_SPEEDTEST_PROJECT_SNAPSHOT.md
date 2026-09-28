# Clash SpeedTest Project Snapshot

> 审计日期：2026-09-22（Asia/Shanghai）
>
> 审计目的：为下一位高级规划模型提供一份以当前 main 与实际本机 runtime 为准的项目事实收口。
>
> 本次只新增本 Snapshot 文件；没有修改代码、UI、配置、PR、分支历史或运行状态。

## 1. Executive Summary

Clash SpeedTest 已经不是上游 CLI 的简单包装，而是一个以 Wails/Vue/TypeScript 为桌面入口、同时保留 Web fallback 与 CLI 的节点事实工具。当前 main 已包含：

- 真实机场/订阅管理、订阅缓存解析和节点选项；
- legacy 批量测速与普通 JSON 历史；
- SQLite monitor raw sample、scheduler、cursor pagination、stats、facets、retention；
- Monitor 任务 UI、Timeline、Inspector、live refresh、catch-up 与 incomplete/gap continuation；
- 稳定节点身份的单节点 latency workbench、attempt_id、异步历史保存和 stale response 防护；
- Mihomo External Controller adapter 与只读 Evidence → Recommendation 路径；
- PR #10 恢复的正式原型 UI，以及当前 main 中的自绘 UiSelect。

当前仍不是完整的 Clash SpeedTest Pro 产品。正式用户工作流目前最可靠的是“订阅缓存 → 单节点延迟测试 → 原始样本/历史 → 持续监测 → Timeline/统计”。吞吐、上传、Antigravity/服务可用性在现代工作台中还没有正式接线；Monitor 任务配置只在进程内存中，重启后不恢复；Controller 与 Auto 的执行能力没有接到正式用户 UI/Monitor 调度；profile 文件依赖进程 cwd，跨 worktree、快捷方式和发行包启动时可能导致机场/节点为空。

本机当前没有运行中的 Clash SpeedTest 进程，因此本次不能把某一次 HTTP 空列表直接归因给当前 main。实际核对到的现场是：

- 当前 checkout 是旧的 codex/monitor-job-ui 分支，HEAD 为 4096fb8，落后 main 七个提交；
- 本地 main、origin/main 和 GitHub 远端 main 都是 58d036e220d873f900f755fdc47b2f6850a0b9b3；
- 当前 cwd 下存在 2 条 ignored profile 记录和 2 个 ignored cache 文件；
- 历史库实际在 C:\Users\Administrator\.clash-speedtest\history，SQLite 中有 25 个 monitor_runs、291 个 monitor_samples，workbench_latency_tests 和 workbench_latency_samples 均为 0；
- 当前没有监听此前使用过的 Web 端口，也没有可核对的运行进程 cwd。

总体判定：当前 main 是“真实后端与部分正式 UI 已形成闭环，但仍处于可用的半成品/阶段性产品”状态；不能把代码组件数量、旧分支 UI、历史文档或旧 soak 结论当作完整产品完成。

### 状态词说明

- VERIFIED：当前 main 中存在完整可追踪路径，并有针对性测试、真实 adapter/runtime 证据或可复核的生产调用链。
- PARTIAL：用户可见路径和 backend 都存在一部分，但覆盖范围、数据语义或 runtime 证据不完整。
- COMPONENT_READY：组件、API 或测试路径已经具备，但没有正式 UI 接线或没有正式生产调用者。
- EXPERIMENTAL：存在实验性实现或实验性证据，不能当作稳定产品契约。
- SUSPECT：已有路径在特定身份/数据组合下可能产生错误归属，不能作为新功能的事实来源。
- WRONG：历史结论或旧口径已被当前证据推翻。
- UNFINISHED：当前实现明确缺少用户预期所需能力。
- UNKNOWN：本次只读审计没有足够证据判定。

## 2. 当前 Git / Runtime 基线

### 2.1 权威 Git 事实

| 项目 | 当前事实 |
| --- | --- |
| 仓库 | suifracti/clash-speedtest |
| 当前工作目录 | D:\ai\work\speedtest |
| 当前 branch | codex/monitor-job-ui |
| 当前 checkout HEAD | 4096fb852ceb721fd41155bdc915685e79ee1869 |
| 当前 branch upstream | origin/codex/monitor-job-ui 已不存在 |
| 本地 main | 58d036e220d873f900f755fdc47b2f6850a0b9b3 |
| origin/main | 58d036e220d873f900f755fdc47b2f6850a0b9b3 |
| GitHub 远端 main | 58d036e220d873f900f755fdc47b2f6850a0b9b3 |
| main 与当前 checkout | 当前 checkout 比 main 落后 7 个提交，无分叉 |
| 当前 working tree | Git-tracked 文件 clean；ignored runtime data 和旧 exe 存在 |
| open PR | 0 |
| 已合并 PR | #1–#10 全部 MERGED |

当前 checkout 的 4096fb8 不是当前 main。它是 PR #8 的 head，不应被当成 PR #10 之后的正式 UI 或当前产品 baseline。

### 2.2 main 是否包含 PR #10 原型 UI

已直接核对当前 main tree 和 GitHub PR 状态：

- main 的最后合并提交是 PR #10 merge commit 58d036e；
- main 中存在 frontend/src/components/workbench/LatencyWorkbench.vue；
- main 中存在 frontend/src/components/common/UiSelect.vue；
- main 的 frontend/src/App.vue 正式 import 并 mount LatencyWorkbench、MonitorJobsView 和 MonitorTimelineView；
- main 的 SourceScopeBar 正式显示“节点工作台 / 持续监测 / 历史记录”；
- PR #10 的 Linux Go Unit Tests、Windows Desktop Build、macOS Native App 三项 CI 均 SUCCESS；
- 当前 checkout 4096fb8 的 tree 没有 LatencyWorkbench.vue 和 UiSelect.vue。直接从当前根目录启动旧 checkout/旧 exe，看到旧 UI 不能证明 main 缺少 PR #10。

结论：PR #10 恢复的原型 UI 确实已经存在于当前 main；当前根目录的 branch/运行物可能显示旧版本，是环境启动错误，不是 main 事实。

### 2.3 本地 branches、worktrees 与误启动风险

当前能影响判断的本地引用与 worktree：

| 类型 | 路径/引用 | HEAD | 事实与风险 |
| --- | --- | --- | --- |
| 当前 worktree | D:\ai\work\speedtest | 4096fb8 | 旧 PR #8 branch；不是 main；最容易被误启动 |
| worktree | C:\Users\Administrator\.codex\worktrees\workbench-latency-persistence\speedtest | 2a5f64f3 | PR #9 branch；不是 main；保留旧阶段代码与另一份 ignored profile data |
| review worktree | D:\ai\work\_pr7_final_review | 14d5203c | PR #7 review detached worktree；不是 main |
| local branch | codex/restore-prototype-ui | b2f5139 | PR #10 head；没有当前 worktree |
| local branch | feature-monitor-timeline-ui | d22ebfc | PR #5 head；没有当前 worktree |
| local branch | fix-monitor-history-test-race | 6abec1c | PR #6 head；没有当前 worktree |
| local branch | refactor/wails-desktop-architecture | 1477115 | 早期架构 branch；没有当前 worktree |
| local ref | remotes/pr7head | 5e54464 | 历史 review ref；不是 main |
| main | refs/heads/main | 58d036e | 当前唯一产品事实基线，但当前没有单独 main worktree |

需要规划模型记住：本机有多个可执行旧 checkout，且 profile/cache 是 ignored 文件，不会自然随 Git branch 切换而成为一致数据集。启动路径、branch、exe 来源和 profile cwd 必须同时核对。

### 2.4 当前运行进程与端口

- 没有名为 clash-speedtest.exe 的运行进程；
- 没有可归属 Clash SpeedTest 的当前进程 cwd；
- 18473、18474、18765、34115 等此前使用过的端口当前没有监听；
- 当前根目录存在一个 clash-speedtest.exe，大小约 53 MB，最后写入时间为 2026-09-19；本次没有运行它，也没有把它的内容冒充为 main runtime；
- 因为当前没有进程，本次不能给出“当前运行实例读取了哪个 profile path”的运行时结论。

### 2.5 当前 profile 与 history 现场

| 数据 | 实际现场 |
| --- | --- |
| 当前 profile 文件 | D:\ai\work\speedtest\airports.json |
| 当前 profile 记录数 | 2；不在 Snapshot 中复制名称或 URL |
| 当前 cache 目录 | D:\ai\work\speedtest\airports-cache |
| 当前 cache 文件数 | 2 |
| Git 状态 | airports.json 和 airports-cache/ 被 .gitignore 忽略；main tree 不包含用户 profile 数据 |
| 实际 history 目录 | C:\Users\Administrator\.clash-speedtest\history |
| SQLite 主库 | history.db |
| SQLite sidecar | history.db-shm 存在；history.db-wal 当前为 0 字节 |
| legacy JSON history | 11 个 JSON 文件 |
| monitor_runs | 25 |
| monitor_samples | 291 |
| workbench_latency_tests | 0 |
| workbench_latency_samples | 0 |
| 其它已核对路径 | repo 内没有 history 数据目录；未发现 %LOCALAPPDATA%\clash-speedtest 或 %APPDATA%\clash-speedtest 下的对应 history 目录 |

### 2.6 cwd 漂移的真实结论

代码事实：

1. core/profiles.DefaultPaths() 使用 os.Getwd()；
2. GUI 和 Wails 启动时都把 profiles.DefaultPaths() 传给 AppService；
3. profile 数据读取自 <process cwd>/airports.json；
4. cache 读取自 <process cwd>/airports-cache/<profile id>.yaml；
5. history.DefaultHistoryDir() 则使用用户 home 下的 .clash-speedtest/history。

因此，“历史还在但机场/节点为空”与启动到没有 profile 文件的 branch/worktree/cwd 完全一致。当前工作目录有 profile/cache，PR #7 review worktree 没有 airports.json，PR #9 worktree 有另一份 ignored profile data；这已经证明不同 worktree 可以看到不同 profile 世界。

本次没有运行应用，所以不能声称此前某个空列表一定由哪一个进程产生。但 cwd 依赖不是猜测，而是当前 main 的明确实现；它是一个真实的产品/启动边界问题。当前没有稳定的用户数据目录、GUI profile path 设置或发行包迁移策略。

## 3. 产品目标

### 3.1 用户目标

产品目标是 Clash SpeedTest Pro v2.1：面向机场/订阅节点的测速、稳定性和可用性事实工具，而不是通用 SaaS dashboard。

用户应能：

- 管理多个机场/订阅并刷新节点；
- 从真实缓存中选择节点，比较 latency、jitter、packet loss、download、upload、服务可用性、出口 IP/地区/纯净度；
- 看到原始样本、失败原因、时间窗口、历史变化和复测结果；
- 建立 24/7 Monitor 任务，以多种 probe 持续写入 SQLite；
- 从真实 evidence 推导 Recommendation、样本充分性和 confidence；
- 在明确的安全边界和用户选择下，未来再接入 Mihomo External Controller 与 Auto。

### 3.2 当前产品原则

- 原始样本是事实来源，统计和推荐只能从原始样本派生；
- 没有数据时显示未测试、无记录或未知，不用 demo/插值/当前结果回填历史；
- 节点逻辑身份至少使用 profile_id + node_key；
- node_identity_key 用于端点/配置身份解释，不能单独替代 profile_id + node_key；
- display name 只用于显示，不是历史主键；
- Monitor 只采集和保存，不应隐式改变用户当前代理选择；
- Recommendation 当前只读，Auto 必须最后做并且 fail-closed。

## 4. 当前架构

### 4.1 调用链

CLI / Wails native / local Web
→ frontend/src 与 adapter/desktop、adapter/web
→ application.AppService
→ core/profiles、core/speedtester、core/history、core/monitor、core/policy、core/controller
→ Mihomo External Controller 或隔离的 Mihomo proxy client
→ local profile/cache、legacy JSON、SQLite history.db。

### 4.2 入口与适配器

- CLI：main.go 的 CLI/TUI 入口，支持 fast/download/full、latency/download/upload/antigravity、duration、rounds 等。
- Wails：Wails v2.16 native desktop；frontend/dist 通过 embed 进入 Go binary；Windows 使用 Mica，macOS 使用默认 title bar。
- Web fallback：本机 HTTP server 绑定 127.0.0.1，静态资源和 API 在同一 local server。
- frontend：Vue 3、Vite、TypeScript、Pinia；主入口 App.vue。
- API bridge：frontend/src/api/bridge.ts 在 Wails binding 与 HTTP API 间归一化；monitor.ts 处理 cursor/stats/facets 的时间与 wire DTO。

### 4.3 两条测速体系

1. Legacy speedtester path：以 airport_id、display name 和旧 TestConfig 为入口，支持批量/单节点、下载/上传/Antigravity，并把完整 RunNodeResult 保存到 legacy JSON。
2. Stable-identity workbench path：以 profile_id + node_key 为入口，后端重新解析 cache 中的 RawConfig，校验 node_identity_key/config_revision_key，执行单节点 latency，并以 attempt_id 写入 SQLite。

这两条路径目前并存，尚未统一成一个覆盖所有测试项目的历史模型。

### 4.4 Monitor 与 policy

- Monitor Scheduler、Runner 和 raw sample store 已形成生产调用链；
- Scheduler 在内存中持有 MonitorJob 和 monitorSchedulers；
- Runner 使用隔离 proxy client 做 HTTP/RTT probe，不调用 Controller.SelectNode；
- Application Evidence path 从当前 MonitorJob 的候选节点和 SQLite raw sample 构建 evidence；
- policy engine 能输出 stay、recommend_switch、insufficient_evidence、confidence 等解释；
- EvaluateAndAutoSwitch 组件方法存在，但当前 main 中没有 Monitor/UI/正式调度调用它。

## 5. 数据与持久化架构

### 5.1 Profile 与 cache

- 逻辑文件：airports.json；
- 订阅缓存：airports-cache/*.yaml；
- 当前默认位置：进程 cwd；
- profile 记录包含订阅 URL；当前 AirportDTO 和 AirportModal 会把 URL 送到 UI 并直接展示；
- cache 解析在后端完成，Monitor/Workbench UI 只提交 profile_id 与 node_key，不提交 raw_config、token 或密码。

### 5.2 Legacy history

默认位置为 C:\Users\Administrator\.clash-speedtest\history（泛化为用户 home 下的 .clash-speedtest/history）。

legacy JSON 保存：

- TestRun；
- airport_id / airport_name；
- RunNodeResult；
- latency samples；
- download/upload 汇总；
- Antigravity status/detail；
- IPInfo、Ping0、IPPure、StabilityInfo。

限制：

- RunNodeResult 的历史身份仍偏向 display name/airport；
- GetNodeTimeline(nodeName) 只按 ProxyName 匹配，跨订阅同名节点可能混合；
- TestSingle legacy path 返回结果，但不调用 legacy history save；
- 没有每一个测试项目的显式 untested/failed/not_available 状态，所以 download 0 不能可靠区分未选择、失败或有效 0。

### 5.3 SQLite

当前 schema 的主要表：

- monitor_runs；
- monitor_samples；
- workbench_latency_tests；
- workbench_latency_samples。

monitor_samples 包含 sample_id、run_id、node_key、node_identity_key、config_revision_key、profile_id、display_name_snapshot、probe_type、target、timestamp、success、latency、ttfb、error、exit IP/region 和 metadata。

workbench_latency_tests 以 attempt_id 为主键，包含 profile/node identity、测试项目、请求/开始/结束时间、状态、latency、jitter、packet loss 和样本计数；workbench_latency_samples 保存每条 latency raw sample。

SQLite 使用 WAL、busy_timeout、foreign_keys 和单写入锁；migration 是幂等的，但当前代码仍明确 TODO formal schema version framework。

### 5.4 Monitor Probe Set

- light：rtt；
- service：rtt、Cloudflare、Google、GitHub HTTP probes；
- heavy：rtt、Cloudflare 50 KB TTFB、Google；
- 当前没有把 download/upload 测速、Antigravity stability 或 IP purity 纳入 Monitor Probe Set。

### 5.5 稳定身份

- profile_id：订阅/机场逻辑身份；
- node_key：包含节点配置/凭据语义的逻辑节点键；
- node_identity_key：传输端点层身份；
- config_revision_key：配置/凭据版本；
- attempt_id：一次 workbench latency 执行与持久化的不可变关联 ID。

主查询、历史详情和 recommendation candidate 必须保留 profile_id + node_key 约束。

## 6. Feature Truth Matrix

下表以当前 main 的源码、当前本机数据和已存在的针对性 Evidence 为准。旧 Obsidian 记录只用于定位历史，不覆盖当前 main 事实。

### 6.1 基础能力

| 功能 | 用户预期 | UI | Backend/API | 正式 runtime 与数据来源 | Evidence / 限制 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| 机场/订阅管理 | 添加、编辑、删除、刷新订阅 | AirportModal 存在且接真实 API | /api/airports 与 Wails List/Create/Update/Delete/Refresh 存在 | 读写 cwd 下 airports.json 与 airports-cache | 代码路径和旧 UI evidence；当前没有从 main live 进程重新操作；URL 直接暴露 | PARTIAL |
| 配置缓存 | 订阅刷新后缓存节点，后续测试从 cache 读取 | 订阅列表显示 node_count、has_cache | FetchSubscription、WriteCache、HasCache、LoadProxies | cwd/airports-cache/*.yaml；当前有 2 个 cache 文件 | cache 真实存在；路径随 cwd 漂移，main 不携带用户数据 | PARTIAL |
| 节点列表 | 展示真实缓存节点并可选择 | Workbench/Monitor node options 使用真实缓存 | GET /api/airports/{id}/nodes、GET /api/monitor/nodes、Wails ListMonitorNodeOptions | backend 从 cache 解析 proxy，UI 收到 credential-free DTO | PR #8 UI evidence 验证过真实节点；当前无 live main 进程 | PARTIAL |
| 节点筛选 | 按订阅、名称、国家/类型、健康或排序筛选 | LatencyWorkbench search/profile/sort；Timeline 有 profile/node/probe/target filters | Monitor cursor/stats/filter 支持 profile、node、probe、target、time | 前端筛选工作台列表，Timeline 后端查询有条件 | 当前 main 源码与前端测试覆盖 | VERIFIED |
| 单节点测速 | 选择一个真实节点即时测速并看到结果 | 现代 UI只有 stable latency 单节点入口；legacy retest 组件仍存在 | /api/test/single 与 /api/workbench/latency-tests | legacy 入口按 airport_id + display name；modern 入口按 profile_id + node_key | legacy TestSingle 不入历史；modern latency 入 SQLite | PARTIAL |
| 批量测速 | 一次对多个节点运行测试 | legacy widgets 在 collapsed legacy-surface 中；当前正式工作台没有清晰的批量启动入口 | /api/test/batch、StartBatch、speedtester worker path | legacy RunNodeResult 后保存 JSON | backend 存在；现代 UI未迁移为稳定身份批量闭环 | PARTIAL |

### 6.2 测速指标

| 指标 | 用户预期 | UI | Backend/API | 正式 runtime 与数据来源 | Evidence / 限制 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| latency | 真实 RTT、样本、失败原因、历史曲线 | LatencyWorkbench 可真实测试、显示 raw samples 和图 | speedtester latency；workbench latency API；monitor rtt | legacy JSON、workbench SQLite、monitor SQLite 三条来源 | workbench PR #9 定向测试和 Web 本地真实 proxy evidence | VERIFIED |
| jitter | 由 latency samples 计算/显示稳定性 | legacy TelemetryGrid/Result DTO 有字段；现代 workbench 只把它作为 DTO 字段，不是主要 UI读数 | Result.Jitter、RunNodeResult.JitterMs、WorkbenchLatencyTest.JitterMs | legacy JSON / workbench SQLite | 后端字段和测试存在；Monitor raw sample 不保存独立 jitter | PARTIAL |
| packet loss | 看到丢包率、失败样本和状态 | legacy UI/DTO 可显示；现代 workbench主要显示成功/失败样本与 P50/P95 | Result.PacketLoss、workbench DTO、legacy JSON | legacy 和 workbench latency result | 真实计算存在；没有统一 monitor packet-loss metric | PARTIAL |
| download | 真实下载速度与失败原因 | modern Workbench tab 存在但显示尚未接入；legacy 结果组件可渲染 | speedtester download、StartBatch/TestSingle、RunNodeResult | legacy batch result/JSON；无 workbench stable history | 汇总值存在，没有过程采样/modern persistence | PARTIAL |
| upload | 真实上传速度与失败原因 | modern Workbench 未接入；legacy result model 支持 | speedtester upload、RunNodeResult | legacy batch result/JSON | 后端/CLI 组件存在，formal modern UI 和 history 未接 | PARTIAL |
| Antigravity | 绑定 token 后判断可用/地区阻断/波动 | PreferencesModal 可绑定凭据；WorkBench service selector 明确显示“已支持接口待接入工作台” | speedtester antigravity、OAuth/token API、稳定性/IP probing | legacy RunNodeResult 保存 status/detail/TTFB/Stability/IP | 解析和分类有单元测试；无现代工作台/Monitor 独立历史闭环 | PARTIAL |
| 服务可用性 | 对公共服务/API/目标做真实可用性检查 | modern service tab disabled；Timeline 可看 monitor service_google/service_github samples | Monitor service ProbeSet；legacy Antigravity | monitor_samples 中的 HTTP probe；legacy Antigravity | service probe 不等于 Antigravity，不含完整 service matrix | PARTIAL |
| IP/地区/纯净度 | 查看出口 IP、地区、IDC/住宅、风险/欺诈分 | legacy SampleInspector/TelemetryGrid 有字段；Timeline Inspector 有 ExitIP/Region | core/ip、speedtester antigravity IPInfo、ConvertResult | legacy JSON 中 Ping0/IPPure/IPInfo；monitor 只保存 exit_ip/exit_region | 真实字段存在；现代 workbench 无完整 IP purity 接线 | PARTIAL |

### 6.3 历史与持久化

| 功能 | 用户预期 | UI | Backend/API | 正式 runtime 与数据来源 | Evidence / 限制 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| 普通测速历史 | 浏览历史批次、删除、对比、打开报告 | AirportConsolidatedMatrix 旧历史矩阵存在 | /api/history、/api/history/{id}、compare、report；Wails 对称方法 | legacy JSON；当前有 11 个 JSON 文件 | 旧批量保存链路已存在；身份仍是 legacy scope | VERIFIED |
| SQLite | 历史/monitor/workbench 可重开 | Timeline/Workbench 通过 SQLite-backed API 读取 | core/history/db.go schema、WAL、migration | 用户 home history.db；当前真实存在 | DB reopen 在 PR #9/monitor evidence 中测试；当前 DB 可读 | VERIFIED |
| Raw Sample | 每个 probe/ping 保存原始成功/失败记录 | Timeline 和 Inspector 展示真实 raw sample | SaveMonitorSamples、SaveLatencyTest、samples APIs | monitor_samples/workbench_latency_samples | 当前 291 monitor samples；workbench 当前 0 | VERIFIED |
| cursor pagination | 大量历史按稳定 cursor 分页，不用 offset | Timeline load older / newest | /api/monitor/samples/cursor，keyset by timestamp + sample_id | SQLite cursor read model | PR #4/#5 与前端 store tests；recommendation drain 也使用 cursor | VERIFIED |
| stats | 在窗口内显示 count、success rate、P50/P95、errors | TimelineEvidenceBar | /api/monitor/stats、GetDerivedStats | 从 monitor_samples 派生 | 生产查询与测试有覆盖 | VERIFIED |
| facets | 用历史真实维度构建 node/probe/target/profile filters | Timeline filter bar | /api/monitor/facets、GetMonitorSampleFacets | 从最近窗口 raw samples 派生 | 前端 contract/store/component tests | VERIFIED |
| retention | keep_all/30d/90d/180d/custom 清理 raw samples 和 orphan runs | 当前没有正式 retention/settings UI | POST /api/monitor/retention、Wails ApplyRetention | SQLite batch deletion 与 partial result | DB/application/Web tests覆盖 partial failure；没有产品 UI入口 | COMPONENT_READY |
| history DB restart recovery | 关闭再打开 store 后仍能读已保存记录 | Workbench/Timeline读取路径具备 | NewStore/OpenDB 与查询 API | 用户 home history.db | PR #9 重开查询和历史 evidence；当前 DB 真实可读 | VERIFIED |
| monitor job restart recovery | 应用重启后自动恢复之前的 Monitor job | UI 文案明确写“重启后不会恢复任务” | 没有 job definition persistence/restore | monitorSchedulers 是 AppService 内存 map；SQLite只保存 run/sample | 历史短跑的重启续跑不能证明产品自动恢复 job | UNFINISHED |
| legacy name-only timeline | 同名节点历史不跨订阅混合 | 旧 history API 可打开 | GetNodeTimeline(nodeName) 按 ProxyName 匹配 | legacy JSON | profile_id 不在旧结果主键中；新 UI不能复用 | SUSPECT |

### 6.4 24/7 Monitor

| 功能 | 用户预期 | UI | Backend/API | 正式 runtime 与数据来源 | Evidence / 限制 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| job 创建 | 选择 profile、真实节点、probe、间隔、超时并创建 stopped job | MonitorJobsView 表单 | POST /api/monitor/jobs、CreateMonitorJobFromRequest | 后端从 cache 重解析 RawConfig，UI只传安全稳定键 | PR #8 Web UI evidence；主线代码已合并 | VERIFIED |
| Start/Pause/Resume/Stop | 操作后状态由 backend read-back 确定 | MonitorJobsView 生命周期按钮 | 对应 /start、/pause、/resume、/stop；Wails 同名方法 | in-memory Scheduler 状态 | PR #8 真实 Web UI evidence 覆盖四状态 | VERIFIED |
| Scheduler | 首轮立即执行、周期运行、防重叠、skipped 可审计 | UI显示 job/runs，不自行实现 scheduler | core/monitor/scheduler.go | 内存 scheduler + SQLite run/sample store | scheduler tests 与 monitor evidence | VERIFIED |
| Probe Set | 选择 light/service/heavy | UiSelect 表单 | MonitorJobCreateRequest 校验三种 ProbeSet | Runner 真实执行 RTT/HTTP/TTFB | 不含 download/upload/Antigravity；所以不是完整服务测速 | PARTIAL |
| Raw Sample persistence | 每轮每节点每目标持久化成功/失败原始数据 | Timeline可读 | SaveMonitorRun/SaveMonitorSamples | SQLite monitor_runs/monitor_samples | 当前 DB 有 25 runs/291 samples | VERIFIED |
| Timeline | 按 node/profile/probe/target/time 浏览样本 | MonitorTimelineView、SampleTimeline | cursor + range query | 从 SQLite raw samples 读，不用演示点 | PR #5 UI/evidence 与 main 前端组件 | VERIFIED |
| Inspector | 点击/键盘定位样本，查看目标、错误、ExitIP/Region | TimelineInspector | sample DTO 包含原始字段 | 当前真实样本详情 | 前端组件测试和源代码核对 | VERIFIED |
| live refresh | 页面在 job 运行时增量刷新最新样本 | Timeline store 15 秒 polling | newest cursor page + merge by sample_id | 前端内存 read model，不改变数据库事实 | store tests覆盖 | VERIFIED |
| catch-up | polling期间 burst 不漏样本 | store 内 head re-read + drain pages | cursor head query | 合并按 sample_id，跨页追赶 | PR #5 gap 修复与前端 store tests | VERIFIED |
| incomplete/gap continuation | 超过单次预算时诚实显示 incomplete，并跨轮继续追赶 | Timeline状态栏/空态/错误态 | pendingGaps continuation state | 前端内存维护未闭合 gap | PR #5 B-06/B-07/B-08 evidence | VERIFIED |

### 6.5 Workbench

| 功能 | 用户预期 | UI | Backend/API | 正式 runtime 与数据来源 | Evidence / 限制 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| 稳定 node identity | 节点变化或同名时仍正确归属 | Workbench选项来自 MonitorNodeOption | resolveWorkbenchLatencyProxy 校验配置 | cache + NodeKey/Identity/Revision | PR #9 identity/isolation tests | VERIFIED |
| profile_id + node_key | 逻辑节点主键跨订阅隔离 | scopeKey 使用二者 | history query/detail 强制二者 | SQLite index(profile_id,node_key) | PR #9 cross-profile test | VERIFIED |
| attempt_id | 一次测试、异步保存、详情读回不可混淆 | UI 保存状态按 attempt 关联 | attempt_id primary key/idempotency | workbench tables | PR #9 idempotency/detail tests | VERIFIED |
| latency result | 真实单节点 latency 与失败样本先显示 | 立即测试按钮、当前读数、raw graph | POST /api/workbench/latency-tests | speedtester latency-only path | PR #9 Web/local proxy evidence | VERIFIED |
| async persistence | saving → saved/failed，不让慢保存阻塞结果 | UI显示保存中/已保存/保存失败 | background SaveLatencyTest + event | SQLite transaction；Close等待 persistence WG | PR #9 slow-save/failure tests | VERIFIED |
| history graph | 原始样本画图，失败断线，历史可展开 | LatencySamplePlot 和 detail modal | history list/detail API | workbench raw sample rows | 当前 DB为0；代码/前端测试已覆盖 | PARTIAL |
| stale response protection | 切换节点/attempt时旧响应不能污染新节点 | request ID、scope、attempt guards | detail/list query带scope | 前端本地状态保护，backend详情再校验 | PR #9 frontend tests | VERIFIED |
| observation window in Workbench | 选择4h/24h应改变实际查询范围 | 有 local/full UiSelect 和时间轴标签 | history API目前只按 profile/node/limit | 页面把窗口用于标签/轴，未把 since/until 传给 history query | 代码核对得到语义缺口 | SUSPECT |

### 6.6 Controller

| 功能 | 用户预期 | UI | Backend/API | 正式 runtime 与数据来源 | Evidence / 限制 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| Mihomo External Controller | 连接已有 Mihomo/Clash controller | 当前没有正式 Controller 页面 | adapter/controller/mihomo + status/config/group APIs | HTTP REST client；默认 127.0.0.1:9090 | client/integration/security tests | VERIFIED |
| group/selector | 读取策略组和 selector 当前节点 | 无正式 UI调用者 | ListGroups、GetCurrentSelection | External Controller /proxies | backend adapter存在，未接入主导航 | COMPONENT_READY |
| node select | 用户手动选择策略组节点 | API bridge有方法，当前无组件调用 | POST /api/controller/select、Wails SelectControllerNode | Controller.SelectNode 真实写操作 | backend/service tests；无正式 UI | COMPONENT_READY |
| secret | 带 Bearer secret连接 controller，不在 status 中回显 | config bridge存在，当前无 config UI | Config.Secret、Authorization Bearer、HasSecret/MaskedSecret | 进程内 controller config | client tests；未持久化为产品设置 | VERIFIED |
| localhost safety | 默认只允许 localhost/loopback；远端需显式和安全传输 | Web server 也限制 loopback Host/Origin | Mihomo client remote/TLS checks；Web CSRF/Host checks | 127.0.0.1 bind，remote policy fail-closed | security/integration tests | VERIFIED |

### 6.7 Decision / Recommend / Auto

| 功能 | 用户预期 | UI | Backend/API | 正式 runtime 与数据来源 | Evidence / 限制 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| Evidence | 从当前 MonitorJob 候选集读取完整 raw sample window | 当前无正式用户页面 | GET /api/monitor/recommendation 触发 Evidence path | SQLite cursor drain，候选集来自当前内存 job | PR #7 review/evidence tests | VERIFIED |
| Recommendation | 输出 stay/recommend/insufficient 及原因 | 当前无正式用户页面 | policy.RecommendFromEvidence | evidence snapshot + policy engine | PR #7生产路径测试 | COMPONENT_READY |
| observation window | 明确扫描窗口和实际 first-last observation span | 无 UI | DefaultEvidenceWindow、EvidenceWindow、ObservationWindow | raw sample timestamps | 30h/2h边界和 freshness tests | VERIFIED |
| sample sufficiency | 样本数量、延迟样本深度、freshness不足时拒绝推荐 | 无 UI | evidence sufficiency gates | sample count、latency depth、age/window | PR #7 tests | VERIFIED |
| confidence | 给出可解释 score、sample depth、window adequacy、freshness factor | 无 UI | ConfidenceBasis / Formula / Score | policy evidence snapshot | PR #7 confidence tests | VERIFIED |
| 当前是否只读 | Recommend 和 preview 不得触发切换 | 无 UI | GetMonitorRecommendation 不持 controller write | SelectNodeCalls=0、Executed=false | PR #7 read-only tests | VERIFIED |
| 真实自动切换 | Monitor evidence够时实际调用 Controller.SelectNode，并验证/回滚 | 无正式 UI/调度调用 | EvaluateAndAutoSwitch 方法有执行逻辑，但当前 main 无生产调用者 | 没有正式 Monitor→Auto 生产链 | 静态 call-site 核对；policy 文件明确 Auto 未接入 | UNFINISHED |
| Auto 总体 | 用户明确配置后自动切换 | 无 UI | policy 支持 mode_auto，推荐路径不执行 | mode 可被组件接收，但不被正式调度消费 | 不能因 ModeAuto 枚举或函数存在判定完成 | UNFINISHED |

## 7. UI 当前状态

### 7.1 当前 main 的正式主导航

当前 main 的 App.vue 正式挂载：

1. 节点工作台；
2. 持续监测；
3. 历史记录（Monitor Timeline）；
4. 管理订阅 modal；
5. 旧历史矩阵 modal；
6. 运行偏好/凭据 modal。

这不是当前 checkout 4096fb8 的完整内容。当前旧 checkout 的 App.vue 有 MonitorJobsView 和 MonitorTimelineView，但没有 main 新增的 LatencyWorkbench；因此必须用 main ref 查看正式 UI。

### 7.2 节点工作台

- 节点来源是 GET /api/monitor/nodes 的真实 cache 解析结果；
- 主键范围是 profile_id + node_key；
- 支持订阅选择、搜索、排序、节点选择、立即测试；
- 当前可真实测试的 project 只有 latency_stability；
- 有原始 latency sample 图、成功/失败/超时显示、保存中/已保存/保存失败状态、历史详情和键盘/悬停定位；
- P50/P95 是从真实 raw latency samples 派生的辅助读数；
- 没有节点时显示真实空态，不填充 demo；
- 吞吐 tab 和服务可用性 tab 存在位置，但明确标记尚未接入。

### 7.3 持续监测

- MonitorJobsView 可选择订阅、真实节点、light/service/heavy、interval、timeout；
- 创建后明确保持 stopped，要求用户显式 Start；
- Start/Pause/Resume/Stop 后重新读取 backend 状态；
- recent runs 取真实 monitor runs；
- UI 文案明确：job 配置仅本进程保留，重启后不会恢复；
- 可进入 Timeline，按 profile 与稳定 node identity 过滤。

### 7.4 历史记录与 Timeline

- 主导航“历史记录”实际是 Monitor raw sample Timeline；
- 支持时间范围、订阅、节点、probe、target、cursor older、live refresh；
- Evidence bar 读取 stats；
- Inspector 只展示真实存在的 raw sample；
- 旧历史矩阵仍通过工具入口存在，服务于 legacy JSON 批次、删除、对比和报告；
- 两套历史 UI 并存，不应把旧批次矩阵当成稳定身份 workbench history。

### 7.5 延迟与稳定性

当前 main 的现代工作台中，延迟是唯一真正接线的测试项目。它有：

- latency raw sample；
- failure/timeout；
- jitter 与 packet loss DTO；
- P50/P95 派生；
- 历史图；
- attempt/persistence 状态；
- 节点切换 stale response 防护。

UI 没有把 jitter/packet loss 做成与 latency 同等完整的现代指标卡，也没有 Monitor-level jitter metric。

### 7.6 吞吐与服务可用性

- 吞吐：现代 workbench tab 是占位/未接入；legacy speedtester 的 download/upload 能运行并返回汇总结果，但没有 stable-identity workbench persistence 或过程 samples。
- 服务可用性：Monitor service probe 能保存 Cloudflare/Google/GitHub HTTP 结果；Antigravity 另有 legacy backend；现代 workbench service tab 仍未接正式结果接口。

### 7.7 自绘 UiSelect

main 中的 UiSelect 已替换 PR #10 范围内的 native select。它实现：

- button/combobox/listbox 语义；
- click、Enter、Space、Arrow、Home、End、Escape；
- disabled option；
- Teleport 到 body；
- resize/scroll 后重新定位；
- 点击外部关闭。

这是当前 main 正式 UI 的真实组成，不是未合并原型。

### 7.8 Modal 与 settings

- AirportModal：添加、编辑、删除、刷新，真实 API；直接展示订阅 URL。
- AirportConsolidatedMatrix：旧批次历史、详情、删除、compare、HTML report。
- PreferencesModal：Antigravity token 输入、OAuth login、token status；主题区只是静态显示 Dark，没有真正的主题设置。
- Controller config/policy/audit 没有正式 modal 或主导航页面。

### 7.9 当前 native Wails 交互证据

PR #9 记录过 Wails 原生进程可以启动，但浏览器和 native window 的鼠标/键盘端到端因 CUA 通道失败未验证。本次没有重复启动或重跑该交互。对“源码接线存在”可以判定，对“当前 main native UI 已被人工端到端验收”应保持 UNKNOWN。

## 8. 已完成阶段

以下按阶段压缩，不罗列每个 commit：

| 阶段 | 做了什么 | 为什么做 | 最终状态 | 对现在的影响 |
| --- | --- | --- | --- | --- |
| 上游 CLI / speedtester 基线 | Mihomo proxy 解析、latency、download、upload、TUI/CLI、过滤和旧输出 | 保留原有可用测速能力 | legacy CLI 与核心测速器仍可用 | 现代 UI 仍复用其中部分 Result/metric 语义 |
| GUI / legacy history | airport 管理、批量测速、普通 JSON 历史、report/export、triage/inspect | 把 CLI 测速放入用户可操作界面 | 后端/API/旧 modal 存在 | 身份仍以 airport/display name 为主，成为后续迁移债务 |
| Wails/Vue/TS 架构迁移 | Go Core、Application Service、Wails Desktop、Vue/Vite/TS、Web adapter | 给 Windows/macOS 桌面和未来 Monitor 留边界 | 已合入 main，Wails v2.16，frontend/dist embed | 当前正式架构基础 |
| Controller-first | core/controller、Mihomo client、policy/decision state、手动选择、审计 | 不自建代理内核，控制已有 Mihomo | backend/adapter ready，UI未接，Auto未接 | 安全边界和未来 Auto依赖 |
| Monitor Core | MonitorJob、Runner、Scheduler、ProbeSet、SQLite monitor runs/samples | 可靠采集原始事实 | 生产代码和测试已合并 | 当前 24/7 能在单进程工作 |
| History / Retention | keyset cursor、stats、facets、retention、migration continuity | 让 raw sample 可查询、统计和清理 | backend ready，retention暂无正式 UI | Timeline/Evidence 的数据基础 |
| Timeline | raw sample timeline、Inspector、filters、live refresh | 把事实可视化而不造点 | 当前 main 正式 UI存在 | 是当前最完整的 Monitor 用户面 |
| Evidence → Recommend | 观察窗口、样本充分性、profile/revision/candidate隔离、confidence、只读 recommendation | 让决策可解释且不误切换 | API/backend/test ready，用户入口未做 | Auto 仍明确后置 |
| PR #8 Monitor job UI | MonitorJobsView、生命周期、真实节点选择 | 让 Monitor Core 可被用户操作 | merge d863b55；当前 main保留 | 任务配置仍进程内 |
| PR #9 Phase 1 latency persistence | profile_id + node_key + attempt_id、workbench tables、async save、reopen、stale guards | 建立第一个现代 UI 真实闭环 | merge 97d2d89；源码和 Evidence存在 | 当前正式工作台只覆盖 latency |
| PR #10 原型 UI 恢复 | LatencyWorkbench、LatencySamplePlot、SourceScopeBar 重做、自绘 UiSelect、产品导航 | 把原型交互接到真实 latency/Monitor backend | merge 58d036e；main正式包含 | 当前根目录旧 branch 不代表这一阶段 |

## 9. 未完成能力

按当前事实，以下不能写成“已完成”：

1. 稳定的跨启动 profile 数据目录和发行包数据迁移；
2. 现代 workbench 的多节点批量、download、upload、Antigravity、公共服务/流媒体；
3. download/upload 过程采样和可重开的过程图；
4. 把 legacy batch/single 迁移到 profile_id + node_key + attempt_id 的统一测试记录；
5. 每个测试项目的 success/failed/untested/not_available 明确语义；
6. Monitor job 定义持久化与应用重启恢复；
7. Monitor 对 download/upload/Antigravity/IP purity 的正式 probe contract；
8. Retention 的正式设置 UI和用户可解释结果；
9. Controller 状态、group、手动选择、policy、audit 的正式用户页面；
10. Monitor → Evidence → Controller Auto 的真实生产调用链；
11. 统一 legacy JSON 与 SQLite 的历史模型和正式 schema version；
12. 当前 main 的发行包级别 profile/cache 安装策略、跨架构 macOS 策略和人工 native UI 验收。

## 10. 当前已知问题

### Blocker

#### B-01：当前工作根目录会误启动旧 branch/UI

当前 D:\ai\work\speedtest 是 codex/monitor-job-ui@4096fb8，不是 main；main 比它多 7 个提交。当前 checkout 没有 main 的 LatencyWorkbench 和 UiSelect，根目录还有一个旧时间戳的 clash-speedtest.exe。若直接从当前根目录启动，会得到旧版本结果，且可能读取本目录的另一套 ignored profile/cache。

这属于环境/启动阻塞，不应误判为 PR #10 或 main 的产品代码缺陷。规划模型在任何 runtime 判断前必须明确 branch、HEAD、exe 来源和 cwd。

#### B-02：profile/cache 依赖 cwd，导致主工作流可能从空数据开始

当前 main 的 GUI/Wails 默认没有稳定的用户 profile path，而是读取进程 cwd。启动到另一个 worktree、快捷方式工作目录、临时 build 目录或没有 ignored profile 的发行目录时，/api/airports 和 /api/monitor/nodes 会为空，用户无法选择节点、立即测速或创建 Monitor job。

本机当前 cwd 有数据，所以它不是“当前无数据”；但它是可重复触发的真实产品/启动边界问题，不是只改启动命令即可永久解决的文档问题。历史仍在用户目录、profile却在cwd，是问题的核心。

### Relevant

#### R-01：24/7 Monitor 任务不是 durable job

MonitorJob 和 Scheduler 只存在 AppService 的内存 map。SQLite 保存 run/sample，但没有 job definition 和 restore。当前页面也承认重启不恢复。因此它能做单进程持续采集，但不能把“24/7”解释为跨应用重启的持续服务。

#### R-02：现代工作台只完成 latency

吞吐、上传、Antigravity、服务可用性仍是 legacy/backend 或占位状态。用户看到的是完整项目导航，但真正可操作的正式 Workbench 只有单节点 latency/stability。

#### R-03：legacy batch/single 与现代身份模型分裂

legacy API 使用 airport_id + display name；旧 name-only timeline 可能混合同名节点；legacy TestSingle 不落历史。现代 workbench 已使用 profile_id + node_key，但尚未覆盖批量与其它 metrics。

#### R-04：Controller backend 与 Auto 没有正式产品入口

Mihomo client、group、SelectNode、policy、audit 和 EvaluateAndAutoSwitch 组件存在，然而 frontend 只有 bridge 定义，没有正式组件调用者；Monitor runner 还明确禁止 SelectNode。当前 Recommendation 是只读，不能声称已有自动切换。

#### R-05：订阅 URL 可能作为敏感凭据被回传和显示

AirportDTO.URL 是 raw URL，AirportModal 会直接显示并作为 title 使用。订阅 URL 常常含 token；本 Snapshot 不复制任何真实 URL。当前 local-only 绑定降低了远程暴露面，但不能把 raw URL 回传视为安全完成。

#### R-06：Workbench 的观察窗口目前有 UI 语义缺口

LatencyWorkbench 显示“最近 4 小时/最近 24 小时”选择器和对应轴标签，但 history API 查询目前只带 profile_id、node_key、limit，没有 since/until。代码层面这更像显示层窗口，而不是严格的数据窗口，应在规划时决定是补齐查询语义还是改名。

### Parked

#### P-01：完整 24h/发布级 soak

历史 evidence 已明确把一次约 6 分 54 秒运行修正为 24H SOAK: INCOMPLETE。完整 24h soak 只属于明确的 release acceptance，不是当前事实收口的默认动作。

#### P-02：吞吐过程采样与更丰富的 Monitor probe

需要先定义真实字节/时间过程样本、失败/未测试语义和资源策略；不能从汇总速度插值造图。Antigravity、download/upload 接入 Monitor 也应独立规划。

#### P-03：Controller UI 与 Auto

当前推荐链路保持只读是正确安全边界。Controller UI、手动切换 UX、policy persistence、Auto 执行/验证/回滚属于后续阶段，不应因为现有函数存在就提前视为当前工作流。

#### P-04：跨平台人工 native UI 与发行包验收

CI 已覆盖 Windows build、macOS arm64 bundle 和 Linux tests，但 native Wails 的人工交互、macOS Intel/Universal、发行包 profile migration 尚未形成发布级证据。

#### P-05：视觉重构和未来订阅服务信息

包括主题设置、套餐/流量/到期信息、机场官网入口、IPv6/订阅整理等，属于产品方向中的未来扩展，不是当前事实闭环的必需修复。

### Historical / Closed

- PR #1–#10 已全部合并；不要把仍保留的 local branch/worktree 当成待合并工作。
- PR #10 merge 58d036e 已在 main；不要重新恢复或重新合并原型 UI。
- PR #5 的 live refresh burst/gap continuation 相关问题已有后续修复和测试；不要用旧的“可能永久漏样本”描述当前 main。
- PR #7 的 Evidence → Recommendation 读写边界、candidate/profile/revision isolation、observation window 和 confidence review 已有相应修复证据；不要把它误写成 Auto 已完成。
- 早期“24H SOAK: PASS”口径已被 corrected evidence 推翻，属于 WRONG；当前正确口径是短时 smoke PASS、历史重启读回/短跑恢复证据存在、24h soak INCOMPLETE。
- PR #8/PR #9 的旧分支 head 4096fb8/2a5f64f3 是历史提交，不是当前 main。

## 11. 技术债

1. Profile path 仍使用 cwd，且用户数据被 .gitignore 忽略，缺少统一的 user-data directory contract。
2. Legacy JSON 与 SQLite 并存，历史查询、身份模型和 retention 边界不统一。
3. SQLite migration 没有 formal schema version table。
4. Monitor job 定义未持久化；controller config/policy/audit 也主要保存在进程内存。
5. legacy result model 用零值承载部分未测试语义，容易把未执行误读为 0。
6. legacy name-only timeline 与新 profile/node scope 并存，容易被错误复用。
7. Workbench window selector 尚未真正约束历史 query。
8. Monitor runner 对部分保存/更新错误的处理仍有忽略返回值的路径；对现代即时测试必须保持“完成但未保存”可见。
9. Web SSE handler 仍设置 Access-Control-Allow-Origin: *；server 绑定 loopback 和 Host 校验降低风险，但该写法不应作为通用安全模板。
10. 当前 CI 是 build/test matrix，不是实际用户订阅、发行包、跨重启服务或 native UI 的替代证据。

## 12. 安全边界

### 已存在的边界

- Web server 绑定 127.0.0.1；
- Host 必须是 localhost/loopback；
- mutating request 有 loopback Origin/Referer 检查；
- 不使用 wildcard CORS 作为普通 API策略；
- Mihomo remote controller 默认拒绝非 loopback；
- remote controller 需要显式 AllowRemote，非 loopback 明文 HTTP 还需要显式高风险 opt-in，默认要求 HTTPS；
- controller secret 通过 Bearer header 发送，status 只返回 HasSecret，client 有 masked secret；
- Monitor 和 Workbench 的公开 DTO 不带 RawConfig、订阅凭据或密码；
- 后端根据 profile cache 重新解析 RawConfig，而不是相信前端传入的节点配置；
- Recommendation path 不调用 SelectNode，不修改 policy/decision state；
- Antigravity token 写入用户 home 时使用 0600；本 Snapshot 不记录 token 或订阅 URL。

### 尚需注意的边界

- AirportDTO 和 AirportModal 仍暴露 raw subscription URL，URL 中可能含 token；
- local-only server 不是操作系统级身份认证，同一用户会话/本机其它进程仍可能访问 loopback；
- SSE 的 wildcard header 需要单独评估；
- Controller config/policy 没有完整的持久化、审计和 UI确认流程；
- 不应把 IP/地区/纯净度字段视为商业或安全结论，它们依赖第三方 probe 的当前返回和证据完整性。

## 13. 跨 Windows/macOS 状态

### Windows

- Wails native configuration 有 Windows Mica；
- PR #10 GitHub CI 的 Windows Desktop Build 通过；
- 本机是 Windows，能够核对 ignored profile/cache 和用户 home history；
- 当前本机没有运行中的 native/Wails 进程；
- 当前根目录 exe 属于未知旧 build 来源，不应当作 main 发行物。

### macOS

- Wails configuration 有 macOS native options；
- PR #10 CI 的 macOS Native App build 通过，目标为 darwin/arm64；
- 没有本机 macOS runtime、人工窗口操作或 Intel/Universal 证据；
- profile cwd 语义、用户数据目录、App bundle 工作目录仍需要跨平台发布设计。

### Linux / Web

- PR #10 Linux Go Unit Tests 通过；
- Web adapter 是本地 loopback fallback，可作为 API/浏览器入口；
- 历史 Web 真实操作 evidence 使用过本地临时订阅/代理，但没有当前 main 用户订阅 runtime 证据。

跨平台总判定：构建矩阵 VERIFIED；发布级用户数据、native 交互和跨重启行为 PARTIAL/UNKNOWN。

## 14. 测试与 Evidence 现状

### 可复用的已存在 Evidence

| Evidence | 可证明什么 | 不能证明什么 |
| --- | --- | --- |
| PR #10 CI | main 当前包含的 Linux test、Windows build、macOS arm64 native build 能通过 CI | 不能证明真实 profile data、native人工操作或长期运行 |
| MONITOR_JOB_UI_EVIDENCE.md | 在 PR #8 分支基线上的真实 Web UI 节点选择、job 创建、Start/Pause/Resume/Stop、Timeline read-back | 未启动 Wails 原生窗口；任务配置重启不恢复；不是当前 main live process |
| PHASE1_LATENCY_PERSISTENCE_REPORT.md | PR #9 stable identity、SQLite workbench save、async saving→saved/failed、reopen、Web真实进程 API、前端 12 files/182 tests、构建 | 本地临时订阅/代理，不是真实用户订阅；native鼠标键盘未验证；不含 Auto/吞吐/Monitor restore |
| PR7_FINAL_SHORT_REVIEW_14d5203c.md | Evidence window、sample sufficiency、candidate isolation、confidence、recommendation read-only、SelectNodeCalls=0 | 不含 Auto 执行；不等于 UI 已接；未重新做长 soak |
| EVIDENCE-CORRECTED-2026-09-18.md | short real-world smoke PASS；历史重启/续跑 evidence；24h只有约6分54秒所以 INCOMPLETE | 不能证明 24h PASS；不能证明产品会自动恢复 job definition |
| 当前本机 history.db | SQLite 可读，monitor_runs=25、monitor_samples=291、workbench tables=0 | 不能证明这些数据属于当前 main live process，也不能证明当前 UI在用它们 |

### 验证边界

- 本次遵循最小充分验证，没有启动程序、没有重复全量测试、没有重跑 CUA/native UI、没有重跑 24h soak；
- 代码事实以 git show main:/git grep main: 为准，因为当前 cwd checkout 不是 main；
- runtime 事实只来自当前文件、数据库、进程、端口和 Git 状态核对；
- 对当前没有进程的 HTTP API，不写成 VERIFIED runtime；
- 旧 Evidence 中的“代码/adapter/test verified”和“真实用户发行版 verified”保持区分。

## 15. 已明确“不应该重复做”的工作

1. 不要把 4096fb8、2a5f64f3、14d5203c 当成当前 main，也不要从旧 worktree 启动后重新判断 PR #10 UI。
2. 不要因为 PR #10 已 merge 再重复恢复原型 UI或自绘 UiSelect。
3. 不要重新跑已经有针对性证据的 PR #5 gap continuation、PR #7 recommendation read-only、PR #9 latency persistence/stale response，除非有新代码、新失败或具体未解问题。
4. 不要把旧的“24H SOAK: PASS”重新打开；正确记录是约6分54秒短跑，24h INCOMPLETE。
5. 不要为了证明普通代码形状而再写固定源码快照、调用次数或重复的自证式测试。
6. 不要在本次 snapshot 后顺手修 profile path、URL masking、UI window filter 或 Auto；这些是后续决策，不是本次授权。
7. 不要扫描 Raw/Archive 全量历史；需要时只读对应 Evidence/Handoff/Git history。
8. 不要创建新的长期 Feature Truth 制度文件；本文件矩阵仅用于本次事实收口。

## 16. 产品方向

当前合理的产品顺序仍是：

Monitor Core / Scheduler / Raw Sample
→ History / Retention
→ Timeline / Inspector / Statistics
→ Evidence → Recommendation
→ Controller UX
→ Auto。

当前 main 实际已经覆盖到：

- legacy 测速与历史；
- Monitor Core；
- History/Retention backend；
- Timeline；
- Evidence/Recommend backend；
- 单节点 latency persistence；
- 原型 UI 正式接入。

但它还没有完成“所有 metrics 在同一稳定身份模型下可测试、可持久化、可重开、可解释”的产品目标。下一阶段不应从“Auto 函数存在”开始，而应先决定 profile 数据边界和 Workbench/Monitor 的持久化产品契约。

## 17. 依赖关系

### 17.1 主数据流

profile/cache path
→ 真实 node options
→ Workbench 或 Monitor job 创建
→ 实际 probe/test
→ raw sample / attempt persistence
→ Timeline、stats、facets、history graph
→ Evidence、sufficiency、confidence、Recommendation
→ 未来才是 Controller select / Auto。

### 17.2 关键依赖

| 上游 | 下游 | 当前关系 |
| --- | --- | --- |
| 稳定 profile path | 所有订阅节点、Workbench、Monitor | 当前是最高优先级的运行前提 |
| profile_id + node_key | workbench history、monitor evidence、跨订阅隔离 | 已在新路径成立，legacy仍未迁移 |
| Raw Sample persistence | Timeline、stats、facets、Recommendation | 已成立 |
| Monitor job definition persistence | 跨重启24/7、Recommendation长期候选集 | 当前缺失 |
| Workbench metric state model | throughput/service/Antigravity modern UI | 当前缺失 |
| Controller UI/config persistence | 手动切换和 Auto 用户控制 | 当前缺失 |
| Evidence sufficient | Recommendation | 已成立但无正式 UI |
| Recommendation + explicit policy | Auto | 当前没有生产调用链 |

### 17.3 不应倒置的依赖

- 不能先做 Auto 再补 evidence、身份和持久化；
- 不能先做吞吐图再定义过程 sample contract；
- 不能先把旧 display name history 画进新 profile-scoped workbench；
- 不能在 profile path 未决定前把空节点问题当成 frontend rendering bug；
- 不能把 monitor raw sample 的 service_google/service_github 当成 Antigravity 完成。

## 18. 可选下一阶段

下面是供规划模型选择的范围，不是本次执行计划：

### Option A：Runtime baseline closure

- 选择稳定的 profile/cache 用户数据目录或显式配置目录；
- 明确 Wails/GUI/CLI/release bundle 的启动路径；
- 统一 main checkout、exe、worktree 启动方式；
- 解决 profile 空列表和 raw URL masking。

依赖最少，能先消除最容易误判的环境事实。

### Option B：Workbench parity

- 把 batch、多节点、download/upload/Antigravity/service 接入 stable identity；
- 为每个 test project 建立 attempt/metric status；
- 明确 success、failed、untested、not_available；
- 统一 legacy JSON 与 SQLite 的读写迁移边界。

这是把“现代工作台”从 latency-only 推向产品目标的路径。

### Option C：Monitor durability

- 保存 Monitor job definition；
- 重启时显式恢复或要求用户确认恢复；
- 区分历史 raw sample 恢复与活跃 scheduler 恢复；
- 为 job/config revision/retention 建立正式 schema version。

这是把“单进程持续采集”推进到真正可运营的 24/7。

### Option D：Controller UX / Auto

- 先做 status、group、manual select、policy、audit 的正式 UI；
- 明确用户锁定、冷却、候选集和切换确认；
- 再接 EvaluateAndAutoSwitch，要求验证/回滚和审计；
- 保持 monitor runner 本身不直接 SelectNode。

只有在 A/B/C 的数据边界稳定后才适合进入。

### Option E：Release acceptance

- Windows、macOS 目标架构和发行包；
- profile/cache 首次启动策略；
- native UI 人工操作；
- 真实订阅但不把敏感凭据写入 evidence；
- 明确的 24h soak 入口和停止条件。

这不是当前默认动作。

## 19. 需要规划模型做决定的问题

1. Profile/cache 最终应放在稳定的 user data directory，还是保留 cwd 但由 GUI/CLI 强制显式配置？
2. “24/7 Monitor” 是否必须跨应用重启恢复 job，还是允许第一版只承诺单进程持续采集？
3. Workbench 下一项优先补齐 throughput/service，还是先把 legacy batch 迁移到 stable identity？
4. legacy JSON 与 SQLite 是长期双存储，还是以统一 SQLite metric/attempt model 为主并保留兼容读取？
5. raw subscription URL 是否必须在所有 DTO/UI 中 mask；如果是，哪些操作仍允许用户复制完整 URL？
6. Workbench 的 4h/24h window 是否要求真正传入 since/until 并影响 query，还是改成只表达展示窗口？
7. Controller UI 是否在 Auto 之前作为独立里程碑；Auto 的正式触发者是 Monitor scheduler、独立 orchestrator 还是用户确认动作？
8. Windows/macOS 发行物如何提供 profile/cache：首次导入、用户选择目录、应用数据目录还是显式配置文件？
9. 发布验收需要哪些真实 runtime 证据：Web API、Wails native UI、跨重启、真实订阅、长 soak，以及每个平台的最低组合？
10. 当前 main 是否需要一个“唯一可启动的 main worktree/构建产物”约定，避免旧 branch/worktree/ignored data继续产生错误 UI 结论？

## 附：本次事实核对范围

已读取并作为辅助依据：

- Obsidian 根 AGENTS.md；
- 所有项目通用工作规则；
- 工程任务拆分与功能正确性审计；
- Git 与 GitHub 协作规范；
- 03-项目与工程/机场/00-总览.md；
- 03-项目与工程/机场/02-clash-speedtest 魔改.md；
- 03-项目与工程/机场/产品方向.md；
- 与当前阶段直接相关的 Handoff、Evidence、Phase 1 report、PR #7 review、corrected soak verdict；
- 当前 main 的 profile、history、monitor、application、adapter、frontend、CI 关键源文件；
- 当前 Git refs、GitHub PR 状态、worktree、进程、端口、ignored profile/cache 和实际 SQLite。

没有读取 Raw/Archive 全量内容，没有复制任何订阅 URL、token、secret 或用户节点详情。

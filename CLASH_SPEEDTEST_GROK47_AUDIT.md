# Clash SpeedTest Grok 4.7 独立审计

> 审计日期：2026-09-22
>
> 审计身份：独立事实审计，只读。没有改源码、没有改分支、没有改用户数据、没有改 Obsidian。
>
> 对照对象：`CLASH_SPEEDTEST_PROJECT_SNAPSHOT.md`（下称 Lunamax Snapshot）
>
> 状态词使用仓库规则里的原义：`VERIFIED` 要求用户可见意图、正式 runtime、truth authority 和当前证据一致。只有测试、只有组件、只有历史 PASS，不够升到 `VERIFIED`。

# Executive Verdict

Git、GitHub、worktree 和本机 SQLite 计数这层事实是可信的。Lunamax 对 `main` SHA、当前 checkout、PR #1–#10 已合并、profile 跟进程 cwd、history 在用户目录，这些结论与本次独立核对一致。

当前最大的不确定性不是“main 上有没有这段代码”，而是：本机现在没有正在运行的 Clash SpeedTest 进程，当前工作根目录也不是 `main`。正式 UI、单节点延迟和 Monitor 生命周期的用户操作，这次没有在 `main` 窗口里重新走一遍。能确定的是源码调用链和已经落盘的数据。

规划前必须先接受三件事：

1. 产品基线是 `origin/main` `58d036e220d873f900f755fdc47b2f6850a0b9b3`。`D:\ai\work\speedtest` 当前检出的是已合并的旧分支 `codex/monitor-job-ui` `4096fb8`，落后 `main` 7 个提交，工作区里的前端和 exe 都不是 PR #10 之后的正式界面。
2. Lunamax 自己把 `VERIFIED` 放宽成“有调用链，并且有测试或历史 evidence 之一”。这个词和工程规则里的 `VERIFIED` 不是同一个词。矩阵里的 `VERIFIED` 不能直接拿去当发布验收。
3. 本机用户库里的 Monitor 样本停在 2026-09-19，工作台延迟表是 0 行。默认策略的样本新鲜度是 5 分钟，而且推荐必须挂在一个还活着的内存任务上。现在这台机器上没有一份够格的现行推荐证据。

# Independent Current-State Map

权威基线：

| 项 | 事实 |
| --- | --- |
| 远端与本地 `main` | `58d036e220d873f900f755fdc47b2f6850a0b9b3`，2026-09-20 17:32 +0800，PR #10 merge。GitHub `main` 同一 SHA |
| 当前 checkout | `codex/monitor-job-ui` `4096fb852ceb721fd41155bdc915685e79ee1869`。它是 `main` 的祖先，没有额外提交。上游分支已删除 |
| 远端分支 | 只剩 `main`。开放 PR 为 0。PR #1–#10 都是 MERGED |
| 本地 worktree | 根目录在旧 PR #8 head；`workbench-latency-persistence` 在 PR #9 head `2a5f64f`；`D:\ai\work\_pr7_final_review` detached 在 `14d5203` |
| 不可移动 tag | `architecture-controller-v1-pass` `1477115`、`monitor-core-v1-pass` `2d603a1`、`monitor-history-v1-pass` `ae9293a`、`monitor-timeline-v1-pass` `d22ebfc`、`monitor-evidence-recommend-v1-pass` `14d5203`。五枚都在 `main` 历史上 |
| PR #10 CI | run `35502599452` success。Linux unit、Windows desktop build、macOS arm64 bundle 都 pass。这是构建证据 |
| 当前进程 | 没有 `clash-speedtest` 进程，18473/18474/18765/34115 没有监听。根目录 exe 53,435,904 字节，写入时间 2026-09-19 21:12:44，早于 PR #9/#10 |
| 跟踪状态 | 已跟踪文件相对 `4096fb8` 是干净的。未跟踪文件是两份报告本身。`airports.json`、`airports-cache/`、`*.exe` 被忽略 |

正式入口，按 `main` 源码，不按当前工作区文件：

| 用户功能 | 正式入口 | runtime | truth authority | 状态 |
| --- | --- | --- | --- | --- |
| 无参数启动 | `main` → `runDesktop` | Wails v2.16.0，embed `frontend/dist`，绑定 `desktop.App` | 与 Web 共用 `application.AppService` | PARTIAL |
| `--web` 或 Wails 启动失败 | `runGUI` → `adapter/web`，只听 `127.0.0.1` | 同一套 Vue 与同一 `AppService` | 同上 | PARTIAL |
| `--cli` 或给出 `-c` | `main.go` 直接走 `speedtester`、机场菜单、TUI | 不经过 `AppService`，不写工作台延迟表，不写 Monitor 任务 | CLI 自己的测速结果和可选 gist/仓库上传 | PARTIAL |
| 节点工作台 | `main` 的 `LatencyWorkbench` | `POST /api/workbench/latency-tests` 与 Wails `RunWorkbenchLatencyTest` | SQLite `workbench_latency_tests` / `workbench_latency_samples`，键是 `profile_id + node_key + attempt_id` | PARTIAL |
| 吞吐 / 服务页签 | 工作台页签可见 | 点击后明确不调用正式测试 | 没有对应工作台表 | UNFINISHED |
| 持续监测 | `MonitorJobsView` | create/start/pause/resume/stop API 与 Wails 方法 | 任务在 `AppService.monitorSchedulers` 内存里；run/sample 进 SQLite | PARTIAL |
| 导航“历史记录” | `MonitorTimelineView` | cursor / stats / facets | `monitor_samples` | PARTIAL |
| 旧批量测速 | 工作台里折叠的 `legacy-surface`，仍挂着 TelemetryGrid 等旧组件 | `/api/test/batch` 会 `history.Save` 到 JSON；`TestSingle` 返回结果但不 `Save` | `~/.clash-speedtest/history/*.json`，身份偏机场和显示名 | PARTIAL |
| 旧历史矩阵 | 工具按钮打开 `AirportConsolidatedMatrix` | `/api/history` | 同上 JSON | PARTIAL |
| 机场/订阅 | `AirportModal` | `/api/airports*` | `<cwd>/airports.json` 与 `<cwd>/airports-cache/` | PARTIAL |
| 推荐 | 无 Vue 调用者。有 `GET /api/monitor/recommendation` 和 Wails 方法 | `GetMonitorRecommendation` 只读，`job_id` 必须指向活着的内存任务 | 该任务节点上的 `monitor_samples`，再进既有 policy engine | COMPONENT_READY |
| 自动切换 | 无 UI、无 HTTP、无 Wails 绑定 | `EvaluateAndAutoSwitch` 只在测试里被调用。`ModeAuto` 可以被 policy API 写进内存，没有生产循环消费它 | 无生产切换链 | UNFINISHED |
| 手动切节点 | 无 Vue 调用者 | `POST /api/controller/select` 与 Wails `SelectControllerNode` 会调用 `SelectNode` | 外部 Mihomo Selector。默认拒绝非 loopback；非 loopback 明文还要显式 opt-in | COMPONENT_READY |
| 保留策略 | 无设置页 | `POST /api/monitor/retention` 与 Wails `ApplyRetention` | SQLite 批量删除。默认语义是 `keep_all`，没有启动时自动清理 | COMPONENT_READY |
| 观察窗口控件 | 工作台 UiSelect，文案写最近 4 小时 / 最近 24 小时 | 历史请求只有 `profile_id + node_key + limit: 20`。`samplesForKey` 不按 `windowMode` 过滤 | 控件不约束查询，也不约束本地样本 | WRONG |
| “加入持续监测” | 工作台两个按钮 | 只 `emit('open-monitor')`，`App.vue` 只把视图切到监测页。不创建任务，不把选中节点带过去 | 与按钮文案不一致 | WRONG |

本机数据，只读打开，没有写入：

| 数据 | 现场 |
| --- | --- |
| `D:\ai\work\speedtest\airports.json` | 2 条记录，更新时间 2026-09-16 与 2026-09-17。名称是「良心云」「69云」。URL 未抄录 |
| 同目录 cache | 2 个 yaml，合计约 434 KB |
| latency worktree 的 `airports.json` | 另 1 条，名称 `Runtime Subscription`，更新时间 2026-09-20 14:48。cache 1 个文件，76 字节 |
| PR #7 review worktree | 没有 `airports.json` |
| 用户 history | `C:\Users\Administrator\.clash-speedtest\history` |
| `history.db` | 339,968 字节。WAL 0 字节，shm 存在。表：`monitor_runs` 25，`monitor_samples` 291，`workbench_latency_tests` 0，`workbench_latency_samples` 0 |
| Monitor 样本时间 | 2026-09-18 06:21 UTC 到 2026-09-19 13:16 UTC。6 个 `node_key`，1 个 `profile_id` |
| legacy JSON | 11 个文件，日期在文件名上是 2026-09-16 到 2026-09-17 |
| 同目录还有 | `settings.json` 29 字节（2026-09-20），`report.html` 约 418 KB（2026-09-17） |

延迟工作台曾经用临时目录和临时订阅走过一次 Web 进程证据，报告写明进程已停、数据不是用户库。这和用户库 0 行不矛盾，也不能把那次临时库当成用户现在打得开的历史。

# Agreement With Lunamax

这些重要事实成立，本次独立核对得到同一结果：

- 产品事实基线是 `main` `58d036e`，不是当前目录的 `4096fb8`。当前分支是 PR #8 head，落后 7 个提交，没有分叉。远端只剩 `main`，PR #1–#10 已合并。
- `main` 上有 `LatencyWorkbench.vue`、`UiSelect.vue`，主导航是「节点工作台 / 持续监测 / 历史记录」。当前工作区没有 `LatencyWorkbench.vue`。直接运行根目录旧 exe，看到的是旧界面。
- Wails 与 `--web` 共用 `AppService`。Web 只绑定 `127.0.0.1`。
- `profiles.DefaultPaths()` 用 `os.Getwd()`。History 默认在用户主目录 `.clash-speedtest/history`。两套 worktree 的 ignored `airports.json` 不是同一份数据。
- Monitor 任务配置只活在进程内存。页面原文写明重启不恢复任务，也不会把历史样本显示成活任务。SQLite 只存 run 和 sample。
- `EvaluateAndAutoSwitch` 没有 Monitor、UI 或 HTTP 生产调用者。推荐路径要求 `SelectNodeCalls == 0`。Monitor runner 不调用 `SelectNode`。
- 现代工作台真正会发测试请求的项目只有 latency。吞吐和服务页签标成尚未接入。
- 用户库计数就是 25 runs、291 samples、0 条工作台延迟记录、11 个 legacy JSON。
- 默认策略是 `monitor_only`。保留策略没有正式设置页。Controller 配置页不存在。
- PR #10 的三项 CI 是成功的构建/测试，不是人工窗口验收，也不是 macOS Intel/Universal 或发行包验收。
- 折叠区里的旧批量工作台、旧历史矩阵、legacy JSON 按显示名归属，仍然和稳定身份路径并存。
- 24 小时 soak 的正确口径是未完成。这次没有重开它。

# Conflicts

```text
TOPIC=CLI 是否进入桌面应用调用链
LUNAMAX_CLAIM=第 4.1 节把 CLI / Wails / local Web 画成同一条链：frontend 与 desktop/web adapter → AppService → profiles/history/monitor。
GROK_FINDING=Wails 和 Web 确实进入 AppService。CLI/TUI 在 main.go 里直接构造 speedtester，走机场菜单和输出/gist/仓库上传，不调用 AppService，不写 workbench_latency_*，也不注册 Monitor job。
EVIDENCE=main.go 的 shouldRunGUI 分支与其后的 CLI 加载；全库没有 CLI 对 history.NewStore 或 RunWorkbenchLatencyTest 的调用。
RECOMMENDED_TRUTH=桌面两条入口共用 AppService。CLI 是第三条行为分叉，不能算进这条调用链。
```

```text
TOPIC=VERIFIED 这个词的含义
LUNAMAX_CLAIM=Snapshot 第 1 节把 VERIFIED 定义成：main 上有完整调用链，并且有针对性测试、adapter/runtime 证据或可复核生产调用链之一。
GROK_FINDING=工程规则里的 VERIFIED 还要求用户可见意图、正式 runtime、truth authority 和当前范围验收一致。按 Lunamax 的定义，一批只有单元测试或只有临时进程证据的行会升成 VERIFIED。两份报告的 VERIFIED 不能互换。
EVIDENCE=99-系统与后台/AI/工程任务拆分与功能正确性审计.md 的状态表；Snapshot 第 1 节「状态词说明」。
RECOMMENDED_TRUTH=规划时把 Snapshot 的 VERIFIED 读成「代码契约有历史证据」。发布级 VERIFIED 仍按工程规则，当前大多数用户功能停在 PARTIAL 或 COMPONENT_READY。
```

```text
TOPIC=工作台「观察窗口」的状态
LUNAMAX_CLAIM=4 小时/24 小时选择器只改标签和轴，历史 API 只有 profile_id、node_key、limit。状态标成 SUSPECT。
GROK_FINDING=描述本身对。状态偏低。samplesForKey 把最多 20 次测试的样本摊平排序，完全不读 windowMode。full 模式的轴还写成 00:00–24:00，和「最近 24 小时」也不是同一件事。控件承诺的数据窗口没有实现。
EVIDENCE=main:frontend/src/components/workbench/LatencyWorkbench.vue 的 fetchWorkbenchLatencyHistory({limit:20}) 与 samplesForKey；轴标签随 windowMode 改变。
RECOMMENDED_TRUTH=WRONG。修法是让查询真正接受时间范围，或改掉控件文案。这是产品语义错误，不是证据不足。
```

```text
TOPIC=Evidence 在当前用户数据上是否已经是可用生产功能
LUNAMAX_CLAIM=Evidence 行状态 VERIFIED。候选集来自当前内存任务，证据是 PR #7 测试。
GROK_FINDING=函数契约和「没有 job_id 就拒绝、不用历史把已删除节点加回候选」与源码一致，这层不该重审。当前用户库不能支撑一次默认策略下的有效推荐：没有持久化任务，进程重启后 job 不存在；仅有的 291 条样本停在 2026-09-19，默认 MaxSampleAge 是 5 分钟。
EVIDENCE=application/monitor_evidence.go collectEvidenceNodes；core/policy/policy.go DefaultSwitchPolicy.MaxSampleAge = 5 分钟；只读查询 history.db 的 MIN/MAX(timestamp)。
RECOMMENDED_TRUTH=推荐引擎保持 COMPONENT_READY。当前数据上的可用推荐是 UNFINISHED。PR #7 的数学和只读边界不要重开。
```

# Missing Information / Omissions

1. 系统睡眠。Scheduler 用 `time.NewTicker`。代码里没有电源事件、唤醒补偿或补采。睡眠期间错过的周期不会逐个补上，暂停/恢复也只是进程内标志，不是操作系统 resume。
2. 「加入持续监测」会让人以为选中的节点已经进入任务。实现只切换到监测页。证据面板上的那个按钮甚至在没有选中节点时也能点。
3. `gui` 包仍是一套独立实现：`gui.NewServer`、`gui.TestManager`、内嵌 `gui/web/dist`。`main` 只调用 `gui.LaunchApp` 去开浏览器窗口。这套旧 HTTP 服务和旧页面没有正式入口，但随包编译，和 `AppService`、折叠区里的 Vue 旧工作台是三套测速表面。
4. 用户 Monitor 样本的时间范围、节点数和订阅数。计数 291 是对的，但没写它们全部早于 2026-09-19 13:16 UTC、只有 1 个 profile、6 个 node key。因此看不出默认新鲜度门槛会把它们全部挡在推荐之外。
5. 两份 profile 的具体分歧。根目录是 2 条、2026-09-16/17、两个较大 cache。latency worktree 是 2026-09-20 的 1 条测试订阅和 76 字节 cache。这不是同一份数据的两份副本。
6. 任务持久化一旦补上，会和 `keep_all`、没有保留设置页绑在一起。现在任务一退出就停，库增长被会话长度挡住。把任务恢复做成默认自动启动之后，SQLite 会按探针数量持续涨，而用户没有产品内的清理入口。
7. 延迟结果先显示 `saving`，保存有 30 秒超时，正常 `Close` 会等完。进程在保存完成前被杀掉，用户已经看到结果，SQLite 里可以没有这一行。用户库目前就是「表已建好，行数是 0」，和「用户目录里从未成功落过一条工作台延迟记录」一致。

# Feature Truth Corrections

只列需要改口的状态。其余 Lunamax 行可以沿用，但要把他们的 `VERIFIED` 理解成代码契约证据，不是本次窗口验收。

| 功能 | Lunamax | 建议 | 原因 |
| --- | --- | --- | --- |
| latency 这个指标整体 | VERIFIED | PARTIAL | 工作台 SQLite、Monitor RTT、legacy JSON 三条权威同时存在。用户工作台表是 0 行。原生窗口未验收 |
| 工作台 latency result / async save / attempt 身份 / stale guard | VERIFIED | PARTIAL | PR #9 的测试和临时 Web 进程证据可以保留。它们没有落在当前用户库，也没有 main 上的窗口操作证据 |
| 工作台 history graph | PARTIAL | PARTIAL | 维持。0 行是当前用户事实 |
| 观察窗口 | SUSPECT | WRONG | 见冲突 3 |
| Evidence | VERIFIED | COMPONENT_READY | 见冲突 4。只读和窗口数学不要降级重做 |
| Recommendation | COMPONENT_READY | COMPONENT_READY | 维持 |
| Auto | UNFINISHED | UNFINISHED | 维持。函数存在不等于有生产调用者 |
| 手动 Select / group 读取 | COMPONENT_READY | COMPONENT_READY | 维持。API 和 Wails 绑定是真入口，正式页面没有 |
| Mihomo client、secret、loopback 策略 | VERIFIED | COMPONENT_READY | 安全和客户端测试说明边界已经写进代码。这次没有连接真实 controller，正式 UI 也没调用 |
| live refresh / catch-up / incomplete / Inspector | VERIFIED | PARTIAL | `frontend/src/stores/timeline.ts` 相对 timeline tag 没有再改，PR #5 的 store 证据仍指向这份代码。当前没有运行中的任务，PR #10 又改过 Timeline 视图，窗口行为未在 main 上复验 |
| Monitor job 创建与四态按钮 | VERIFIED | PARTIAL | 行为代码在 PR #8 之后没有再改 scheduler。PR #10 改过 `MonitorJobsView.vue`（约 90 行）。旧 Web evidence 不能自动覆盖改后的页面。重启不恢复仍然成立 |
| 节点筛选 | VERIFIED | PARTIAL | 工作台筛选是前端对已加载选项的本地过滤。它依赖 cwd 里能读到的节点，本次没有在 main 进程上操作 |
| 机场管理 | PARTIAL | PARTIAL | 维持，并加上「两个 worktree 已是两个 profile 世界」 |
| 加入持续监测 | Snapshot 未单列 | WRONG | 文案是加入，调用是切页 |
| 吞吐 / 现代服务页签 | PARTIAL 或未完成清单 | UNFINISHED | 页签在，正式工作台接口不在。Monitor 的 HTTP 探针不是这个页签 |
| macOS 运行 | 构建矩阵 VERIFIED，运行 PARTIAL/UNKNOWN | 构建 SUCCESS 可保留；运行 UNKNOWN | 本机无法提供 macOS 窗口证据 |

# Architectural Risks

1. profile 跟 cwd、history 跟用户目录。换 worktree、快捷方式或发行目录，节点列表会空，旧样本还在。根目录和 latency worktree 已经是两套订阅。
2. 三套测量权威会继续分叉：legacy JSON 按显示名，Monitor sample 按 profile/node/probe，工作台延迟按 attempt。推荐只吃 Monitor sample，而且只吃活任务的候选集。工作台里新测出的延迟不会自动变成推荐证据。
3. `ModeAuto` 和 `SelectNode` 已经在进程里。自动执行今天没有生产调用者，手动选择已经从 HTTP 和 Wails 暴露。以后若有人把 scheduler 接到 `EvaluateAndAutoSwitch`，缺少的是产品确认，不是底层函数。
4. 任务恢复和保留策略必须一起设计。先持久化并自动拉起任务，而保留仍是 `keep_all` 且没有界面，SQLite 会在几个月的持续采样后变成清理事故。
5. 睡眠唤醒没有补采。24 小时曲线会在睡眠处断开，跳过计数解释不了这段空白。
6. SQLite migration 仍是注释里的 TODO，没有 schema version 表。下一张任务表或统一历史表会踩在幂等 ALTER 上。
7. `gui` 旧服务器还在编译单元里。后续修测速时如果改错文件，会修到没有入口的那套，或把两套行为修得更不一致。

# Product / UX Risks

主工作流在源码里已经能说出一句：从 cwd 订阅缓存选一个节点，做单节点延迟，再单独去建一个不会跨重启的监测任务，时间轴看的是 Monitor 样本。用户界面没有把这句话说清。

同一屏里叠着五套「历史」：工作台延迟图、导航里名为历史记录的 Monitor 时间轴、旧历史矩阵、legacy JSON 批次、折叠区里的旧批量工作台。延迟、吞吐、服务三个页签看起来像一个测量模型，实际只有延迟会打到新接口。

「加入持续监测」和「观察窗口」都在教错误操作模型。推荐没有页面，用户看不到样本不够、任务不在、或数据已超过 5 分钟这些原因。机场管理和订阅 URL 明文展示会占掉测速工具的注意力，而且 URL 经常带 token。

这是信息架构已经开始散的状态。继续加 Controller 页、吞吐图或自动切换，会把三套历史和两套身份模型再浇一层。

# Data & Persistence Risks

- 订阅文件被 gitignore，不随分支走。当前至少有两个互不相同的 `airports.json`。
- 用户 `history.db` 已被带工作台表的程序打开过，所以 schema 到了 PR #9，但延迟行数是 0。临时证据库没有留在这个文件里。
- Monitor 样本是 2026-09-18 到 2026-09-19 的历史，不是正在增长的 24 小时序列。
- legacy JSON 停在 2026-09-17 前后，和 Monitor 样本、工作台延迟表不是同一段使用记录。
- 默认不删除 raw sample。保留 API 存在，产品路径没有调用它。
- 工作台保存失败或进程被杀时，界面可以已经显示结果，而数据库没有对应 attempt。
- `AirportDTO.URL` 会把订阅地址送到 UI。本次没有抄录任何 URL。

# Cross-platform Risks

- Windows：CI desktop build 在 `58d036e` 上成功。本机没有正在运行的正式窗口。根目录 exe 是 2026-09-19 的旧产物。
- macOS：CI 只证明 darwin/arm64 bundle 能编出来。没有本机运行、签名、公证、Intel/Universal，也没有 App bundle 的工作目录证据。cwd 型 profile 在 bundle 里会落到不确定的当前目录。
- 睡眠/唤醒和发行包首次启动目录，两边都没有产品实现。
- Linux CI 是 Go 测试，不是桌面运行验收。

# Things That Should NOT Be Reopened

- 不要把 `4096fb8`、`2a5f64f`、`14d5203` 重新当成待合并工作，也不要为了「看一眼 PR #10」再从旧 worktree 启动后重写 UI 结论。
- 不要重新合并原型 UI，不要把 UiSelect 再做一遍。PR #10 已在 `main`。
- 不要重跑 PR #5 的 gap/catch-up 测试套件来证明 `timeline.ts` 没坏。该文件相对 timeline tag 没有差异。PR #10 只动了时间轴视图外壳。
- 不要重开 PR #7 的只读推荐、候选集隔离、30 小时窗口可达性。当前缺口是没有活任务、样本过期、没有页面，不是那次审查的数学被推翻。
- 不要因为用户库 0 行就重跑 PR #9 的临时订阅 Web 证据。0 行说明证据没进用户库，不说明那次临时库测试失败。
- 不要把 24 小时 soak 从 INCOMPLETE 改回 PASS，也不要为这次规划默认再跑一轮长稳。
- 不要为了核对同一 SHA 再编一次 Windows/macOS CI。2026-09-22 已看到 `58d036e` 的成功 run。
- 不要用「审核者换了」为理由重做已关闭的运行时验收。

# Questions Astra Pro Must Resolve

1. profile 与 cache 的唯一目录合同是什么：用户数据目录，还是显式配置路径。在这决定之前，不要把空节点列表当前端缺陷。
2. 第一版「持续监测」是否必须跨进程重启。如果必须，保留策略和用户确认恢复是否同一批交付。
3. legacy JSON、Monitor sample、工作台 attempt 是长期三套权威，还是收成一套可按项目扩展的记录。推荐以后吃哪一套，要先定。
4. 观察窗口是补上真实 `since/until`，还是删掉现在的时间承诺。图表继续加之前要先选。
5. Controller 页面是否作为 Auto 之前的独立里程碑。Auto 的正式触发者是监测循环、独立调度器，还是一次用户确认。在这之前保持 `EvaluateAndAutoSwitch` 没有生产调用者。
6. 折叠的旧批量工作台和 `gui` 旧服务器，是冻结待删，还是仍算支持入口。这决定修测速时要改几个实现。
7. 按工程规则，延迟闭环和 Monitor 四态要升到 `VERIFIED` 时，最低证据是什么：main worktree 上的 Web API、Wails 窗口操作、真实订阅但不记录凭据、还是跨重启。现在这些都还没同时具备。
8. 本机是否规定唯一允许启动的 `main` 目录和产物，避免旧分支、旧 exe 和另一份 ignored profile 再产生一次「功能退回」的误判。

# Recommendations to Astra Pro

最可靠的事实是 GitHub `main` `58d036e`、本地 `main` 与它相同、当前目录落后 7 个提交且无分叉、PR 全部合并、profile 在 cwd、history 在用户目录、用户库 25/291/0/0，以及推荐和 Auto 没有正式用户路径。

Lunamax Snapshot 的仓库现场、数据计数和「不要把旧 checkout 当成 main」可以当事实底稿。它的状态列不能原样进入路线图。凡是标成 `VERIFIED` 的用户功能，先降到本报告的修正表，再决定要不要补证据。

必须先处理的问题只有两类。第一类是会让后续开发测错对象的：启动目录必须是 `main`，profile 目录必须先定合同。第二类是会在功能变多后直接伤到用户判断的：观察窗口语义、监测按钮语义、三套历史不要再各长一套。

应避免的方向：从 Auto 或新的 Controller 切换开始；把 Monitor 的 HTTP 探针或 legacy Antigravity 说成工作台服务可用性已完成；在保留策略缺席时先做静默的跨重启监测；用显示名历史去填新的 profile 图；为了升状态重跑已经关闭的 PR #5、PR #7、PR #9 和 24 小时 soak。

本报告没有启动应用，没有重跑测试，没有连接外部代理控制器。macOS 真实窗口、系统睡眠后的实际采样空洞、以及 `main` 构建产物里的鼠标路径，仍然是 UNKNOWN。

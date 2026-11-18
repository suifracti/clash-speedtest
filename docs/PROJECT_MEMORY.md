# CST｜项目记忆

开工与全周期规则见 [AGENTS](../AGENTS.md)。范围在 [路线](roadmap-round3.md)，已交付证据在 [进度](backend-round3-progress.md)，版本和能力保留沿用 [version-consistency](version-consistency.md)，不复制新的当前待办表。

## 按用户说法找经验

先读此索引，再读命中的条目正文与实际实现；索引只帮助定位，不替代原因和证据。无匹配时再查项目任务源、相关差异和历史，不能因此断言以前没做过。

| 用户说法或本次触及范围 | 到哪里找 |
|---|---|
| 打了新包仍是旧功能；改构建/包装入口 | L-001；scripts/build-mac-candidate.sh |
| 迁移/合并后功能丢了、换工作树或执行者 | L-002；docs/version-consistency.md |
| 用量不完整、检测失败、取消/保存、历史身份 | L-003；docs/backend-round3-progress.md |
| 切换机场后体检变空或显示上个机场的数据 | L-003；frontend/src/components/AirportReport.vue |
| 下载只有短文本、速度口径/稳态均值/峰值、超时、备用源限流与旧测速结果 | L-004；core/speedtester/download_stream.go；application/workbench_download.go；frontend/src/domain.ts |
| Clash Verge 更新订阅后不想重填链接；关联已有账户 | L-005；application/verge_subscriptions.go |
| 某解锁服务只想测指定地区或排除指定地区；Antigravity 批量风险与连接耗时 | L-006；frontend/src/unlock.ts；core/autopick/autopick.go；frontend/src/components/QueueModal.vue |
| 手动刷新被 30 分钟冷却拦住；订阅代理/直连/物理重试原因、换链接后今日与 14 天用量断掉 | L-007；application/subscription_refresh.go；application/subscription_usage.go；core/subscriptionusage/continuity.go |
| 延迟大批量卡顿或分段；工作台/检查器/报告结论不一致；订阅更新后同名节点 7 天指标丢失、旧批次超过 30 条漏历史；双栈回退后一直走 IPv6 | L-008；core/history/latency_scopes.go；core/history/latency_batch_progress.go；frontend/src/latencyHistory.ts；frontend/src/latencyReport.ts；core/speedtester/physical_family_darwin.go |
| Windows 启动报 build tags 或 WebView2 句柄错误；Node 26 跑前端测试报 localStorage；新数据目录显示本地服务未连接 | L-009；frontend/src/workspace.ts；scripts/prepare-legacy-assets.mjs |
| SQLite 外键/busy_timeout 在连接池失效；Windows 盘符 URI 报错；v11 升级备份与历史清理中断 | L-010；core/history/db.go；core/history/sqlite_uri.go；core/history/upgrade_backup.go；cmd/history-orphans/main.go |
| 同配置 YAML 别名节点在延迟批次被跳过；VLESS pcs/vcn 特殊 TLS 导出失真；推荐/体检漏读旧 revision | L-011；core/speedtester/speedtester.go；core/speedtester/vless_tls.go；application/export.go；frontend/src/nodeQuality.ts |
| 持续监测抢占预算、暂停时无法手动触发、Mac 监测未绕开 TUN；本机通知重复告警或授权失败 | L-012；core/monitor/；core/speedtester/；application/local_notifications.go |
| Netflix DoH 超时吞掉结论、YouTube 误判送中（pagead）、Gemini/Spotify 机房限制、Disney+ 握手 EOF、Gemini/TikTok/U-NEXT/DMM/Prime Video/ABEMA DoH 修复；出口 IP 类型 | L-013；core/publicservice/probes.go；core/publicservice/unlock_ai.go；core/publicservice/unlock_regions.go；core/publicservice/target_ipv4.go；core/publicservice/checker.go；frontend/src/ipKind.ts |
| 自动精选端口被占后 Verge 订阅失效、运行期改设置被覆盖或报告丢用途；入口超 100 地址丢国家；Clash/Verge 本机对照与报告导图 | L-014；application/auto_pick.go；application/auto_pick_subscription.go；core/entry/lookup.go；application/clash_comparison.go；frontend/src/components/LatencyReportImage.vue |
| Windows Clash Verge 命名管道、同名节点换端口变更抵消与非 Darwin 场景推荐 | L-015；adapter/controller/mihomo/dial_windows.go；core/profiles/node_changes.go；frontend/src/recommendations.ts |
| 延迟 Keep-Alive 复用失效、Windows TUN Fake-IP DNS 拖慢物理出口、测量轮次并发锁竞争 | L-016；core/speedtester/speedtester.go；core/speedtester/physical_entry_other.go；core/history/measurement_round.go |
| UI 像 AI 通用模板、只换皮、重构漏功能；每个功能块开工前要读记忆 | L-017；docs/ui-rebuild-20261118.md；Obsidian 项目 UI 研究与 Decisions |

## 默认自动执行：不等用户提醒

日常记录与联动继承 Vault 根 `AGENTS.md` 的“2 项目记录”，全周期复盘见“5.1 全周期复盘与覆盖”。通过[仓库 AGENTS](../AGENTS.md)定位总规则和本项目主源；本页只维护场景索引、具体经验与已有覆盖点。

## L-001 包装成功不等于核心已更新

适用：Mac 构建、换包、用户称新功能丢失或行为仍旧。

原 AGENTS 已记录：曾手工构建前端、漏掉 go build，却把上一包的 core 再包装。这是已知历史教训，不是对当前 App 的故障诊断。

做法：真实候选统一 `scripts/build-mac-candidate.sh`，前端 → macOS 12 production core → 包装，任何一步失败停止；包装脚本单独只处理已有 core。核实际内嵌提交和产物身份，不给未知二进制猜填 SHA。文档更新不打包。

验证边界：构建脚本成功、manifest 静态检查、实际启动与用户体验分别报告；`candidate_audit.py` 不证明业务运行。需要检查已有源码/脚本时回到实际版本，本次只读了约定，未重新打包。

## L-002 代码已合并，不证明旧成果与用户运行版齐全

适用：换工作树、合并分支、迁移版本、换执行者。

原入口记录过 IPOK 后端、VLESS pcs/vcn、Mac 系统代理识别曾漏带并于 2026-10-13 做了迁回/离线核对。该历史报告不能升级为今天所有能力通过，也不能只统计合并 PR 判定无遗漏。

下次核对：远端、工作树/dirty、实际构建和用户运行版分别识别；按既有能力与版本对照检查真正受影响的入口、接线和行为。保留未提交资产，不用 reset 或旧包覆盖未知现场。已有分支成果比较真实内容，不凭 commit 独有就重建功能。

## L-003 部分数据与服务失败不能强行归因

适用：用量头、账号/节点检测、取消/保存状态、历史比较。

依据：进度页 B2.1 接受小数/科学计数并对缺计数保留 null/partial；AGENTS 要求账户隔离、节点 identity/revision 与执行/判据/保存状态分开。HTTP 可达不等于业务成功，缺证据不能当节点坏、零用量或模型可用。

下次检查：用已存记录及相关用例解释真正阶段、条件和未知项；不能为解释失败自动发起全量实网、付费调用或修改运行代理状态。真实个人配置/节点信息只留私人位置，公开记忆写脱敏行为要求。

体检切换机场的异步读取需在启动时固定机场和节点范围，并沿用已有 generation 模式，只让最新请求写回数据、月费、错误和忙状态。旧机场响应晚到会覆盖当前结果，不能因此认定当前机场没有历史。`frontend/src/components/AirportReport.vue` 已处理，回归用例位于 `airport-report.spec.ts`，离线模拟新请求先完成、旧请求后返回。

2026-11-18 同类边界：`home/InspectorLatency.vue` 原只阻止旧请求回写，却没有在切换节点时清除已经读取的 `saved` 历史，新请求等待或失败时会显示上一节点的数值。现在按完整节点范围及显示名切换清除旧历史和变化标记，无节点时也使旧 generation 失效；同节点刷新可保留已有历史。`inspector-latency-round.spec.ts` 已先复现“切到 B 仍显示 A 的 123 ms”再验证修复。

2026-11-18 订阅账本与体检接续：切换机场时读取隔离之外，保存也必须在第一次 await 前固定目标机场和编辑值。原月费保存等待 GetSettings 时切到另一机场，会读到新机场的表单，导致原编辑丢失；现冻结目标和值、合并最新设置、只在仍查看原机场时回写表单，保存期间防重复提交。账户台账进入体检沿当前查看的机场，体检切换同步浏览范围，不改变测量来源选择。用量日/周/月、原始区间与最近观测仍按账户隔离，缺失/部分不填零。airport-report.spec.ts 的延迟保存切换回归先失败后通过。

2026-11-18 报告读取补充：报告目录的 Promise.all 原会因精选或监测一处失败而丢掉其他已读报告，并同时显示“还没有”。现手动/监测/精选分别保留已读结果，单个批次或监测轮次读取失败只说明该部分缺口；未完整读完的一轮不冒充完整报告。混合类型按记录时间排序，错误与真实空目录分开。前三条目录交互回归先复现，监测目录未接入的合成异常路径再次确认嵌套读取问题；31 项报告/历史相关用例、vue-tsc/构建及正常/部分失败页面检查通过。比较与分类算法未变，导图的原数字排版约束继续保留。

2026-11-18 检测进度补充：队列原先将保存失败覆盖到检测结果列，成功但仍保存中的项又会被默认过滤隐藏，磁盘错误还可能被简写成连接失败。现执行、检测结果、保存各自呈现；下载结果列仅忽略保存对颜色的影响，其他摘要仍沿用原保存告警。默认显示进行中与需留意项，全部项目可核对排队/成功项并按 50 项分页；延迟大批次仍使用原轻量汇总，不追加逐项轮询。启动未确认不画进度条，纯保存阶段不显示无效取消，确认原请求、取消保留已有结果及重试保存合同不变。3 条新回归先失败，相关 59 项及构建通过；合成预览实际检查取消和重试保存，没有真实测速或写生产历史。

2026-11-18 全解锁实测后的修正：服务拒绝、地区限制、HTTP 拒绝和缺凭据保留原原因，只有明确传输类结论才套用连接错误缩写。AI 网页解锁历史只写规则通过和实际一致次数，流媒体记录仍提示播放未验证。手动服务和下载轮次在批量检测与报告目录按日期呈现，组合轮次显示全部项目，既有延迟批次入口仍可比较。先读轮次摘要，再按页联接实际结果，沿原 request ID 和完整账户/节点/key/revision 读取；不从时间或节点名推断旧记录的归属。

2026-11-18 检测准备补充：`runPlan` 原直接引用可变草稿，并在入口解析 await 之前既不占用准备状态，也不核对返回后的草稿/生命周期，导致关闭窗口仍启动、重复提交、参数与节点配置在等待中改变。现启动时复制完整节点 scope、参数和服务选择，固定当时的地区适用范围；准备状态单独于实际检测状态，字段锁定，关闭/换计划/退出使旧准备失效。解析返回后重新核对当前节点配置，旧响应不能清空新草稿或原队列。入口解析失败仍留错误且允许明确选定的检测继续；原启动未确认、取消已执行任务、结果与保存重试合同不变。首轮新增用例有 7 条失败，最终 9 条准备/预算用例及完整前端 80 文件/514 项通过。仅合成预览验证准备取消，无真实检测。

2026-11-18 全局查找与显式检测：浏览范围、勾选集合和检测草稿是独立状态。查找应覆盖所有已载入账户并显示机场/账户；打开结果不扩大当前范围或改勾选。详情里明确指定的现行节点可以发起检测，`runPlan` 在准备前后对全量已载入非公告节点校验完整 identity/revision 与 node_key，不能误用工作台范围拒绝范围外的明确选择，也不能因此放宽到旧配置。

## L-004 下载短响应、计速与备用源要分开判断

适用场景包括 HTTP 200 却只收到短文本、连接慢导致下载速度偏低、测速源限流，以及比较不同来源的旧记录。

`76d6e58` 修复短响应假成功。响应提前结束且字节数不足请求量时归为 `response_too_short`，不生成速度，保存最多 200 字符的脱敏 `response_summary` 与 `content_type`。状态码 200 不能单独证明下载成功，旧的 completed 短响应也在读取时按同一口径显示。

`d87cd8d` 将速度统一为 `bytes_read / phase_timings_ns.response_body`，界面换算为 MiB/s，连接及握手耗时另列。旧记录有响应体阶段时读取时计算，没有该阶段就显示未记录，不能拿总耗时补成速度，也不改历史库。`ce0261b` 统一复用 `outcomeLabel` 显示中文结果。

`35e25ed` 将备用源替换为 OVH，按请求量发送 Range 并接受 HTTP 206。当前源返回 403/429 后换另一源一次，记录与比较沿实际目标来源，双源均失败时保留两次原因。旧来源记录仍按其实际来源显示。

入口在 `application/workbench_download.go` 的 `executeWorkbenchDownload`、`downloadSourcesFailure`，传输与采样在 `core/speedtester/download_stream.go`，旧记录显示和计速在 `frontend/src/domain.ts` 的 `downloadOutcome`、`downloadSpeed`、`downloadAverageSpeed`、`downloadPeakSpeed`、`downloadSourceName`，队列文案在 `frontend/src/measurementRounds.ts`。对应交付见[进度页 2026-11-02 下载各节](backend-round3-progress.md#2026-11-02-下载短响应)。

2026-11-15 补充（下载包体积、超时拆分与稳态速度口径）：
- **默认测试包与预算**：工作台默认下载测试包调高至 `20 MiB` / `15s`（`core/speedtester/download_stream.go` 的 `DefaultDownloadStreamBytes = 20<<20`、`DefaultDownloadStreamTimeout = 15*time.Second`；前端 `workspace.ts` 默认与 `TestPlanModal.vue` `15 秒` 选项），流读取缓冲区由 32 KiB 扩至 64 KiB。
- **握手/连接与正文传输分离计时**：`download_stream.go` 不再共享单个 `http.Client.Timeout`，改为 `connectTimer`（`localClient.Do`）与 `bodyTimer`（进入 `response_body` 后启动）各自独立享有 `MaximumDuration`，同时将 `application/bounded_deadline.go` 的 `newWorkbenchDownloadLifecycleContext` 上限倍增至 `2 * MaximumDownloadStreamTimeout`、任务级超时传 `2 * timeoutSeconds`，防止冷启动建连/TLS 慢挤压有效响应体传输时间。
- **稳态 / 峰值 / 全程均速并列展示**：`frontend/src/domain.ts` 将主速度 `downloadSpeed` 调整为优先取 `response_body` 后段稳态速率（当 `response_body >= 2s` 且有效采样点 $\ge 2$ 时，截取覆盖尾部至少 1.0 秒且占比 $\ge 40\%$ 总字节数的采样段去除 TCP 慢启动预热），并用 `downloadAverageSpeed`（全程均速 `bytes_read / response_body`）与 `downloadPeakSpeed`（采样切片峰值）分别呈现于工作台、`NodeInspector.vue` 与 `MeasurementResults.vue`，不回改旧历史库。

2026-11-18 补充（末尾停滞与推荐排序）：后端采样器仅在新增字节时写采样，最后一条采样的时间不一定是正文传输结束。原 `downloadSpeed` 用最后采样当尾部终点，导致 4 秒收到 10 MiB、再停滞 11 秒的 `time_limit` 记录仍展示停滞前的高速，并可能排在持续下载节点前面。现以 `response_body` 完整时长和最终 `bytes_read` 作为尾部窗口终点；保持去慢启动的窗口条件、全程均速、实际采样峰值和无采样旧记录回退，不补造采样或改历史库。离线速度用例与推荐排序用例先复现后修复，原慢启动用例保留；工作台、检查器、记录和推荐共同复用该函数。

2026-11-18 补充（同条件上一条与预算版本）：后端 `PreviousWorkbenchDownloadAttempt` 除冻结规则和实际测速源外，须匹配实际 `network_path` 的 method/interface/dns_mode/dns_bind_verified/socket_bind_verified/address_family/address_source 及节点协议类型，避免 IPv4 结果误拿较新的 IPv6 结果比较。新下载快照 `rule_version=4` 标记连接与正文独立预算；旧 v3 混有预算分离前后的记录，保留原样，不推断或重写历史。计划明确每节点连接与下载合计至多两倍所选时长（含换源），历史条件区分新旧规则。

2026-11-02 历史证据边界：当时保存的四次实网结果均为直接选择 OVH，HTTP 206 且各收到 10 MiB。403/429 后自动换源由离线测试覆盖，这四次实测不能替代该路径的现场验证。原生截图只确认原生选源页面，不能当作四节点结果截图。11-15 证据见后续增量覆盖；11-18 本次仅离线验证，没有重新测速或核实当前运行包。

2026-11-18 修复（`cfe04fc`）：`normalizeUnverifiedPathFailure` 保留 `local_path_unverified` 作为归因状态，并保存实际 `end_reason`、`failure_phase`、`error_class` 和原错误说明。工作台、队列与记录的共用结果文案同时显示观测到的失败和出口未确认。历史已有阶段也可显示；旧错误原文未写入时无需补猜。未知出口无有效速度，单次跨源差异也不足以认定限流。

2026-11-18 核查与修复（`7dc34ec`、`322d8f8`）：固定小字节上限会结束仍在提速的传输。同源测试需核最终测速源，单次较长传输也可用实际累计采样比较前段和剩余区间。Cloudflare 自己的测量实现按持续时间决定是否增大请求体积。[Cloudflare Speedtest](https://github.com/cloudflare/speedtest/blob/main/README.md)

定量测速以 `downloadReading` 显示实际计算口径。正文不足 2 秒标为短测均速，取尾部窗口时标为后段估算，其余为全程均速。单节点定量预填 50 MiB，批量 20 MiB，沿用 15 秒正文默认值和用户可编辑上限。已有结果达到已知上限后可打开同源加量计划，启动仍需提交。检查器趋势按实际源、上限和其余条件分段。自动换源的结果 JSON 保留初次 HTTP 403/429 及实际测速源的状态，老记录缺初次状态时照实读取。

2026-11-18 自适应下载 v5 改为按实测速度决定流量。工作台新计划默认 `mode=adaptive`，以 OVH 的长响应采样。首字节后预热 1 秒，以同一响应连续三个约 1 秒窗口的实际字节和单调时钟计算速率。三者各自距中位数不超过 10% 时停止，保存 `speed_stable` 与中位数 `stable_speed_mbps`。低速稳定也能结束。超过两秒没有形成采样窗口时重新积累稳定窗口。正文连续 5 秒无字节时收尾，持续波动仍受可调的正文时限约束。

自适应模式按实际数据报告稳定速度或未稳定均速，分别标明已稳定、停滞、到时和测速文件传完。有效正文耗尽但未达到稳定条件时保留均速；短错误页和空响应没有有效速度。Cloudflare 要求请求指定响应大小，作为可选源或 403／429 换源时可能先传完。列出的大小属于服务器响应，自适应读取按实际停止条件终止，换源独立采样。后台旧调用、定量重测和 v4 以前历史继续使用原规则；不迁移 SQLite。自适应和定量结果以独立规则版本显示，未稳定读数仅供参考。入口见 `DownloadStreamOptions.Adaptive`、`download_stability.go` 和 `WorkbenchDownloadTestRequest.Mode`。

## L-005 从 Clash Verge 同步订阅时保留账户关联并支持同步名称

适用：本机 Verge 已更新订阅或修改链接，或首次在“数据与备份”初始配置页／“机场与账户”页点击“从 Clash Verge 导入／同步”。从标准配置目录的 `profiles.yaml` 读取 remote 项及其本地节点缓存，以 `uid` 关联 `Subscription.VergeUID`，后续按该绑定更新原 CST 订阅 ID。首次只对唯一完整 URL 相同的账户建议关联；尚无机场时默认预选“新建机场”，已有账户且链接不同时由用户选择已有账户或新建，名字相同不自动合并；每行提供可编辑的“同步名称”输入框与“同步名字”开关（新建机场默认使用该名称，关联或同步已有账户时勾选或修改名称即同步更新订阅名与单订阅机场名，Verge 侧改名时标记 `name_changed`）。

`core/profiles/verge.go` 只输出缓存的 `proxies` 根，保留节点标量类型；用量缺字段保留未知。`application/verge_subscriptions.go` 复用 `refreshSubscriptionWithFetchLocked` 的 prepared fetch，记录 `fetch_path=verge`，沿用节点变化、失败清除、缓存和快照提交；本机读取不占用 HTTP 429 冷却。未初始化存储（`airports.json` 不存在）下可直接导入首个订阅并完成初始化，若导入失败则删除新建 `airports.json` 保持 `needs_choice`；已有存储下导入失败恢复原订阅配置与原缓存。

链接变化后的用量仍由既有连续性读取规则衔接，`usageAccountKey` 沿 URL 哈希。普通 API 摘要只传 UID、名称、时间与关联状态，完整 URL 留后端。Wails 和 Web 接口均已接入 `AirportsView.vue`、`DataView.vue` 与 `ClashVergeImport.vue`（未初始化状态下导入成功后自动 `w.boot()`）；新建、改链接复用原账户、未初始化导入与失败回退、账户选择均由临时数据单测覆盖。

2026-11-18 导入界面接续：按“Clash Verge 来源 → CST 目标账户”分别显示来源名、更新时间、目标位置和账户名称。自定义账户名不能反写到来源名，否则用户会误以为本机 Verge 文件也已改名。UID/准确 URL 关联规则不变；已绑定目标锁定，无匹配时不按名称猜账户。导入接口成功就记录绑定与成功结果；后续列表读取失败单独说明，提供只重新加载账户的入口，不要求重提导入。初始化导入在 boot 前通知成功，避免页面切换吞掉结果。合成场景验证了原账户同步、新建机场及窄窗口，不代表真实 Verge 绑定验证。

## L-006 解锁地区正反向规则与 Antigravity 批量风险及耗时归因

适用：配置某项解锁只测指定地区或排除指定地区；开启/批量勾选 Antigravity；解释 Antigravity 队列里的模型可用与代理建连耗时。

`4bb77e7`、`0c344f0`、`de31a46` 在现有 `unlock_service_regions` 字段上支持每服务独立地区规则：默认无前缀列表为“只测所选”，`!地区码`（`excludedRegion`）为“排除所选”，空列表为不限地区（后端无需新增字段）。`unlockServiceSkipReason` 遇到只测列表跳过未选地区（“不在该项设定的检测地区内”），遇到排除列表只跳过所选地区（“已排除此地区，不检测”），入口与节点名均无法识别地区时仍继续检测。空列表无法保存 `!` 前缀，`UnlockRegionEditor.vue` 用 `pendingExclude` 暂存模式并在只测/排除切换时保留已勾地区。

`b690d3d` 将 Antigravity 从自动精选（`core/autopick/autopick.go` 的 `defaultCatalog`）与工作台分组/全选默认目录（`defaultSelectableServices`）排除，只允许显式手动勾选。`7cb86bb` 取消此前每次最多 3 个节点的硬拦截，超过 3 个节点时由 `antigravityBatchRisk` 在计划弹窗与工作台显示多 IP 风控与额度消耗提醒，不再禁用开始按钮或在 `runPlan` 拒绝提交。`8b794ea` 在 `QueueModal.vue` 中把“已处理”与“确认模型可用（仅 `matched` 且真实模型回答）/地区拒绝（`streamGenerateContent` HTTP 400）/未确认”分开，并标明 `details.proxy_connect_ms` 是已结束调用（含同时发起的无凭据 Cloudflare trace 与 Google 请求）的代理建连累计耗时，不能归因给单次请求。上述逻辑由前端与 Go 离线单测覆盖，未发起真实付费模型调用。

## L-007 订阅抓取路径、手动冷却豁免与换链接用量连续性

适用：解释订阅刷新走了哪条网络路径或为何失败；手动刷新被间隔限制；修改订阅链接后节点缓存、失败状态与 14 天/今日用量的衔接。

`4952869` 将每机场 30 分钟间隔（`usageRefreshAirportGap`）仅作用于自动刷新，手动刷新（单机场、全部、行内、仅刷新用量）不再受 30 分钟冷却限制；自动刷新仍按 2 小时周期并参考最近一次手动刷新时间。`c747674`、`7e546be`、`91d9d50`、`9e9f8bf` 将 `fetch_path` 细分为 `proxy`、`normal`、`physical`（及本地导入的 `verge`），旧记录 `direct` 标为路径未区分；普通网络（或代理）传输层失败时复用入口 DNS 的物理网卡客户端重试一次，物理重试仍失败时合并保存两段脱敏原因（总长上限 200 字，避免后一段被截断），HTTP 429、取消与超时不盲目重试。`d69b53f`、`44a226b`、`480101c` 在 `SubscriptionOverview.vue` 与刷新弹窗中将刷新任务完成时间与订阅 `last_attempt_at` 合并取每订阅唯一最新状态，避免旧手动失败覆盖后续自动成功，并展示保存的路径与原因。侧栏与工作台节点计数统一复用 `airportViewing.ts` 过滤公告项。

`67d5392` 在 `UpdateSubscription` 链接变化时直接复用单订阅刷新（旧缓存参与节点增删改比较，成功清除旧失败并写 `subscription_update`，失败恢复原链接与设置，仅改名/备注不抓取）。`36e3577` 在 `core/subscriptionusage/continuity.go` 的 `ContinueAccounts` 中按只读方式衔接换链接后的用量：当同机场旧 `AccountKey` 已停止记录、新旧 `Total` 一致且累计已用流量不减（多候选取最小非负增量）时，将旧快照归入新 `AccountKey` 供 14 天趋势与今日用量连续计算，不改写数据库与 `usageAccountKey` 生成规则。

2026-11-18 节点数量补充：已载入节点的机场总数必须采用实际清单并集，不能取多个账户数量的最大值。侧栏/范围条、管理范围、标题和查找共用与合并列表相同的 `uniqueAirportNodes`：同机场 identity 与显示名相同才合并，别名、不同端点、跨机场及缺少 identity 的条目分别保留；过滤公告。账户自身的历史、冻结 scope 与配置数量不合并。只有未提供实际清单时，元数据数量才保留原有估算口径。合成的两个不同账户各 32 个节点，标题应与列表同为 64，而不是 32。

2026-11-18 编辑范围补充：已有账户的“编辑机场资料”只提交机场名称、入口与备注，不读取或重交第一个账户的完整 URL。账户编辑明确所属机场，空账户备注保持空；仍支持旧式仅机场 URL 的编辑，真正空机场可改资料。`UpdateAirport` 的脱敏链接拒绝条件只适用于非空 URL，空 URL 继续表示仅改资料，临时数据测试确认多账户 URL、User-Agent、Verge UID、缓存不变且不发起抓取。读取完整来源时先开窗口，支持取消/重试，旧响应不能替换后开的草稿；保存前固定值、锁定表单和重复提交，提交成功与列表刷新失败分开反馈。创建成功定位新机场，不被旧搜索条件藏住。

单账户“请求设置”重新接回现有 `SetSubscriptionUserAgent` / Web user-agent 接口。留空使用默认值，只影响下次获取这个账户的订阅；保存只更新对应账户字段，不调用节点或用量刷新。不是全局代理、VPN 或系统客户端设置。普通 DTO 只展示脱敏 URL，完整源仍只在明确编辑请求中读取；本轮没有动真实来源、凭据或生产数据。

2026-11-18 日常管理能力恢复：对照 `70f25a0^` 的 `AirportMaintenancePanel.vue`，发现新界面只保留提醒日输入，却漏了待办、标记/撤销本月完成、下次更新/最近结果，以及公告入口的发现与保存。现在台账直接显示这些状态，按本机日历月判断完成，跨月自动重新到期；入口只解析已缓存的本机场公告，去重并保留账户归属，加入入口不会访问网址。账户台账应从全部已载入节点找本机场公告，不受工作台测量范围影响。独立更新计划与全局自动更新分别说明，旧自定义间隔不丢失；保存冻结草稿、失败原地重试，提交成功与列表刷新失败分开。维护编辑器在窄窗口保留固定底部操作。

删除来源须在配置持久化成功后才清节点缓存。旧顺序在 Windows 配置文件只读时会返回删除失败但缓存已经消失，临时目录中对账户/整个机场均复现；现先保存配置再清缓存。成功删除只移除对应来源和缓存，SQLite 历史及其他账户不变，既有监测不一并删除或立即取消。UI 删除前读取并列出关联任务，未知状态明确显示，失败留在确认窗口；提交成功即移除已知旧条目，节点和任务列表分别重读，局部读失败不要求再次删除。最后一个账户移除后真正空机场计为 0，不能用 legacy 默认账户兜底；有旧式 URL/缓存的机场仍保留兼容。`account-maintenance.spec.ts`、`airport_delete_test.go` 覆盖这些合同，Windows 只读故障注入仅在 Windows 执行；历史保留测试跨平台。

## L-008 延迟单批次轻量进度、同名节点跨配置连续性、三处结论对齐与双栈回退

适用：大批量延迟测速进度与完成拉取；订阅更新后同名节点的 7 天指标与配置分界线；工作台、检查器、报告的延迟结论不一致；Mac 物理双栈回退后地址族固定问题。

`4657444`、`ee35394`、`ccfb073`、`99a57f8` 移除前端 200 节点分段，每轮延迟作为单批次提交（后端上限 10000，默认并发 16）；运行中由 `core/history/latency_batch_progress.go` 的 `GetLatencyBatchProgress`（`GET /api/workbench/latency-batches/{batch_id}/progress`）只读汇总计数，SSE 批次事件不携带全量节点明细，批次结束后一次读取完整结果并弹出报告。`f9903a6` 在 `application/workbench_latency_batch.go` 中将内部取消记为 `context_cancelled`、超时记为 `deadline_exceeded`，仅用户点击停止记为 `user_cancelled`。

`059fd63`、`2f08443`、`fb6abca` 在 `frontend/src/latencyHistory.ts` 的 `readLatencyHistoryWithConfig` 中汇总同订阅账户（`profile_id`）同名节点跨 `node_identity_key` / `config_revision_key` 的历史轮次：工作台 7 天响应率、波动和中位延迟默认保留同名节点的跨配置历史参与显示、排序与筛选（`LatencyChart.vue` 在新配置首次测速处画竖线标记“配置已变化”），检查器同时通过 `latencyHistorySplit` 区分 `current` 与 `old` 轮次；节点改名不猜测合并，底层历史 scope 不改写。同机场下不同订阅账户不能只因同名合并。

2026-11-18 修正发现旧身份的入口：原来读取全局最近 30 个批次再永久缓存，导致更早批次、无批次的已保存记录、仅监测的旧身份及打开应用后的再次换入口漏读；失败还会缓存为空。现由 `core/history/latency_scopes.go` 直接查询已保存延迟与 RTT 样本，以准确 `profile_id`、显示名和 `[since, until)` 时间窗返回完整 `node_key/identity/revision`。时间资格沿原始样本判断，跨上界结束的轮次仍保留窗内采样；预检未执行沿结束时间。新增 Web `GET /api/history/latency-scopes` 和 Wails `ListLatencyHistoryScopes`，前端每次历史读取重新发现范围，错误显式传给既有界面，去重和变化标记使用完整范围而非仅 revision。复用原分页读记录，不迁移库、不修改历史或节点身份。SQLite/HTTP 临时数据测试覆盖账户、改名、时间边界、重复范围；前端覆盖旧身份、读失败恢复、后续新记录和 legacy revision 冲突。

`f9903a6`、`4febe64`、`6a3f74e`、`8845c67` 统一工作台（`homeModel.ts` 的 `latencyVerdict`）、检查器（`inspectorLatency.ts`）与批次报告（`latencyReport.ts`）的判定优先级：单站 6 次全失败且其余站有响应优先判为橙色“部分站点不通”（`partialLatencySites`）；响应比例统一用 `latencyResponseUnstable`（不足 5/6 为“不稳定”，其余有失败为“偶有失败”）；稳定延迟超过 `latencySlowThresholdMs`（1500 ms）统一判为“偏慢”；失败原因展示具体超时/被拒绝/被重置/被关闭，HTTP 状态码 ≥300 计为已连通并单独统计站点拒绝。`ff4b28b` 在 `core/speedtester/physical_family_darwin.go` 的 `DialContext` 中仅在 IPv4 或 IPv6 实际连接成功后才置 `settled=true`（IPv6 成功时才置 `selectedIPv6=true` 并关闭 IPv4 适配器），双栈均失败（`ipv6_failed`）时保留 IPv4 供后续采样继续先试 IPv4。

2026-11-18 工作台七天摘要按需读取。普通浏览只读取当前页，显式按七天响应率或波动排序、按响应率过滤时读取当前机场范围全部节点。各页共享读数截止时间，列表按账户、身份、revision、node_key 和准确名称缓存汇总值，详情仍按原接口读取原始轮次。工作台共用六个节点读取位置，翻页或离开时不再启动已不需要的读取，刷新后丢弃旧返回。全范围读取结束才应用排序/筛选，失败单列；筛选待定时允许操作明确勾选的节点，按筛选范围启动检测等确认后开放。合成 256 节点、每页 20 条时，范围发现首屏由 256 次降到 20 次。更多证据见 UI 重构记录。

## L-009 Windows 平台适配、本地构建、Node 26 测试与未初始化数据页连接状态

适用：Windows 环境拉起桌面端或 Web 服务、连接本机 Clash Verge 控制器、读取 Windows 系统代理与物理网卡、运行前端 Vitest、首次打开空数据目录（尚未初始化存储）看到侧栏状态异常。

Windows 下编译或运行 Wails 桌面端需显式传入 `-tags production`（`.github/workflows/ci.yml` 与 `run.ps1` 已默认带上；构建前需有 `frontend/dist` 与 legacy embed 资产），否则落入 Wails 默认的 `app_default_windows.go` 弹出 build tags 提示框；在无交互或沙箱隔离桌面（如 `WinSta0\exebox-*`）中，`WebView2` 的 `CreateCoreWebView2Controller` 会因窗口句柄跨桌面报 `80070578: Invalid window handle`，此时可用 `--web --port 8999 --listen 127.0.0.1 --browser none --data-dir <dir>` 启动本地 Web 服务调试，真实桌面双击 `build/bin/clash-speedtest.exe` 独立验收。`frontend/vite.config.ts` 已自动为未带 `--localstorage-file` 的 Node `v25/v26` 进程注入按进程隔离的临时文件路径（`cst-vitest-localstorage-${process.pid}.json`），并在 Windows 下设置 `fileParallelism: false` 避免多 worker 并发争用同一 SQLite localStorage 文件报 `disk I/O error`，直接运行 `npm test` 即可通过。

Windows 与跨平台运行边界：`application/clash_comparison.go` 在 `auto` 模式且默认 `/var/run/clash-verge-service/users/<uid>/verge-mihomo.sock` 不存在时，通过 `core/profiles/verge.go` 的 `LoadVergeControllerConfig` 自动读取本机 Clash Verge `clash-verge.yaml` 与 `config.yaml` 中的 `external-controller-pipe` / `external-controller-unix` / `external-controller` 与 `secret`（Windows 下优先连接 Named Pipe `\\.\pipe\verge-mihomo-...`，并由 `adapter/controller/mihomo/dial_windows.go` 在 Service Mode 下自动从 `-sidecar-release-` / `-sidecar-` 回退到 `-production-` 管道）；`core/profiles/fetch_network_windows.go` 从注册表 `Internet Settings` 读取 `ProxyEnable` 与 `ProxyServer`（支持统一 `host:port` 与分协议 `http=...;https=...`）接入订阅抓取的 `system_proxy` 策略；`core/speedtester/physical_entry_other.go` 将非 Darwin 平台的 `ResolvePhysicalHost` 与 `NewPhysicalHTTPClient` 绑定到 `AutoDetectPhysicalInterface()` 检出的物理网卡（非回环地址使用 `mihomo/component/dialer` 绑定接口，显式置 `Proxy: nil`），并过滤 `198.18.0.0/15` 与 `fdfe:dcba:9876::/64` Fake-IP；`adapter/desktop/notifications.go` 在 Windows 下使用 `ms-settings:notifications` 打开系统通知设置。

`frontend/src/workspace.ts` 的 `boot()` 此前在 `setup.state !== 'ready'` 时提前返回而未调用 `events(...)`，导致新数据目录停留在“数据与备份”初始化页时侧栏一直显示“本地服务未连接”。现已抽出 `subscribeEvents()` 在 `api.setup()` 成功后立即建立事件连接（非 `ready` 状态下忽略业务事件刷新，初始化成功后直接复用同一连接），并在 `dispose()` 中重置 `unsubscribe` 与 `connected`。

2026-11-18 启动与访问恢复补充：首次读取不能在 `setup=ready` 但节点清单尚未返回时就显示空工作台；清单读取失败也不等于零节点。`booted` 与 `nodesReady` 分开表示启动已结束和清单已知，首次失败进入可重试的连接页，已载入工作台的普通刷新保留内容。待初始化时数据入口与事件连接仍可用；初始化后直接指向添加订阅，空工作台不再先要求打开空选择器。认证期间导航与旧操作窗口暂时不可操作，登录表单保留到读取结束；原队列/已接受请求与结果保留，不自动重提检测。

`boot()` 并发调用合并为同一读取；认证、setup、清单/设置/服务/任务/批次、事件回调和启动用量补读沿现有 lifetime 检查，退出/认证失效后的旧响应不覆盖重新打开的工作区。恢复终态延迟批次时，读取完整批次之后同样再核生命周期，旧确认响应不能重新加入批次或改变新会话的忙状态。认证规则、凭据存储、后端初始化与生产数据不改；合成登录只使用专用预览的虚构口令和内存状态。

## L-010 SQLite 连接池 Pragma、Windows 文件 URI、v11 升级门禁与事务清理

适用：修改 `core/history` 数据库连接、迁移、升级备份、孤儿检查或历史保留清理。

`56b02e9` 在 `core/history/db.go` 中通过编码后的 DSN 为连接池每个连接设置 `foreign_keys`、`busy_timeout` 与 `synchronous`，避免仅在首个连接执行 pragma 导致并发连接外键级联失效；`cmd/history-orphans`（`core/history/orphans.go`）以 `mode=ro` / `query_only` 只读检查孤儿记录，绝不自动改库。`1dec197` 在 `core/history/sqlite_uri.go` 中将 Windows 盘符路径规范化为 `file:///C:/...`，防止 `C:` 被解析为 URI authority；2026-11-15 进一步将 `core/history/backup.go`（在线备份源与临时快照）和 `core/history/upgrade_backup.go`（v11 回滚校验）中遗漏的直拼 `&url.URL{Scheme: "file", Path: ...}` 统一收敛到 `sqliteFileURI`，修复 Windows 下导出备份与迁移旧版历史报 `invalid uri authority` 的问题。

`91afa7b`、`114296f`、`897ebc0`（`core/history/upgrade_backup.go`）要求既有 v11 库在执行 DDL 升级前必须先完成经校验的 WAL 备份（Windows 下不支持安全私有根目录权限时直接阻断 DDL 升级并保留原库），备份清理失败时显式暴露并隔离；`core/history/backup.go` 的 `BackupTo` 在创建临时备份前显式校验目标目录写权限位（`0o200 != 0`），确保 Windows 下只读目录（`0500`）同样 fail-closed。`fc16f63`、`3e3736e` 将工作台与监测历史清理放入事务并感知取消，清理已终止但缺 `finished_at` 或孤立的 `interrupted`/`persistence_failed` 轮次，同时保留进行中记录。

2026-11-18 容量与清理界面补充：旧 MonitorRetentionPanel 的 SQLite/WAL/shm 占用、阈值与保护状态曾在重构中漏掉，现回到“历史容量与保留”；读取失败保留上次读数并标旧，未提供字段保持未知，不把数据库文件占用当磁盘剩余空间。监测页策略入口直达这一分类，修正旧两列样式在分类切换后仍把正文挤成半列。保存设置不会删除历史或立即启动监测；按当前选择预览是单独动作，冻结 cutoff，未知数量不可确认、已知 0 条不可执行。后端按 500 条一批提交，后续失败可以已经删除一部分；Web 500 JSON 的 partial/counts 不能只提取 error 丢弃，Wails 错误会丢掉返回值，故 ApplyRetention 用 CST_RETENTION_ERROR 封装同样的部分完成事实。前端保留数量，异常后使旧预览失效，须按同一 cutoff 重读剩余范围再确认，不能沿旧数字直接重试或关闭后声称记录未变。清理后重读容量，读取失败不覆盖成功/部分结果；文件未缩小、保护仍生效都照实呈现。不变更删除谓词/事务/调度。临时 SQLite 在第 3 批失败，核实已删 1000、剩余 200 与桌面错误载荷一致；前端新增 10 项回归、完整 87 文件/589 项通过。DataView 的备份导出成功与随后状态刷新失败亦分开；备份完成时间在最终落盘/下载阶段的语义仍需后续临时故障核对。

2026-11-18 备份完成边界续查（上述待核对项已处理）：临时 ZIP 生成成功不等于最终输出成功。原 ExportDataBackup 在生成阶段更新 last-backup-at，桌面最终 rename 失败或 Web 断开/短写/取消后仍出现新时间，临时目录测试先复现。现生成只负责 ZIP/WAL 快照，桌面 Sync/Close/rename 全部成功后才记录，Web 完整交给 HTTP writer 且请求未取消才记录；Web 输出不证明浏览器已落盘。沿用原归档布局、元数据 v1、排除项与旧时间文件，不重写历史记录。时间原子写入，写入/读取错误用独立 last_backup_error 呈现；已导出的文件不会因记时失败被删除，原生错误明确文件已保存，Web 保持有效 ZIP，不在尾部追加 JSON。读取不到 setup 是未知，没有时间文件是暂无记录，不能推出从未备份。前端 2 项新增回归先失败，相关 31 项、构建通过；Go application/desktop/web 全包及 vet 通过，WAL 恢复/取消/既有文件保护契约保留。浏览器仅检查合成提示与 640px 布局；未生成生产备份、验证原生对话框或用户视觉验收。相关实现 application/data_backup.go、adapter/desktop/native_ui.go、adapter/web/server.go。

## L-011 节点身份与配置修订解耦、YAML 别名归一化、VLESS 特殊 TLS 与导出保真

适用：节点配置比较与去重、延迟批次未执行排查、推荐/体检历史读取、VLESS 特殊 TLS 参数及 Clash 配置导出。

`cbd05f2` 修复 `core/speedtester/speedtester.go` 中 `buildProxyServerPortKey` 对 YAML 解析出的嵌套 `map[any]any` 直接 `json.Marshal` 失败导致 `EquivalentProxyConfiguration` 恒为 `false` 的 bug：归一化嵌套 map 后，同配置别名节点正常合并去重，不再被误判为“多个 TLS/连接配置”而在批次中整项跳过。`415d739`、`63a436b` 将公告/余额提示条目与真实节点分开，避免公告别名吞掉同配置真实节点。

`7fec4ec` 在 `core/speedtester/vless_tls.go` 與 `application/export.go` 中恢复 VLESS `pcs`/`vcn` 与 insecure 别名解析及私有 TCP TLS 校验，证书约束计入 `config_revision_key`；未准备好的路径拒绝静默忽略证书约束，普通 Clash/Mihomo 导出无法保真时整次拒绝，且导出时剥离内部 `_cst_` 运行时字段（`fb2511f`）、同机场缓存节点先去重防误报歧义（`db86ef6`）。`911db84`、`7255be4` 在推荐页与机场体检中通过 `ListNodeHistoryRevisions` 聚合同一 `node_identity_key` 的历史 revision（6 并发限流），避免配置微调后漏读旧记录。

## L-012 持续监测预算准入、物理网卡绕 TUN 与本机通知去重

适用：24/7 持续监测调度、容量保护、恢复准入、Mac 监测绕开本机 TUN、桌面原生通知。

`8e61026`、`6780a3f`、`d0269c2`、`9c008e5`、`2de0518`、`b0fddba` 在 `core/monitor/` 中建立按键集分页的原始样本库、容量阈值保护、持久化全局共享预算与采样分档准入；暂停状态下仍允许手动触发单次监测（`d708035`），预算调整或启动恢复时保持队列准入与配置阻断语义（`4b45e00`、`02f1e6e`）。`6627e70` 将 Mac 持续监测的 DNS 与 socket 统一走物理网卡客户端（检测后关闭连接与代理），仅当 DNS 与 socket 均核验通过时写入 `tun_bypass_interface`；`core/monitor/runner.go` 在收到 HTTP 响应但 Windows 计时器刻度内耗时为 `0` 时将 `Latency` 保底为 `1ns`，避免回环或极快响应被误判为未测得延迟。

`6128b16`、`fa74fc6` 在 `application/local_notifications.go` 中支持故障率、低余量、到期三类阈值提醒（包含 `completed_with_issues` / `completed_with_save_failures` 批次），仅在设置变更与每 5 分钟轮询检查，相同告警本机去重、恢复后才可再提醒；勾选开启时先调 `requestAuthorization`，系统拒绝则保持关闭并提示打开系统通知设置。

2026-11-18 监测 UI 核对补充：首次巡检/精选读取失败必须显示状态未知，保留读数的后续失败标明旧状态，不能沿默认布尔值写“未启动/关闭”。保护暂停时 `config.enabled` 仍可能为真，停止按钮须允许关闭已保存计划。状态轮询与节点最后检测读取沿 generation 隔离，保存/停止或切节点后，旧响应不得覆盖新状态。自动精选保存时固定提交快照，等待期间继续编辑的表单保留；未成功读取配置前不可用默认值覆盖保存。离线回归先复现以上边界，相关 73 项通过。创建监测仍按独立订阅拆任务，部分成功续建不重复，冻结范围不随订阅更新扩张。

2026-11-18 监测时间带补充：账户最新一页先按节点过滤，再拿过滤后最早时间判断分页边界，会把无匹配或较早未读时段误报为空白。边界应来自原始页；跨越边界、以及边界同时间戳仍可落入下一页的半小时格都标“未完整读取”，不能据部分响应涂满成功色。样本按冻结账户/key/identity/revision 匹配；同配置其他任务样本可能计入，界面明确口径。首次失败显示未知，同范围刷新失败保留上次成功读取的时间窗，换范围清旧图。不增加读取页数；29 项相关测试及构建通过。

2026-11-18 监测配置迁移补充：对照 `70f25a0^` 的旧 MonitorTransfer，发现重构后任务导入/导出和监测目录入口丢失。现由 `MonitorView → MonitorTransfer` 恢复，仍用 `speedtest-monitor` v1 文件；允许旧 light/service/heavy 和现有四站/六站方案，不改冻住的范围。导出只复制任务创建字段与冻结节点上下文，不从当前节点列表补写 revision，不包含 URL/密钥、任务 ID、历史、运行或随应用恢复状态。导入先检查文件上限/参数/完整上下文，再按账户/key/identity/revision 核对已有缓存；同名、同身份的其他账户不能替换。预览匹配不代表后端一定接受，重复配置仍由后端拒绝。用户明确选择后再核对缓存；部分成功立即保留，新提交只处理未确认新增的所选项，所有成功新建保持停止。Web/桌面共用文件保存的取消语义，监测目录分别走既有 Web kind=monitor / 新桌面 OpenMonitorDataFolder。26 项文件/交互回归及既有创建持久化/停止恢复契约通过；合成浏览器核对 3 项配置、1 项 revision 不匹配、2 项分次成功和导出 4 项。未导入真实任务、未启动监测或操作生产库。

## L-013 解锁探测判据（Netflix / YouTube / AI / Disney+）与出口 IP 类型多数票

适用：维护 `core/publicservice` 解锁规则、排查误判送中或 DoH 超时、解释出口 IP 类型（机房/家宽/移动、原生/广播）及机房限制提示。

`4327e3d`、`28fcbe4`、`38eb812` 对齐并修正解锁判据：Netflix 在 `core/publicservice/checker.go` 与 `probes.go` 中将 Fast.com 预检提到节点 DoH 之前（出结论即记 `target_dns_skipped=fast_api_decided`，避免节点 DoH 超时吞掉结论），地区优先取 `client.location.country`、`targets[0]` 仅作后备，预检未决再走双片目（`70143836`/`81280792`）；YouTube Premium 请求 `/premium?hl=en`，排除页面自带 `google.cn/pagead` 遥测避免误判送中，非 CN 的不可用标橙色“不向该 IP 提供 Premium（常见于机房/代理 IP）”；ChatGPT 用 trace `loc` + 官方 compliance，Claude 用 trace `loc` + `claude_regions.json`，Gemini 识别 `,2,1,200,"CHN"` 与 `google.com/sorry`（标橙色 `verification_required` 计入已测分母但不计可用）；Spotify 代理注册拒绝记橙色“限注册”；`disney_unlock` 因多节点 TLS 握手均 EOF 无响应，已停用请求并直接返回“暂不支持”（`4e1ab98`）。

`3d0bd9f` 在 `frontend/src/ipKind.ts` 中对已保存的 `IPPure`、`Ping0`、`IPOK` 结果按每来源一票（IPv4 优先）做多数票表决：明确区分 `机房`/`家宽`/`移动`，将 `非住宅`/`非 IDC`/`未提供` 与平票保留为 `未知`；原生/广播由 `origin_type`、`isBroadcast`/`is_native` 或注册国与使用国对比判定。出口为机房时仅在 Gemini 验证、YouTube 拒提供、Spotify 限注册原因后追加机房提示，不改变解锁结论与自动精选规则。`c775dcc`、`0734ba4` 在每日纯净度与机场体检排名中优先排满轮次节点并区分纯净度口径。

2026-11-15 补充（解锁与地区探针六项实测修复）：
- **Gemini（`gemini_unlock` v5，`core/publicservice/unlock_ai.go`）**：Google 页面 bootstrap 标记已切换为转义 JSON `\"45631641\":true`，在保留 `,2,1,200,"ISO3"` 与不可用国家/地区（含 `CHN`/`HKG`/`MAC` 等）先行判定及完整 200 未截断前提下，扩展 `geminiAvailablePattern` 同时识别 `45631641,null,true` 与转义/未转义 `"45631641":true`。
- **TikTok 与 SlardarWAF 挑战识别（`tiktok_unlock` v3，`core/publicservice/unlock_regions.go` & `probes.go`）**：`www.tiktok.com/` 无头请求返回 `SlardarWAF`（`_wafchallengeid`）挑战页；将 `pageHasExplicitChallenge` 扩展识别 `slardarwaf` / `_wafchallengeid` 避免再把 WAF 页误判为 `unknown`，并将 `tiktokRegionURL` 切至稳定提供 `__UNIVERSAL_DATA_FOR_REHYDRATION__`（含 `webapp.app-context.region`）的 `https://www.tiktok.com/explore`。
- **U-NEXT（`unext_unlock` v2，`unlock_regions.go`）**：Apollo Server CSRF 防护拦截无 `Content-Type` / preflight 头的 GET 查询返回 HTTP 400 `CSRF_ERROR`；为 `checkUNextRegion` 传入 `application/json` 并在 `regionRequest` 对 `unextRegionURL` 附加 `apollo-require-preflight: true`，正常读取 `resultStatus`（467 判非日本、200/475 判日本可用）。
- **DMM TV（`dmm_unlock` v2，`unlock_regions.go`）**：`https://tv.dmm.com/vod/` 已改为静态 Next.js 外壳（不含 SSR 地区字段）；改由官方客户端接口 `POST https://api.tv.dmm.com/graphql` 执行未鉴权 `query FetchClient { client { isForeignAccess } }`，`false` 判 JP 可用，`true` 判非日本地区。
- **Prime Video（`prime_video` v3，`checker.go` & `probes.go`）**：此前 `streamingRule` 设为 64 KiB 且 `checkPrimeVideo` 调用 `readPage` 漏传 `rule.MaximumBodyBytes` 导致约 489 KB 首页恒遭截断；改为 `MaximumWebDocumentBytes`（2 MiB）并透传 `rule.MaximumBodyBytes`，同时严格保留“无 `isServiceRestricted` 不推断可用”诚实口径。
- **目标固定 DNS 预算（`core/publicservice/target_ipv4.go`）**：`prepareTargetIPv4` 原硬编码 `5s` 子超时在高延迟/并发代理上易在 DoH TLS 握手阶段超时，改为按父 `ctx` 剩余期限动态分配至 `min(8s, remaining-2s)`（保底 `5s`，不延长检查总限）。

2026-11-18 修复（`cfe04fc`）：`checkIPify` 直接保留 `boundedGET` 已判明的请求失败，正常响应中的格式或地址族不符仍标未确认。Hulu 使用的 `readPage` 在非 2xx 正文读取失败时保留 HTTP 状态与正文失败阶段，主动取消优先，2xx 读取失败仍按传输异常处理，原挑战判据沿用。Netflix 的 `prepareTargetIPv4` 按阶段快照分别计算 Fast.com 前置、DoH 与目标请求，只有 DNS 自己的时限触发时才记录 `target_dns_deadline`，整项与传输超时仍用原值。Prime Video 缺失服务限制标记时保留未确认。

2026-11-18 全量复测后修复（`39b4783`）：Netflix 的 Fast.com 结论只属于当前一次观察，后续采样重新请求。每次可选前置最多用剩余时限的三分之一且不超过 5 秒，为备用片目检查保留时间。首次需要片目时才经同一节点查询 DoH，整次检测只解析一次，前置请求计数也排除在片目之外。片目已收到 HTTP 拒绝且正文读取失败时保留状态与读取阶段，证据不足以确认封禁。离线先复现一次请求报多次一致、前置耗尽整项时限和片目 403 被覆盖，修复后验证混合前置/片目、一次解析及准确计数。隔离实网验证多次观察及前置超时后进入片目。

## L-014 自动精选本地订阅与设置保护、入口百条分批画像、Clash 双轮对照与报告导图

适用：自动精选（`auto-pick`）配置/运行/本地订阅、物理网卡解析节点入口、本机 Clash/Verge 实时对照、延迟报告导出 PNG。

`019aa2e`、`ebc1270`、`a1f0c28`、`8cb3183`、`5471464`、`42d8c02` 在 `application/auto_pick.go` 与 `auto_pick_subscription.go` 中实现定时精选：延迟决定“日常”组，必需解锁服务（默认 ChatGPT、Claude、Netflix 完整片库，不含 Gemini 与 Antigravity）决定流媒体/AI 组，本轮选空或失败时保留上一份 `subscription.yaml`。本地订阅仅监听 `127.0.0.1`（默认 17891，校验 16 字节 hex token 与 `.yaml` 后缀）；**首选端口被占用时本轮临时换随机端口但绝不覆盖 `subscription-port` 文件**，防止 Clash Verge 已填地址重启后失效；构建节点列表前预分配 `make([]autopick.Node, 0, len(nodes))` 防止 slice 扩容导致指针丢失追加用途；`finishAutoPickReport` 结束时重新读取磁盘配置，仅更新 `LastRunAt`/`NextRunAt` 而不覆盖运行期用户修改的设置。

`899105a`、`fd6bc58` 在 `core/entry/lookup.go` 中经物理网卡解析节点入口并按 100 个地址一批循环查询画像（修复超出 100 个地址被上游静默丢弃导致无国家的问题），未测出口的节点线路标签置灰注明“按节点名称推断”。`0103591`、`573b450`、`64c8641`、`db43f58` 在 `application/clash_comparison.go` 与 `frontend/src/clashComparison.ts` 中通过本机 Clash socket/TCP 接口与物理网卡各跑 2 次采样对照（不写工作台轮次库），展示活跃路由链并在手动切换代理组前确认，两边均失败时归为“节点本身不稳定”。`457b551`、`c73cb9a` 在 `LatencyReportImage.vue` 中导出 PNG 时去掉 `tabular-nums` 以避免导出渲染器字距挤压。

## L-015 Windows Clash Verge 命名管道、同名节点换端口变更抵消与非 Darwin 场景推荐

适用：Windows 下连接 Clash Verge 2.x 内核、计算订阅更新节点变更（`node_changes`）、以及在非 Darwin 平台生成“现在用哪个”候选推荐。

1. **Windows Clash Verge 2.x 命名管道与双配置文件读取**：Clash Verge Rev 2.x 在 Windows 下默认把 `verge_mihomo_socket` 写入 `clash-verge.yaml` 并使用命名管道（`\\.\pipe\verge-mihomo-...`），服务模式运行时实际管道常将 `-sidecar-release-`/`-sidecar-` 切为 `-production-`。`core/profiles/verge.go` 同时合并 `config.yaml` 与 `clash-verge.yaml`，`adapter/controller/mihomo/dial_windows.go` 支持 `\\.\pipe\...` 拨号并在配置管道不存在时自动回退至对应 `-production-` 管道。
2. **订阅节点换端口不重复计入新增与下线**：机场常在保持节点显示名不变的情况下轮换入口域名或端口（导致 `node_identity_key` 变化）。`core/profiles/node_changes.go` 在按同一 `Identity` 匹配改名后，进一步抵消 `Removed` 与 `Added` 中的同名项，避免同一节点名同时出现在“新增”和“下线”列表中。
3. **非 Darwin 同条件推荐与完整/部分下载同组排序**：在不伪造 `socket_bind_verified`/`dns_bind_verified` 的前提下，`frontend/src/recommendations.ts` 优先采用具备物理出口核验（`comparable=true`）的候选组；若当前平台无物理套接字核验（如 Windows），则允许同条件的当前工作台首字节延迟与有效下载记录进入对应用途候选，且下载推荐按同一 `downloadCondition` 聚合（不再因 `byte_limit` 完整下载与 `time_limit` 部分下载分属不同子键而导致慢节点多时反压快节点）。

2026-11-18 推荐到历史的配置范围：原 `RecommendationView.inspect` 以时间戳在延迟/下载/服务中依次猜记录，遇到同一时间的不同项目会打开错误配置，且只覆盖 node_key/revision、未携带真实 identity。现由候选生成阶段携带对应用途的完整证据 scope、服务 ID，日常候选定位 Cloudflare；详情不修改工作台原来的服务勾选与显示站点。候选直接标注旧配置、混合配置或缺标识，导出仍使用当前节点。日常来源只计入选中条件组的实际观察，排除不参与推荐的旧条件，不改变非 Darwin 与下载同组排序算法。

历史详情不能把“传入的配置”直接称作“当前配置”：以当前节点清单的完整 scope 确认；查看旧配置时显式标注，重新检测/监测仅使用可明确定位的现行配置，已不在清单中的配置只读历史。回归先复现同时间 AI 候选误开延迟配置，再验证完整 identity、服务定位、Cloudflare 定位、混合/未知标识、旧配置标签与新计划范围；相关 21 项通过，合成浏览器走通旧配置 AI 记录 → 当前配置计划，未开始检测。

2026-11-18 报告操作延伸：不能用“当前节点清单查不到”阻止已保存报告打开历史。归档报告入口携带完整 scope、手动/监测来源、延迟项目与报告结束时间，详情在对应时段读取；现行配置的原工作台聚焦路径保留。导出须核对所有待导出节点的完整 key/identity/revision，不能把已变化节点悄悄滤掉后仍称已导出报告范围；有缺项时告知数量并让用户明确选择当前范围。4 条失败先复现，补正常导出和来源/时间窗后相关 57 项通过；合成浏览器走通八天前已移除节点的报告→历史样本与导出缺项提示，没有真实导出或检测。

2026-11-18 节点历史接续：归档详情的刷新必须继续沿报告截止时间，不能悄悄改查今天；当前配置尚无历史、只有一条旧配置时，仍需将传入配置与历史列表合并供选择。配置列表失败单独提示并可重试，不掩盖已读记录；记录读取失败不显示“没有历史”，已退出目录的服务保留历史名称。节点详情用独立范围栏呈现来源/项目/版本/时段，下载和服务明细复用 MeasurementResults，保留稳态/均速/峰值、原始样本、出口、判据、同条件比较及保存重试。4 条失败先复现，阶段完整前端 78 文件/500 项通过；最后的嵌入标题/来源整理后相关 69 项及构建通过。合成页面核对旧报告刷新、24 条原始样本、640px 和当前下载详情，未读取真实数据或发起检测。

2026-11-18 比较到历史的延伸：入口须带上比较项目、实际手动/监测来源、所选站点和证据截止时间；不能回落为工作台的另一个服务或今天的默认窗口。`initialServiceId=''` 明确表示全部服务，`undefined` 才沿用默认项目；使用 `??` 保留这个差别。服务证据直接打开比较单元格对应的 Attempt，不能按节点再次从更晚工作台记录挑“最新”。账户和完整 scope 保持，打开/切换不发探测。

## L-016 延迟 Keep-Alive 复用、Windows TUN Fake-IP DNS 绕过与轮次并发锁

适用：与上游延迟实测出现稳态偏慢、物理入口 DNS 固定卡 ~5 秒（Windows 开启 TUN 时）、CLI TSV 重复行，或高并发写测量轮次偶现 `SQLITE_BUSY_SNAPSHOT`。

1. **延迟测速首字节计时后排空短响应保持 Keep-Alive 复用**：`core/speedtester/speedtester.go` 的 `testLatencyWithClientContext` 原在 `io.CopyN(io.Discard, resp.Body, 1)` 后立即 `resp.Body.Close()`，导致 >1B 的探测响应（如 Apple 68B、GitHub ~42B、Microsoft 22B、Firefox 8B）因未读到 `io.EOF` 而被迫关闭连接，第 2–6 次“稳定延迟”退化为重复 TCP+TLS 握手（实测曾偏慢 10–35 ms）。现先记录首字节 TTFB `dur := time.Since(start)`，开启 Keep-Alive 时再最多排空 512 字节使短响应到达 `EOF`；若 `serverModeDirectDownload` 直测大文件 URL，额外附加 `Range: bytes=0-0` 请求头；CLI `--tsv` 通过新增 `TestProxiesFinalUntil` 只输出每节点最终一轮结果，避免同节点重复多行。
2. **Windows 开启 TUN 时跳过 Fake-IP DNS 服务器**：Clash Verge 等内核在 Windows 开启 TUN 后常把虚拟适配器 DNS（`198.18.0.2:53`）设进系统默认首选 DNS，导致 `ResolvePhysicalHost` 经物理网卡拨 `198.18.0.2:53` 等满 5 秒超时。`core/speedtester/physical_entry_other.go` 与 `interface.go` 显式调用 Mihomo `resolver.AddSystemDnsBlacklist("198.18.0.1", "198.18.0.2", "fdfe:dcba:9876::1", "fdfe:dcba:9876::2")`，并在自定义 `net.Resolver.Dial` 遇到 Fake-IP DNS 地址时直接报错跳过，必要时回退 `resolver.LookupIPv4`（将入口 DNS 解析由 5 秒降至几十毫秒）。
3. **测量轮次状态更新统一持锁串行写库**：`core/history/measurement_round.go` 的 `FinishMeasurementRound`、`ReconcileMeasurementRounds`、`MarkMeasurementNotExecuted` 与 `FinishAutomaticMeasurementRound` 补齐 `d.mu.Lock()` / `defer d.mu.Unlock()`，消除高频并发更新轮次状态时的 `SQLITE_BUSY_SNAPSHOT` 竞争。

2026-11-18 修复（`cfe04fc`）：`testLatencyWithClientContext` 在最多 512 字节的可选补读前确认首字节时延，补读失败不再推翻已测样本。首字节读取超时、取消、空正文和 HTTP 响应时延语义沿用。旧实网样本的请求级 deadline 在收到响应前发生，需按实际传输阶段排查。

2026-11-18 全量复测后修复（`60aa631`）：`readMeasurementLatency` 读取批次错误时按实际执行状态填充。已采样的部分响应或全部超时将错误放入 `execution_error`，真正未执行/配置变化使用 `not_executed_reason`。轮次详情显示原样本和原因，旧快照仍可读，SQLite 无迁移。隔离生产版验证同一轮次的全部计划项和翻页。

## L-017 UI 必须从本项目使用流程形成构图，不能用模板换皮替代重构

2026-11-18 用户授权全面重做导航、工作台与全部功能的界面组织，由当前代理自行决定；要求好用、符合人的习惯、看起来好、有辨识度，反对“程序员审美”和千篇一律的 AI 模板。开工必须先看 Obsidian 项目记忆；每个新的功能块开始前再次读对应条目和现有实现，不能用一次泛读覆盖后续全部改动。

本轮首稿的纸白/青绿、顶部导航、大标题加常规表格预览被用户明确否定为难用、模板化。**未获认可，不作为新视觉基线**；不能再以换品牌色、圆角、侧栏/顶栏或口号声称真正重构。Obsidian 原 UI 研究已记录用户“给我们这个项目定制一套属于我们这个项目的 UI，而不是一个公共的模板……深度定制集成”的要求，另项目拍卖图录的经验只用于理解领域构图，不机械复用其颜色与造型。

功能保留需核对实际入口、接线与行为，不是复制组件名。范围/账户、节点 identity/revision、站点与完整服务目录、地区正反规则、历史/多轮/比较/导出、预算、取消、保存重试、Clash/Verge、监测/自动精选、通知、设置与数据入口均须有迁移位置。当前源码日常站点与历史兼容站点分别保持，旧记忆的站点数量不用于反改现行目录。功能迁移及本轮完成证据在 [UI 重构记录](ui-rebuild-20261118.md)，浏览器合成预览不等于原生/实网或用户视觉认可。

2026-11-18 比较页重整：多节点比较是独立阅读任务，统一用宽对照窗口，固定控制与表头，只滚动表格；按指标/服务逐行对齐，保留所有已保存服务及历史规则名称。不能套用工作台三列指标样式，否则单元格被挤窄、P95 被隐藏。四个日常站点和两个兼容历史站点可在比较内切换；下载完整条件折叠保留，不把历史速度冒充最近成功。服务使用共同结论，但“无法判断”旁仍须保留 HTTP 拒绝等具体依据，不能为了统一文案抹掉失败原因。最终 79 文件/505 项前端回归及构建通过，双/四节点和 640/1280/1600 合成浏览器验证通过；不代表视觉获用户认可。迁移与证据见 UI 重构记录。

2026-11-18 检测配置接续：参数按延迟、下载、解锁、IP 分段展开，折叠处保留所选源、次数和时限；节点/账户范围与预算并排，窄窗口顺序阅读。服务采样次数对仅 IP 计划同样可改；预算只列实际所选项目，地区跳过按服务及账户计数说明，全部跳过时明确留在配置页而非启动空队列。Antigravity 手动选择与额度提示、下载双源/独立时限、预设保留其他已选项目不变。**在用目录以 `core/publicservice.Catalog()` 为准**；底层 `catalog` / `RuleFor` 还保留网页入口等旧 ID 供历史读取，不应只凭缺少 `superseded_by` 就把它们重新加入选择器。最终完整 514 项及构建通过，合成浏览器验证 OVH、准备锁定与取消、迟到响应、IP 独立采样和 640px 无横向溢出；仍不是用户视觉认可。

2026-11-18 导航接续：页面的可见名称必须可查（如“订阅”“为你选线”），节点结果用两行明确名称及机场/账户，支持空格组合地区、账户和名称；超过首批 40 项提供继续显示，不能悄悄截断。其他操作弹窗已打开时，Ctrl/⌘ K 不叠加查找窗口；合成浏览器验证账户辨识、范围外详情到明确检测、分页键盘及窄窗口。仍须保留用户视觉尚未认可的边界。

2026-11-18 工作台对照接续：综合列表以同一组站点列纵向读节点，站点名放在固定表头，行内保留实际采样与未知状态；横向浏览保留节点身份，表头与正文同步移动。站点显示选择直接可见，只改视图，不改变检测目标；仍以现行四站为日常目录，六站历史兼容另循既有详情/比较路径。键盘上下及 Home/End 只移动阅读位置，Enter/点击才打开详情，复选框按键不被列表劫持；浏览、勾选和检测不能混成一个动作。用量来源带保留各机场状态/账户入口，单账户缺到期日仍须写未知。先复现 3 条交互/到期问题，完整 87 文件/594 项及构建通过；640px 的滚动表头与固定身份、宽屏和五种视图列对齐均用合成数据核对，不代表原生或用户视觉验收。

2026-11-18 连续阅读接续：详情不能用当前页首行作为找不到原节点时的替代，否则筛选或配置刷新会悄悄换人。打开时保留节点完整 scope 和当时的筛选顺序，上一条/下一条可跨页，但不自动改勾选；明确勾选后可比较，返回仍是原阅读节点，单节点检测不因已选两条而扩大。node_key 与 identity/revision 都核对，旧配置/已移除节点只读，Clash 使用/对照和重测禁用；范围外现行节点可明确配置检测，但不能加入不可见的勾选，也不扩大机场范围。长记录固定节点身份/账户/操作，章节只滚动当前阅读容器；宽屏侧栏需同时让开导航和工具栏，不能仅扣导航高度。出口详情只读入口缓存，解析由显式按钮发起，保留原批量解析入口。新增 6 条交互/范围回归；完整 87 文件/599 项通过后，最终相关 48 项与构建通过。浏览器合成链路、640px 与宽屏核对通过，仍不代表原生/实网或用户视觉验收。

2026-11-18 延迟读数接续：曲线与站点范围共用从零开始的自动量程，不能用固定 1500 ms 下限把几十毫秒挤在底部，也不能截断更慢的样本。取消与真实判据无关的 300/800 ms 背景分档；既有稳定延迟、P95、响应率和 1500 ms 偏慢判据保持。无响应放在独立轨道，缺值不连线；只有首次响应时稳定值为空属于样本不足，不等于全失败，未测仍是未测。同次四站读数一起显示，保留实际响应数量、原因与配置变化。站点名称继续提供查看范围的统计/读取失败提示，数值说明只解释当前轮次，不能修复混淆时删掉原范围证据。原生 dialog 的顶层会遮住 Teleport 到 body 的提示，提示必须跟随发起元素进入当前 dialog，离开后回到页面；新增行为回归和合成浏览器验证见 UI 重构记录，不代表原生 App 或用户验收。

2026-11-18 工作台阅读层级接续：宽窗口把来源与套餐放到独立账本，线路对照区从首屏开始。窄窗口默认保留机场身份、余额、今日用量、刷新状态与入口，展开后看比例和三日明细。刷新范围放在账本内展开，避免原浮层被滚动边界裁切。页面完整重载后核样式，旧的全宽来源规则会覆盖开发热更新中的正确显示，已移除冲突。查看机场只筛选已纳入范围，旧 `viewAirport('all')` 会纳入所有注册机场，不能接到“范围内全部”或页头返回按钮。纳入仍有明确入口，原保存勾选、账户状态和多种检测视图保持。并排详情按实际线路区宽度分配，窄时用原弹窗。验证和尺寸见 UI 重构记录。

2026-11-18 主导航与任务接续：工作台一直保留筛选和列表，返回时应在重新显示之后恢复阅读位置。只检查 `scrollTo` 被调用不够，实看发现 `v-show` 尚未显示时位置会被裁成零。明确节点定位优先于恢复位置，认证失效清除本次滚动。顶部与任务状态共同常驻，按真实高度给表头/详情留位置。队列和延迟报告要有两个入口，组合检测后的延迟结果不能盖掉其他项目及保存重试。启动未确认不画比例，纯保存不给无效取消；已打开的纯延迟任务仍可自动转报告，关闭后结束不抢焦点。完整历史、快捷查找、取消与保存合同沿既有入口。实际交付和边界见 UI 重构记录。

## 维护方法

方法统一继承总规则“2 项目记录”“5.1 全周期复盘与覆盖”，从下方实际覆盖记录接续。

## 本次覆盖与缺口

2026-11-10 规则核查截点 `main@35e25edaa541ed9e803dc7d17ee73050b503e5fa`：全文读取根 AGENTS 与第三轮路线，读取进度前 65 行和相关 Vault 入口/交接。整理当前入口、历史计划与上述教训，不改变产品功能或测试结果。

未逐文件/逐提交读取全仓，未覆盖所有分支、本机 dirty、真实 App/代理或生产数据库；未运行构建、Go/前端检查或实网探测。**全历史处理游标未建立**，不能把读取截点当全部历史已总结。规则写入不代表记忆 Hook/CI 已部署，也不恢复已暂停的调查或监测。

## 2026-11-10 接手与持续维护补充范围

本次从 `main@4e4256adfe609726977b432864887d116ee08ca1` 读取现行 AGENTS/本页及项目入口，补自然称呼索引、每轮主动写回和上下文恢复/纠错做法。未完成的历史覆盖仍按原记录；这次规则提交不前移全历史处理点，未重跑产品、部署拦截器或建立后台任务。

## 2026-11-11 下载链路增量归并范围

本次以 `main@fd754680434d433544bc432559f8c7a569d4e712` 为读取截点，完整读取现行总规则、项目入口及本页，定向读取进度末段和 `76d6e58`、`d87cd8d`、`ce0261b`、`35e25ed` 的有关实现与既有脱敏结果，归入 L-004。本次没有重跑测试、构建、实网请求或检查生产历史，未前移全历史处理游标。下次涉及下载时从 L-004 及其实现接续，其他历史覆盖仍按原有缺口记录。

## 2026-11-15 本仓全量分叉提交复盘与 Windows 接手范围

本次分两批完成 `upstream/main..main`（`7549994`..`a7bb500`，共 560 条提交、479 条非 merge 提交）的全量历史复盘，并全文读取 `docs/backend-round3-progress.md`（1–797 行）、`docs/version-consistency.md`（1–57 行）、`docs/roadmap-round3.md`（1–82 行）：

1. **近期提交批次（`c747674`..`a7bb500`）**：实际读取 `de31a46`、`7cb86bb`、`b690d3d`、`8b794ea`、`4952869`、`91d9d50`、`67d5392`、`36e3577`、`ff4b28b`、`fb6abca`、`ccfb073`、`8845c67`、`a7bb500` 等提交及源码（`frontend/src/unlock.ts`、`core/subscriptionusage/continuity.go`、`core/history/latency_batch_progress.go`、`frontend/src/latencyHistory.ts`、`frontend/src/components/home/homeModel.ts`、`core/speedtester/physical_family_darwin.go`、`frontend/src/workspace.ts`），提炼为 L-006～L-009。
2. **早期与中期分叉批次（`7549994`..`4b0f557`）**：完整遍历 426 条非 merge 提交清单，并实际读取 `56b02e9`、`1dec197`、`114296f`、`7fec4ec`、`cbd05f2`、`38eb812`、`28fcbe4`、`3d0bd9f`、`019aa2e`、`a1f0c28`、`42d8c02`、`899105a`、`fd6bc58`、`0103591`、`db43f58`、`457b551`、`c73cb9a`、`c775dcc`、`0734ba4` 的提交 diff 与核心实现文件（`application/auto_pick.go`、`application/auto_pick_subscription.go`、`application/clash_comparison.go`、`frontend/src/ipKind.ts` 等），提炼为 L-010～L-014。

验证与覆盖边界：属于本仓分叉改动的 `upstream/main..main` 提交历史与进度文档已全部过目并建立 L-001～L-014 索引（若后续深入某一早期 PR 的逐行边角细节，仍以对应源码与 commit diff 为准）；Windows 环境已完成 `go vet ./...`、`core/history` SQLite/Windows 专项测试、前端 73 文件 449 项 Vitest 与 `npm run build`，未触碰本机生产库、未执行实网测速或付费模型调用。

## 2026-11-15 下午增量覆盖（UI 重构、下载/延迟优化与上游对照排障）

本轮基于实网与本地 Web 服务排障反馈，同步更新 `L-004`、补入 `L-015` 与 `L-016`，覆盖全功能 UI 重构（`bdb1f92`..`55ed495`）、Windows Clash Verge 2.x 命名管道与同节点换端口/非 Darwin 推荐适配（`7f24210`、`ceb9a21`），以及下载预算分离（`20 MiB / 15s` 与稳态/峰值/均速）、延迟 Keep-Alive 512B 排空、Windows TUN Fake-IP DNS 跳过、`measurement_round.go` 并发锁修复（均完成 `npm run build`、`npm test`、`go vet ./...` 与受影响 Go 包测试）。

## 2026-11-15 晚间增量覆盖（延迟/下载复核与解锁探针六项修复）

在 `http://127.0.0.1:8999` 本地调试实例上复核延迟与 20 MiB / 15s 下载测速确认稳定后，完成 `L-013` 解锁探测六项修复（`gemini_unlock` v5、`tiktok_unlock` v3 与 `SlardarWAF` 挑战识别、`unext_unlock` v2、`dmm_unlock` v2、`prime_video` v3 及 `target_ipv4.go` 动态 DoH 预算）；同步通过 `go vet ./...`、`core/publicservice` + `application` Go 单测、前端 73 文件 451 项 Vitest、`npm run build` 及双节点受控实网复验（未执行 Antigravity 或全机场批量测速，未重编 `.exe`）。

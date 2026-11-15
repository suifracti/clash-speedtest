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
| 下载只有短文本、速度口径偏低、备用源限流与旧测速结果 | L-004；application/workbench_download.go；frontend/src/domain.ts |
| Clash Verge 更新订阅后不想重填链接；关联已有账户 | L-005；application/verge_subscriptions.go |
| 某解锁服务只想测指定地区或排除指定地区；Antigravity 批量风险与连接耗时 | L-006；frontend/src/unlock.ts；core/autopick/autopick.go；frontend/src/components/QueueModal.vue |
| 手动刷新被 30 分钟冷却拦住；订阅代理/直连/物理重试原因、换链接后今日与 14 天用量断掉 | L-007；application/subscription_refresh.go；application/subscription_usage.go；core/subscriptionusage/continuity.go |
| 延迟大批量卡顿或分段；工作台/检查器/报告结论不一致；订阅更新后同名节点 7 天指标丢失；双栈回退后一直走 IPv6 | L-008；core/history/latency_batch_progress.go；frontend/src/latencyHistory.ts；frontend/src/latencyReport.ts；core/speedtester/physical_family_darwin.go |
| Windows 启动报 build tags 或 WebView2 句柄错误；Node 26 跑前端测试报 localStorage；新数据目录显示本地服务未连接 | L-009；frontend/src/workspace.ts；scripts/prepare-legacy-assets.mjs |
| SQLite 外键/busy_timeout 在连接池失效；Windows 盘符 URI 报错；v11 升级备份与历史清理中断 | L-010；core/history/db.go；core/history/sqlite_uri.go；core/history/upgrade_backup.go；cmd/history-orphans/main.go |
| 同配置 YAML 别名节点在延迟批次被跳过；VLESS pcs/vcn 特殊 TLS 导出失真；推荐/体检漏读旧 revision | L-011；core/speedtester/speedtester.go；core/speedtester/vless_tls.go；application/export.go；frontend/src/nodeQuality.ts |
| 持续监测抢占预算、暂停时无法手动触发、Mac 监测未绕开 TUN；本机通知重复告警或授权失败 | L-012；core/monitor/；core/speedtester/；application/local_notifications.go |
| Netflix DoH 超时吞掉结论、YouTube 误判送中（pagead）、Gemini/Spotify 机房限制、Disney+ 握手 EOF；出口 IP 类型与原生/广播判定 | L-013；core/publicservice/probes.go；core/publicservice/checker.go；frontend/src/ipKind.ts；frontend/src/exitLanding.ts |
| 自动精选端口被占后 Verge 订阅失效、运行期改设置被覆盖或报告丢用途；入口超 100 地址丢国家；Clash/Verge 本机对照与报告导图 | L-014；application/auto_pick.go；application/auto_pick_subscription.go；core/entry/lookup.go；application/clash_comparison.go；frontend/src/components/LatencyReportImage.vue |

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

## L-004 下载短响应、计速与备用源要分开判断

适用场景包括 HTTP 200 却只收到短文本、连接慢导致下载速度偏低、测速源限流，以及比较不同来源的旧记录。

`76d6e58` 修复短响应假成功。响应提前结束且字节数不足请求量时归为 `response_too_short`，不生成速度，保存最多 200 字符的脱敏 `response_summary` 与 `content_type`。状态码 200 不能单独证明下载成功，旧的 completed 短响应也在读取时按同一口径显示。

`d87cd8d` 将速度统一为 `bytes_read / phase_timings_ns.response_body`，界面换算为 MiB/s，连接及握手耗时另列。旧记录有响应体阶段时读取时计算，没有该阶段就显示未记录，不能拿总耗时补成速度，也不改历史库。`ce0261b` 统一复用 `outcomeLabel` 显示中文结果。

`35e25ed` 将备用源替换为 OVH，按请求量发送 Range 并接受 HTTP 206。当前源返回 403/429 后换另一源一次，记录与比较沿实际目标来源，双源均失败时保留两次原因。旧来源记录仍按其实际来源显示。

入口在 `application/workbench_download.go` 的 `executeWorkbenchDownload`、`downloadSourcesFailure`，传输与采样在 `core/speedtester/download_stream.go`，旧记录显示和计速在 `frontend/src/domain.ts` 的 `downloadOutcome`、`downloadSpeed`、`downloadSourceName`，队列文案在 `frontend/src/measurementRounds.ts`。对应交付见[进度页 2026-11-02 下载各节](backend-round3-progress.md#2026-11-02-下载短响应)。

证据边界截至 2026-11-02。保存的四次实网结果均为直接选择 OVH，HTTP 206 且各收到 10 MiB。403/429 后自动换源由离线测试覆盖，这四次实测不能替代该路径的现场验证。原生截图只确认选源页面，不能当作四节点结果截图。本轮只复读已有实现差异与脱敏记录，未重新测速或核实当前运行包。

## L-005 从 Clash Verge 同步订阅时保留账户关联

适用：本机 Verge 已更新订阅或修改链接，用户在账户页点击“从 Clash Verge 导入／同步”。从标准配置目录的 `profiles.yaml` 读取 remote 项及其本地节点缓存，以 `uid` 关联 `Subscription.VergeUID`，后续按该绑定更新原 CST 订阅 ID。首次只对唯一完整 URL 相同的账户建议关联；链接已变化时由用户选择已有账户，名字相同不自动合并。

`core/profiles/verge.go` 只输出缓存的 `proxies` 根，保留节点标量类型；用量缺字段保留未知。`application/verge_subscriptions.go` 复用 `refreshSubscriptionWithFetchLocked` 的 prepared fetch，记录 `fetch_path=verge`，沿用节点变化、失败清除、缓存和快照提交；本机读取不占用 HTTP 429 冷却。导入失败恢复原订阅配置，原缓存由既有刷新恢复逻辑保留。

链接变化后的用量仍由既有连续性读取规则衔接，`usageAccountKey` 沿 URL 哈希。普通 API 摘要只传 UID、名称、时间与关联状态，完整 URL 留后端。Wails 和 Web 接口均已接入 `ClashVergeImport.vue`；新建、改链接复用原账户、失败保留、账户选择用临时数据覆盖，本机清单只读核对。

## L-006 解锁地区正反向规则与 Antigravity 批量风险及耗时归因

适用：配置某项解锁只测指定地区或排除指定地区；开启/批量勾选 Antigravity；解释 Antigravity 队列里的模型可用与代理建连耗时。

`4bb77e7`、`0c344f0`、`de31a46` 在现有 `unlock_service_regions` 字段上支持每服务独立地区规则：默认无前缀列表为“只测所选”，`!地区码`（`excludedRegion`）为“排除所选”，空列表为不限地区（后端无需新增字段）。`unlockServiceSkipReason` 遇到只测列表跳过未选地区（“不在该项设定的检测地区内”），遇到排除列表只跳过所选地区（“已排除此地区，不检测”），入口与节点名均无法识别地区时仍继续检测。空列表无法保存 `!` 前缀，`UnlockRegionEditor.vue` 用 `pendingExclude` 暂存模式并在只测/排除切换时保留已勾地区。

`b690d3d` 将 Antigravity 从自动精选（`core/autopick/autopick.go` 的 `defaultCatalog`）与工作台分组/全选默认目录（`defaultSelectableServices`）排除，只允许显式手动勾选。`7cb86bb` 取消此前每次最多 3 个节点的硬拦截，超过 3 个节点时由 `antigravityBatchRisk` 在计划弹窗与工作台显示多 IP 风控与额度消耗提醒，不再禁用开始按钮或在 `runPlan` 拒绝提交。`8b794ea` 在 `QueueModal.vue` 中把“已处理”与“确认模型可用（仅 `matched` 且真实模型回答）/地区拒绝（`streamGenerateContent` HTTP 400）/未确认”分开，并标明 `details.proxy_connect_ms` 是已结束调用（含同时发起的无凭据 Cloudflare trace 与 Google 请求）的代理建连累计耗时，不能归因给单次请求。上述逻辑由前端与 Go 离线单测覆盖，未发起真实付费模型调用。

## L-007 订阅抓取路径、手动冷却豁免与换链接用量连续性

适用：解释订阅刷新走了哪条网络路径或为何失败；手动刷新被间隔限制；修改订阅链接后节点缓存、失败状态与 14 天/今日用量的衔接。

`4952869` 将每机场 30 分钟间隔（`usageRefreshAirportGap`）仅作用于自动刷新，手动刷新（单机场、全部、行内、仅刷新用量）不再受 30 分钟冷却限制；自动刷新仍按 2 小时周期并参考最近一次手动刷新时间。`c747674`、`7e546be`、`91d9d50`、`9e9f8bf` 将 `fetch_path` 细分为 `proxy`、`normal`、`physical`（及本地导入的 `verge`），旧记录 `direct` 标为路径未区分；普通网络（或代理）传输层失败时复用入口 DNS 的物理网卡客户端重试一次，物理重试仍失败时合并保存两段脱敏原因（总长上限 200 字，避免后一段被截断），HTTP 429、取消与超时不盲目重试。`d69b53f`、`44a226b`、`480101c` 在 `SubscriptionOverview.vue` 与刷新弹窗中将刷新任务完成时间与订阅 `last_attempt_at` 合并取每订阅唯一最新状态，避免旧手动失败覆盖后续自动成功，并展示保存的路径与原因。侧栏与工作台节点计数统一复用 `airportViewing.ts` 过滤公告项。

`67d5392` 在 `UpdateSubscription` 链接变化时直接复用单订阅刷新（旧缓存参与节点增删改比较，成功清除旧失败并写 `subscription_update`，失败恢复原链接与设置，仅改名/备注不抓取）。`36e3577` 在 `core/subscriptionusage/continuity.go` 的 `ContinueAccounts` 中按只读方式衔接换链接后的用量：当同机场旧 `AccountKey` 已停止记录、新旧 `Total` 一致且累计已用流量不减（多候选取最小非负增量）时，将旧快照归入新 `AccountKey` 供 14 天趋势与今日用量连续计算，不改写数据库与 `usageAccountKey` 生成规则。

## L-008 延迟单批次轻量进度、同名节点跨配置连续性、三处结论对齐与双栈回退

适用：大批量延迟测速进度与完成拉取；订阅更新后同名节点的 7 天指标与配置分界线；工作台、检查器、报告的延迟结论不一致；Mac 物理双栈回退后地址族固定问题。

`4657444`、`ee35394`、`ccfb073`、`99a57f8` 移除前端 200 节点分段，每轮延迟作为单批次提交（后端上限 10000，默认并发 16）；运行中由 `core/history/latency_batch_progress.go` 的 `GetLatencyBatchProgress`（`GET /api/workbench/latency-batches/{batch_id}/progress`）只读汇总计数，SSE 批次事件不携带全量节点明细，批次结束后一次读取完整结果并弹出报告。`f9903a6` 在 `application/workbench_latency_batch.go` 中将内部取消记为 `context_cancelled`、超时记为 `deadline_exceeded`，仅用户点击停止记为 `user_cancelled`。

`059fd63`、`2f08443`、`fb6abca` 在 `frontend/src/latencyHistory.ts` 的 `readLatencyHistoryWithConfig` 中汇总同机场同名节点跨 `node_identity_key` / `config_revision_key` 的历史轮次：工作台 7 天响应率、波动和中位延迟默认保留同名节点的跨配置历史参与显示、排序与筛选（`LatencyChart.vue` 在新配置首次测速处画竖线标记“配置已变化”），检查器同时通过 `latencyHistorySplit` 区分 `current` 与 `old` 轮次；节点改名不猜测合并，底层历史 scope 不改写。

`f9903a6`、`4febe64`、`6a3f74e`、`8845c67` 统一工作台（`homeModel.ts` 的 `latencyVerdict`）、检查器（`inspectorLatency.ts`）与批次报告（`latencyReport.ts`）的判定优先级：单站 6 次全失败且其余站有响应优先判为橙色“部分站点不通”（`partialLatencySites`）；响应比例统一用 `latencyResponseUnstable`（不足 5/6 为“不稳定”，其余有失败为“偶有失败”）；稳定延迟超过 `latencySlowThresholdMs`（1500 ms）统一判为“偏慢”；失败原因展示具体超时/被拒绝/被重置/被关闭，HTTP 状态码 ≥300 计为已连通并单独统计站点拒绝。`ff4b28b` 在 `core/speedtester/physical_family_darwin.go` 的 `DialContext` 中仅在 IPv4 或 IPv6 实际连接成功后才置 `settled=true`（IPv6 成功时才置 `selectedIPv6=true` 并关闭 IPv4 适配器），双栈均失败（`ipv6_failed`）时保留 IPv4 供后续采样继续先试 IPv4。

## L-009 Windows 本地构建、Node 26 测试与未初始化数据页连接状态

适用：Windows 环境拉起桌面端或 Web 服务、运行前端 Vitest、首次打开空数据目录（尚未初始化存储）看到侧栏状态异常。

Windows 下编译或运行 Wails 桌面端需显式传入 `-tags production`（并先执行 `node scripts/prepare-legacy-assets.mjs` 与 `frontend` 的 `npm run build`），否则落入 Wails 默认的 `app_default_windows.go` 弹出 build tags 提示框；在无交互或沙箱隔离桌面（如 `WinSta0\exebox-*`）中，`WebView2` 的 `CreateCoreWebView2Controller` 会因窗口句柄跨桌面报 `80070578: Invalid window handle`，此时可用 `--web --port 8999 --listen 127.0.0.1 --browser none --data-dir <dir>` 启动本地 Web 服务调试，真实桌面双击 `build/bin/clash-speedtest.exe` 独立验收。Node `v25/v26` 内置全局 `localStorage` 在未传 `--localstorage-file` 时返回 `undefined` 并覆盖 jsdom，运行 `npm test` 需设置 `NODE_OPTIONS="--localstorage-file=<temp-file>"`。

`frontend/src/workspace.ts` 的 `boot()` 此前在 `setup.state !== 'ready'` 时提前返回而未调用 `events(...)`，导致新数据目录停留在“数据与备份”初始化页时侧栏一直显示“本地服务未连接”。现已抽出 `subscribeEvents()` 在 `api.setup()` 成功后立即建立事件连接（非 `ready` 状态下忽略业务事件刷新，初始化成功后直接复用同一连接），并在 `dispose()` 中重置 `unsubscribe` 与 `connected`。

## L-010 SQLite 连接池 Pragma、Windows 文件 URI、v11 升级门禁与事务清理

适用：修改 `core/history` 数据库连接、迁移、升级备份、孤儿检查或历史保留清理。

`56b02e9` 在 `core/history/db.go` 中通过编码后的 DSN 为连接池每个连接设置 `foreign_keys`、`busy_timeout` 与 `synchronous`，避免仅在首个连接执行 pragma 导致并发连接外键级联失效；`cmd/history-orphans`（`core/history/orphans.go`）以 `mode=ro` / `query_only` 只读检查孤儿记录，绝不自动改库。`1dec197` 在 `core/history/sqlite_uri.go` 中将 Windows 盘符路径规范化为 `file:///C:/...`，防止 `C:` 被解析为 URI authority。

`91afa7b`、`114296f`、`897ebc0`（`core/history/upgrade_backup.go`）要求既有 v11 库在执行 DDL 升级前必须先完成经校验的 WAL 备份（Windows 下不支持安全私有根目录权限时直接阻断 DDL 升级并保留原库），备份清理失败时显式暴露并隔离。`fc16f63`、`3e3736e` 将工作台与监测历史清理放入事务并感知取消，清理已终止但缺 `finished_at` 或孤立的 `interrupted`/`persistence_failed` 轮次，同时保留进行中记录。

## L-011 节点身份与配置修订解耦、YAML 别名归一化、VLESS 特殊 TLS 与导出保真

适用：节点配置比较与去重、延迟批次未执行排查、推荐/体检历史读取、VLESS 特殊 TLS 参数及 Clash 配置导出。

`cbd05f2` 修复 `core/speedtester/speedtester.go` 中 `buildProxyServerPortKey` 对 YAML 解析出的嵌套 `map[any]any` 直接 `json.Marshal` 失败导致 `EquivalentProxyConfiguration` 恒为 `false` 的 bug：归一化嵌套 map 后，同配置别名节点正常合并去重，不再被误判为“多个 TLS/连接配置”而在批次中整项跳过。`415d739`、`63a436b` 将公告/余额提示条目与真实节点分开，避免公告别名吞掉同配置真实节点。

`7fec4ec` 在 `core/speedtester/vless_tls.go` 與 `application/export.go` 中恢复 VLESS `pcs`/`vcn` 与 insecure 别名解析及私有 TCP TLS 校验，证书约束计入 `config_revision_key`；未准备好的路径拒绝静默忽略证书约束，普通 Clash/Mihomo 导出无法保真时整次拒绝，且导出时剥离内部 `_cst_` 运行时字段（`fb2511f`）、同机场缓存节点先去重防误报歧义（`db86ef6`）。`911db84`、`7255be4` 在推荐页与机场体检中通过 `ListNodeHistoryRevisions` 聚合同一 `node_identity_key` 的历史 revision（6 并发限流），避免配置微调后漏读旧记录。

## L-012 持续监测预算准入、物理网卡绕 TUN 与本机通知去重

适用：24/7 持续监测调度、容量保护、恢复准入、Mac 监测绕开本机 TUN、桌面原生通知。

`8e61026`、`6780a3f`、`d0269c2`、`9c008e5`、`2de0518`、`b0fddba` 在 `core/monitor/` 中建立按键集分页的原始样本库、容量阈值保护、持久化全局共享预算与采样分档准入；暂停状态下仍允许手动触发单次监测（`d708035`），预算调整或启动恢复时保持队列准入与配置阻断语义（`4b45e00`、`02f1e6e`）。`6627e70` 将 Mac 持续监测的 DNS 与 socket 统一走物理网卡客户端（检测后关闭连接与代理），仅当 DNS 与 socket 均核验通过时写入 `tun_bypass_interface`。

`6128b16`、`fa74fc6` 在 `application/local_notifications.go` 中支持故障率、低余量、到期三类阈值提醒（包含 `completed_with_issues` / `completed_with_save_failures` 批次），仅在设置变更与每 5 分钟轮询检查，相同告警本机去重、恢复后才可再提醒；勾选开启时先调 `requestAuthorization`，系统拒绝则保持关闭并提示打开系统通知设置。

## L-013 解锁探测判据（Netflix / YouTube / AI / Disney+）与出口 IP 类型多数票

适用：维护 `core/publicservice` 解锁规则、排查误判送中或 DoH 超时、解释出口 IP 类型（机房/家宽/移动、原生/广播）及机房限制提示。

`4327e3d`、`28fcbe4`、`38eb812` 对齐并修正解锁判据：Netflix 在 `core/publicservice/checker.go` 与 `probes.go` 中将 Fast.com 预检提到节点 DoH 之前（出结论即记 `target_dns_skipped=fast_api_decided`，避免节点 DoH 超时吞掉结论），地区优先取 `client.location.country`、`targets[0]` 仅作后备，预检未决再走双片目（`70143836`/`81280792`）；YouTube Premium 请求 `/premium?hl=en`，排除页面自带 `google.cn/pagead` 遥测避免误判送中，非 CN 的不可用标橙色“不向该 IP 提供 Premium（常见于机房/代理 IP）”；ChatGPT 用 trace `loc` + 官方 compliance，Claude 用 trace `loc` + `claude_regions.json`，Gemini 识别 `,2,1,200,"CHN"` 与 `google.com/sorry`（标橙色 `verification_required` 计入已测分母但不计可用）；Spotify 代理注册拒绝记橙色“限注册”；`disney_unlock` 因多节点 TLS 握手均 EOF 无响应，已停用请求并直接返回“暂不支持”（`4e1ab98`）。

`3d0bd9f` 在 `frontend/src/ipKind.ts` 中对已保存的 `IPPure`、`Ping0`、`IPOK` 结果按每来源一票（IPv4 优先）做多数票表决：明确区分 `机房`/`家宽`/`移动`，将 `非住宅`/`非 IDC`/`未提供` 与平票保留为 `未知`；原生/广播由 `origin_type`、`isBroadcast`/`is_native` 或注册国与使用国对比判定。出口为机房时仅在 Gemini 验证、YouTube 拒提供、Spotify 限注册原因后追加机房提示，不改变解锁结论与自动精选规则。`c775dcc`、`0734ba4` 在每日纯净度与机场体检排名中优先排满轮次节点并区分纯净度口径。

## L-014 自动精选本地订阅与设置保护、入口百条分批画像、Clash 双轮对照与报告导图

适用：自动精选（`auto-pick`）配置/运行/本地订阅、物理网卡解析节点入口、本机 Clash/Verge 实时对照、延迟报告导出 PNG。

`019aa2e`、`ebc1270`、`a1f0c28`、`8cb3183`、`5471464`、`42d8c02` 在 `application/auto_pick.go` 与 `auto_pick_subscription.go` 中实现定时精选：延迟决定“日常”组，必需解锁服务（默认 ChatGPT、Claude、Netflix 完整片库，不含 Gemini 与 Antigravity）决定流媒体/AI 组，本轮选空或失败时保留上一份 `subscription.yaml`。本地订阅仅监听 `127.0.0.1`（默认 17891，校验 16 字节 hex token 与 `.yaml` 后缀）；**首选端口被占用时本轮临时换随机端口但绝不覆盖 `subscription-port` 文件**，防止 Clash Verge 已填地址重启后失效；构建节点列表前预分配 `make([]autopick.Node, 0, len(nodes))` 防止 slice 扩容导致指针丢失追加用途；`finishAutoPickReport` 结束时重新读取磁盘配置，仅更新 `LastRunAt`/`NextRunAt` 而不覆盖运行期用户修改的设置。

`899105a`、`fd6bc58` 在 `core/entry/lookup.go` 中经物理网卡解析节点入口并按 100 个地址一批循环查询画像（修复超出 100 个地址被上游静默丢弃导致无国家的问题），未测出口的节点线路标签置灰注明“按节点名称推断”。`0103591`、`573b450`、`64c8641`、`db43f58` 在 `application/clash_comparison.go` 与 `frontend/src/clashComparison.ts` 中通过本机 Clash socket/TCP 接口与物理网卡各跑 2 次采样对照（不写工作台轮次库），展示活跃路由链并在手动切换代理组前确认，两边均失败时归为“节点本身不稳定”。`457b551`、`c73cb9a` 在 `LatencyReportImage.vue` 中导出 PNG 时去掉 `tabular-nums` 以避免导出渲染器字距挤压。

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

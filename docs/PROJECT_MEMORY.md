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

## 维护方法

方法统一继承总规则“2 项目记录”“5.1 全周期复盘与覆盖”，从下方实际覆盖记录接续。

## 本次覆盖与缺口

2026-11-10 规则核查截点 `main@35e25edaa541ed9e803dc7d17ee73050b503e5fa`：全文读取根 AGENTS 与第三轮路线，读取进度前 65 行和相关 Vault 入口/交接。整理当前入口、历史计划与上述教训，不改变产品功能或测试结果。

未逐文件/逐提交读取全仓，未覆盖所有分支、本机 dirty、真实 App/代理或生产数据库；未运行构建、Go/前端检查或实网探测。**全历史处理游标未建立**，不能把读取截点当全部历史已总结。规则写入不代表记忆 Hook/CI 已部署，也不恢复已暂停的调查或监测。

## 2026-11-10 接手与持续维护补充范围

本次从 `main@4e4256adfe609726977b432864887d116ee08ca1` 读取现行 AGENTS/本页及项目入口，补自然称呼索引、每轮主动写回和上下文恢复/纠错做法。未完成的历史覆盖仍按原记录；这次规则提交不前移全历史处理点，未重跑产品、部署拦截器或建立后台任务。

## 2026-11-11 下载链路增量归并范围

本次以 `main@fd754680434d433544bc432559f8c7a569d4e712` 为读取截点，完整读取现行总规则、项目入口及本页，定向读取进度末段和 `76d6e58`、`d87cd8d`、`ce0261b`、`35e25ed` 的有关实现与既有脱敏结果，归入 L-004。本次没有重跑测试、构建、实网请求或检查生产历史，未前移全历史处理游标。下次涉及下载时从 L-004 及其实现接续，其他历史覆盖仍按原有缺口记录。

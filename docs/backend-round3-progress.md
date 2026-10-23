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

## UI：来源条样式微调
- 异常名单明确默认关闭；“只看”使用蓝色链接样式；左卡片提示改为单行“点击切换机场”。
- 未新增测试；`go vet`、`go test ./...`、现有 378 项前端测试与构建通过。

## 用户实测第 1 批：工作台
- 去掉左机场卡，侧栏控制来源；多订阅逐行刷新，全部模式合计并标订阅数。
- 延迟的 HTTP 响应统一视为连通成功；检查器显示最近一轮数值/成功数/范围，满 3 轮才画实际点折线，相同 HTTP 说明合并。
- 根据既有 DNS/socket 证据显示“已绕开本机 TUN（网卡）”；无新接口、字段或库修改。SG04 副本实看 378 ms · 6/6 · 292–1409 ms，结论“能用”。
- 验证：HTTP 403 主路径与超时失败路径，来源单行刷新；Go vet/test、378 项前端测试和构建通过。

## 用户实测第 2 批：解锁检测
- 当前选择收敛为 AI 4 / 流媒体 6 / 日本 6 项，默认 AI+流媒体；只按服务证据给出可用、不可用、无法判断，同节点最多 6 项并行。
- 表格三列组计数，检查器按组展示全名/结论/时间与实际历史方块；旧项目归默认收起的“其他（旧检测项）”，记录不删不迁移。
- 新规则 ID：`chatgpt_unlock/claude_unlock/gemini_unlock/disney_unlock/spotify_unlock/tiktok_unlock/unext_unlock/dmm_unlock/radiko_unlock`；复用其余 7 项及原历史 details。
- 验证：Go vet/test、380 项前端测试与构建通过；SG04 副本只查看旧记录，未发真实解锁请求，新增规则的站点实测留待用户正常检测。

## DNS 核对与接线
- Mac 工作台已使用物理网卡 DNS 并过滤假 IP；SG04 10/20 的 YouTube US 旧记录确认 DNS/socket 都绑定 en1，10/18 同路径曾返回 SG，不能据此归因于当前 TUN。
- 旧持续监测改用现有物理 DNS/socket 客户端，检测后关闭连接与代理；不新增 DoH、重试或数据库迁移。
- 新样本元数据 `tun_bypass_interface` 仅在 DNS/socket 均已核验时保存；记录页显示“已绕开本机 TUN（网卡）”，旧记录不补写。
- 验证：主路径与 DNS 未核验路径；Go vet/test、382 项前端测试及构建通过，未新建监测任务或实网检测。

## 待定
- 暂无新增保护或扩展项。

## UI：检查器延迟定稿（替代前两版竖条方案）
- 三项汇总区分稳定延迟/首次连接/响应；120px 四站合并图按采样或轮次时间绘制，失败断线红叉；每站共用 0–1500ms 范围条。
- 下载显示速度/源/时间，3 次记录才画走势；429/403 中文说明及“换源重测”预选另一源；解锁实际历史方块 12px、间距 3px。
- 新请求字段 `source_id` 支持 Cloudflare/Cachefly，复用原抓取与限额；本项手动换源，自动换源仍按第 4 批实施，无历史数据库迁移。
- 验证：不新增测试，更新旧界面断言；Go vet/test、378 项现有前端测试与构建通过；实机仅查看 SG04 已有记录。

## 用户实测第 1 批补充：检查器四处小修
- 顶部结论复用稳定延迟算法；范围刻度与横条共用列宽；当前项目区块先于出口区，“综合”保留原顺序。
- 429 缩为“测速网站限流（429），不是节点问题”；来源条和多订阅逐行刷新沿用已完成实现。无新增接口或历史修改。
- 验证：复用现有测试与构建，SG04 已有记录实看，换包保留回退版本。

## 用户实测第 3 批：出口与真实落地
- 新增“出口与信誉”标签与多选批量入口，复用信誉计划；显示遮挡 IP、IP 库国家、真实落地、IPPure、Ping0、CF 系数、ASN 人机比。
- 汇总最新 IP 库、CF loc、Google 地区，与名称地区比；差异橙色、任一大陆来源红色，缺来源如实提示。此标签表格占满一行，详情放下面以容纳 7 列。
- 新规则 `google_region` 复用 YouTube 地区抓取，写原有 `details.service_region`；复用已保存 YouTube/Gemini 地区，无库迁移。
- 验证：Go vet/test、382 项前端测试与构建；新增一致/错配及大陆路径，SG04 副本只查看，不发真实请求。

## 用户实测第 4 批：下载换源
- Cloudflare / Cachefly 两源，只有 403/429 换另一源一次；复用节点客户端和原总时限，取消旧下载 admission 冷却，服务站点限流不变。
- 结果 JSON 新增可选 `target_url`，显示、趋势和“和上次比”按实际源；两源均受限才显示“测速源都限流”，无数据库迁移。
- 界面说明首选源及自动换源；收尾 Netflix 完整档位文案、出口表列宽与香港/澳门/台湾国家文字识别。
- 验证：换源成功/两源限流两个离线路径；Go vet/test、382 项前端测试和构建通过，未发真实下载请求。

## 解锁与出口显示收尾（2026-10-21）
- 解锁统计只计可用/不可用，未知与未测不入分母；无明确结果时表格灰色“未测”、顶部“还没做解锁检测”。
- 明细原因独占第二行；出口来源合并为去重标签，未测显示“未测”，结论只显示一次，恢复右侧检查器。
- 无新接口或字段，不改查询与历史；不新增测试，仅更新一处原有显示断言。
- 验证：Go vet/test、382 项现有前端测试与构建通过；换包后只查看原 SG04 历史并截图。

## 解锁分区与 Netflix 档位（2026-10-21）
- 未测项灰色“未测”且不显示原因；Netflix 双片目判定完整/仅自制/不可用，旧受限记录灰色保留，注明未验证账号播放。
- 作品 ID 配置：`core/publicservice/netflix_titles.json`；新结果标记 `title_check_version=dual_title_v1`；地区配置：`frontend/src/unlock-config.json` 的 `regions`。
- 默认日本项只测 JP：优先落地地区、再名称；批量/组合创建队列前跳过，表格“—”与检查器说明同步；出口表四列，风险取 IPPure/机器占比较高者。
- 验证：主路径及关键失败离线测试、Go vet/test、前端测试与构建；换包后用 SG04 已有记录截图，不发起真实检测。

## 三节点 TUN 对照（2026-10-21）
- 日本组纳入默认选择；默认选择键升为 `speedtest.selected-unlocks-v2`，此后保留用户选择。
- 仅 SG04 专线、一个日本节点、一个美国节点；延迟、三组解锁、出口信誉各一轮，不做下载或持续监测。
- 实测发现并修复并行轮询不更新局部状态导致队列卡住；已完成项保留，仅续跑原计划剩余项。
- 对照与截图保存在本机 `.ui-preview/three-node-tun-20261021/`；用户要求保持 TUN，取消关闭轮，不能据此判断 Google US 的成因。

## 服务适用地区（2026-10-21）
- 各服务独立地区列表，空为不限；日本服务沿用 JP 默认，组合/批量队列和统计共用规则。
- 本机字段 `unlock_service_regions`、`skip_by_applicable_regions`；设置增加服务/地区编辑器，选择界面改“按适用地区跳过”，默认开。
- 不新增接口；节点优先真实落地、无结果按名称，跳过项不进入计划与进度。
- 验证：设置保存回读与地区主路径/不适用路径、Go vet/test、前端测试和构建；用 SG04 副本查看，不新增实测。

## 解锁按服务矩阵（2026-10-21）
- 解锁标签新增“按节点 / 按服务”；共用选择项目、筛选和检测范围，地区列按真实落地、无结果按名称；矩阵单独设置紧凑表格样式。
- 格子只计可用/不可用，未知不入分母；服务适用地区规则同第二件，右侧列节点结论/原因及最近时间。
- “检测未测”复用单服务计划；“导出可用节点”复用 Clash 配置组，仅读现有接口，无后端改动。
- 验证：矩阵范围/计划/导出主路径与导出失败、Go vet/test、前端测试与构建；SG04 副本看图，不新增真实检测。

## YouTube / Google 地区修复（2026-10-21）
- 旧三节点均 HTTP 200、未截断、保存 US；历史未保存最终 URL/HTML，不能复原 consent；先读记录后才发限定复测。
- 新结果保留 `final_url`、`region_field`/`region_raw` 和页面提示；拒绝把 INNERTUBE_CONTEXT_GL 当地区，consent/无明确地区均未知。
- Premium 读取 ytInitialData 实际内容，保留浏览器 UA/Accept-Language；真实页面确有地区拒绝标题，不能硬判美国节点可用。
- 验证：主路径/consent及默认字段失败路径、Go vet/test；限原三节点+一个 HK，只复测 YouTube/Google，TUN 保持开。

## AI 地区判法（2026-10-21）
- ChatGPT 改为 trace loc + 官方 compliance/cookie_requirements；Claude 为 trace loc + 官方支持地区表；Gemini 读取明确可用标记。
- 规则版本升为 2；官方列表 `core/publicservice/claude_regions.json`，旧记录不迁移、不删除；不登录、不发送聊天或调用 Antigravity。
- 保留浏览器 UA/Accept-Language、既有超时和站点限流；缺证据、挑战与限流均无法判断。
- 验证：主路径/缺证据及地区拒绝、Go vet/test；原 SG04/JP/US 三节点仅复测三项，TUN 开启。

## 矩阵地区与部分解锁显示（2026-10-21）
- 地区列和适用地区跳过使用 IP 库 / Cloudflare / 名称多数；三者互异选 IP 库，Google 不一致只在矩阵明细提示。
- Netflix 仅自制改为部分，计入已测但不计可用，格子橙色且不进入可用导出；地区跳过勾选框移入适用地区卡片。
- 前端字段 `consensusRegion` / `googleNote`；不改历史库，无新接口，不追加检测。
- 验证：既有统计断言及多数/分歧路径、Go vet/test、前端测试和构建；用本轮已有实测结果截图。

## 三节点 trace 与 TUN 结论（2026-10-21）
- 原 JP04 / SG04 专线 / US04 的 ChatGPT、Claude trace loc 均与节点地区一致，本轮请求确实经过所选节点，未见本机 TUN 干扰。
- 旧 Google US 来自 YouTube 默认 INNERTUBE_CONTEXT_GL 的误读，不是本轮 TUN 串线；不扩大为所有站点、所有时间的路由保证。
- 依据：61a0af9 前保存的三节点 AI / YouTube 原始响应与结果，本项不新增检测。

## Gemini 地区与 Google 验证页（2026-10-21）
- Gemini 旧 CN 来自 bootstrap 数组 `,2,1,200,"CHN"`；原请求已为 `en-US,en;q=0.9`，不是 zh-CN 回显。
- 英文请求读到 CN 时红色“Google 判定为中国大陆”，沿用真实落地 Google 来源；新明细 `request_language` / `region_field` / `region_raw`。
- Gemini / YouTube / Google 地区遇 sorry/验证页记 `verification_required`，橙色“需人机验证”，独立于未知、不计可用/不可用分母。
- 验证：Go vet/test、前端 389 项及构建；TUN 开启，原 JP04/SG04/US04 本轮 Gemini 均 302 验证跳转，无新 CN 证据。

## Disney+ / Spotify / TikTok 判法修复（2026-10-21）
- Disney+ 修正地区 GraphQL 公共 SDK 认证；Spotify 改注册地区校验（不提交邮箱/密码/同意条款）；TikTok 读取完整页面启动地区。
- 三项规则版本升为 2；Spotify 明确代理拒绝记 `service_rejected`，显示“Spotify 拒绝代理 IP 注册”；旧记录不迁移。
- 实测仅原三节点四项共 12 次，Disney+/Spotify 定向补测 6 次：TikTok JP/SG/US 均取得地区，Spotify 三项均拒绝代理 IP。
- Disney+ 三节点仍在 TLS 握手 EOF，未取得地区结论；浏览器 TLS 复用无效已撤回，不再追加探测。原始返回和截图仅存本机。

## Gemini 旧 CHN 记录定位（2026-10-21）
- 节点：良心云 / 默认订阅 / `🇺🇸美国04|流媒体|0.1x`（美国 04 · 流媒体 · 0.1×）。
- 台北时间 2026-10-21 14:30:47.818149 开始，14:30:53.309014 完成；UTC 为 06:30:47.818149Z–06:30:53.309014Z。
- 记录 `public_service_attempt_f8204db4fda74169decb1e06`；Gemini bootstrap 数组读到 CHN 并保存 `service_region=CN`，原请求已使用 en-US。
- 核对本机已保存 `ai-results.json`、节点身份和时间；仅读取，没有为此新增请求或改写旧记录。

## 解锁统计与注册限制收尾（2026-10-21）
- “需人机验证”计入已测分母，不计可用；表格、检查器顶部/分组、按服务矩阵统一，SG04 AI 为橙色 2/3。
- Spotify 代理注册拒绝显示橙色“限注册”，原因“识别为代理 IP，已有账号通常不受影响”；不改写旧记录。
- 前端共用 `unlockMeasured`，注册限制复用部分状态；无新接口，不追加 Gemini/Spotify/TikTok 请求。
- 验证：既有统计测试与主路径更新、Go vet/test、前端测试/构建，并用原 SG04 副本看图。

## Disney+ 停止支持（2026-10-21）
- 仅最后试一次不同办法：原 SG04，Firefox TLS 指纹；仍为握手 EOF，无 HTTP 响应，到此停止，不再补丁或追加检测。
- 删除无效设备/令牌/GraphQL 请求流程；`disney_unlock` 新任务直接“暂不支持”，不创建节点客户端、不发请求。
- 从默认及分组勾选去掉；自定义列表显示禁用项，旧检测记录保留可查看；原有成功记录保持原结论。
- 验证：不打开客户端的主路径、默认选择/显示测试与 SG04 副本；本轮真实检测仅上述 1 次，TUN 始终开启。


### 2026-10-21 延迟 19:00 排查与地址顺序修复
- 确认 bug：香港高速04／日本高速05／新加坡高速01 每次采样重试失效 IPv4 首地址，约 2.5 秒后才连到下一地址（DNS 分别 184／86／107 ms，并非 IPv6 先试）；现在同一轮优先复用成功地址，不增加重试。
- 授权各复测一次：稳定中位 3064→617、3112→853、2934→425 ms，均只跑四站延迟；TUN 保持开启。
- 日本01／02 CUCM 均为 `context deadline exceeded`；flow=xtls-rprx-vision、Reality、TCP、SNI=download-porter.hoyoverse.com 与可通 Vless 同类，safari／ios 也有可通对照，未发现解析错误。日本02 CTCU 的慢不符合上述地址问题；线路／Reality／WS 对端需用户用 Verge 对照，不能确认境外绕路。
- 旧样本只有总耗时及整轮 DNS/TCP 汇总，缺逐次 TLS/首包分段，不能补造；48 项中 44 项实际采样，另 4 项因 revision 配置冲突未执行。

### 2026-10-21 延迟检测报告
- 侧栏、手动结束、记录页“报告”共用报告窗口：四计数、同条件比较、最快五个、问题分组、重测计划／Clash 导出／逐个明细；监测轮只入列表。
- 复用批次与监测轮次／样本；Web 监测样本读取补接已有 `offset`，不新建表。未执行单列，不算无响应；疑似问题仅用用户指定两条规则。
- 验证：地址优先顺序主路径＋现有全失败路径、报告主路径／未执行路径；本地 go vet/go test/npm test/build，SG04 副本页面检查及换包截图。


### 2026-10-21 检查器改版 1：稳定延迟统一
- 表格站点值、站点范围条中位圆点／数字、表格与检查器结论统一使用去掉每站首次后的中位数；首次连接保留在大数字和悬停提示。
- 复用 `stableLatency`、`inspectorSamples`、`inspectorMedian`，没有足够稳定采样时不拿首次填充；无新增接口／表。
- 验证：现有前端测试与构建、本地 Go 检查、SG04 副本同节点对照并换包截图。


### 2026-10-21 检查器改版 2：本次与历史
- 本次固定为一轮，历史独立切换 24 小时／7 天／30 天（默认 7 天），显示轮次可用率、稳定中位、P90−P10 波动、最近失败；超过 30 轮按天画中位。
- 最近 10 轮可展开，点击切换本次并可回到最新；复用现有节点延迟／监测历史接口和采样工具，不建表。
- 只新增一个主路径测试：点击旧轮次再回到最新；更新旧断言以区分首次和稳定值，现有测试／构建与 SG04 副本检查后换包截图。


### 2026-10-21 检查器改版 3：按历史选节点
- 延迟表格增加可点击排序的“7 天可用率／波动”，与历史区同一轮次口径；未测留空，不拿本次值代替历史。
- “稳定能连的”改为近 7 天有响应轮次占比 ≥90%，独立于顶部时间筛选；复用节点历史读取，不新增接口／表。
- 验证：不新增测试，现有前端测试／构建与 SG04 副本排序、筛选检查后换包截图。


### 2026-10-21 历史区实测反馈（取代上方旧可用率口径）
- 历史／表格统一为采样“响应率”，列出失败轮数、最近失败站点与次数；当前轮加标识，部分失败写清站点并标橙。
- 波动改名“波动幅度”，不足 5 轮不算；仅 1 轮隐藏历史指标／图，保留说明和列表。
- 检查器非综合标签只展开当前项目，出口等收为底部可切换小标签；7 天列在任务结束或用户刷新时更新，普通历史读入不触发重读。
- 验证：不新增测试，现有前端测试／构建、SG04 和单轮节点副本页面检查、换包截图。


### 2026-10-21 检查器改版 4：报告定位轮次
- 报告节点名打开工作台检查器并定位报告内的采样轮；直接带入已保存结果，即使不在顶部时间范围内也可查看，回到最新会清除定位。
- 前端临时状态 `latencyReportFocus`，复用原 attempt／monitor run ID，不新增接口或表；导航时解除阻挡该节点的筛选，筛选提示同步采样响应率口径。
- 验证：不新增测试，既有轮次切换主路径与全套前端／Go 检查；SG04 副本从 10/20 报告点入并确认 373 ms／864 ms／24/24，换包截图。

### 2026-10-21 延迟批次 4 项未执行排查（revision 冲突）
- 是 bug：`buildProxyServerPortKey` 对 YAML 解析出的嵌套 `map[any]any` 直接 `json.Marshal` 必然失败，`EquivalentProxyConfiguration` 恒为 false；同配置别名被当成“多个 TLS/连接配置”，resolver 跳过整项。
- 修复：比较前递归归一化 YAML map；别名去重随之恢复（同配置的日本 07/09、新加坡 07/08 并回 06/08，节点表少 4 行，正好是原先跳过的 4 项）。
- 无新字段／接口；验证：SG04 副本缓存冲突组 4→0，新增嵌套 map 等价／路径不同两条断言，`go vet`、`go test ./...` 通过；未发起真实检测。

### 2026-10-21 合并后历史记录核对
- 只读查 SG04 副本全部表：日本 07/09、新加坡 07/08 在任何记录表、批次项里均为 0 条；这几个别名从未单独入库，合并不隐藏记录。
- 保留行历史仍在库、未改动：日本06 5 条、日本08 6 条、新加坡06 5 条；它们属于 10/16 前旧 revision，按现有“同 revision 才显示”口径，工作台历史区默认看不到（合并前同样如此）。
- 无需迁移或改库；未发起检测，无字段／接口变更；换包为 cbd05f20，截图 `.ui-preview/alias-merge-20261021/`。

### 2026-10-21 历史区包含旧配置
- 读取改为聚合同一订阅、同一节点名的所有已保存配置：先从批次项找回被订阅更新替换的旧身份，再复用 `node-revisions` 与原延迟／监测分页接口；旧轮次带 `oldConfig` 标记。仅前端，无新字段／接口／库改动。
- 显示：轮次列表灰色“旧配置”标签；历史走势图在配置变化时间画竖虚线，悬停“10/16 22:42 订阅更新了这个节点的配置”；指标含新旧配置。
- 表格“7 天响应率／波动幅度”复用同一读取函数，自动包含旧配置。
- 验证：SG04 副本日本06 历史区 30 天 5 轮全带标签、虚线 10/16 22:42，表格 26%／1541 ms；395 项前端测试与构建通过，未发起真实检测。

### 2026-10-21 旧配置不拖累当前判断
- 新增 `latencyHistorySplit`（`all`／`current`／`old`）：旧配置单独计价，`allOld` 表示范围内只有旧配置轮次。仅前端，无新接口／库改动。
- 表格两列：全旧配置时数字置灰加“旧”，不参与这两列排序、不进“稳定能连的”筛选；新旧都有时悬停显示“当前配置 N 轮 X% · 旧配置 M 轮 Y%”。
- 检查器四个指标下加一行“当前配置：N 轮 · 响应率 X%”，当前配置没测过时写“当前配置还没测过”。
- 配置变化虚线悬停：旧配置与新配置的 node_key 服务器段不同时，追加“服务器地址也变了，旧记录可能不是同一台机器”（只加提示，匹配逻辑不变）。
- 验证：SG04 副本日本06／01／08 文案与灰标签正确，排序把全旧节点排到末尾；48 节点切延迟标签首个数字 234 ms、全部出数 1.46 s；`npm test` 395 项、类型检查与构建通过，未发起真实检测。

### 2026-10-21 解锁判定对照 subs-check：Netflix
- 照 `checkNetflixCDN` 先请求 Fast.com 测速接口，403 判封禁，拿到地区即“完整片库”；失败才走两部片目（自制 70143836／非自制 81280792），任一 403 判封禁。
- 地区优先取 `client.location.country`，`targets[0]` 只作后备：上游只读后者，新加坡 04 实测报 JP，与同节点片目页的 SG 不一致；两处来源都记进 details。
- 待办：Fast.com 现在排在节点 DoH 之后，DoH 超时会连带跳过预检（香港 01 本轮即如此），下一轮把预检提前。

### 2026-10-21 解锁判定对照 subs-check：YouTube Premium
- 请求 `youtube.com/premium?hl=en`，请求头与地区字段顺序照 `CheckYoutube`；有 Premium 标记才读地区，读不到写“无法判断”，不默认 US。
- `www.google.cn` 只在非 pagead 遥测时算送中；首轮四节点全判 CN 正是页面自带 `google.cn/pagead` 遥测，已修并补测试。
- 复测：新加坡／日本／美国均“该地区不可用”（页面头图原文 not available in your country），香港 01 TLS 握手 EOF 无结论；无节点判 CN。

### 2026-10-21 解锁判定对照 subs-check：Gemini
- 不可用国家表照 `CheckGemini`：CHN、RUS、BLR、CUB、IRN、PRK、SYR、HKG、MAC；CHN 写“Google 判定为中国大陆”，其余“Gemini 在该地区不可用（XX）”。
- 验证页仍是橙色“需人机验证”；美国 04 从旧的 CN 记录变为 302 → `google.com/sorry/index`，页面不再出现 CN。

### 2026-10-21 解锁判定对照 subs-check：Disney+
- 照 `CheckDisney` 补完 devices → token → GraphQL 地区流程（含 JP 特判与主页兜底），仅在授权范围内用新加坡 04 试一次。
- 仍为 TLS 握手 EOF、没有任何 HTTP 响应，维持“暂不支持”，进度里写原因，不再追加尝试。

### 2026-10-21 四节点真实验证
- 节点：新加坡 04、日本 01、美国 04、香港 01；只跑 Netflix、YouTube Premium、Gemini，Disney+ 仅新加坡 04 一次，TUN 全程开启。
- 结果：Netflix 新／日／美均完整且地区与节点一致，香港 01 经节点 DoH 超时未出结论；YouTube 无节点判 CN；Gemini 四节点都“需人机验证”。
- 检查：`go vet`、`go test ./...`（20 包）、前端 396 项与构建通过；截图 `.ui-preview/round4-unlock-20261021/round4-matrix.png`、`round4-sg04-unlock.png`。

### 2026-10-21 解锁收尾：Fast.com 提前
- 预检挪到节点 DoH 之前，出结论就跳过固定 IPv4；DoH 超时不再吞掉结论。
- 香港 01 只复测 Netflix 一项（TUN 开启）：1 个请求得完整片库（HK），details 记 `target_dns_skipped=fast_api_decided`。
- 新增一条主路径断言（先 Fast.com、后不解析）；go test 全量通过，别项未动。

### 2026-10-21 解锁收尾：YouTube 文案与配色
- 非 CN 的“not available”改成“不向该 IP 提供 Premium（常见于机房/代理 IP）”，橙色；只有判定 CN 才红。
- 服务端 summary 同步，前端对旧记录也改用新文案；补橙/红两态测试，前端 397 项通过。

### 2026-10-21 第四轮第2项：定时精选任务（后端）
- 新增 `core/autopick`（配置／报告模型、三档时间网格、请求估算、分组常量，无网络 IO）与 `application/auto_pick.go`：复用 `StartWorkbenchLatencyBatch` 测延迟、`publicservice.Checker.Check` 测解锁、`prepareExportProxies` 导出 Clash。
- 判据：本轮响应率 ≥ 90%（默认）且中位延迟 ≤ 上限（默认 800 ms）进“日常”；勾选解锁组时组内服务须全通过才追加“流媒体”／“AI”，一组都没过则排除。
- 报告写 `auto-pick/reports/*.json`（保留 50 份，不动 `history.db` schema），标签“自动精选”，不弹窗；本轮全空或失败时沿用上一份成功结果并在 notes 写明，订阅文件不写空。
- 接口：`GetAutoPickStatus`／`SaveAutoPickConfig`／`RunAutoPickNow`／`ListAutoPickReports`／`GetAutoPickReport`，Web 与桌面双通道；默认关闭。
- 验证：`go build ./...`、`go test ./application/ ./core/autopick/ ./adapter/...` 通过；新增主路径（选中写入）与失败路径（没选中保留上次）两条测试。

### 2026-10-21 第四轮第2项：本地订阅地址
- `application/auto_pick_subscription.go`：`net.Listen` 绑 `127.0.0.1`，端口默认 17891，占用时回退随机；令牌 16 字节 hex 与端口写入数据目录，重启复用。
- 路由 `/sub/{token}.yaml`，只认 GET／HEAD，令牌不符、路径穿越、扩展名不对一律 404；`Cache-Control: no-store`；仅有订阅文件时才启动监听。
- 文件同时存 `<数据目录>/auto-pick/subscription.yaml`（含节点凭据，已在 `.gitignore` 覆盖的数据目录内，不进 Git、不上传）。
- 验证：真机 curl `200`／`text/yaml`／2622 字节，错误令牌与 `/sub/../config.json` 均 `404`，`cmp` 与落盘文件逐字节一致。

### 2026-10-21 第四轮第2项：自动精选卡片与报告入口
- “持续监测”页新增 `AutoPickCard.vue`：开关、运行时间（6 小时／12 小时／每天几点）、范围（机场 + 地区筛选）、检测项目、两条规则；估算“节点数 × 项目”，超 500 标橙。
- 显示下次运行、上一轮摘要（如“选中 4 个 · 日常 4 · 10/21 23:35”，沿用上次时标橙）、订阅地址 + 复制 + 一句 Clash Verge 说明；运行前弹确认层写明节点数与请求数。
- 新增 `AutoPickReportModal.vue`；`LatencyReports.vue` 并行读取自动精选报告，以“自动精选”徽章排在前列。
- 验证：真实手动运行一次（良心云 · 新加坡 · 只测延迟）选中 4 个并写入订阅；`npm test` 397 项、`npm run build` 通过；截图 `.ui-preview/round4-autopick-20261021/`。

### 2026-10-21 第四轮第2项：收尾修正
- 首选端口被占用时，本轮临时换端口但不再改写 `subscription-port`，免得已填进 Clash Verge 的地址在下次启动后失效；补一条回归测试（占用端口 → 换端口 + 文件不变）。
- 卡片“响应率／稳定延迟”两个输入框的单位改成同行显示，不再折行。
- 换包 `a1f0c287`：单实例、窗口 1360×900、17891 订阅仍 200；截图同目录更新。

### 2026-10-22 自动精选：必需服务与历史归档
- AI 组默认必需 ChatGPT、Claude，流媒体组默认必需 Netflix 完整片库；Gemini、Antigravity 默认不要求（验证页与绑定凭据几乎过不了），卡片每组可展开勾选必需服务，估算按实际勾选项计。
- 挑选：延迟决定“日常”，解锁只决定用途组；“需人机验证／无法判断”算没通过，但不影响节点进日常组，也不再整体剔除。
- 稳定延迟改为“去掉首次采样后的中位数”，与检查器口径一致；每轮延迟与解锁结果都写进 `measurement_rounds`（trigger=auto_pick），节点历史区标“自动精选”，解锁结果按 attempt→staged→committed 落库。
- 重挑：用上一轮报告（ap-1792596937-4da5e3，10 个新加坡节点）重放新规则，仍选中 4 个（285／563／740／786 ms），订阅逐字节一致；新报告 ap-1792599658-repick 注明“未发起新检测”。
- 卡片说明补一句“定时任务和订阅地址只在 CST 运行时有效。”

### 2026-10-22 第四轮第3项：入口与出口
- 新增 `core/entry`（`<数据目录>/entries.json`：按主机缓存解析结果、按 IP 缓存画像 30 天、本机位置 1 小时）与 `speedtester.ResolvePhysicalHost`／`NewPhysicalHTTPClient`：DNS 与 HTTP 都绑物理网卡，绕开 TUN 的 fake-IP；`POST /api/workbench/entries`＋桌面 `ResolveWorkbenchEntries`，`refresh=false` 只读缓存、不发请求。
- 出口表新增“线路”列（如“上海 联通 → 新加坡 AWS”＋直连／中转／国内中转；Cloudflare 任播单列为“任播”），检查器出口区顶部一行“本机 → 入口 → 出口”；本机在境外且入口在大陆时，出口区与延迟报告“偏慢”组带绕路说明。
- 出口检测计划开始前顺带解析入口；出口标签多选栏新增“解析入口”（不走节点、不写检测历史）。自动精选卡片补一句“延迟只测 Cloudflare 一站”。
- 实测（只解析入口）：良心云 50 个节点入口全在境外——AWS 香港／东京／新加坡、Cloudflare 任播、台湾中华电信、英国 Only Servers、美国 FDCservers；无大陆入口。本机 中国·联通（182.*.*.176）。
- 收尾修正：IP 画像改成按 100 个一批循环查询（原来一批超过 100 个就静默丢掉后面的地址，109 个地址时有 11 个没有国家）；出口没测过的节点，线路标签置灰并提示“按节点名称推断”，测过出口的才是绿／黄。
- 验证：`go test ./application/ ./core/entry/ ./core/speedtester/ ./adapter/...`、前端 403 项、`npm run build` 通过。
- 根证书弹窗溯源（2026-10-22）：弹窗出现在**首次打开新打包的 `CST Mac fd6bc581.app`** 之后。时间线：先停掉 18621 的 Web 预览 → 用 `打开已有SG04历史.command` 启动新包 → 等约 18 秒，进程在但 `System Events` 读不到窗口 → 置前并 `screencapture`，截图里是“CST Mac fd6bc581 想更改系统根证书”。当时 `capture.cjs`／`probe.cjs` 用的 headless Chrome 早已退出；且弹窗标题是应用包名而不是浏览器，所以不是截图脚本触发的。代码与依赖里没有 `security add-trusted-cert`／`SecTrustSettings` 调用（已检索），来源未定位，先记为待查。
- 处理约定：**以后遇到系统弹窗一律点“不允许”并截图留证，不要点允许**；本次已点“不允许”，未改动系统钥匙串，界面和订阅都正常。

### 2026-10-22 第四轮第4项：历史展示（心跳条／时段分布／机场体检）
- 检查器延迟历史顶部加心跳条：最近 50 轮（30 天，不随范围选择器缩水）每格一轮，全响应绿／部分响应橙／无响应红，旧配置同色浅一档（opacity .42）；悬停写时间、稳定延迟、响应数。上方固定窗口 24 小时／7 天／30 天响应率，因此历史改为一次读 30 天、范围选择器只做本地过滤。
- 时段分布：按本机时区把范围里的轮次分到 24 格，每格写稳定延迟中位数与失败率（无响应采样占比，8e7717e 补），颜色越慢越深（同一范围内相对归一），有失败的时段加红边；不足 2 轮的时段留空，不写 0。结论只在固定 4 小时块与其他时段的中位数比 ≥1.5 倍、且两边各 ≥3 轮时给出（例如“晚 20–23 点稳定延迟约为其他时段的 2.2 倍”），否则不写。旧配置轮次也计入并在说明里注明。
- 机场体检：把保存的节点测试按时间聚成轮（间隔 ≤10 分钟、至少 2 个节点；单节点检查不算一轮），每轮一格按有响应节点比例着色；故障记录两类——整轮超过一半节点无响应（相邻轮合并成一次）、单节点连续 3 轮无响应（恢复轮为结束时间，未恢复写“仍在持续”），每条写起止时间、影响节点数、节点名。全部用现有 `workbench_latency_tests` 拼，没有新表。
- 线路补“跨境直连”：本机在中国大陆、入口在境外且与出口同国时显示，不加绕路提示；`routeKind` 增加本机国家参数，原有直连／中转／国内中转口径不变。刷新入口后出口表 79 行：跨境直连 59、中转 9、任播 11（样本里没有大陆入口，所以没有国内中转）。
- 测试：新增 `history-pulse.spec.ts` 5 项（时段分组、结论阈值、两类故障事件主路径），线路规则补 1 项；前端 409 项、`go test ./application/ ./core/entry/ ./core/speedtester/ ./adapter/...`、`npm run build` 通过。
- 截图：`.ui-preview/round4-history-20261022/`（`inspector-jp01-cucm-history.png`、`inspector-sg04-dedicated-history.png`、`airport-health-liangxin.png`）。这份副本的轮次稀疏：日本 01 高速 CUCM 6 轮（24 小时 0%、7 天 58%、30 天 48%），时段只有 15 点（1,576 ms／33%）与 21 点（1,890 ms／50%）有 ≥2 轮，其余留空；新加坡 04 专线 11 轮（80%／82%／73%），时段 16 点 277 ms／8%、21 点 226 ms／50%、22 点 552 ms／0%；结论句两边都因差异不到 1.5 倍留空；良心云 30 天只有 10 个成批轮次、没有整轮故障，故障列表里是两条节点连续离线（台湾专线 01 五轮、美国 04 三轮）。
- 收尾：8e7717e 补上时段格失败率后重新打包 `CST Mac 8e7717e3.app`（core sha256 518b14a5…，minimum_macos 12.0.0，ad-hoc 签名），入口脚本切到新包、回滚脚本指向 `f081027f`；停 18621 预览后启动验收——单实例、窗口 1360×900、本地订阅 `/sub/<token>.yaml` 返回 200／2622 字节，无系统弹窗。

### 2026-10-22 第四轮第4项界面修正：时段条、全失败合并、体检响应率
- 时段分布改成一行 24 格：每格一小时，0／3／6…21 标刻度；有数据的格子按本范围内相对快慢上色（18%–70% 强调色），有失败加红边；不足 2 轮的格子画浅灰底而不是空白；数字只留在悬停提示（几点、轮数、稳定延迟、失败率）。格子下面一行“最慢：22 点 1,890 ms · 16 点 1,576 ms”，不足两个时段有数据就不写；1.5 倍结论句阈值不变。
- 顺手修掉上色底色：`heat()` 原来混 `--hd-soft`，这个变量在主题里没有定义，`color-mix` 整条失效，数据格其实是透明；改用 `--hd-empty-bg` 后颜色才真正显示。
- 四站全部无响应时合并文案：轮次列表写“0/24 · 四站全失败”，“最近一次失败”写“10/21 19:00 · 四站全部无响应”；只有部分站失败才逐站列“某站 N 次无响应”。判定条件是这一轮响应数为 0 且四个站都有失败样本。
- 体检页“可用率（节点轮次）”改为按采样的“响应率”：`84%（3,004/3,583 次）`，下面一行“131 轮中 37 轮有失败”；删掉“一轮任一采样有响应即记可用”的旧说明。心跳条下面补图例“绿＝全部节点有响应 · 橙＝部分节点无响应 · 红＝超过一半无响应”。
- 验证：`npm run build` 通过，前端 409 项通过；截图 `.ui-preview/round4-history-20261022/inspector-jp01-hour-strip.png`（日本 01，16／22 点有数据）与 `airport-health-liangxin.png`（良心云，10 格心跳、2 条节点连续离线）。

### 2026-10-22 第四轮第5项：出口 IP 类型（机房／家宽／移动 · 原生／广播）
- 先查已保存记录，不新增来源。IPPure 的 `ippure_ip_quality` 里有 `ip_type`（`住宅（第三方判断）`／`非住宅`／`未提供`）、`origin_type`（`原生 IP`／`广播 IP`／`未提供`）、`asn`、`organization`、`fraud_score`；Ping0 的 `ping0_ipv4`／`ping0_ipv6`／`ping0_ip_quality` 里有 `ip_type`（`机房 IDC`／`非 IDC（不等于家宽）`）、`asn`、`organization`、`risk_score`。两边都没有“移动”字段，也没有 ASN 类型；IPPure 的 `origin_type` 是唯一能判原生／广播的字段。另发现现有 IPOK（`ipok_ipv4`／`ipok_ipv6`）也保存了 `ip_type`（`hosting`／`residential`）与 `isp`，但四个目标节点的 IPOK 记录都在旧的 `config_revision_key` 下（如新加坡 04 专线是 `rev_fff61959…`，当前 `rev_64e3839e…`），不会进入当前配置的判定；美国 04 没有 IPOK 记录。因此没有补来源、没有发新请求。
- 判定口径：每个来源一票（IPPure、Ping0、IPOK；同一来源的 IPv4／IPv6 只算一票，IPv4 优先）。`机房 IDC`／`hosting` 记机房，`住宅`／`residential` 记家宽，`移动`／`mobile` 记移动；`非住宅`、`非 IDC`、`未提供` 明确不当作机房或家宽，记未知。多数票胜出，平票写未知。原生：`origin_type` 为广播／原生；`isBroadcast`、`is_native` 布尔值；或同时有 `registered_country_code` 与 `country_code` 且不同时记广播；没有信号就写未知。类型和原生分别投票，互不牵连。
- 显示：出口表在“风险”前新增“IP 类型”列，标签如 `机房·原生`、`机房·广播`、`机房·未知`、`未知`；机房灰、家宽绿、广播橙（广播优先于类型着色），未知虚线上灰。检查器出口区新增一行“IP 类型：机房（AWS）· 原生 · 来源 IPPure、Ping0”；缺字段时写“原生未知”或“未测（IPPure / Ping0 / IPOK 没有类型记录）”，不补 0、不猜。
- 解锁联动只加提示、不改判定：Gemini“需人机验证”、YouTube“不向该 IP 提供 Premium”、Spotify“限注册”三条原因，在出口为机房时追加“出口是机房 IP，这类服务常拒绝或要求验证”。判定状态、可用数和自动精选规则都没动；三处都走新的可选 `ipType` 参数，没传时行为与原来一致。
- 离线验证（30 天范围、当前配置）：新加坡 04 专线 Ping0 IPv4／IPv6 都是机房、IPPure 两次都返回“未提供”，所以是 `机房·未知`，来源只写 Ping0；美国 04 流媒体 IPPure 非住宅＋原生、Ping0 机房，是 `机房·原生`，来源 IPPure、Ping0；日本专线 04 IPPure 广播、Ping0 机房，是 `机房·广播`；日本专线 01、香港专线 01、日本专线 02 当前配置没有 IPPure／Ping0 类型记录，保持 `未知`。没有为这四个节点额外发请求。
- 测试：新增 `ip-kind.spec.ts` 2 项（两票机房压过一票、冲突平票未知、原生按注册国与实际使用国判定）；`npm run build` 通过，前端 67 个文件 411 项通过。
- 截图：`.ui-preview/round5-iptype-20261022/`（`exit-table-ip-type.png` 专线列表、`exit-table-us04.png` 美国 04 机房·原生、`inspector-sg04-exit.png` 新加坡 04 检查器）。

### 2026-10-22 根证书弹窗溯源（现有证据不支持归因于 CST）
- 包内只有两个二进制：`CST-Candidate`（只用 `execv`／`getenv`，仅链 libSystem）与 `cst-core`（无 `SecTrustSettings*` 符号、无 `Authorization*`／`SecItemAdd` 引用，字符串里也没有 `add-trusted-cert`）；入口脚本只有 `open -n` 加参数，打包只有 ad-hoc `codesign --sign -`。
- 系统侧：14:26:48 启动时只有 LaunchServices／RunningBoard 注册；cst-core 前 5 分钟的 XPC 对端没有 `trustd`／`securityd`／`authd`；14:00–15:30 全库没有 `SetTrustSettings`／`trust-settings` 授权记录，`trustd` 在 14:20–15:00 零日志。
- `SecurityAgent`（渲染“修改系统根证书”密码弹窗的进程）所查日志中未见当天启动或活动记录，最后一次是 10-21 23:53；该时段唯一授权请求来自周期性 `mdmclient`（14:20、14:21 也在跑）与 `systemmigrationd`，与 CST 无关。
- 结论：现有证据不支持将弹窗归因于 CST；原截图缺失，真实来源无法确认。不改代码、不打包；以后遇弹窗按“点不允许＋截图（带标题与按钮）”记录。

### 2026-10-23 第五轮第1项：自动精选卡片（未保存不启动、用服务端估算、重进恢复轮询）
- 未保存拦截：表单与最近一次保存的配置做规范化比对（数组排序、地区去首尾空白），有改动就显示“设置有修改，请先保存”、运行按钮禁用；保存失败保留编辑内容且仍不可运行；确认框只用 `status.estimate` 的节点数与检测项数。
- 文案：改成“本轮 N 个节点 · 每节点 M 项检测 · 共 K 项检测”，N/M/K 取自 `status.estimate`（后端已按地区筛选并去重——同一机场 50 个节点、筛选“日本”后是 13 个），下面小字“延迟每节点采样 6 次；解锁项目可能包含多次请求。”，采样数读 `config.sample_count`；未保存时只写“保存后更新范围与数量”，500 提醒改称“项”。
- 轮询：`load()` 首次读到 `running=true` 就复用原 3 秒轮询；轮询只更新状态（`applyStatus`），用户正在编辑时不覆盖表单；结束时刷新摘要、报告入口与订阅更新时间，离开页面停止。
- 验证与换包：新增 `auto-pick-run-state.spec.ts` 2 项（未保存不能启动＋保存失败保留编辑；重进运行中任务轮询到完成并停止），旧 required-services 断言同步新文案，前端 68 文件 413 项与 `npm run build` 通过；截图 `.ui-preview/round6-autopick-20261023/`；包 `CST Mac 54714640.app`（core sha256 `c676076a…`），入口切新包、回滚指向 `3d0bd9fc`，验收单实例 PID 94667、窗口 1360×900、订阅 200／2622、无系统弹窗。

### 2026-10-23 第五轮第2项：自动精选报告用途与运行期保存的设置
- 用途丢失：检测计划存的是 `report.Nodes` 元素指针，后续 append 扩容后指针指向旧数组，解锁通过追加的 Purposes 写不进最终报告；改为进循环前 `make([]autopick.Node, 0, len(nodes))` 预分配，节点用途、分组计数与订阅一致，旧报告按原样保留不回填。
- 设置覆盖：`finishAutoPickReport` 去掉本轮旧 `cfg` 参数，结束时重新 `LoadAutoPickConfig()` 只更新 `LastRunAt`／`NextRunAt`（用户已关闭就保持关闭）；本轮检测与报告仍用启动时配置，读不到配置就不写回。
- 验证：多节点解锁用例补“落盘报告首个节点含 AI 用途”断言，新增 `TestAutoPickKeepsSettingsSavedDuringRun`（运行中关闭并改地区阈值，结束后新设置保留、报告仍记旧规则）；两条断言都先在旧代码上跑出失败。`go vet`、`go build ./...`、`go test ./...`（21 个包）通过，未发真实检测请求。
- 截图 `.ui-preview/round7-report-20261023/auto-pick-report-offline.png`（临时数据，注明“离线示例”；日常 3／流媒体 2／AI 2 与各节点用途一致）；提交 `42d8c02`、包 `CST Mac 42d8c02e.app`（core `ce935258…`），入口切新包、回滚指 `54714640`，验收单实例 PID 2248、窗口 1360×900、订阅 200／2622。

### 2026-10-23 第五轮第3项：一条命令构建 Mac 包
- 新增 `scripts/build-mac-candidate.sh`：前端构建 → `go build -tags production -buildvcs=true -trimpath`（SDK 15.5／macOS 12／`-X main.commit`）→ 现有打包脚本；`set -euo pipefail`，任一步失败即停，`CANDIDATE_SHA` 与 HEAD 不符直接拒绝，成功后打印包路径与 core 内嵌提交（不符即报错，脏树警告）。
- `package-mac-candidate.py` 职责与 CI 用法不变（仍只打包已有核心）；`AGENTS.md` 新增“日常打包”一节，日常只跑这一条命令，退出旧版／切入口／保留回滚仍按原方式手工操作。
- 按用户要求把 `auto_pick.go` 注释里的“解锁结果和用途会丢”改成“追加的用途可能丢失”。
- 验证：`bash -n` 通过；新入口完整构建两次（提交前脏树 `vcs.modified=true`、提交后 `false`），包 `CST Mac 52d66ee8.app`（core `aeb933f4…`、`vcs.revision=52d66ee…`、minos 12.0），入口切新包、回滚指 `42d8c02e`；验收单实例 PID 7879、窗口 1360×900、订阅 200／2622，未发节点检测。

### 2026-10-23 第五轮第4项：首启“需要再次激活”确认（不改应用）
- 复现一次即出现：99×117 属于 cst-core 主窗（pid 16971、标题“SpeedTest · 节点检测”），是台前调度左侧条带的缩略显示，不是辅助窗口也不是取错窗口；主窗创建后 0.4 s 已由 `fitInitialWindow` 设为 1360×900 居中并 onscreen=true，不存在“要激活才显示／布局”。
- 原因：入口经 Terminal 打开；`SETFRONT` 记录 CST 两次拿到前台（14:37:22.591／23.303），Terminal（pid 758）两次夺回（22.919／24.924，紧随其窗口关闭与重新激活），台前调度随即把非前台应用的主窗收进条带——触发条件是残留的多个 Terminal 窗口。
- 同一入口再启动一次（验收）：`SETFRONT` 只切到 CST 一次（14:43:23.094）后保持前台，窗口 1360×900 连续 50 s 可见、`active=true`，订阅 200／2622、单实例 PID 18046。
- 处置：不改应用、不打包；交付说明改为“99×117 是台前调度缩略图，不是过渡小窗”，被收进左条时点 Dock 图标一次即可。

### 2026-10-23 第五轮第5项：自动精选报告中文化（只改前端）
- 说明列改成“完整服务名 + 中文结论”（如“ChatGPT 需人机验证”“Netflix 地区受限”），未通过项排前、长文正常换行；新增 `frontend/src/autoPickReportText.ts`：`reportServiceText` 失败在前排序，`reportPurposeText` 按该份报告的 `rules.required_services` 与节点 `services`／`purposes` 生成“未入 AI：ChatGPT 需人机验证”，全通过写“所选用途条件均通过”、缺依据写“未记录”；复用 `unlock.ts` 的 `outcomeReasons`，只翻译已存 outcome。
- 删掉页头下方重复的“分组：…”一行；用途说明移出“服务列表非空”判断：无 `excluded_reason` 的节点都显示用途说明，服务为空时仍先写“通过延迟规则”，延迟排除仍只显示原 `excluded_reason`；布局、后端、报告格式与精选规则未动。
- 验证：离线示例覆盖全部通过／部分用途未入选／延迟排除（香港 01、日本 02、新加坡 03、美国 04）；副本里把新加坡 03 的 `services` 临时清空后显示“通过延迟规则”＋“未入 AI：未记录”，原报告未改、不计失败；截图 `round10-report-zh-20261023/auto-pick-report-zh.png` 与 `round11-report-empty-services-20261023/auto-pick-report-empty-services.png`；未加测试，前端 69 文件 414 项与 `npm run build` 通过。
- 提交 `ffe9ab2`；`scripts/build-mac-candidate.sh` 构建 `CST Mac ffe9ab26.app`（core `8a5fbb2e…`、内嵌 `ffe9ab2…`、`vcs.modified=false`），入口切新包、回滚指 `3140fc9e`；验收单实例 PID 24329、窗口 1360×900、订阅 200／2622、无系统弹窗。

### 2026-10-23 Verge 内核只读研究：当前节点与四次对照检测
- 入口：PID 2123 参数指定 Unix socket `/var/run/clash-verge-service/users/501/verge-mihomo.sock`（当前用户权限 0600）；`curl --unix-socket <路径> http://localhost/version` 无 Authorization 即 200，mihomo v1.19.32；用户配置虽有 secret，本次未使用或输出；TCP 控制地址为空，7897 是代理端口，配置中的临时 socket 被进程参数覆盖。
- 对应：`/proxies` 不返回 server/port，结合唯一读取的 Verge `clash-verge.yaml` 与 CST 当前 SG04 缓存按名称+server+port 比对；55/55 配置条目一致，去掉两条公告为 53/53 真实节点（100%，无歧义）；CST 两机场去重共 84 个，对应 53/84=63.1%，其余 31 个飞鸟云节点未加载进 Verge（两个订阅各 31 条）；服务 runtime/config.yaml 权限不可读，server/port 依据为用户侧生成配置，API 核对全部加载名称。
- 组→当前选择：GLOBAL→香港高速01 BGP CMCU；良心云→美国01流媒体；AI服务、视频音乐、开发服务、Telegram、Google、DNS出口六组→良心云；自动选择→新加坡专线03；故障转移→日本高速01 CTCU（共10组，组选择不等于实际连接）。
- `/connections` 仅读一次：1316 条连接，1163 条代理链终点均为美国01流媒体，153 条 DIRECT；原始 chains 为终点在前，例如 `[美国01流媒体, 良心云, AI服务]`，按路由方向为 AI服务→良心云→美国01流媒体；不输出访问域名或连接明细。
- 日本01高速CUCM：10/23 19:18:07（UTC+8）Verge 单次 /delay，10秒后 HTTP 504 `Timeout`；CST 最近 10/21 19:00，四站 0/24，Cloudflare 0/6，原因为 `context deadline exceeded`。
- 日本02高速CUCM：10/23 19:18:17 Verge 单次 /delay，10秒后 HTTP 504 `Timeout`；CST 最近 10/21 19:00，四站 0/24，Cloudflare 0/6，同为 `context deadline exceeded`；两节点本次在 Verge 也未测通；按用户本次裁定关闭“待用户对照”，作为节点本身不通处理，不再列为 CST 问题（历史测试地址不同的事实保留）。
- 新加坡04专线：10/23 19:18:27 Verge 单次 /delay，约5.06秒返回 HTTP 503 `An error occurred in the delay test`，无更细原因；CST 最近 10/21 23:35，Cloudflare 6/6，去首次后的稳定中位 552 ms（首次969 ms）。
- 香港01专线：10/23 19:18:32 Verge 单次 /delay 返回 HTTP 200、956 ms；CST 最近 10/21 19:00，Cloudflare 6/6，稳定中位275 ms（首次601 ms），四站稳定汇总267 ms。
- 条件与取舍：仅4次授权 /delay、timeout=10000，无重试/切节点/设置修改/新CST检测，TUN保持开启，数据库 mode=ro；本次目标按用户要求为 `https://cp.cloudflare.com/generate_204`，CST历史实际为 `https://speed.cloudflare.com/__down?bytes=1`，时间、URL及单次/多采样口径不同，不能据延迟差判性能优劣；接入可行，但现有CST客户端只有TCP HTTP传输，需另行批准Unix socket接入；“当前节点”应读连接链，不能只取GLOBAL.now。

后续对照统一使用 CST 实际目标 `https://speed.cloudflare.com/__down?bytes=1`，CST 与 Verge 紧接着各测一次；不再使用 cp.cloudflare.com。

### 2026-10-23 Clash 对照（第一项）
- 新增 mihomo Unix socket 连接与本机 `clash_connection` 设置；默认 Verge socket，可手填 TCP/secret，secret 不记日志。
- `GetClashNodeStatus` / `CompareClashNode`：仅点击时 CST→Clash 相邻各一次，统一 `speed.cloudflare.com/__down?bytes=1`，未加载节点禁用；四种结论并排显示，不写历史。
- `go vet`、`go test ./...` 与前端 70 文件/415 项通过（原有短超时用例重跑通过）；仅新增结论判断主路径测试。
- 已推 `0103591`、换包 `01035910`；香港01专线唯一一次对照：CST `timeout: no recent network activity`，Clash 533 ms，提示“可能是 CST 的检测问题”；TUN 未动、无追加复测，截图留本地。

### 2026-10-23 GLOBAL 恢复与后续切换限制
- 原选择 `🇭🇰香港高速01|BGP|CMCU` → 验证时 `🇭🇰香港专线01|BGP|流媒体` → 恢复后 `🇭🇰香港高速01|BGP|CMCU`；本轮仅 GET /proxies 再确认已恢复，良心云仍为 `🇺🇸美国01|流媒体|0.1x`。
- 后续不切换任何代理组；仅用户再次明确允许时，才在全部工作最后用 Telegram 验证，并恢复原选择。

### 2026-10-23 香港01专线对照超时只读调查
- 错误原文：`Get "https://speed.cloudflare.com/__down?bytes=1": timeout: no recent network activity`，来自 Hysteria2 的 QUIC IdleTimeoutError；未保存当次阶段/路径，不能分辨建隧道、目标 TLS 或等 HTTP 响应，不能把它直接叫握手超时。
- SG04 history/history.db 只读：香港01专线仅1轮（10/21 18:59:56，rev_405879ab19958033），无监测采样；Cloudflare首次601 ms，失败或>5秒0/1；四站首次601/555/698/540 ms，失败或>5秒0/4，全部24次成功，样本不足以证明首次通常慢或只是偶然。
- 对照与正常测速共用 scoped proxy→TestSinglePhysicalContext→PreparePhysicalAutoProxy、物理网卡绑定与DNS（历史en1/IPv4）；区别为对照单站1次/10秒、socket预算1跳过IPv6备选，正常四站6次预算24允许备选（该历史AAAA超时，仍IPv4成功）；旧IPv4成功地址优先修复仍共用，但仅TCP适用，此节点Hysteria2固定一个IPv4，不走该重试路径。
- 结论：单次QUIC超时原因未定，未证实CST bug、惯常冷启动慢或偶发；待定为单次预算跳过IPv6备选的口径差异，不能据此归因。本轮无新检测/切组/历史写入，不改代码、不加预热、不换包，后续实测等用户决定。

### 2026-10-23 两次 Clash 对照与协议历史统计
- SG04只读全量延迟样本（10/14–10/21）：Hysteria2失败494/2876=17.18%（97轮），VLESS失败2172/4955=43.83%（176轮）；QUIC空闲超时4/2876=0.14%，占Hysteria2失败0.81%；Trojan/SS及监测采样无记录，不支持Hysteria2更易失败，非同条件实验不推断协议优劣。
- CST锁定sing-quic 65d17608159e与Verge mihomo v1.19.32的1c242664697a源码对照：三项默认参数无差异（QUIC空闲30s、握手空闲5s/总握手10s、keepalive10s）；新版新增可配置HandshakeTimeout默认0仍跟随调用context，未发现CST更紧，不调整依赖或参数。
- CompareClashNode改为CST连续2次→Clash连续2次，cst/clash返回数组，界面逐次并排写“第1次/第2次”；PhysicalSocketBudget=24与正常四站六次一致，允许IPv6备选而不增加目标采样；失败加协议名，QUIC错误标“QUIC 空闲超时”并保留原文。
- go vet、go test ./...、前端70文件415项及build通过，未新增测试；已推64c8641并换包64c8641b（内嵌同提交、clean），原生窗口确认“各2次”，旧573b450b保留；不发新检测、不切代理组；后续仅新加坡04专线，等用户确认后再点对照。

### 2026-10-23 新加坡04专线唯一一次对照验收
- 原生界面仅点一次“和 Clash 对照”（20:05）：第1次 CST 645 ms / Clash 4275 ms，第2次 CST 499 ms / Clash 1302 ms，两次界面均显示“两边都正常”。
- 未重测、未切换任何代理组；截图留本地 `.ui-preview/clash-integration-20261023/sg04-double-comparison.png`，没有修改源码或历史数据库。
- 待办（现在不做）：以后统计VLESS失败率，先排除全部样本均失败的节点（例如日本CUCM），再统计剩余VLESS节点失败率，与原始43.83%区分。

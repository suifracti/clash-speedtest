# 参考项目与借鉴清单（第四轮起用）

用户 2026-10-21 决定：等 GPT 做完当前这轮（延迟 bug 排查、检测报告、检查器历史区），再参考已有开源项目来优化 CST，借鉴现成做法，加快进度。

下面的项目情况来自 Claude 已有的知识，**还没有上网逐个核实**。开始参考前，先确认每个项目还在维护、功能和许可证，再决定借什么。

## 借鉴原则
- **借思路和判定方法，不整段复制代码。** 要复制代码，先看许可证（GPL 类代码不能直接并入），在提交说明里写明来源。
- **只借能解决我们现有问题的东西。** 每一项都要对应到下面“我们要解决的问题”里的某一条，不因为别人有就加。
- 照旧遵守 `roadmap-round3.md` 顶部的“工作方式”：小步推送，不过度防御，不过度测试。

## 我们要解决的问题
1. 解锁检测的判定还不够稳：Disney+ 暂不支持，Gemini 常遇验证页，YouTube 地区读不到。
2. 历史与健康：要能看节点过去几天、几周的稳定性，并据此选节点。
3. 报告：每轮检测要有总结，能回看，能分享。
4. 自动化：定时检测，自动挑出能用的节点，导出成 Clash 配置组。
5. 入口和出口：分清中转线路的入口，判断真实落地地区。

## 项目清单

| 项目 | 是什么 | 重点借鉴 | 对应问题 |
|---|---|---|---|
| faceair/clash-speedtest | 命令行，测订阅节点的延迟和下载速度；CST 最初就是从它起步的 | 上游的新修复（mihomo 内核升级、测速源），定期对照合并 | 1 |
| beck-8/subs-check | Go 写的，定时检测订阅节点（连通、速度、流媒体和 AI 解锁），保留能用的节点推给 Sub-Store / mihomo，有网页界面 | 定时检测加自动导出的流程；各个解锁项的判定实现，逐项和我们对照 | 1、4 |
| lmc999/RegionRestrictionCheck（以及 1-stream 等分支） | Shell 脚本，按地区检测流媒体和 AI 解锁 | 我们解锁判定的来源。逐项核对 Netflix、Disney+、YouTube、Gemini、ChatGPT、Claude、TikTok、日本各站的最新判法 | 1 |
| AirportR/FullTClash | Telegram 机器人，测机场的延迟、解锁、入口出口拓扑，生成图片报告 | 报告图片的版式；入口和出口的拓扑分析，用来识别中转线路 | 3、5 |
| Sub-Store | 订阅管理：合并、改名、按检测结果筛选节点 | 节点改名和筛选规则的写法；和 CST 导出的配置组怎么衔接 | 4 |
| Uptime Kuma | 通用的在线状态监控，有长期历史和可用率 | 历史怎么展示：90 天可用率条、故障时间线、响应时间图 | 2 |
| sinspired/subs-check-pro | subs-check 的增强分支：检测节点、测速、解锁，自动生成 mihomo/sing-box 订阅，有 WebUI，集成 Sub-Store | 和 subs-check 一起对照，看它补强了哪些 | 1、4 |
| kutovoys/xray-checker | 持续监测 VLESS/VMess/Trojan/SS 节点的可用性，提供 Prometheus 指标和 API | 长期监测的数据怎么组织，指标怎么设计 | 2 |
| Gatus | 通用的在线状态监控，记录响应时间和健康状态 | 和 Uptime Kuma 一起参考历史展示 | 2 |
| SSRSpeedN | 多线程测速，同时测流媒体解锁 | 并发测速和解锁的编排 | 1 |
| xxf098/LiteSpeedTest、tindy2013/stairspeedtest-reborn | 较早的、带界面的测速工具，能导出结果图片 | 结果图片版式，只作参考（多半已不维护） | 3 |

## 核实结果（2026-10-21，Claude 用 GitHub API 和源码核对）
CST 自己的许可证是 GPL-3.0（fork 自 faceair/clash-speedtest）。

| 项目 | 星数 | 许可证 | 最近更新 | 结论 |
|---|---|---|---|---|
| faceair/clash-speedtest | 967 | GPL-3.0 | 2026-05 | 上游，仍在维护。偶尔对照合并修复 |
| beck-8/subs-check | 5.3k | GPL-3.0 | 2026-09 | **最值得参考**。许可证与我们相同，代码可以移植，注明来源即可。解锁项有 claude、disney、gemini、netflix、openai、spotify、tiktok、youtube、iprisk（scamalytics）；Crontab 定时、Apprise 通知、结果页可筛选排序、`/sub` 订阅输出。只保存当前快照，没有历史 |
| sinspired/subs-check-pro | 133 | GPL-3.0 | 2026-10 | 活跃。多了：住宅/机房 IP、原生 IP、运营商类型识别；`/analysis` 地区与协议分布图；自建测速源（Cloudflare Workers，可随机分流） |
| 1-stream/RegionRestrictionCheck | 205 | AGPL-3.0 | 2026-10 | 已基本重写，比 lmc999 原版更新。AGPL：**只借判定方法，不复制代码** |
| lmc999/RegionRestrictionCheck | 5.1k | AGPL-3.0 | 2025-12 | 原版，更新放缓。同上，只借方法 |
| AirportR/FullTClash | 827 | BSD-2 | 2025-05 | **已归档**。只参考报告图片版式和入口/出口拓扑的思路 |
| Sub-Store | 10.7k | AGPL-3.0 | 2026-10 | 活跃。只参考改名、筛选规则的写法 |
| Uptime Kuma | 92.7k | MIT | 2026-10 | 活跃。参考历史展示：心跳条、24 小时/30 天可用率、响应时间图、故障记录 |
| Gatus | 12.4k | Apache-2.0 | 2026-10 | 活跃。参考状态页和条件判定的表达方式 |
| kutovoys/xray-checker | 930 | AGPL-3.0 | 2026-09 | 活跃。只参考监测指标怎么组织 |
| SSRSpeedN、LiteSpeedTest、stairspeedtest-reborn | — | GPL/MIT | 2023 | 不再维护，不参考 |

源码里看到、可以直接对照的判法（subs-check `check/platform/*.go`）：
- **Netflix**：先请求 Fast.com 的 Netflix 测速接口，一次就能拿到地区；没有结论时，再用两部片子判断（81280792 非自制，70143836 自制），任一返回 403 判为封禁。
- **YouTube**：请求 `youtube.com/premium?hl=en`。页面里出现 `www.google.cn` 就判 CN；出现 “not available in your country” 判不可用；有 Premium 标记时，再从 `INNERTUBE_CONTEXT_GL` 等字段读地区。
- **Gemini**：读 `,2,1,200,"XXX"` 三位国家码。CHN、RUS、HKG、MAC 等视为不可用（我们已经在用同样的标记）。
- **Disney+**：bamgrid 的 devices → token → 地区接口流程。我们之前在所有节点上 TLS 握手就失败，原因可能在网络层，不在流程本身。
- **IP 风险**：scamalytics 欺诈分，可以作为 IPPure、Ping0 之外的第三个来源。

## 建议借鉴的优先级（待用户确认）
1. **解锁判定对照 subs-check**（对应问题 1，改动小、见效快）：
   - Netflix 先查 Fast.com 拿地区。
   - YouTube 加 `hl=en`，用 google.cn 判 CN。
   - Gemini 的不可用国家表对齐。
   - Disney+ 用它的完整流程在 1 个节点上试一次：通了就恢复，不通保持“暂不支持”。
2. **定时检测 + 自动导出**（问题 4）：参考 subs-check 的 Crontab 加订阅输出。CST 按计划定时跑选定项目，每轮把“能用的节点”写成一个固定的 Clash 配置组文件或本地订阅地址，Clash Verge 可以直接订阅它。复用现有的监测调度和导出功能。
3. **入口与出口**（问题 5）：解析节点服务器地址，得到入口 IP 和入口所在地区，在出口表和检查器里显示“入口 上海联通 → 出口 新加坡”。这样就能回答 CUCM 这类中转线路为什么慢。参考 FullTClash 的拓扑思路，自己实现。
4. **历史展示**（问题 2）：照 Uptime Kuma 做节点和机场的心跳条（每轮一格），加“故障记录”时间线，放进检查器历史区和机场体检页。
5. **IP 类型**（问题 1）：参考 subs-check-pro，加住宅/机房、原生 IP 判断，并入“出口与信誉”。
6. **报告导出图片**（问题 3）：最后做，参考 FullTClash 版式。

## 第四轮开始时的做法
1. Claude 先核实清单：项目是否还在维护、许可证、和我们相关的功能。每个项目写 3 行以内的结论，补进本文件。
2. Claude 按“我们要解决的问题”排出借鉴项和优先级，用户确认。
3. GPT 一次做一项，每项提交说明里写明借鉴自哪个项目、借了什么。

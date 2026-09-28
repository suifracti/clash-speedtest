# Clash SpeedTest — Astra 规划前综合报告

角色：Planning Synthesizer / Systems Analyst。日期：2026-09-22。

本报告综合两份材料，不构成第三次仓库审计、Feature Truth Matrix、发布验收或最终路线图。建议 Astra 先读第 15 节，再按决策编号回查证据与依赖。

来源简称：

- **L**：[CLASH_SPEEDTEST_PROJECT_SNAPSHOT.md](CLASH_SPEEDTEST_PROJECT_SNAPSHOT.md)，Lunamax 主事实快照；用户提供的独立评价为 `FACT_HIGH_STATUS_MEDIUM`。
- **G**：[CLASH_SPEEDTEST_GROK47_AUDIT.md](CLASH_SPEEDTEST_GROK47_AUDIT.md)，Grok 独立审计。引用以原文章节、函数或证据文件名定位。

本次只读取上述材料、生成本报告并核对内容；没有查询当前仓库源码/Git/数据库、启动程序或重跑证据。下文“当前”均指两份材料在 2026-09-22 记录的 `main@58d036e` 基线，不表示本次重新确认了实时现场。源码、数据库与外部工程规则的引用均为材料转述。产品缺陷与未验收范围分开记录；没有当前窗口验收，不等于已有组件失效。

## 1. Evidence Hierarchy

后续判断必须遵守：

```text
当前 main / production runtime callgraph
> 当前 schema / persistence / API authority
> 当前代码 + targeted runtime evidence
> 当前 Git / PR / tag
> Obsidian 当前项目主源
> Lunamax / Grok 报告
> 历史计划
```

两份报告不是投票。重复同一结论不能将转述升级为本次一手核验；细化到函数、查询条件或数据时间范围的证据，强于笼统完成度标签。Git 证明版本与合并关系，不能证明用户数据、运行行为或发布可用性。

本报告的 `CONFIDENCE` 表示材料对**该限定命题**的支持程度，不表示功能成熟度。`HIGH` 是材料一致且有具体依据、无已知反证；`MEDIUM` 是单方补充或状态解释有范围限制；`LOW` 是仅有推测。若更高层证据冲突而无法判断，应写 `UNRESOLVED`，列出最小裁决证据，不平均、不强行选边。

状态词与置信度分离：`VERIFIED` 只能用于明确范围内已有充分证据的行为；`PARTIAL` 表示产品路径或证据范围不完整；`COMPONENT_READY` 表示组件/API 已具备而正式产品路径未闭合；`EXPERIMENTAL` 表示实验契约；`SUSPECT` 表示有可信风险但未确证行为；`WRONG` 表示承诺或结论已被具体实现反驳；`UNFINISHED` 表示明确缺实现；`UNKNOWN` 表示证据不足。不要把所有历史 VERIFIED 统一解读为发布已验收，也不要为改标签自动补做验收。

## 2. Canonical Facts

只列影响规划的 16 条事实；相关范围限制属于事实本身。

### F01 — 产品版本基线

FACT=材料记录本地 main、origin/main、GitHub main 均为 `58d036e220d873f900f755fdc47b2f6850a0b9b3`，为 PR #10 合并提交；PR #1–#10 已合并，无开放 PR。根目录 checkout 为旧 PR #8 head `4096fb852ceb721fd41155bdc915685e79ee1869`，落后七个提交且无分叉。
CONFIDENCE=HIGH
SOURCE=L §2.1–2.3；G Independent Current-State Map、Agreement。
PLANNING_IMPACT=以该 main 为规划基线；旧分支和旧产物不是未交付工作，现场操作不能默认使用根目录版本。

### F02 — 技术栈

FACT=Go Core、Wails v2.16、Vue 3 / TypeScript / Vite / Pinia；本地 Web fallback；SQLite 与 legacy JSON 并存。
CONFIDENCE=HIGH
SOURCE=L §4–5；G 正式入口表。
PLANNING_IMPACT=存在可复用的桌面应用基础；材料没有支持更换语言、UI 框架或数据库引擎的证据。

### F03 — 正式入口

FACT=无参数走 Wails；`--web` 或 Wails 启动失败走 local Web。两者共用 AppService。`--cli` 或 `-c` 进入独立 CLI/TUI speedtester 路径，不经过 AppService，不写 workbench 表或注册 Monitor job。
CONFIDENCE=MEDIUM
SOURCE=G 正式入口表、Conflicts 第 1 项（main.go 分支）；L §4 的桌面适配器描述。
PLANNING_IMPACT=桌面/Web 的新功能不会自然覆盖 CLI；入口支持范围须显式定义。

### F04 — Monitor

FACT=任务创建及 Start/Pause/Resume/Stop 有 AppService/API/Wails 路径；Scheduler/Runner 采样并保存 SQLite run/sample。任务定义和状态只在内存，不跨应用重启恢复。light/service/heavy 覆盖 RTT/HTTP/TTFB，不等于 download/upload/Antigravity/IP purity 全指标监测。
CONFIDENCE=HIGH
SOURCE=L §4.4、5.4、10 R-01；G Agreement、正式入口表。
PLANNING_IMPACT=可规划为单进程采集基础，不能按 durable 24/7 服务计算完成度。

### F05 — Timeline

FACT=导航“历史记录”实际是 MonitorTimelineView，读取 monitor_samples；cursor、stats、facets、Inspector、live refresh/catch-up/gap continuation 有代码和既有证据。PR #10 改过视图，未证明当前 main 窗口交互已验收。
CONFIDENCE=HIGH
SOURCE=L §6.3–6.4、14；G Feature Truth Corrections、Things That Should NOT Be Reopened。
PLANNING_IMPACT=复用查询及 store 基础，同时明确它不是所有即时测试的统一历史。

### F06 — Workbench

FACT=现代工作台正式接线的是稳定身份单节点 latency，使用 profile_id + node_key + attempt_id，支持 raw samples、异步保存与防陈旧响应。吞吐/服务页签未接正式工作台测试；批量和更多指标仍在旧路径或未完成。PR #9 有临时订阅 Web 证据，当前用户库工作台表为零行。
CONFIDENCE=HIGH
SOURCE=L §4.3、6.5、14；G 正式入口表、Feature Truth Corrections。
PLANNING_IMPACT=扩指标是在已有闭环上扩展契约；用户库零行既不能证明已日常使用，也不能推翻临时库证据。

### F07 — History / SQLite

FACT=SQLite 有 monitor_runs、monitor_samples、workbench_latency_tests、workbench_latency_samples；已用 WAL、busy_timeout、foreign_keys、单写入锁及幂等 migration，但无正式 schema version 框架。legacy batch 写 JSON；legacy TestSingle 不保存历史，name-only timeline 有跨订阅同名混合风险。
CONFIDENCE=HIGH
SOURCE=L §5.2–5.3；G Data & Persistence Risks、Architectural Risks。
PLANNING_IMPACT=需要决定逻辑记录与兼容边界，不能由“三类记录”直接推导必须合并物理表或更换 SQLite。

### F08 — Controller

FACT=Mihomo External Controller adapter、group 读取、手动 Select API/Wails 绑定存在，正式 Vue 页面无调用者；默认 loopback，remote/明文需显式放行，secret 有既定处理。材料未提供本次真实 controller 连接证据。
CONFIDENCE=HIGH
SOURCE=L §6、12；G 正式入口表、Feature Truth Corrections。
PLANNING_IMPACT=组件可复用，产品入口未完成；“无 UI”不意味着“无可执行手动 API”。

### F09 — Recommend / Evidence

FACT=只读 GetMonitorRecommendation 有 API/Wails 方法而无 Vue 调用者；候选集必须来自活内存 job，证据来自其 Monitor samples；默认 MaxSampleAge 为五分钟。工作台新测的 latency 不自动进入推荐证据。
CONFIDENCE=HIGH
SOURCE=L §4.4、17；G Conflicts 第 4 项、Architectural Risks 第 2 项。
PLANNING_IMPACT=引擎可复用，但候选生命周期、来源、时效与解释入口决定推荐能否成为用户功能。

### F10 — Auto

FACT=EvaluateAndAutoSwitch 只有测试调用，未接正式执行循环/UI/HTTP/Wails；ModeAuto 可由 policy API 写入内存但无生产循环消费。默认 monitor_only，Monitor Runner 不 SelectNode。
CONFIDENCE=HIGH
SOURCE=L §4.4、10 R-04；G 正式入口表、Agreement。
PLANNING_IMPACT=Auto 为 UNFINISHED；后续接线将引入真实控制后果，不能作为普通展示功能处理。

### F11 — Airport / Profile

FACT=机场增删改查、订阅刷新、cache 解析、真实 node options 已有路径；后端重解析配置，Workbench/Monitor 提交稳定键。AirportDTO/Modal 仍回传并显示 raw subscription URL，可能含 token。
CONFIDENCE=HIGH
SOURCE=L §5.1、10 R-05、12；G Agreement、Product / UX Risks。
PLANNING_IMPACT=保留解析与身份边界；目录、导入和 URL 展示契约仍影响可用性与凭据暴露面。

### F12 — 数据位置与配置身份

FACT=profile/cache 默认取进程 cwd 的 airports.json、airports-cache；history 默认取用户 home 的 `.clash-speedtest/history`。两份 profile 世界不同：根目录两条实际订阅，PR #9 worktree 一条临时测试订阅；PR #7 review worktree 无 profile。
CONFIDENCE=HIGH
SOURCE=L §2.5–2.6；G Omissions 第 5 项、数据现场。
PLANNING_IMPACT=稳定数据定位与已有数据选择/迁移必须一起考虑；不能简单覆盖或自动认定某个 worktree 数据最权威。

### F13 — 本机历史证据的时效

FACT=材料记录用户库 25 runs、291 samples、0/0 workbench rows，另有 11 个 legacy JSON；Monitor 数据截至 2026-09-19 13:16 UTC，仅一 profile、六 node_key；材料检查时无应用进程。因此不能支撑当时默认策略的现行推荐。
CONFIDENCE=HIGH
SOURCE=L §2.5、14；G 数据现场、Conflicts 第 4 项。
PLANNING_IMPACT=历史存在不等于当前候选可用或样本新鲜；不能把库大小作为持续运行/推荐 readiness 的代理指标。

### F14 — Windows / macOS

FACT=PR #10 CI 的 Windows desktop build、macOS darwin/arm64 bundle、Linux unit 通过。Windows 当前正式窗口、macOS 实际运行及 Intel/Universal、签名/公证和发行包首次启动无充分验收证据。
CONFIDENCE=HIGH
SOURCE=L §13–14；G Cross-platform Risks、CI 记录。
PLANNING_IMPACT=构建基础可用；发布平台范围及对应证据单独决定，不自动要求补齐所有平台。

### F15 — UI 的真实状态

FACT=PR #10 的原型 UI、UiSelect 及三主导航已进 main；折叠 legacy 工作台与旧历史矩阵仍在。观察窗口只改标签/轴而不约束数据；“加入持续监测”只切页，不创建任务或携带选中节点。
CONFIDENCE=HIGH
SOURCE=L §2.2、10 R-06；G 正式入口表、Conflicts 第 3 项、Omissions 第 2 项。
PLANNING_IMPACT=已有 UI 无需重做；当前两个行为承诺错误需要纠正，历史范围和入口含义须明确。

### F16 — 连续采集与保存边界

FACT=retention API 存在，默认 keep_all，无正式设置页或启动自动清理；scheduler 无系统睡眠/唤醒补采机制。Workbench 结果先显示 saving，保存有 30 秒超时，正常 Close 等待保存，强杀可能丢失尚未提交的结果。
CONFIDENCE=MEDIUM
SOURCE=L §6.3、6.5；G Omissions 第 1、6、7 项。
PLANNING_IMPACT=恢复、保留、采集缺口解释、保存完成承诺必须纳入相关阶段合同；当前数据不能证明发生过强杀丢失。

## 3. Conflict Resolution Table

以下用逐项字段表呈现全部四项冲突。`RESOLVED` 仅表示两份材料足以收口该命题，不表示本次重新验收功能。没有依据把未决产品选项计为事实冲突。

### C01 — CLI 是否共用 AppService

TOPIC=CLI 调用链
LUNAMAX_POSITION=L §4.1 将 CLI / Wails / local Web 画进共同 AppService 链。
GROK_POSITION=CLI 在 main.go 分支直接调用 speedtester；桌面/Web 才共用 AppService。
STRONGER_EVIDENCE=G 指明 shouldRunGUI 分支及后续 CLI 加载，无 CLI 调用 history.NewStore / RunWorkbenchLatencyTest；强于 L 的概括架构箭头。
CANONICAL_TRUTH=RESOLVED：必须画成分叉；CLI 不继承桌面持久化和 Monitor 契约。
CONFIDENCE=MEDIUM
IF_UNRESOLVED=当前材料足以收口；若后续有反证，仅追踪目标 main 的入口分支到 AppService/持久化调用者，不审全仓。
PLANNING_IMPACT=不能假定一次 AppService 改动就实现三入口 parity。

### C02 — VERIFIED 的含义

TOPIC=状态词是否可互换
LUNAMAX_POSITION=完整调用链加测试、adapter/runtime 或可追踪调用链等证据可标 VERIFIED。
GROK_POSITION=所引工程规则要求用户意图、正式 runtime、authority 与当前范围验收一致，L 的标签偏宽。
STRONGER_EVIDENCE=L 明文定义及 G 所引《工程任务拆分与功能正确性审计.md》状态表，已经足以证明两个用法不同；本次未重读外部规则，不声称确认其完整原文。
CANONICAL_TRUTH=RESOLVED：两报告的 VERIFIED 不可互换，更不能直接折算发布验收。沿用具体代码/证据结论，产品状态按范围重新表达。
CONFIDENCE=HIGH
IF_UNRESOLVED=若要正式授予组织级 VERIFIED，只需读取该规则当前状态表并明确目标入口/范围；这不阻塞本报告的语义收口。
PLANNING_IMPACT=避免虚假完成率，也避免为了统一标签重跑所有证据。

### C03 — Workbench 观察窗口

TOPIC=4h/24h 是证据不足还是实现错误
LUNAMAX_POSITION=历史请求不带 since/until，仅改显示，标 SUSPECT。
GROK_POSITION=samplesForKey 也不读取 windowMode，full 轴为 00:00–24:00，不等于滚动最近 24h，标 WRONG。
STRONGER_EVIDENCE=L 已承认查询缺时间范围；G 补足本地样本不筛选，排除了“后端不限但前端正确过滤”的可能。
CANONICAL_TRUTH=RESOLVED：当前控件承诺的数据窗口未实现，WRONG。应改数据约束还是文案属于未决产品方案。
CONFIDENCE=HIGH
IF_UNRESOLVED=若目标版本变化，只查请求参数、samplesForKey 与窗口文案三处；无需全量图表回归。
PLANNING_IMPACT=不可基于该控件已有真实时间范围继续增加统计/图表。

### C04 — Evidence 是否已在用户数据上可用

TOPIC=引擎契约与现行推荐可用性
LUNAMAX_POSITION=Evidence 标 VERIFIED，依据 PR #7 与内存 job 候选调用链。
GROK_POSITION=当前无活 job，用户样本过五分钟 freshness 门槛；组件可用不代表当前可推荐。
STRONGER_EVIDENCE=collectEvidenceNodes 的活 job 约束、DefaultSwitchPolicy.MaxSampleAge 与数据库时间范围，比函数测试的产品化外推更直接。
CANONICAL_TRUTH=RESOLVED：引擎/只读边界保留既有证据，产品层为 COMPONENT_READY；材料现场无法生成有效现行推荐，正式用户闭环 UNFINISHED。这不是推荐数学错误。
CONFIDENCE=HIGH
IF_UNRESOLVED=未来若判断另一运行现场，只需目标 job 候选状态、相应样本时间/范围和 policy 门槛；不重开 PR #7 数学审查。
PLANNING_IMPACT=推荐阶段需要候选生命周期、数据时效和解释入口，不能靠读取旧 DB 直接交付。

四项均在上述限定范围收口；`UNRESOLVED_CONFLICT_COUNT=0`。main 当前窗口行为、睡眠现场、用户零行原因仍有 UNKNOWN，但它们不是被强行裁决的四项冲突。

## 4. Overclaim Corrections

G 原文没有独立编号的七项 OVERCLAIM 清单。本节按其 Feature Truth Corrections 中七组 **VERIFIED 下调项**一一整理；观察窗口放 C03，“加入持续监测”放 O02，不人为凑入或重复计数。

### OC01 — latency 整体

CLAIM=latency 指标整体 VERIFIED。
ACTUAL_STATE=legacy JSON、Monitor RTT、Workbench SQLite 均有实现，但记录权威不同；用户 workbench 为零行，当前 main 原生操作证据缺失。
WHY=一个指标名覆盖了不同用户路径，局部证据不能代表全路径。
CORRECT_STATUS=PARTIAL
PLANNING_IMPACT=按入口与数据契约定义交付，不将 latency 底层重写。

### OC02 — Workbench latency 闭环整组

CLAIM=result、async save、attempt 身份、stale guard 已作为正式用户闭环 VERIFIED。
ACTUAL_STATE=PR #9 定向测试与临时 Web 进程证据存在；本机用户库和 main 窗口使用尚未被这些证据覆盖。
WHY=临时环境与当前用户环境是不同证据范围；保存中也不是 durable commit。
CORRECT_STATUS=PARTIAL
PLANNING_IMPACT=复用身份、异步保存与隔离设计；明确 saved 承诺即可，不因零行重做整个闭环。

### OC03 — Evidence

CLAIM=Evidence 已是 VERIFIED 用户能力。
ACTUAL_STATE=PR #7 组件契约成立；无正式页面，现场无活 job/新鲜证据。
WHY=函数正确性不能填补候选生命周期和产品调用者。
CORRECT_STATUS=COMPONENT_READY
PLANNING_IMPACT=补产品数据与入口合同，保留只读与隔离结论。见 C04。

### OC04 — Controller 客户端与安全边界

CLAIM=Mihomo client、secret、loopback 策略的 VERIFIED 可代表 Controller 产品完成。
ACTUAL_STATE=客户端/API 和安全边界有组件证据，正式 UI 无调用者，本次无真实 Controller 连接。
WHY=组件安全契约与用户可执行、可理解的配置流程不同。
CORRECT_STATUS=COMPONENT_READY
PLANNING_IMPACT=不重写 adapter；也不把已有 API 当作用户确认和执行审计已完备。

### OC05 — Timeline 交互整组

CLAIM=live refresh/catch-up/incomplete/Inspector 均可按当前窗口 VERIFIED。
ACTUAL_STATE=timeline.ts 相对历史 tag 未变，已有 store 证据；PR #10 改过视图，当前没有 main 任务窗口证据。
WHY=store 行为证据可继承，修改后的视图交互不能无限外推。
CORRECT_STATUS=PARTIAL
PLANNING_IMPACT=保留 store 验证，只在实际交付涉及的视图边界存在具体缺口时补最小检查。

### OC06 — Monitor job 与四态操作

CLAIM=创建任务、Start/Pause/Resume/Stop 已是当前产品 VERIFIED。
ACTUAL_STATE=PR #8 Web evidence 与 scheduler 行为证据存在；PR #10 改过 MonitorJobsView，且任务仍不恢复。
WHY=旧页面证据不能自动覆盖改后页面，更不能证明跨进程服务。
CORRECT_STATUS=PARTIAL
PLANNING_IMPACT=分别讨论单进程生命周期、视图接线与 durable job，禁止合并成“Monitor 重做”。

### OC07 — 节点筛选

CLAIM=节点筛选整体 VERIFIED。
ACTUAL_STATE=工作台本地过滤已加载节点；Timeline 有服务端条件；能加载哪些节点取决于 profile/cache 目录。无当前 main 操作证据。
WHY=过滤逻辑证据不证明各入口均具备真实节点数据。
CORRECT_STATUS=PARTIAL
PLANNING_IMPACT=先区分输入为空和筛选错误；没有证据要求重写筛选算法。

## 5. Important Omissions

以下严格对应 G 的七项遗漏。分类表示对规划的影响，不是 P0 或自动新增实施任务；有些遗漏已经由本报告补齐事实，无需再验证。

| ID / 原遗漏 | 分类 | 规划应吸收的事实与影响 | 需要在哪个边界处理 |
| --- | --- | --- | --- |
| O01 系统睡眠 | **Planning Critical** | ticker 无唤醒补采；“24/7”承诺若忽略桌面睡眠，路线图会承诺无法采到的数据。代码机制明确，实际睡眠孔洞未做现场确认。 | 决定持续监测承诺时；允许缺口、恢复后解释或独立常驻方式均待选。禁止补造历史样本。 |
| O02 “加入持续监测” | **Implementation Critical** | 只切页，既不携带节点也不创建 job；有按钮可在未选择节点时点。具体语义错误可直接定位，无需架构重审。 | 修改该工作流前选择改名或携带节点进入创建确认；“加入”不自动授权启动采集。 |
| O03 无正式入口的旧 gui | **Later Concern** | gui.NewServer/TestManager/gui/web/dist 仍编译，main 只用 LaunchApp；它与当前 Vue 折叠旧区不是同一对象。 | 当前可以冻结并注明不可作为修复入口；若声明 CLI/旧 GUI 支持或做入口清理，再决定删除/保留。 |
| O04 样本时间与覆盖 | **Planning Critical** | 291 条并非新鲜、广覆盖的持续流；五分钟策略下不可作为现行推荐基础。 | 本报告已补事实；推荐范围决定时纳入，勿为补该信息再采样或测引擎。 |
| O05 两份 profile 不同 | **Planning Critical** | 真实订阅与临时测试订阅不是同一数据副本；迁移策略若默认任取一份会导致错误数据世界。 | 数据定位/导入合同决定时；不可覆盖真实订阅或把临时数据自动并入用户库。 |
| O06 恢复与 keep_all 耦合 | **Planning Critical** | 自动恢复把采样时长从会话延长到长期，而默认不删、无清理 UI。增长是架构风险，材料未测增长速率或证明当前容量事故。 | 决定恢复承诺时同时纳入保留/容量责任，具体配额和 UI 可留到该阶段。 |
| O07 saving 与强杀 | **Implementation Critical** | 正常 Close 等待不覆盖强杀；结果可见早于 SQLite 提交。零行不能证明丢过记录，更不能确定原因。 | 更改保存承诺、退出策略或发布验收前；以 saving/saved/failed 区分观察和落盘，是否增强耐久性待选。 |

分类计数：Planning Critical **4**；Implementation Critical **2**；Later Concern **1**。

## 6. Architecture Dependency Map

下图是逻辑依赖，不是实施顺序或里程碑。`[现有]` 表示材料支持实现存在，`[缺/待决]` 表示合同缺口；路径稳定不自动保证身份稳定。

```text
稳定的数据定位 + 既有 profile 的选择/迁移 [缺/待决]
  → 可重复解析的 profile/cache [已有解析器]
  → profile_id + node_key + revision 校验 [新路径现有]
      ├→ Workbench 即时测试 → attempt + raw latency 保存 [现有]
      │                         → Workbench history/detail [现有，窗口承诺错误]
      └→ Monitor 候选集 → job → Runner → monitor raw samples [现有]
                                     ├→ Timeline / stats / facets / Inspector [现有]
                                     └→ 活 job 范围 + freshness + sufficiency
                                          → Evidence → Recommendation [组件现有]

job 定义/配置版本持久化 + profile 重新解析规则 [缺]
  + 恢复授权/资源与 retention 合同 + 睡眠缺口语义 [待决]
  → 可解释的跨重启监测 [缺]
  → 长期证据连续性与候选集可恢复性 [有条件，不保证样本总是充分]

可信推荐 + 显式 policy/用户控制 + 执行验证/失败处置/审计 [未闭合]
  → Auto 正式触发与执行 [缺]
```

测量模型依赖另行展开：

```text
每个 metric 的原始测量、单位、目标、时间、失败/未测语义 [待统一]
  → Workbench/Monitor 各自适用的执行合同
  → 有来源与稳定身份的持久化记录
  → 可比较的派生事实
  → 图表 / Node Detail / 推荐输入（仅在明确纳入时）

download/upload 字节与时间过程样本合同 [缺]
  → 真实过程保存 [缺] → 可重开吞吐图 [缺]
```

必须保持的限定：

- 当前 Recommend 只吃 Monitor；Workbench → Recommendation **没有现成边**，需要可比性/来源政策，不是简单表 join。
- legacy JSON、Monitor sample、Workbench attempt 的语义收口先于宣称“统一历史”，不要求先把它们合成一张表。
- durable job 不是当前单会话推荐的硬前提；它是跨重启候选恢复和长期监测承诺的前提。
- 单节点手动 Controller 能力不以 Workbench 全指标完成为逻辑前提；Auto 的硬依赖是可信候选/证据、明确授权和执行保障，不能机械套用 L 的 A/B/C 全部完成门槛。
- retention 是持续增长的治理依赖，不是每次临时即时测试的前置开发任务。schema 版本与兼容策略应在新增持久化合同前决定。
- Monitor 隔离 probe 当前独立于用户 Controller 选择；不要把 Controller 接入强加为采样前提。

来源：L §4–5、17；G Architectural Risks、Questions Astra Pro Must Resolve。上述条件依赖为本报告对材料的系统分析。

## 7. Architectural Fault Lines

仅保留有产品后果的 11 条。涉及风险的条目不冒充已发生事故。

### AF01 — 数据定位分裂

FAULT_LINE=profile/cache 随 cwd，history 随用户目录。
CURRENT_EFFECT=不同 worktree 看到不同节点世界，旧历史仍可见。
FUTURE_FAILURE_MODE=快捷方式/发行包迁移后空节点、选错订阅或误把孤立历史当当前配置。
NEEDS_ARCHITECTURAL_DECISION=true
依据：F12 / O05。

### AF02 — 入口与持久化责任分叉

FAULT_LINE=CLI 绕过 AppService；Wails/Web 共用它；无正式服务入口的旧 gui 仍随包编译。
CURRENT_EFFECT=同名“测速”在各路径有不同保存和输出行为，修复可能落在无调用者实现。
FUTURE_FAILURE_MODE=新增指标只修一条却宣称全产品可用；旧实现重新被接线带回旧契约。
NEEDS_ARCHITECTURAL_DECISION=true
依据：C01 / O03。冻结旧 gui 即可暂时控制风险，不要求现在删代码。

### AF03 — 三类测量记录与两套身份语义

FAULT_LINE=legacy airport/display name、Monitor profile/node/probe、Workbench attempt 分属不同记录；旧单测不保存，旧 timeline 仅按名称匹配。
CURRENT_EFFECT=即时结果、普通历史、Monitor 历史不能自然互认，name-only 查询有错误归属风险。
FUTURE_FAILURE_MODE=为“统一图”直接合并旧历史导致跨订阅混合，或为统一表丢失 probe/attempt 各自语义。
NEEDS_ARCHITECTURAL_DECISION=true
依据：F06–F09；并无证据证明普通 history 与 monitor history 正在重复写同一事件，勿把多来源直接称为重复数据 bug。

### AF04 — 持久证据绑定易失候选

FAULT_LINE=sample 落盘，job 定义只在内存；Recommend 又要求活 job。
CURRENT_EFFECT=重启后仍能读历史但不能恢复该推荐上下文。
FUTURE_FAILURE_MODE=错误地从历史重建已删除节点，或持久化 job 后未经配置校验恢复旧凭据/候选。
NEEDS_ARCHITECTURAL_DECISION=true
依据：C04 / F04 / F09。

### AF05 — 长期恢复与容量责任断开

FAULT_LINE=恢复尚缺，retention 默认 keep_all 且只有 API，无产品内设置路径。
CURRENT_EFFECT=已有样本默认累积；当前增长尚受进程会话约束。
FUTURE_FAILURE_MODE=静默自动恢复后长期增长且用户缺少可理解的清理控制。
NEEDS_ARCHITECTURAL_DECISION=true
依据：O06；不声称现已磁盘耗尽。

### AF06 — UI 承诺未由数据合同支撑

FAULT_LINE=观察窗口标签不筛数据；“加入监测”只切页；吞吐/服务页签共享工作台外观但正式 backend 未接。
CURRENT_EFFECT=前两者为错误操作模型；后两页已标未接入，但视觉分组仍易造成能力误解。
FUTURE_FAILURE_MODE=继续加图/卡片时将展示状态当业务 truth，产生看似完整的错误时间比较或任务归属。
NEEDS_ARCHITECTURAL_DECISION=false
依据：C03 / O02 / F15。现有两处行为可局部修正；更广的信息架构取舍见产品决策，不据此要求重建前后端。

### AF07 — Recommendation 与控制执行的接合面

FAULT_LINE=只读推荐无 UI，手动 Select 已有 API，ModeAuto 可配置但没有执行消费者。
CURRENT_EFFECT=“配置为 Auto”和“会自动切换”并不等价，Controller 能力完成度易被高估。
FUTURE_FAILURE_MODE=后续直接把 scheduler 接到执行函数，绕过用户授权、锁定、冷却、验证及失败处置。
NEEDS_ARCHITECTURAL_DECISION=true
依据：F08–F10 / G Architectural Risks 第 3 项。

### AF08 — 桌面生命周期与连续性承诺

FAULT_LINE=ticker 调度没有 OS 睡眠/唤醒语义。
CURRENT_EFFECT=没有解释该类采集空白的完整产品合同；现场睡眠行为仍 UNKNOWN。
FUTURE_FAILURE_MODE=把未采集时段当节点故障/健康，或把跳过计数误认为完整停机解释。
NEEDS_ARCHITECTURAL_DECISION=true
依据：O01。历史空白不能靠“补采”还原过去真值。

### AF09 — schema 演进缺版本合同

FAULT_LINE=现有幂等 migration 没正式 schema version 框架。
CURRENT_EFFECT=当前 schema 可用，无已证实迁移事故。
FUTURE_FAILURE_MODE=新增 job/统一记录后，升级、兼容读取、失败恢复难以明确界定。
NEEDS_ARCHITECTURAL_DECISION=true
依据：F07 / G Architectural Risks 第 6 项；不代表需要更换数据库。

### AF10 — 结果可见与保存完成之间的边界

FAULT_LINE=异步保存将“测完”和“落盘”分开。
CURRENT_EFFECT=已有 saving/saved/failed 和正常退出等待；强杀前未提交的记录可能不存在。
FUTURE_FAILURE_MODE=文案或新调用者把 result-ready 当 durable，用户重开找不到已承诺保存的记录。
NEEDS_ARCHITECTURAL_DECISION=false
依据：O07。当前设计可成立；只有要求更强崩溃耐久性时才需升级合同，不把所有异步保存视为 bug。

### AF11 — 订阅凭据进入通用展示 DTO

FAULT_LINE=节点 DTO 有凭据边界，AirportDTO/Modal 仍直接展示完整订阅 URL。
CURRENT_EFFECT=带 token 的 URL 可在界面及其 title 中出现；没有材料证明已外泄。
FUTURE_FAILURE_MODE=截图、复制或未来复用 DTO 扩大凭据暴露。
NEEDS_ARCHITECTURAL_DECISION=false
依据：F11 / L §12。局部决定 mask、显式查看/复制即可；不支持由此推导全面认证系统重构。

## 8. Product Tensions

### PT01 — 即时测速 vs 长期监测

TENSION=一次测试的结果与持续样本不是同一产品承诺。
WHY_IT_EXISTS=Workbench 以 attempt 为中心，Monitor 以 job/probe/time 为中心，界面却暗示节点可以直接“加入”。
WHAT_GOES_WRONG_IF_UNRESOLVED=用户以为一次测速进入长期证据，或认为历史图覆盖所有测试。
DECISION_NEEDED=二者是明确独立的工作流，还是共享节点上下文的连续工作流；若连接，哪些动作只是预填、哪些创建/启动任务？

### PT02 — 测速工具 vs 机场管理工具

TENSION=订阅管理是采样前提，但也可能扩展成独立产品重心。
WHY_IT_EXISTS=真实 cache 与配置必须管理，未来套餐/流量/到期等方向又会扩大界面范围。
WHAT_GOES_WRONG_IF_UNRESOLVED=管理功能吞掉测量事实表达的空间，并增加凭据暴露与迁移负担。
DECISION_NEEDED=机场管理保持导入/刷新/选节点的必要范围，还是成为第一类管理产品；未来服务信息可明确 Park。

### PT03 — 事实展示 vs 推荐

TENSION=有测量结果不等于有可比较、充分且新鲜的推荐证据。
WHY_IT_EXISTS=推荐只读 Monitor samples、受活 job 和五分钟门槛限制；用户最新 Workbench 测试不被自动使用。
WHAT_GOES_WRONG_IF_UNRESOLVED=出现“刚测完却无推荐”的不可解释体验，或为了给建议而混用不同测量条件。
DECISION_NEEDED=推荐认可哪些来源、怎样显示不足/过期/无任务，是否向普通用户开放推荐入口？

### PT04 — 推荐 vs 自动控制

TENSION=解释性建议与改变用户代理选择的风险不同。
WHY_IT_EXISTS=底层手动选择和 Auto 函数已在，正式政策/用户控制路径未闭合。
WHAT_GOES_WRONG_IF_UNRESOLVED=一次“补接线”变成未经清楚授权的系统行为，推荐不充分却执行切换。
DECISION_NEEDED=手动控制是否单独交付；Auto 触发者、授权、用户锁定、验证和失败处置由谁负责？

### PT05 — 信息密度 vs 可读性；专业证据 vs 普通理解

TENSION=raw samples、窗口、错误、confidence 等专业证据与多个历史入口同时出现。
WHY_IT_EXISTS=五种历史/旧工作台表面并存，导航“历史记录”实际只指 Monitor。
WHAT_GOES_WRONG_IF_UNRESOLVED=用户把不同来源拼成一条时间序列，误读未测零值或窗口范围。
DECISION_NEEDED=先展示哪种任务/结果；来源、时间、保存状态必须在哪里可见；哪些原始细节按需展开？

### PT06 — Controller-first vs 独立运行

TENSION=节点事实采集可独立运行，而控制功能要求外部 Mihomo 上下文。
WHY_IT_EXISTS=Runner 使用隔离 proxy，不修改 Selector；Controller 是另一条 adapter 能力。
WHAT_GOES_WRONG_IF_UNRESOLVED=把 Controller 连接误设为测速前置，或让用户以为独立 probe 在测当前系统代理选择。
DECISION_NEEDED=首要产品是独立节点事实工具、Controller 辅助工具，还是明确分模式；不同结果的上下文如何说明？

### PT07 — 桌面应用 vs 24/7 服务

TENSION=关闭/睡眠可中断的桌面进程与持续服务期待冲突。
WHY_IT_EXISTS=job 内存态、ticker、keep_all，没有跨重启和系统生命周期合同。
WHAT_GOES_WRONG_IF_UNRESOLVED=把 DB 重开当任务恢复，把睡眠空白当故障，或自动恢复产生无感资源消耗。
DECISION_NEEDED=“持续”究竟承诺会话内、重开后恢复，还是独立常驻；需与恢复确认、缺口解释和保留责任一致。

来源：L §3、9–10、19；G Product / UX Risks、Architectural Risks。以上是需 Pro 取舍的矛盾，不是既定方案。

## 9. Product Bugs 与 Environment Errors 分离

### PRODUCT BUG

- **明确的现有产品问题**：cwd 型 profile 默认值导致跨启动数据世界漂移（AF01）；观察窗口承诺未实现（C03）；“加入持续监测”未携带/加入节点（O02）。
- **已有实现缺口/风险，不能冒充已发生事故**：legacy name-only 混合风险、TestSingle 不保存和零值未测歧义；raw URL 展示风险；L §11 所述 Monitor 部分保存/更新错误忽略返回值路径。若该写入链成为修改范围，先定位具体错误与用户反馈，不直接断言已丢数据。
- **未完成产品合同**：durable job、现代吞吐/服务、推荐 UI、Auto、retention UI。它们不是已完成功能突然坏了；是否实施受产品范围决定。
- **不是天然 bug**：异步 saving 在强杀前未落盘。已有状态和 Close 等待成立；只有宣称已 saved 却未保存，或承诺更强耐久性时，才构成相应违约。

### ENVIRONMENT / OPERATION ERROR

- 从 `4096fb8` 根目录、PR #9 / PR #7 旧 worktree 或早于 PR #10 的 exe 启动，看到旧 UI，不证明 main 缺 UI。
- 选错 cwd / profile 世界产生空节点，可以是本次启动操作错误；默认数据位置随 cwd 漂移则仍是产品设计问题，两者不能互相抵消。
- 旧进程占端口是应先排查的运行来源问题，**材料现场没有这种进程或监听**，不能写成已发生根因。
- 因无 live 进程，材料没有证明此前某个 HTTP 空列表究竟来自哪个实例；该历史实例归因保持 UNKNOWN，无需为规划重建现场。
- 临时 evidence 库与用户库不同，用户库零行不证明 PR #9 失败，也不能推出用户从未在任何版本/目录成功保存。

### HISTORICAL / CLOSED

- PR #1–#10 合并；PR #10 UI 与 UiSelect 已进入 main。
- PR #5 store 的 gap/catch-up 修复、PR #7 只读/候选隔离/窗口数学、PR #9 persistence/attempt/stale guards 的原范围证据继续有效。
- 旧“24H SOAK: PASS”已经纠正：短时 smoke PASS，24h INCOMPLETE。纠错已关闭，发布级长稳尚未完成；两者不混淆。
- Wails/Vue/AppService 架构基础已在 main。旧 gui 残留并不意味着架构迁移全未完成。

## 10. What Is Already Good Enough

“足够”指没有证据要求推倒重做，不授予新增 VERIFIED：

| 基础 | 可以保留什么 | 不要外推什么 |
| --- | --- | --- |
| Go Core / AppService | 真实测速、profile 解析、应用服务与 adapters 分层 | CLI 已统一或旧路径无兼容债 |
| Vue 3 / TypeScript / Wails | 现有桌面/Web UI、bridge、组件和打包基础 | 所有页签已接线、两平台均发布验收 |
| SQLite | raw sample / attempt 存储、事务与重开、cursor/stats/facets | job 已恢复、schema 升级合同完备、容量无限 |
| External Controller Adapter | Mihomo client、手动接口、loopback/secret 边界 | Controller UX 或 Auto 已完整 |
| 稳定身份与 Workbench 保存 | profile/node/revision、attempt、异步保存状态、防陈旧响应 | legacy 已迁移、强杀前 saving 必然持久 |
| Monitor raw sample 与 Timeline store | 独立 probe、原始成功/失败、分页和 gap continuation | OS 睡眠连续性、当前改后视图已验收 |
| PR #7 policy/evidence | 只读推荐、候选隔离、窗口与充分性/置信度规则的既有证据 | 现行用户数据可推荐、Workbench 已成为输入 |

原始测量应继续作为派生事实来源。统一语义、补入口或补生命周期合同均可在上述基础上进行；材料不支持整仓重写、换框架或以“大一统模型”抹平测量差异。

## 11. Decision Inventory for Astra Pro

只列问题、选项与限制，不选答案。Must Decide Now 指最终路线图需要的**范围决策**，不是要求立即实现所有相关功能。

### Must Decide Now — 6 项

#### D01 — 数据定位与既有数据权威

QUESTION=profile/cache 的稳定定位合同是什么，根目录真实订阅与临时 worktree 数据怎样选择/迁移？
WHY=所有节点入口、发行启动与恢复语义都依赖它。
KNOWN_OPTIONS=默认用户数据目录；显式配置路径；稳定默认加显式 override。cwd 仅在明确 portable/CLI 语义下保留也是选项。
KNOWN_CONSTRAINTS=保留用户数据和稳定身份；不自动覆盖不同 profile 世界；history 当前在 home；无需为了消除旧 exe 误启动而设计大型版本管理系统。
EVIDENCE=F03、F11–F12、O05、AF01。

#### D02 — 第一阶段核心用户价值与入口范围

QUESTION=目标优先服务即时多指标测速、长期监测，还是二者的明确组合；CLI 和 legacy 活入口各承诺到什么程度？
WHY=决定 Workbench parity、Monitor durability、管理功能和兼容成本的取舍。
KNOWN_OPTIONS=即时测试为核心；监测为核心；保留双工作流并明确连接；CLI 独立兼容或纳入后续 parity。旧无入口 gui 可冻结待定。
KNOWN_CONSTRAINTS=现代正式测试只有 latency；CLI 绕过 AppService；不能把 Monitor HTTP probes 当现代服务测试已交付。
EVIDENCE=F03–F06、PT01–PT02、PT06、AF02。

#### D03 — “持续监测”的承诺

QUESTION=首个目标版本承诺会话内监测、重启恢复，还是独立常驻？睡眠空白如何定义？
WHY=不同答案改变生命周期、资源和保留设计，不能在路线图中只写“完成 24/7”。
KNOWN_OPTIONS=明确仅会话内；恢复定义但需用户启动；经用户设置自动恢复；独立常驻方向。后者没有现成实现证据，只是更大范围选项。
KNOWN_CONSTRAINTS=现有 job 内存态；历史重开不是恢复；不可还原睡眠期间未采事实；恢复需连带保留责任，不静默扩大采集承诺。
EVIDENCE=F04、F16、O01、O06、PT07。

#### D04 — 历史/测量权威与推荐输入边界

QUESTION=三类记录长期分域还是共享逻辑契约，用户“历史”的范围是什么，Recommend 维持 Monitor-only 还是扩输入？
WHY=扩指标和画统一图之前需要明确数据来源与可比性，避免旧显示名污染稳定身份。
KNOWN_OPTIONS=分域并显式标来源；统一逻辑 envelope/查询但保留不同存储；统一扩展记录并兼容读 legacy。推荐输入可保持 Monitor 或经明确规则纳入 Workbench。
KNOWN_CONSTRAINTS=probe、target、时间、配置版本、未测/失败语义必须保留；不从汇总插值造 raw；不要求所有记录物理同表。
EVIDENCE=F07、F09、AF03–AF04、PT03、PT05。

#### D05 — Controller / Auto 在目标版本中的位置

QUESTION=本次最终路线图是否纳入控制；若纳入，是只读推荐、手动控制还是 Auto？
WHY=控制是显著不同的产品和正确性边界，必须先限定范围，细节可后决。
KNOWN_OPTIONS=保持独立事实工具并 Park 控制；先只读推荐/手动 Controller UX；包含 Auto 但将执行保障作为必要条件。
KNOWN_CONSTRAINTS=现有 Runner 不 SelectNode；手动 API 已能执行；ModeAuto 可配置但无执行者；不能以补接线替代授权和失败处置。
EVIDENCE=F08–F10、AF07、PT04、PT06。

#### D06 — 交付对象与验收口径

QUESTION=路线图交付的是代码能力、可用桌面迭代还是正式发行版；目标平台/入口和最低证据范围是什么？
WHY=否则会将 CI 当发行完成，或反过来为每次小修改追加完整双平台/24h 验收。
KNOWN_OPTIONS=明确 Web/桌面指定路径的功能交付；Windows 先交付；Windows+macOS arm64 发行。其它架构可以明确延期。
KNOWN_CONSTRAINTS=现有 CI 和历史定向证据可复用；涉及新交互/跨重启/数据丢失风险才补相应检查；本次不默认发布、不重跑已关闭证据。
EVIDENCE=C02、F14、OC02/05/06、用户最小充分验证规则。

### Decide Before Specific Phase — 7 项

#### D07 — 图表与动作的真实语义

QUESTION=观察窗口改为真实滚动范围还是改文案；“加入监测”改为导航还是预填创建？
WHY=继续扩图/节点流转前必须消除错误操作模型。
KNOWN_OPTIONS=since/until 端到端窗口；诚实表达最近 N 次或展示范围；导航改名；携带节点进入确认表单。
KNOWN_CONSTRAINTS=当前 limit=20 不等于时间窗口；00:00–24:00 不等于最近 24h；不得暗中启动 job。
EVIDENCE=C03、O02、AF06。

#### D08 — 新指标 contract

QUESTION=新增 batch/download/upload/service 前，metric 状态、原始过程样本和 attempt 范围如何定义？
WHY=决定可重开、可比较和失败解释，单纯接卡片不够。
KNOWN_OPTIONS=扩展现有 attempt；各指标独立记录加公共元信息；批次作为多个稳定身份 attempt 的关联层。
KNOWN_CONSTRAINTS=显式区分 success/failed/untested/not_available；吞吐过程必须有独立字节/时间样本；ProbeSet 与服务可用性不混名。
EVIDENCE=L §5、9、17；F06–F07；AF03。

#### D09 — durable job 的实施合同

QUESTION=实现恢复前，job/config 版本、订阅消失/更新、恢复确认、retention 和 schema 升级如何协同？
WHY=把定义写进 DB 不等于可安全恢复，自动采集会改变增长边界。
KNOWN_OPTIONS=仅恢复 stopped 定义；按用户策略恢复；配置变化需确认；保留按时间/用户选择等，具体默认仍待数据与产品需求决定。
KNOWN_CONSTRAINTS=后端重解析 cache；旧历史不能复活已删除候选；不可无声覆盖 profile；不得用“几个月事故”代替容量证据。
EVIDENCE=O05–O06、AF04–AF05、AF09。

#### D10 — 结果落盘承诺

QUESTION=产品是否接受 saving 期间强杀丢失，还是要求更强的崩溃耐久性？
WHY=决定退出行为、恢复机制和保存失败提示，不能由用户库零行倒推出答案。
KNOWN_OPTIONS=沿用明确 saving/saved/failed 与正常退出等待；增加恢复/重试能力；若有明确需求再提高结果交付前的耐久性要求。
KNOWN_CONSTRAINTS=30 秒保存超时不等于所有退出可保存；不得隐瞒失败或把 result-ready 当 saved。
EVIDENCE=O07、AF10、PR #9 范围见 L §14。

#### D11 — Controller 执行责任

QUESTION=进入手动/Auto 阶段前，谁持有配置与 policy、谁触发执行、谁解释和记录结果？
WHY=需要把可执行 API 与用户控制闭合，避免 scheduler 随手成为执行者。
KNOWN_OPTIONS=用户确认动作；独立 orchestrator；受明确 policy 约束的调度触发；手动阶段可先独立存在。
KNOWN_CONSTRAINTS=用户锁定、冷却、候选隔离、充分性、执行验证/失败处置与审计需明确定义；默认 fail-closed；无需等待所有 Workbench 指标完成才讨论手动 UX。
EVIDENCE=L §3.2、18 D；G Architectural Risks 第 3 项；AF07。

#### D12 — 凭据显示边界

QUESTION=Airport URL 默认如何隐藏，哪些动作允许查看/复制完整值？
WHY=订阅管理/分享界面扩展前需限制不必要暴露。
KNOWN_OPTIONS=默认 mask 加显式查看/复制；常规 DTO 不携带完整值、管理操作单独读取。
KNOWN_CONSTRAINTS=保留编辑与导入能力；报告/截图证据不复制 token；loopback 不等于可以随意展示敏感 URL。
EVIDENCE=F11、AF11、L §12。

#### D13 — 发布与运行定位

QUESTION=选定发布范围后，产物/main/cwd 如何识别，首次导入、macOS bundle 数据位置和必要原生证据是什么？
WHY=避免旧 exe、旧目录和配置问题被误报为产品退回。
KNOWN_OPTIONS=明确 main 构建目录/产物约定；可识别的版本与数据位置；首次导入或显式目录选择；按目标平台补必要证据。
KNOWN_CONSTRAINTS=无需删除历史 worktree 才能正确启动；沿用 D01；CI 不覆盖签名、公证和原生交互，但范围之外不自动追加。
EVIDENCE=F01、F12、F14、§9。

### Safe to Defer — 4 项

#### D14 — 无入口旧 gui 的最终清理

QUESTION=冻结的旧 server/UI 何时移除？
WHY=目前可通过标明正式入口控制误改风险，无须抢占核心功能范围。
KNOWN_OPTIONS=冻结待删；在入口收口阶段删除；若决定支持则另定合同。
KNOWN_CONSTRAINTS=不将旧 gui 与仍挂载的 Vue legacy-surface 混为一谈；不向无入口实现投入新功能。
EVIDENCE=O03、AF02。

#### D15 — 扩展平台与长稳

QUESTION=是否支持 macOS Intel/Universal、何时开展完整 24h/发行验收？
WHY=尚未进入目标发布范围时可以 Park，不能为规划默认消耗测试资源。
KNOWN_OPTIONS=保持 arm64 构建基础；按选定发行需求补平台；发布目标明确后安排对应长稳。
KNOWN_CONSTRAINTS=保持 24h INCOMPLETE，不伪造 PASS；D06 若选择相关发行范围则提升为阶段前决定。
EVIDENCE=F14、L §10 P-01/P-04、G DO NOT REOPEN。

#### D16 — 机场周边与视觉扩展

QUESTION=主题、套餐/流量/到期、官网、IPv6/订阅整理等何时纳入？
WHY=不决定当前稳定测量与历史合同，可以明确延期。
KNOWN_OPTIONS=持续 Park；仅在 D02 选择管理工具方向后单独纳入。
KNOWN_CONSTRAINTS=不把未来字段当现有 backend，不为视觉完整填 demo 真值。
EVIDENCE=L §10 P-05、PT02。

#### D17 — 底层换框架/换库

QUESTION=是否需要全面更换 Go/Vue/Wails/SQLite？
WHY=现有材料没有使此方案成立的缺陷依据，当前不应占用规划资源。
KNOWN_OPTIONS=保留现有基础；仅有新的、可定位且无法局部解决的实质证据时重新评估。
KNOWN_CONSTRAINTS=状态下调、三类记录、旧 gui 残留和缺 schema version 均不足以推出全面迁移。
EVIDENCE=§10、F02、F07。

## 12. Candidate Direction Space

这些方向可组合；排列不表示优先级，亦无期限/里程碑。Astra 应先回答 D01–D06 再选择，不照搬 L 的“合理顺序”。

| 方向 | 优点 | 缺点 / 机会成本 | 前置条件 | 影响 |
| --- | --- | --- | --- | --- |
| A：数据定位与运行身份收口 | 消除空节点/旧产物误判，改善导入与恢复基础 | 新测量能力增量小；错误迁移会损伤真实数据 | 识别真实与临时 profile，决定 D01；不要求重建全部产物 | 所有节点入口、发布、durable job；不自动统一 CLI |
| B：Workbench 多指标/批量 | 直接提升测速价值，复用真实 Core | 若只接 UI，会扩大状态与历史分叉；推迟持续监测 | D02/D04 方向明确；对应 metric/attempt/raw contract 先明确，D07 语义修正 | 现代测速、legacy 支持范围、结果可比性 |
| C：逻辑数据模型与历史表达收口 | 降低错误归属，明确来源及推荐边界 | 容易过度抽象并推迟用户价值；迁移有兼容成本 | D04；盘点只限受影响写读路径，不开展新全仓审计 | Workbench/Timeline/Node Detail/Recommend；可保持分表和兼容读取 |
| D：Monitor durability | 支撑重启后继续使用与长期候选上下文 | 引入恢复、容量、睡眠和配置变化责任；不补新测速指标 | D01/D03；恢复/retention/schema 合同；不要求先完成所有 Workbench 指标 | 持续监测、长期证据、发行生命周期 |
| E：只读推荐或手动 Controller 产品化 | 利用已有 adapter/policy，让证据不足等状态可解释 | 可能偏离独立测速核心；手动控制已有真实副作用 | D05；推荐需可用候选与新鲜样本，手动控制需配置和确认边界 | 信息架构、控制责任；不默认包含 Auto |

Auto 只在 D05 明确纳入且相关条件完备后构成候选执行范围；材料不支持把“函数已存在”作为优先接入理由。发布验收是所选方向的交付边界，不是第六条默认开发方向。

## 13. Things Astra Pro Must NOT Assume

1. 有代码/测试/接口 = 用户功能完成；L 的 VERIFIED = 发布验收。
2. 当前目录就是 main；旧 exe 看不到新 UI = PR #10 未交付。
3. Wails/Web/CLI 全都经过 AppService，功能/保存自然同步。
4. CI Windows/macOS build = 原生用户验收；macOS arm64 = Intel/Universal/签名/公证齐全。
5. Monitor 有 scheduler 或 DB 能重开 = job 能恢复，或可在睡眠期间连续采样。
6. 291 样本 = 当前新鲜、多订阅、可推荐证据；它们在材料现场均已过期。
7. DB 有历史 = 当前 profile 仍存在；不同 worktree 的 ignored profile 是同一副本。
8. Workbench latency 刚测完 = Recommend 已得到该证据；当前只吃活 job 下 Monitor samples。
9. Recommendation/ModeAuto/SelectNode 存在 = Auto 有正式执行循环；无 UI = 手动 API 不可执行。
10. service probes = Antigravity/流媒体/现代服务页签完成；heavy TTFB = 下载吞吐。
11. 所有“历史”读同一记录；可用显示名串接旧历史和新 profile-scoped 图。
12. 4h/24h 控件已筛选数据；“加入监测”已创建、预填或启动任务。
13. 用户库零行证明 PR #9 失败或发生强杀丢失；saving 也等于 saved。
14. keep_all 可无限持续运行而无需容量合同；有 retention API = 用户有清理入口。
15. 可以由汇总速度插值生成真实过程图，或用补采还原睡眠时段原始事实。
16. 多条测量路径、旧 gui 或状态降级 = 应全面重写；更强模型接手 = 必须重审历史。

## 14. DO NOT REOPEN

没有新改动、新失败或具体未解问题时：

- 不重新合并 PR #1–#10，不恢复第二遍原型 UI/UiSelect；不将旧 head/worktree 当待合并事项。
- 不重做已落地的 Wails/Vue/AppService 迁移，不把旧 gui 残留扩写成重新选框架任务。
- 复用 PR #5 未变 timeline.ts 的 gap/catch-up 证据；PR #10 视图范围的缺证不能倒推 store 失效。
- 复用 PR #7 只读、candidate/profile/revision isolation、窗口/充分性/置信度的对应证据。现行 job/样本不足不是数学反证。
- 复用 PR #9 临时 Web、attempt/save/reopen/stale guards 证据；用户库零行不要求重跑同一临时用例。
- 保留短时 smoke PASS 与 24h INCOMPLETE；不恢复错误长稳结论，也不为本报告默认补长稳。
- 不为同一 SHA 再编 Windows/macOS 证明 CI，不按接手模型/审核者变化重开验收。
- 不再把已识别的旧 branch/cwd/产物操作误差当大型代码缺陷；历史未知实例的具体归因可 Park，cwd 产品合同仍在 D01。
- 不全量扫描 Obsidian/Raw/Archive，不引入新 Feature Truth 制度、重复测试或无行为后果的清理任务。

重开必须指出受影响行为、旧证据覆盖不到的具体变化，以及最小可回答检查；已知答案直接复用，不为这条规则另写报告。

## 15. Final Planning Brief

### Project Now

材料基线是 main `58d036e220d873f900f755fdc47b2f6850a0b9b3`，PR #1–#10 已合并；根目录仍为旧 PR #8。Go/Wails/Vue 的真实产品基础已具备：机场 cache → 稳定身份单节点 latency → 异步 SQLite 保存；Monitor 在单进程内采集，Timeline 展示其 raw samples。现代吞吐/服务未接，Recommend 是无正式 UI 的只读组件，Auto 没有生产执行链。当前材料无 live 进程；用户库有旧 Monitor 样本但 workbench 零行，不支持现行推荐。

### Biggest Structural Problems

profile/cache 与 history 分处 cwd/home，启动会改变节点世界；三类记录与 legacy 身份未统一解释；持久样本依赖易失 job 才能推荐；跨重启恢复、保留责任和睡眠语义尚未闭合。UI 的观察窗口与“加入监测”有确定承诺错误，需要局部纠正。未验收范围与真实缺实现分开处理。

### Strongest Existing Foundations

保留 Go Core、Vue/TypeScript/Wails、AppService 桌面/Web adapter、SQLite 原始记录与查询、稳定身份/attempt、异步保存状态、Timeline store、External Controller adapter 和 PR #7 只读 policy/evidence。没有证据要求重选框架、数据库或全面重写。历史定向证据仍按原范围有效。

### Key Dependencies

稳定数据定位与 profile 选择 → 可解析且隔离的节点上下文。明确 metric/来源/时间/失败语义 → 可解释历史与派生事实。job 恢复必须连同配置变更、用户恢复策略、retention 和睡眠缺口考虑。现行推荐需要活候选与新鲜充分的 Monitor evidence；接纳 Workbench 需另定规则。Auto 需要明确授权与执行保障，不能靠函数接线完成。分域存储可保留，统一语义不等于统一物理表。

### Decisions Pro Must Make

先决定六个范围问题：**D01** 数据位置及迁移权威；**D02** 即时测速/持续监测的核心价值与入口支持；**D03** “持续”的生命周期承诺；**D04** 历史权威及推荐输入；**D05** 控制是否在目标范围；**D06** 交付平台与最低验收范围。图表动作、metric、恢复/retention/schema、保存耐久性、控制责任、URL 展示及发布细节，分别在相关阶段前决定。候选 A–E 提供取舍，不代表既定排序。

### Things Pro Should Avoid

不要将 L 的 VERIFIED 当发布完成，也不要因降级重审已通过的组件。不要从旧 worktree 推断 main，不用零行推翻临时 evidence，不把旧样本当现行推荐，不从显示名/汇总值造新事实。不要把入口操作问题规划为全面重构，不静默启用恢复或 Auto，不因能力更强就重跑 CI、native 全套或 24h。四项报告冲突已在材料限定范围收口；真正待决的是产品合同与方向选择。

---

报告计数口径：Canonical Facts F01–F16 = **16**；报告冲突 C01–C04 已收口 **4**、未决 **0**；七项遗漏中 Planning Critical **4**；架构裂缝 AF01–AF11 = **11**；Must Decide Now D01–D06 = **6**。`PRO_READY=true` 仅表示综合材料已足以进入最终规划，不表示产品或发布 ready。

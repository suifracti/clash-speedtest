# 迁移前分支整合（2026-10-05）

仓库：`suifracti/clash-speedtest`。整合唯一基线是已经推送的 `codex/mac-migration-20261005` / `70f25a0947d5acf9ac8d35a1414a0a6a85b1c9e9`；独立整合分支为 `codex/mac-integration-20261005`，本地提交 SHA 以交付记录为准。未经用户对新目标明确确认，不推送新分支；不更新 main、不强推、不合并或发布。

## 盘点结果与处理规则

本次 fetch 不 prune；GitHub 实时远端分支查询成功。创建整合分支前：25 个本地分支、18 个实际 origin 分支、20 个 remote 类 ref（另外两个为 origin/HEAD 和历史 pr7head）、15 个 worktree。主目录是唯一 dirty 工作树，其余 14 个干净；detached 的 PR7 review HEAD 14d5203 及历史 pr7head 5e54464 都是迁移基线祖先。新增一个独立整合 worktree 后总数 16，全部保留。

每个分支先按 ancestry 判断，不根据相对最新 HEAD 的整包 diff 套补丁。所有远端 tip、main 和 22 个其它本地功能分支已是迁移基线祖先；没有新的业务提交。唯一非祖先提交为旧 README 的 5b916bd；`git cherry` 为 +，确实不是已重复纳入的完全相同补丁。该提交按内容部分保留，不做整个 cherry-pick，不声称原提交已成为祖先。

| 原有本地分支 | HEAD | 处理结果 |
| --- | --- | --- |
| codex/cst-closeout-01 | `5b916bd875e3` | 8 行 README 独有；适用说明改写保留，其余不采用 |
| codex/cst-p1-03-monitor-restart-recovery | `02f1e6e3b29d` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| codex/cst-p1-04-batch-latency | `de5b3e96cdd9` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| codex/cst-p1-05-public-service | `cc174ab87d44` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| codex/cst-p1-06-download-history | `639fbe8e0c79` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| codex/cst-p1-10-node-detail | `7ef464f26eec` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| codex/cst-p1-11b-monitor-sampling | `4b45e00d0ef2` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| codex/mac-migration-20261005 | `70f25a0947d5` | 本轮唯一整合基线；原分支保持不变 |
| codex/monitor-job-ui | `4096fb852ceb` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| codex/restore-prototype-ui | `b2f513907f9f` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| codex/workbench-latency-persistence | `2a5f64f3cd02` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| feature-monitor-timeline-ui | `d22ebfc35d98` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| feature/monitor-global-budget | `d708035532bc` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| feature/monitor-retention-capacity | `1a18c8f9a526` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| feature/persist-monitor-jobs | `ee743576ba35` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| feature/v5-workbench-enhancements | `b646aafece6f` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| fix-monitor-history-test-race | `6abec1cfcf4f` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| fix/mask-subscription-url | `cbcfd744e7a7` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| fix/migrate-history-data-root | `ceee8994e0a9` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| fix/monitor-persistence-failure | `40d5c41d0416` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| fix/stable-profile-data-root | `bf67bf92a1de` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| fix/workbench-monitor-create-flow | `82d7fb92bbd9` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| fix/workbench-observation-window | `bc4545e54179` | tip 已是整合基线祖先；无未整合增量，不重复应用 |
| main | `219b181ef7be` | 历史基线；不覆盖或更新 |
| refactor/wails-desktop-architecture | `1477115b5d88` | tip 已是整合基线祖先；无未整合增量，不重复应用 |

实际 origin 分支均与上述同名 tip 相符且已经纳入；历史 remote 类 ref 不代表尚未合入的新分支。完整 ref SHA、worktree 路径／状态和文件内容比对留在 Git 外 `artifacts/integration-20261005/inventory.json`。

## 未提交成果、旧实现与实验候选

主目录虽然相对旧 main 仍有 211 个状态条目，实际现存的 290 个交付文件逐个与 70f25a0 Git blob 比对，仅规范化 CRLF：全部相同、无缺失、无新增业务变化。这些前端改写、保存／迟到响应隔离、监测显示、日额度移除和用量功能已经由迁移快照一次纳入；不再根据 main 的 dirty diff 重放。额外现存文件只有私人 HANDOFF／三份旧全景报告、编译产物和过期 evidence，按原迁移边界保留本地。

旧前端 workbench／timeline 组件及其 synthetic harness 已被当前 Vue 页面替换，迁移提交包含真实删除状态；祖先分支中存在旧代码不构成恢复它们的理由。旧每日额度实现也被新版补丁移除，不因全局 budget 分支仍在就恢复额度语义。现有并发／单响应限制不变。

| 候选／证据 | 独有内容 | 验证状态与本轮处理 |
| --- | --- | --- |
| .workbuddy-ai/design | 独立 Monitor 工作台 HTML 演示设计；明确使用演示数据，不连接后端 | 设计候选，非产品实现；不采用、不重做圆环或页面 |
| .ui-preview/compact-simplification | mock fetch 的 Vue 离线预览及截图；导入现已删除的 LatencyWorkbench 等模块 | 过期展示 harness，不能证明当前功能；保留本地，不运行、不合入 |
| .workbuddy-ai/soak | 旧基线的 Python 采样／验收工具、DB、日志和原 verdict | 历史工具与私人证据；不作为当前基线新验证，不恢复监测或任务 |

.workbuddy-ai 共 95 文件、72042730 字节；.ui-preview 共 5 文件、256587 字节。原历史 verdict 保留：2026-09-18、6abec1c 基线、5 节点约 10h56m 扩展长测，不是 24h PASS；记录的 657 次 run 全部 partial_failed。新版全机场 264 节点／16 轮约 76 分钟是另一份证据，不串成“当前版本已运行一天”。失败记录、未知退出原因和原始证据未改写。

## 旧 README 5b916bd 的逐项比较

| 旧说明 | 处理 | 原因 |
| --- | --- | --- |
| Wails EXE 直接 go build、双击、无参数桌面启动 | 不加入本次启动入口 | 当前交付要求源码服务＋Web；新源码还须先构建前端并准备历史嵌入资源，旧直接构建命令不完整 |
| Windows 默认总是 LOCALAPPDATA | 按当前源码改写 | 直接二进制优先邻近 data，go run 才采用平台数据根；用明确绝对目录避免误读旧数据 |
| data-dir／环境变量、显式目录不自动合并数据 | 保留有效语义 | 当前 appdata.Resolve／隔离数据契约仍适用 |
| 订阅、节点、延迟、下载、服务、监测及历史 | 保留并按当前 Web 名称表述 | 当前已实现，已有 Windows 证据；下载仍有界，不新增能力 |
| 监测仅在进程运行时采集；无后台常驻、睡眠采样、自动选路及桌面上传 | 保留，补明确“不承诺睡眠采样” | 当前服务生命周期仍受此限制，不能把终端关闭后的退出当自动续跑承诺 |
| 可选择下次启动恢复运行任务 | 不写成当前 UI 可选入口，保留停止／阻塞不自动恢复 | 后端 ResumeOnLaunch 有条件恢复逻辑，但现交付任务已停止／阻塞、未核当前 UI 对应选项；不擅自新增恢复语义 |

整合只改 README 和接手／来源说明，不修改启动代码、监测配置或产品功能。没有需要用户裁决的测速功能冲突；原旧 README 提交／分支／worktree 与 bundle/patch 均继续保留。

## 验证及接手边界

本轮只检查说明与当前 flags／数据根代码一致、Markdown 本地链接存在、Git 差异无新格式错误，并比较除明确文档增量之外的文件树与 70f25a0 完全一致。不启动服务，不新增真实请求，不做浏览器／桌面操作，不重跑业务测试。

构建证据复用迁移基线的 Windows `go build -buildvcs=false -o NUL .`：业务源码、依赖、锁文件、嵌入准备脚本和启动代码未变，因此文档整合不需要重新编译。该旧检查借用了此前通过的前端 dist，不能宣称 Mac、新前端重建或运行验收已通过。

待修／未验原样保留：最近延迟排序未统一监测 RTT；完整筛选范围指标加载缺口；Mac 实机、真实保存失败 UI 重试、完整鉴权生命周期、每日用量午夜／多日、24小时常驻未验；Windows 进程终止发起来源未知。已有中间失败及原始记录不删除，不把未验写成通过。

SQLite schema 10、配置格式、依赖和 Git 外资源没有改变；私有 ZIP 的原 SHA-256 继续有效，只更新交付清单中的最终整合分支／SHA，不重打数据包。Windows 主目录和全部旧 worktree 保留。Mac 接手最终整合提交后，Windows 不同时修改该基线，后续改动另建分支。

## 整合包仓库的边界

MC 整合包为另一仓库，基线 6a2516054e337c0ffb9795df999f56ea511b7b6b，由其原负责对话在独立分支盘点并交付。PR #21、A/B 候选的暂不合入边界继续保留；测速整合不复制 MC 文件、不代替其独有内容／验证状态盘点，也不自动采用候选。

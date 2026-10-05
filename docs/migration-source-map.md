# 迁移源码来源清点（2026-10-05）

现有 origin=https://github.com/suifracti/clash-speedtest.git，GitHub API 核实 visibility=PUBLIC、defaultBranch=main、远端 main=219b181ef7be45e3245ece5a10d692feb53a1a2c。公开／私有和默认分支不变。git ls-remote 的一次查询连接被重置，随后通过 GitHub API 核实 main；不把本地缓存视为实时远端证明。

Windows 清点：15 个现存 worktree，主目录未提交，其余 14 个干净。24 个原有本地分支中，22 个其它分支的 HEAD 已是 main 祖先，main 本身为基线；唯一不同的是旧 README 候选。完整绝对路径与原始状态留在 Git 外 readonly-inventory.json，避免发布本机用户目录。没有 reset、删除或归档 worktree。

| 原有分支 | HEAD | 相对 main 状态 |
| --- | --- | --- |
| codex/cst-closeout-01 | `5b916bd875e3` | 旧 README 候选：独有 1 提交，未合入 |
| codex/cst-p1-03-monitor-restart-recovery | `02f1e6e3b29d` | 已包含；不重复应用 |
| codex/cst-p1-04-batch-latency | `de5b3e96cdd9` | 已包含；不重复应用 |
| codex/cst-p1-05-public-service | `cc174ab87d44` | 已包含；不重复应用 |
| codex/cst-p1-06-download-history | `639fbe8e0c79` | 已包含；不重复应用 |
| codex/cst-p1-10-node-detail | `7ef464f26eec` | 已包含；不重复应用 |
| codex/cst-p1-11b-monitor-sampling | `4b45e00d0ef2` | 已包含；不重复应用 |
| codex/monitor-job-ui | `4096fb852ceb` | 已包含；不重复应用 |
| codex/restore-prototype-ui | `b2f513907f9f` | 已包含；不重复应用 |
| codex/workbench-latency-persistence | `2a5f64f3cd02` | 已包含；不重复应用 |
| feature-monitor-timeline-ui | `d22ebfc35d98` | 已包含；不重复应用 |
| feature/monitor-global-budget | `d708035532bc` | 已包含；不重复应用 |
| feature/monitor-retention-capacity | `1a18c8f9a526` | 已包含；不重复应用 |
| feature/persist-monitor-jobs | `ee743576ba35` | 已包含；不重复应用 |
| feature/v5-workbench-enhancements | `b646aafece6f` | 已包含；不重复应用 |
| fix-monitor-history-test-race | `6abec1cfcf4f` | 已包含；不重复应用 |
| fix/mask-subscription-url | `cbcfd744e7a7` | 已包含；不重复应用 |
| fix/migrate-history-data-root | `ceee8994e0a9` | 已包含；不重复应用 |
| fix/monitor-persistence-failure | `40d5c41d0416` | 已包含；不重复应用 |
| fix/stable-profile-data-root | `bf67bf92a1de` | 已包含；不重复应用 |
| fix/workbench-monitor-create-flow | `82d7fb92bbd9` | 已包含；不重复应用 |
| fix/workbench-observation-window | `bc4545e54179` | 已包含；不重复应用 |
| main | `219b181ef7be` | 当前基线 |
| refactor/wails-desktop-architecture | `1477115b5d88` | 已包含；不重复应用 |

当前主目录作为最新来源，由前端对话「开始单独修改」和本接手对话核定；前端负责人确认没有独立未整合 worktree。旧收尾对话「luna」确认只保留 5b916bd 的 8 行 README 提交，不维护其它未整合源码。较早「规划」的旧组件说明已被之后前端重构替代，不能据其旧文件列表恢复已删除组件。没有已确认正在别的对话维护而必须合入的新源码。

最新未提交成果：新版 Vue 页面与现有布局、选择和原位趋势；Windows 原生控件／桌面适配源码；订阅与监测范围修正；超时／取消、保存恢复和迟到结果隔离；节点规则、公告清洗；日额度移除；每日订阅更新和日／周／月用量；备份快照；首页与节点详情的监测历史接入。源码和对应测试按当前文件存在性一次记录到新候选分支，不将旧分支相对 HEAD 的整包差异再应用。

新增迁移文件仅涉及 ignore、换行、源码 Web 启动、固定历史资源恢复、CI 资源准备和脱敏接手说明。无功能重构或新业务测试。

候选树排除当前编译产物、node_modules、数据／订阅缓存、凭据、日志、模型、原始 HANDOFF 及三份含本机信息的旧全景报告、旧 synthetic timeline evidence（其模块已删除）。这些文件仍在 Windows 原目录或 Git 历史／私有包中；不从 Windows 删除。README 只加当前接手入口，保留原 CLI 说明但明确不作为 Mac 候选安装入口。

候选仅推送 `codex/mac-migration-20261005`；不得更新 main、改仓库权限、强推、合并、创建发布或标签。推送须由用户对具体提交及文件清单确认。旧 README 候选另有 1276 字节 bundle 及单提交 patch 留在 Git 外，不会和新源码一起重复套用。

历史 GitHub 仓库原本已包含旧 dist 和文档。此分支从最新树取消跟踪，不改写旧提交；完整 clone 的历史仍会有原有对象。若用户要求清除已公开的历史内容，属于需另行决策的历史清理，不能借迁移强推。

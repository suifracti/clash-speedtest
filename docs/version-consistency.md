# 开发、运行版与功能保留检查

代码以 GitHub 明确分支和提交为准，不默认把 `main`、旧工作树、旧 App 或浏览器端口当最新候选。当前开发分支为 `codex/cst-w12i-integration-20261011-6aa2079`；Obsidian 记录任务与决定，Drive 保存脱敏附件。每次改动验证后提交/推送此分支，读回远端 SHA。合并 main、正式发布、生产数据与实网检测仍按对应授权。

## 2026-10-13 实际核对

- 核对 GitHub 全部 42 个已合并 PR：来源 head 和 merge commit 都在候选历史中，没有发现已合入 GitHub 而候选漏带的提交。当前候选包含完整 main，并保留后续 S1–S5、刷新与物理探针修复。
- 但旧 `clash-speedtest-closeout-20261009` 有未提交源码未进 GitHub/当前候选：IPOK 后端与持久化缓存/预算、VLESS `pcs/vcn` 特殊 TLS 参数兼容、Mac 订阅获取的系统 HTTP/HTTPS 代理识别。前端 IPOK 展示/unknown 语义仍在，不等于后端可执行。这三项是待恢复的真实遗漏，不应重新宣称不存在或已完成。
- 旧的 physical_run/physical_observation 已有新私有 physical_probe 与严格 DNS 通路替代；订阅刷新方法部分改名且桥接仍在。不能按旧文件名缺失就整包恢复或重复开发，替代须核对调用及离线回归。
- 本机浏览器 `18479` 对应 2026-10-10 的旧 N1 检测构建，不是当前 Wails 原生候选。不要把该浏览器页面当当前 App 验收证据。未擅自关闭旧实例或改数据根。
- 正在运行的 Mac core 与最近实测二进制 hash 相同，上一包记录的生产源码文件 hash 也相同；但它显示 `dev / commit unknown`，没有完整的当前 clean Git revision 构建 manifest。没有证据说它被错误替换，但构建来源证明不足，不能套用一个猜测 SHA 补齐。

## 每轮的固定动作

1. 在同一现有工作树核对 HEAD、dirty、远端分支、运行 App/PID/数据根。dirty 先保留；只抓取和比较，禁止 reset/覆盖/清理。
2. 比较 main 与候选的独立增量，并核对 `candidate-capabilities.json` 中已接受能力。遗漏、替代、未完成分别记清，不删能力条目凑通过；替代必须有明确映射及行为回归。
3. 运行受影响的离线测试与构建。每轮代码同步 GitHub，明确提交和真实失败；源代码在远端不等于功能已验收。
4. App 从 clean、明确提交重新构建前端和原生 core。Go 链接参数写入 `-X main.commit=<完整SHA>`；构建时记录 source_revision、source_tree、core_sha256、frontend_dist_sha256、build_verified。manifest 必须来自实际 clean 构建，不补写到旧未知二进制。分享可用 App 时带上该记录，重核正在运行的文件/PID。
5. 代码、离线测试、构建、原生交互、实网、保存/重开分开报告；历史未提交能力及实际缺口保留在 GitHub，不仅藏在本地 ZIP/长聊天。

源码检查命令（只读 Git、源码与指定 core，不读取订阅或数据库）：

```sh
python3 scripts/candidate_audit.py --online --branch codex/cst-w12i-integration-20261011-6aa2079
python3 scripts/candidate_audit.py --online --app /absolute/path/cst-core --manifest /absolute/path/build-manifest.json
python3 -m unittest discover -s scripts/tests -v
```

`--online` 只访问 GitHub Git 远端，不做检测请求。没有远端核对、dirty、main 不在候选、能力缺失、App manifest 不明或不匹配都会明确拒绝。只跑源码检查时 `runtime_acceptance=NOT_CHECKED`，不会冒充 App 已验收；manifest 关联也不等于独立审计编译器或运行行为。

当前新增候选 CI 仅 Linux 跑检查器离线 fixture 与源码/远端/能力检查，不扩 Windows 或实网。三项能力未恢复前，它应报告失败并保留 JSON，这是已知遗漏的门禁，不是偶发失败，不重跑掩盖。旧测试/构建的通过不能覆盖这些遗漏。

下一步按三项清单逐个复用旧源码和 fixture 做最小迁回，分别核对 TLS、预算/失败证据、缓存和读取行为；不整包恢复旧 UI，不迁移或打开生产 SQLite，不重跑 IPOK 或节点。

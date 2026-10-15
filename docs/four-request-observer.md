# 有界原生验收观察器

`scripts/four_request_guard.py` 是两节点、四请求原生验收的辅助观察器，不属于 App 的测速通路。它不发 HTTP、不操作 UI、不读取订阅凭据；请求次数、目标、超时和重试必须在启动前由原生计划及后端保证。SQLite 采样属于事后观察，不能当作硬请求防火墙或实际物理包流向证明。

2026-10-15 修复的是此前私有采集器在结果已保存后仍等到绝对截止、误关验收窗口的问题。旧证据、原始截止原因和真实结果不改写。测量成功或失败均可正常结束观察，不把失败重试成成功。

## 输入与收尾

仅在得到当轮网络授权后，准备一个全新绝对证据目录：

- `candidate-entry.json`：`core` 与 `data_root` 的绝对路径；独立数据根必须已有且只有一个相符的 `--gui --no-auto-credentials` 进程。
- `build-manifest.json`：已验证候选的 `core_sha256`。
- `verified-nodes.json`：恰好两枚冻结节点的 `profile_id`、`identity_sha256`、`revision`，不存节点密钥。
- `private/`：仅本地 socket 观察，不能原样对外上传。

先运行 `python3 scripts/four_request_guard.py /absolute/fresh/evidence-directory`，确认 `GUARD_READY` 后，才可在已授权的原生开始操作前写 `arm-live-guard.json`（`armed_at_epoch`）。保留原180秒绝对截止。出现 `run-boundary.json` 的旧目录拒绝再次运行，防止覆盖原证据；本说明本身不授权实网。

当**本轮**四项匹配 request、机场/节点/revision 和服务 ID，轮次 finished，执行终态、保存 saved、结果行确实存在时，记录 `completed_saved` 并只结束观察，不关闭 App。历史终态、仅三个结果、仍运行、仍保存、保存失败或缺失结果不能冒充完成。正常读库使用同一只读事务核对，连接及时关闭。

范围/预算越界、持续读库错误或绝对截止仍保留停止保护。手动 stop 文件表示停止请求，不再把它直接标为已保存成功。PID 已退出或身份变化不向替代进程发信号。此版本的真实系统适配器未重新实网验收；测试使用临时 SQLite 和进程/时钟替身，不发送真实信号。

## 离线核对

`python3 -m unittest discover -s scripts/tests -p test_four_request_guard.py -v`

原缺陷固定用例先复现 `absolute_deadline != completed_saved`，修复后通过。完整 fixtures 同时覆盖保存失败、结果缺失、旧轮次、范围改变、越界、进程退出、观察读取错误和原证据保护。优化模式 `python3 -O` 下安全门禁也有效。

本轮未改网络、超时、并发、排序、UI 或数据库结构。批量原始失败根因及实际 TUN 转发仍须分别核验，不能凭观察器修好或离线回归升级为已解决。

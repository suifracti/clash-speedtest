# Mac 接手：源码服务＋Web 界面

迁移日期：2026-10-05。仓库：https://github.com/suifracti/clash-speedtest 。现有公开状态保持不变；默认分支仍为 main。原迁移分支 `codex/mac-migration-20261005` 的 `70f25a0947d5acf9ac8d35a1414a0a6a85b1c9e9` 已经用户确认推送。迁移前整合改用独立分支 `codex/mac-integration-20261005`；此整合分支目前只在 Windows 本地，需用户另行确认后才能推送。Mac 以最终整合提交为准，SHA 见交付记录及该分支的 `git rev-parse HEAD`。

## 来源与状态

源码来源是 Windows 主目录的实际最新内容：历史基线 `219b181ef7be45e3245ece5a10d692feb53a1a2c` 加上原有未提交、未跟踪源码及删除状态，已一次性记录于 `70f25a0`，包含 2026-10-04-history-scope-guard 检查点。只拉取历史基线会遗漏新版。前端负责人已确认没有另一份未整合副本。本次再核全部分支及工作树，主目录现存 290 个交付文件与 `70f25a0` 一致，没有新增业务代码。旧 README 提交 `5b916bd875e3737ba2b41a542d91fa671cbd838d` 中适用的运行范围／隔离数据说明已按当前源码改写保留；不整包套用旧提交，逐项处理见 [分支整合说明](docs/branch-integration-20261005.md)。

见 [来源与分支清点](docs/migration-source-map.md)、[既有验证说明](docs/windows-validation-20261005.md)。保留 Windows 原目录及全部 worktree，直到 Mac 拉取并验证；不重放旧整包补丁，不重做圆环或页面设计。

## 依赖

- Git；从上述 fork 克隆，不使用 README 旧的 `go install github.com/faceair/clash-speedtest@latest`。
- Go 1.25+，以 go.mod 为最低契约。Windows 已有通过证据使用 Go 1.27.0、windows/amd64，GOTOOLCHAIN=auto。
- Node.js：既有构建使用 v26.7.0、npm 11.19.0。frontend/package-lock.json 锁定依赖；用 npm ci，不升级依赖或重跑 npm audit fix。CI 原使用 Node 20，不把它视为本轮新版 Mac 验证。
- 当前 lockfile 的主要版本：Vue 3.5.42、Vite 5.4.21、TypeScript 5.6.3、vue-tsc 2.2.12、Vitest 2.1.9、jsdom 25.0.1。以锁文件完整依赖树为准。
- Mac 当前源码仍导入 Wails v2.16.0，即使运行 --web，也在编译期使用 Darwin Objective-C / Foundation / Cocoa / WebKit。需要 Xcode Command Line Tools 和 CGO；按需执行 `xcode-select --install`，使用 `CGO_ENABLED=1`。不要求安装 Wails CLI，不制作 .app 或 EXE。Mac 实机及 SDK 兼容性尚未验证。
- Mihomo v1.19.19、modernc.org/sqlite v1.59.0，由 go.mod/go.sum 获取，不依赖额外系统代理或 SQLite 服务。

## 拉取、构建与首次最小验证

整合分支获准推送后（在此之前不可将旧迁移分支当最终整合版本）：

```sh
git clone --branch codex/mac-integration-20261005 https://github.com/suifracti/clash-speedtest.git
cd clash-speedtest
git rev-parse HEAD             # 与 Windows 最终交付 SHA 比对
node scripts/prepare-legacy-assets.mjs
(cd frontend && npm ci && npm run build)
CGO_ENABLED=1 go build -o /tmp/clash-speedtest-migration-check .
```

前端生产资源不入库，必须先构建再编译 Go；`main.go` 嵌入 `frontend/dist`。另一个历史 gui 包也有嵌入资源依赖：prepare-legacy-assets 只从现有 Git 历史的固定基线恢复三个校验过的文件到忽略目录 `gui/web/dist`，不替换当前前端。完整 clone 已包含所需对象；浅克隆缺对象时先 `git fetch origin 219b181ef7be45e3245ece5a10d692feb53a1a2c`，或使用 Git 外列出的 legacy-web-assets.zip。来源、大小、SHA-256 见 [非 Git 文件清单](docs/migration-extras.md)。

首次使用隔离空数据目录验证启动，不直接覆盖迁入的个人数据：

```sh
mkdir -p "$HOME/Library/Application Support/ClashSpeedTest-MigrationCheck"
CGO_ENABLED=1 bash ./run-web.sh --no-auto-credentials \
  --data-dir "$HOME/Library/Application Support/ClashSpeedTest-MigrationCheck"
```

浏览器访问 http://127.0.0.1:18474/ ，确认页面与各入口可打开、无前端资源 404，再正常 Ctrl+C 退出。不启动全机场监测，不追加网络探测。服务需要终端进程存活，关闭终端或电脑睡眠不保证继续采样；本交付不安装后台服务。

## 个人数据、配置与开发

通过本说明的 go run 启动且未指定数据根时，Mac 默认 `~/Library/Application Support/ClashSpeedTest`；直接运行自编译二进制时，现有逻辑优先使用二进制旁的 data 目录。优先级是 --data-dir、CLASH_SPEEDTEST_DATA_DIR、默认路径；显式路径必须绝对。建议先将 private-state ZIP 解压到一个新目录，再把其 `data/` 子目录作为 --data-dir，核对历史及订阅读回后再决定长期位置。ZIP 不包含 OAuth 凭据，含订阅地址和节点密钥，只能私下传输，不能提交 GitHub。不要将同一 SQLite 文件放到同步盘由两机同时写入。

历史数据库已采用 schema 10，包括监测定义、原始样本、每日订阅用量快照。快照通过 SQLite backup API 包含已提交 WAL，不复制 WAL/SHM。浏览器当地的机场选择等 localStorage 不在服务器快照中；Mac 需重新选择。节点勾选及页面视角没有新增跨刷新永久保存语义。

当前监测定义保留，服务重开不会擅自续跑；部分旧定义因订阅刷新移除节点而 blocked。不要自动改冻结范围或恢复任务。用户已取消 Codex 定时巡检，本迁移不重建它。个人凭据在 Mac 的 Antigravity 重新登录；--no-auto-credentials 用于隔离检查，正常使用时可不带。不要复制 Windows 原始 OAuth token 或机器专属配置到仓库。

本轮整合只改说明，不改变数据库 schema 10、配置格式、源码依赖或非 Git 资源；原私有数据包和资源包的 SHA-256 继续有效。到 Mac 接手前，Windows 不再修改该整合基线；新的功能工作另建分支，不与 Mac 同时改同一交付版本。

开发时后端仍在 18474；另开终端：

```sh
cd frontend
SPEEDTEST_API_URL=http://127.0.0.1:18474 npm run dev -- --host 127.0.0.1
```

打开 Vite 的 http://127.0.0.1:5173/ 。前端生产源码修改后执行 npm run build，并在无活动检测时正常重开源码服务，以重新嵌入资源。不要沿用旧 PID。

## 可用功能与待办

已有 Windows 证据：多机场圆环、多机场／跨订阅节点选择；延迟、有界下载、手动服务与综合视角；监测创建／停止及历史；明确来源的 RTT 和 Google/GitHub 基础 HTTP 健康条、趋势与详情；名称规范、公告识别、地区／机场分组、可选跟随工具栏；凭据自动发现入口；每日订阅更新与日／周／月用量记录。用量来自机场累计计数差值，首次、失败、重置和跨期漏采保持未知／单列，不把本地探测流量冒充机场账单。

确定待修：最近延迟排序仍只看手动记录，未统一监测 RTT；指标排序也未保证完整筛选范围的数据已加载。服务基础 HTTP 的 2xx/3xx 成功不代表严格服务规则、平台解锁或模型请求通过。

未验：Mac 实机；真实存储保存失败后的 UI 重试；完整鉴权生命周期；每日用量午夜／多日／计数重置实机；24 小时服务常驻。旧长测只有约 76 分钟、16 轮的证据。Windows 外部进程终止来源未知，不把它归因为应用崩溃或声称已解决。

Mac 最小业务验证只补平台缺口：在确认私有数据路径后读取既有历史，核对来源／范围；如需要新增真实请求，只用 1–2 个代表节点、短超时及下载最多 1 MiB。监测创建／重开全机场测试须由用户另行确定范围。无源码变更时复用 Windows 既有组件证据，不先跑全量测试或故障矩阵。

原 CI 的桌面矩阵只补了历史嵌入资源准备，尚未运行；本候选分支不触发其现有 push 过滤器。旧 tag／GoReleaser 发布流程未适配本次源码交付，不作为可用发布入口。本轮不创建 PR、tag、EXE、.app 或发布。

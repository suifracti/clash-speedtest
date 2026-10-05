# 迁移必需的非 Git 文件

编译产物、个人数据和凭据不放入候选树。以下区分「构建必须」与「继续个人状态时必须」。完整本地传输清单为 Git 外 `artifacts/migration-20261005/private-transfer-manifest.json`，包含每个私有文件的用途、大小和 SHA-256；此目录未推送。

## 构建必须：历史 gui 包嵌入依赖

这三个文件是旧包的编译期 go:embed 依赖，不是当前 Vue 前端的源码或产物。总计 772964 字节。从已公开仓库固定基线 219b181ef7be45e3245ece5a10d692feb53a1a2c 的三个 Git blob 获取；`node scripts/prepare-legacy-assets.mjs` 校验大小和 SHA-256，只填充忽略目录，不套回旧源码。也可私下复制 legacy-web-assets.zip（218192 字节），校验后在仓库根解压。

| 文件 | 字节 | SHA-256 |
| --- | ---: | --- |
| gui/web/dist/index.html | 218151 | 7c82e81d521a6c06a395f53136a27d2527a1fb3ae941079e58f4ebcd077337bc |
| gui/web/dist/tailwind.js | 407279 | 176e894661aa9cdc9a5cba6c720044cbbf7b8bd80d1c9a142a7c24b1b6c50d15 |
| gui/web/dist/vue.global.prod.js | 147534 | 4963101441ded7e420c05665e7c616b2f2e3851c99e1cf8af84d29d6f10e77da |

ZIP SHA-256：ecda39b41efc27a73660febcbb646141a05046302e922f4b6efcb8e55a856cf4。完整 clone 能从 Git 历史恢复，所以不强制私下传输 ZIP；源码压缩包或浅克隆则必须补齐这项依赖。不能只忽略目录后声称任意源码解压都可直接编译。

当前 frontend/dist 在 Mac 用 package-lock.json 和 npm ci / npm run build 生成，Windows dist、node_modules 和 Go 临时二进制都不传。

## 继续个人状态／原始证据

- private-state-20261005.zip：2813948 字节，22 文件、解压数据总计 15455419 字节。包含个人设置、订阅登记、节点配置缓存、SQLite 一致快照和 legacy 记录。继续历史和订阅状态需要它；空数据启动不需要。含订阅密钥，限私下拷贝到新数据目录，不能上传 GitHub。归档及每文件 SHA-256 在私有 manifest 中。
- private-evidence-20261005.zip：1098839 字节，24 文件。原始 QA JSON、浏览器动作记录和既有截图；包含个人节点信息，只用于私下验收复核，不是运行依赖。Git 内仅有脱敏验证说明，其它历史日志仍保留在 Windows。
- closeout-readme.bundle：1276 字节，保留旧 README 候选 SHA 5b916bd875e3737ba2b41a542d91fa671cbd838d，main 历史为前置对象。只需人审；不自动整合。先 git bundle verify，再 fetch 到单独候选 ref，不能覆盖当前分支。
- closeout-readme-5b916bd.patch：1805 字节，仅供查看上述单提交差异；不应与 bundle 同时应用。
- OAuth token 不包装、不复制入 Git；Mac 重新登录 Antigravity。Windows 原 token 及旧根目录订阅文件保留，私有 manifest 记录其大小／SHA 和不包装的理由；旧根文件不属于当前显式 data 根的运行依赖。

私有拷贝后用 `shasum -a 256 文件名` 对照 manifest。SQLite 快照已合并提交过的 WAL，不带 WAL/SHM。不要将原在线数据库直接复制成不一致备份。

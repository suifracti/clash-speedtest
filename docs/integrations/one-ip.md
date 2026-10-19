# One IP 功能对照与接入（2026-10-19）

参考 [zhihui-hu/one-ip](https://github.com/zhihui-hu/one-ip)，核对版本 `3d6531437cd1bf5e03711339c7d96b674d3e1e16`。参考入口为 `src/views/ai/platforms.ts`、`src/views/ai/probe.ts`、`src/views/link/targets.json`。上游 AGPL-3.0；本次只核对公开目标与功能含义，用 CST 现有 Go 检测链独立实现，没有复制上游程序或嵌入其前端。

## 接入范围

- 14 个手动可选的轻量资源检测：微信、QQ、Telegram、哔哩哔哩、抖音、小红书；ChatGPT、Claude、Grok、Gemini、Perplexity、DeepSeek、通义千问、Kimi。
- 全部请求通过当前所选节点的既有独立客户端。没有调用 one-ip Worker 代测，不把 Worker 的出口作为节点出口，不带账号、Cookie、模型凭据或发起对话。
- 单次 GET；200 时校验图像或 robots 内容，HTML 登录/错误页与空响应不算资源成功。拒绝、限流、挑战、跳转、超时分别保留。资源可达不代表账号、消息或 AI 对话可用。资源地址由本站固定目录维护，上游更改路径可能使单个资源不可用。
- 现有网页、地区、账号检测保留；资源检测是另一种轻量观察。新增资源不自动加入已有定时巡检。

## 现有实现对照

| 项目 | one-ip | CST 处理 |
| --- | --- | --- |
| AI 基础访问 | favicon / robots；部分站点使用 trace；多数 no-cors 资源读不到状态码 | 保留既有网页/地区规则，同时新增单独资源选项；读取实际 HTTP 状态，不能把 403 当正常资源 |
| 微信 | 官方静态图标 | 采用同一公开资源，校验实际图像；不声称验证发消息 |
| ChatGPT / Claude / Grok / Gemini | 不代表登录或对话成功 | 保持同一边界；修复已有 ChatGPT / Claude / Gemini 在读取 403/503 正文前返回，漏掉挑战标记的问题 |
| HTTP 耗时 | 浏览器 fetch 耗时；与跨域/CORS相关 | 保留首字节响应计时，不改成浏览器 fetch 或 ICMP；HTTP 拒绝仍可有有效延迟 |
| 官方服务状态 | 平台全局状态聚合 | 暂不放入节点可用结论，因为它不测当前节点 |
| 指纹 / WebRTC / 验证码 | 浏览器环境，需要浏览器操作 | 本轮不引入节点检测；没有当前节点浏览器环境的等价证据 |

## 紧凑延迟区

首页显示 Cloudflare、Google、GitHub、Apple 四组竖条。每格一轮，左旧右新，悬停预览、点击固定并打开历史；站名和数值也能打开详情。默认收起重复的响应数、时间和说明，数值悬停可读。Microsoft 与 Firefox 从新建目标/活动服务目录退出，旧历史与旧六站冻结监测仍可追溯。

新手动综合延迟采用 `multi://latency-four-v1`，新监测采用 `latency_four_v1`；旧 `multi://latency-v1` / `latency_six_v1` 仍展开原六站，避免改写旧范围。测试计划请求数按四站实际数量计算。

## 验证范围

模拟响应验证资源内容、跳转、403 与挑战分类；检查四站新范围和六站旧范围，运行相关前端/Go 测试与构建。没有调用真实账号、进行整机场测速或声称所有外部目标均已实网可用。原 IPPure Cloudflare 系数拒绝问题仍保留，不由本次接入自动解决。

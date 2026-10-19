package publicservice

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

const (
	DefaultTimeout          = 10 * time.Second
	MaximumTimeout          = 30 * time.Second
	MaximumResponseBytes    = 64 * 1024
	MaximumWebDocumentBytes = 2 * 1024 * 1024
	RuleVersion             = 1
)

// Rule is an immutable entry in the public-service catalog. The catalog is
// deliberately code-owned; callers cannot supply an arbitrary target URL.
type Rule struct {
	ServiceID        string `json:"service_id"`
	SupersededBy     string `json:"superseded_by,omitempty"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	Region           string `json:"region,omitempty"`
	ResultKind       string `json:"result_kind"`
	Description      string `json:"description"`
	BatchDefault     bool   `json:"batch_default,omitempty"`
	RuleVersion      int    `json:"rule_version"`
	TargetURL        string `json:"target_url"`
	Method           string `json:"method"`
	SuccessCriterion string `json:"success_criterion"`
	RedirectPolicy   string `json:"redirect_policy"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	MaximumBodyBytes int    `json:"maximum_body_bytes"`
	Accept           string `json:"accept,omitempty"`
	APIVersionHeader string `json:"api_version_header,omitempty"`
}

var catalog = []Rule{
	{
		ServiceID: "cloudflare_204", Name: "Cloudflare 204 连通性", RuleVersion: RuleVersion,
		Category: "基础网络", Region: "全球", ResultKind: "connectivity", Description: "轻量确认节点能否访问 Cloudflare 连通性端点。", BatchDefault: true,
		TargetURL: "https://cp.cloudflare.com/generate_204", Method: http.MethodGet,
		SuccessCriterion: "HTTP status exactly 204; indicates only this target matched its connectivity rule",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "google_204", Name: "Google 204 连通性", RuleVersion: RuleVersion,
		Category: "基础网络", Region: "全球", ResultKind: "connectivity", Description: "轻量确认节点能否访问 Google 连通性端点。", BatchDefault: true,
		TargetURL: "https://www.google.com/generate_204", Method: http.MethodGet,
		SuccessCriterion: "HTTP status exactly 204; indicates only this target matched its connectivity rule",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "github_api_root", Name: "GitHub 公共 API 根端点", RuleVersion: RuleVersion,
		Category: "开发服务", Region: "全球", ResultKind: "connectivity", Description: "验证 GitHub 公共 API 根索引，不使用账号。", BatchDefault: true,
		TargetURL: "https://api.github.com/", Method: http.MethodGet,
		SuccessCriterion: "HTTP status exactly 200, JSON response, and root-index fields current_user_url and repository_url are non-empty",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
		Accept: "application/vnd.github+json", APIVersionHeader: "2026-03-10",
	},
	{
		ServiceID: "antigravity", Name: "Google Antigravity 可用性", RuleVersion: 5,
		Category: "AI 服务", Region: "全球", ResultKind: "account_availability", Description: "Google Antigravity (反重力) AI 辅助模型可用性探测。使用已绑定 Google 凭据发起极短模型请求，区分账号、地区与网络故障。",
		TargetURL: antigravityBase + "streamGenerateContent?alt=sse", Method: http.MethodPost,
		SuccessCriterion: "使用已绑定 Google 凭据，经所选节点查询项目和模型并发起短请求；仅实际模型输出判定可用。最多 3 个模型相关请求，另有 1 次不带凭据的 Cloudflare 出口观察；共享总超时，可能消耗少量账号额度。出口观察不代表 Google 实际看到的 IP",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "cloudflare_trace", Name: "出口 IP 与 Cloudflare 机房", RuleVersion: 2,
		Category: "出口与 IP", Region: "全球", ResultKind: "exit_profile", Description: "读取 Cloudflare trace，显示出口 IP、国家和接入机房。", BatchDefault: true,
		TargetURL: "https://www.cloudflare.com/cdn-cgi/trace", Method: http.MethodGet,
		SuccessCriterion: "HTTP 200 且 trace 至少包含 ip 与 loc；结果用于出口画像，不代表 IP 纯净度",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "exit_ipv4", Name: "IPv4 出口地址", RuleVersion: 1,
		Category: "出口与 IP", Region: "全球", ResultKind: "exit_profile", Description: "通过仅支持 IPv4 的公开端点读取节点出口；不代表其它网站看到同一 IP。",
		TargetURL: "https://api.ipify.org?format=json", Method: http.MethodGet,
		SuccessCriterion: "响应含有效 IPv4 地址；未取得结果不能直接判定节点不可用",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: 1024, Accept: "application/json",
	},
	{
		ServiceID: "exit_ipv6", Name: "IPv6 出口地址", RuleVersion: 1,
		Category: "出口与 IP", Region: "全球", ResultKind: "exit_profile", Description: "通过仅支持 IPv6 的公开端点读取节点出口；失败可能是此节点或上游不支持 IPv6。",
		TargetURL: "https://api6.ipify.org?format=json", Method: http.MethodGet,
		SuccessCriterion: "响应含有效 IPv6 地址；失败只能说明本次未确认 IPv6 出口",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: 1024, Accept: "application/json",
	},
	{
		ServiceID: "ping0_ip_quality", Name: "Ping0 IP 风险画像", RuleVersion: 2,
		Category: "出口与 IP", Region: "全球", ResultKind: "ip_quality", Description: "读取 Ping0 的出口、ASN、IDC 判断与风险分。", BatchDefault: true,
		TargetURL: "https://ping0.cc/geo/json", Method: http.MethodGet,
		SuccessCriterion: "HTTP 200 且 JSON 包含出口 IP；风险分按 Ping0 原值独立展示",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: 8 * 1024, Accept: "application/json",
	},
	{
		ServiceID: "ippure_ip_quality", Name: "IPPure 画像与风险", RuleVersion: 7,
		Category: "出口与 IP", Region: "全球", ResultKind: "ip_quality", Description: "同一节点查询IPPure IPv4风险、Cloudflare请求环境系数及ASN人机流量比；各项独立保留结果与失败状态。",
		TargetURL: "https://my.ippure.com/v1/info", Method: http.MethodGet,
		SuccessCriterion: "官方API须返回当前节点的有效公开IPv4及0–100整数风险分；Cloudflare系数、人机比独立校验与保存，不混用分数",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: 16 * 1024, Accept: "application/json",
	},
	{
		ServiceID: "apple_captive", Name: "Apple 网络连通性", RuleVersion: 1,
		Category: "基础网络", Region: "全球", ResultKind: "connectivity", Description: "验证 Apple captive portal 的固定 Success 响应。",
		TargetURL: "http://captive.apple.com/hotspot-detect.html", Method: http.MethodGet,
		SuccessCriterion: "HTTP 200 且响应包含 Success",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "microsoft_connect", Name: "Microsoft NCSI 连通性", RuleVersion: 1,
		Category: "基础网络", Region: "全球", ResultKind: "connectivity", Description: "验证 Windows 网络状态检测端点。",
		TargetURL: "http://www.msftconnecttest.com/connecttest.txt", Method: http.MethodGet,
		SuccessCriterion: "HTTP 200 且响应包含 Microsoft Connect Test",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "firefox_portal", Name: "Firefox 网络连通性", RuleVersion: 1,
		Category: "基础网络", Region: "全球", ResultKind: "connectivity", Description: "验证 Firefox captive portal 的固定 success 响应。",
		TargetURL: "http://detectportal.firefox.com/success.txt", Method: http.MethodGet,
		SuccessCriterion: "HTTP 200 且响应包含 success",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "grok_web", Name: "Grok 网页与 Cloudflare 挑战", RuleVersion: 2,
		Category: "AI 服务", Region: "全球", ResultKind: "web_access", Description: "区分网页可达、Cloudflare 挑战和 HTTP 拒绝；不使用 Grok 账号。", BatchDefault: true,
		TargetURL: "https://grok.com/", Method: http.MethodGet,
		SuccessCriterion: "收到网页响应；cf-mitigated=challenge 单独记为 Cloudflare 挑战，不误判为断网",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "chatgpt_web", Name: "ChatGPT 网页与挑战", RuleVersion: 3,
		Category: "AI 服务", Region: "全球", ResultKind: "web_access", Description: "区分网页可达、Cloudflare 挑战和 HTTP 拒绝；不登录账号。",
		TargetURL: "https://chatgpt.com/", Method: http.MethodGet,
		SuccessCriterion: "收到网页响应；Cloudflare 挑战单独展示，网页可达不代表账号可用",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "netflix_unlock", Name: "Netflix 片目与地区检查", RuleVersion: 3,
		Category: "流媒体解锁", Region: "全球", ResultKind: "streaming_unlock", Description: "自动经被测节点查询目标IPv4，保留原Host与证书校验；比较两部片目及明确国家限制，不代表播放。",
		TargetURL: "https://www.netflix.com/title/81280792 + /title/70143836", Method: http.MethodGet,
		SuccessCriterion: "经同一节点DoH查询一次并固定本轮IPv4；每片目最多1MiB，保留TTL、原域名与证书校验；片目可访问/地区提示分开，不读取播放分片",
		RedirectPolicy:   "follow_same_host", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: 1024 * 1024,
	},
	{
		ServiceID: "tver_jp", Name: "TVer 日本网页可达性", RuleVersion: 2,
		Category: "日本服务", Region: "日本", ResultKind: "regional_reachability", Description: "检测 TVer 网页响应；不把首页可达等同于可播放。",
		TargetURL: "https://tver.jp/", Method: http.MethodGet, SuccessCriterion: "最多跟随3次同服务HTTPS跳转，最终2xx且无挑战标记才记网页可达；不证明播放解锁",
		RedirectPolicy: "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "abema_jp", Name: "ABEMA 日本网页可达性", RuleVersion: 3,
		Category: "日本服务", Region: "日本", ResultKind: "regional_reachability", Description: "自动经被测节点解析IPv4并核对网页；明确错误页单列，不代表播放。",
		TargetURL: "https://abema.tv/", Method: http.MethodGet, SuccessCriterion: "经同一节点查询一次IPv4并固定本轮；原Host/SNI校验，最多跟随3次同服务HTTPS跳转；2xx无挑战及明确错误页才记网页可达，不证明播放",
		RedirectPolicy: "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "nhk_plus_jp", Name: "NHK ONE（原 NHK Plus）网页入口", RuleVersion: 2,
		Category: "日本服务", Region: "日本", ResultKind: "regional_reachability", Description: "跟随原 NHK Plus 的官方新入口；不使用账号，不代表播放解锁。",
		TargetURL: "https://plus.nhk.jp/", Method: http.MethodGet, SuccessCriterion: "最多跟随3次同服务HTTPS跳转，最终2xx且无挑战标记才记网页可达；不证明播放解锁",
		RedirectPolicy: "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "unext_jp", Name: "U-NEXT 日本网页可达性", RuleVersion: 2,
		Category: "日本服务", Region: "日本", ResultKind: "regional_reachability", Description: "检测 U-NEXT 网页响应；不使用账号。",
		TargetURL: "https://www.video.unext.jp/", Method: http.MethodGet, SuccessCriterion: "最多跟随3次同服务HTTPS跳转，最终2xx且无挑战标记才记网页可达；不证明播放解锁",
		RedirectPolicy: "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "bbc_iplayer_uk", Name: "BBC iPlayer 英国网页可达性", RuleVersion: 2,
		Category: "其他地区", Region: "英国", ResultKind: "regional_reachability", Description: "检测 BBC iPlayer 网页响应；不把首页可达等同于播放解锁。",
		TargetURL: "https://www.bbc.co.uk/iplayer", Method: http.MethodGet, SuccessCriterion: "最多跟随3次同服务HTTPS跳转，最终2xx且无挑战标记才记网页可达；不证明播放解锁",
		RedirectPolicy: "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "naver_kr", Name: "Naver 韩国网页可达性", RuleVersion: 2,
		Category: "其他地区", Region: "韩国", ResultKind: "regional_reachability", Description: "检测韩国 Naver 门户网页响应。",
		TargetURL: "https://www.naver.com/", Method: http.MethodGet, SuccessCriterion: "收到 2xx/3xx 网页响应；仅表示网页可达",
		RedirectPolicy: "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "abema_unlock", Name: "ABEMA 日本地区判定", Category: "日本解锁", Region: "日本", ResultKind: "streaming_unlock", RuleVersion: 3,
		TargetURL: "https://api.abema.io/v1/ip/check?device=android", Method: http.MethodGet,
		Description:      "自动经被测节点解析IPv4，保留域名与证书校验；地区结论与anonymous_ip拒绝分开，不代表播放。",
		SuccessCriterion: "经同一节点DoH一次并固定本轮IPv4；isoCountryCode=JP判定日本入口，其他地区为海外档位；403且JSON message=anonymous_ip仅判服务拒绝匿名/代理IP，不推断节点故障或地区封锁",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	streamingRule("fod_unlock", "FOD 日本地区判定", "日本", "https://geocontrol1.stream.ne.jp/fod-geo/check.xml?time=1624504256", "读取 FOD 地区控制端点，不下载视频。", "XML FLAG TYPE=true 判定地区允许"),
	streamingRule("hulu_jp_unlock", "Hulu Japan 入口限制检查", "日本", "https://id.hulu.jp/", "识别明确的 restrict 跳转；首页可达不代表地区或播放解锁。", "明确 restrict 跳转记为受限；200 仅记网页可达；403 不推断为地区封锁"),
	{
		ServiceID: "youtube_premium", Name: "YouTube Premium 地区判定", Category: "流媒体解锁", Region: "全球", ResultKind: "streaming_unlock", RuleVersion: 3,
		TargetURL: "https://www.youtube.com/premium", Method: http.MethodGet,
		Description:      "读取公开地区及订阅方案；仅此规则每请求最多读取2MiB网页，不验证账号或播放。",
		SuccessCriterion: "可识别地区且页面实际订阅方案及按钮存在，才记公开方案判据通过；广告文案不算；不验证购买或播放",
		RedirectPolicy:   "probe_specific", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumWebDocumentBytes,
	},
	streamingRule("prime_video", "Prime Video 地区判定", "全球", "https://www.primevideo.com/", "读取 Prime Video 首页公开的 territory 与限制标记。", "currentTerritory 存在且 isServiceRestricted 明确为 false 才通过地区判据，不代表播放"),
	{
		ServiceID: "cloudflare_doh", Name: "Cloudflare DNS over HTTPS", Category: "基础网络", Region: "全球",
		ResultKind: "connectivity", Description: "验证 DNS 响应成功且包含解析结果。", RuleVersion: 3,
		TargetURL: "https://cloudflare-dns.com/dns-query?name=cloudflare.com&type=A", Method: http.MethodGet,
		SuccessCriterion: "HTTP 200、DNS Status=0 且 Answer 非空", RedirectPolicy: "do_not_follow",
		TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes, Accept: "application/dns-json",
	},

	webRule("fastly_web", "Fastly 边缘网络", "基础网络", "全球", "https://www.fastly.com/", "确认节点能否到达 Fastly 官网入口。"),
	webRule("aws_web", "AWS / CloudFront 入口", "基础网络", "全球", "https://aws.amazon.com/", "确认节点能否到达 AWS 全球入口。"),
	{
		ServiceID: "xiaomi_204", Name: "小米国内网络对照", Category: "基础网络", Region: "中国", ResultKind: "connectivity", RuleVersion: 4,
		TargetURL: "https://connect.rom.miui.com/generate_204", Method: http.MethodGet,
		Description:      "国内204端点对照；必须实际返回204，普通网页或跳转不算固定响应通过。",
		SuccessCriterion: "HTTP 204，且不跟随重定向", RedirectPolicy: "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},

	webRule("github_web", "GitHub 网页", "开发服务", "全球", "https://github.com/", "检测 GitHub 网页入口。"),
	{
		ServiceID: "github_codeload", Name: "GitHub Codeload", Category: "开发服务", Region: "全球", ResultKind: "endpoint_reachability", RuleVersion: 4,
		TargetURL: "https://codeload.github.com/octocat/Hello-World/zip/refs/heads/master", Method: http.MethodHead,
		Description:      "只用HEAD确认归档下载入口响应，不读取归档正文。",
		SuccessCriterion: "最多跟随3次同服务HTTPS跳转；HEAD最终2xx且无明确挑战响应头，只记入口可达，不下载正文",
		RedirectPolicy:   "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: 0,
	},
	webRule("gitlab_web", "GitLab 网页", "开发服务", "全球", "https://gitlab.com/", "检测 GitLab 网页入口。"),
	webRule("npm_registry", "npm Registry", "开发服务", "全球", "https://registry.npmjs.org/-/ping", "检测 npm 官方 Registry 轻量 ping 端点。"),
	webRule("pypi_json", "PyPI JSON API", "开发服务", "全球", "https://pypi.org/pypi/pip/json", "检测 PyPI 包索引 JSON 入口。"),
	webRule("docker_hub", "Docker Hub", "开发服务", "全球", "https://hub.docker.com/", "检测 Docker Hub 网页入口。"),
	webRule("docker_registry", "Docker Registry", "开发服务", "全球", "https://registry-1.docker.io/v2/", "检测 Docker Registry 鉴权入口；401 代表服务已到达。"),

	webRule("google_search", "Google Search", "AI 与平台", "全球", "https://www.google.com/search?q=connectivity", "检测 Google 搜索入口；仅说明网页响应。"),
	webRule("youtube_web", "YouTube 网页", "AI 与平台", "全球", "https://www.youtube.com/", "检测 YouTube 网页入口。"),
	webRule("gemini_web", "Google Gemini 网页", "AI 与平台", "全球", "https://gemini.google.com/", "检测 Gemini 网页入口及可能的挑战/跳转。"),
	webRule("claude_web", "Claude 网页", "AI 与平台", "全球", "https://claude.ai/", "检测 Claude 网页入口及可能的挑战。"),
	webRule("xai_web", "xAI 官网", "AI 与平台", "全球", "https://x.ai/", "与 Grok 网页分开检测 xAI 官网入口。"),
	webRule("xai_api", "xAI API 鉴权入口", "AI 与平台", "全球", "https://api.x.ai/v1/models", "未携带凭据；401 代表 API 网络与鉴权入口可达。"),
	webRule("x_web", "X 网页", "AI 与平台", "全球", "https://x.com/", "检测 X 网页入口。"),
	webRule("discord_web", "Discord 网页", "AI 与平台", "全球", "https://discord.com/", "检测 Discord 网页入口。"),
	webRule("steam_store", "Steam 商店", "AI 与平台", "全球", "https://store.steampowered.com/", "检测 Steam 商店入口。"),
	webRule("reddit_web", "Reddit 网页", "AI 与平台", "全球", "https://www.reddit.com/", "检测 Reddit 网页入口及风控拒绝。"),

	webRule("disneyplus_web", "Disney+ 网页入口", "流媒体入口", "全球", "https://www.disneyplus.com/", "仅检测网页入口；不声称播放解锁。"),
	webRule("spotify_web", "Spotify 网页入口", "流媒体入口", "全球", "https://open.spotify.com/", "仅检测网页入口；不声称账号或曲库解锁。"),
	webRule("appletv_web", "Apple TV+ 网页入口", "流媒体入口", "全球", "https://tv.apple.com/", "仅检测网页入口；不声称播放解锁。"),
	webRule("hulu_us_web", "Hulu US 网页入口", "美国服务", "美国", "https://www.hulu.com/welcome", "仅检测网页入口；不使用账号。"),
	webRule("max_web", "Max 网页入口", "美国服务", "美国", "https://www.max.com/", "仅检测网页入口；不声称播放解锁。"),
	{
		ServiceID: "peacock_web", Name: "Peacock 网页入口", Category: "美国服务", Region: "美国", ResultKind: "regional_reachability", RuleVersion: 4,
		TargetURL: "https://www.peacocktv.com/", Method: http.MethodGet,
		Description:      "检测网页入口及明确的地区不提供服务提示；不声称播放解锁。",
		SuccessCriterion: "最多跟随3次同服务HTTPS跳转；地区提示页须有明确正文才记受限，普通2xx无挑战只记网页可达，不验证播放",
		RedirectPolicy:   "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	webRule("paramount_web", "Paramount+ 网页入口", "美国服务", "美国", "https://www.paramountplus.com/", "仅检测网页入口；不声称播放解锁。"),

	webRule("amazon_jp", "Amazon.co.jp", "日本服务", "日本", "https://www.amazon.co.jp/", "检测日本 Amazon 网页入口；不代表可下单。"),
	webRule("yahoo_jp", "Yahoo! Japan", "日本服务", "日本", "https://www.yahoo.co.jp/", "检测 Yahoo! Japan 门户入口。"),
	webRule("rakuten_jp", "乐天市场", "日本服务", "日本", "https://www.rakuten.co.jp/", "检测日本乐天市场入口；不代表优惠或支付可用。"),
	webRule("pixiv_web", "Pixiv", "日本服务", "日本", "https://www.pixiv.net/", "检测 Pixiv 网页入口及挑战页。"),
	webRule("niconico_web", "Niconico", "日本服务", "日本", "https://www.nicovideo.jp/", "检测 Niconico 网页入口；不声称直播/视频播放解锁。"),
	webRule("danime_web", "d Anime Store", "日本服务", "日本", "https://animestore.docomo.ne.jp/", "检测 d Anime Store 入口；不使用账号。"),
	webRule("fod_web", "FOD 网页入口", "日本服务", "日本", "https://fod.fujitv.co.jp/", "网页入口对照；真实地区判定请使用 FOD 地区检测。"),
	webRule("hulu_jp_web", "Hulu Japan 网页入口", "日本服务", "日本", "https://www.hulu.jp/", "网页入口对照；真实地区判定请使用 Hulu Japan 地区检测。"),
	webRule("lemino_web", "Lemino", "日本服务", "日本", "https://lemino.docomo.ne.jp/", "检测 Lemino 网页入口。"),
	webRule("radiko_web", "radiko", "日本服务", "日本", "https://radiko.jp/", "检测日本广播服务网页入口；不声称音频播放解锁。"),
	webRule("dmm_tv_web", "DMM TV", "日本服务", "日本", "https://tv.dmm.com/vod/", "检测 DMM TV 网页入口。"),
	webRule("nintendo_jp", "Nintendo 日本商店", "日本服务", "日本", "https://store-jp.nintendo.com/", "检测 Nintendo 日本商店入口；不代表账号或支付地区。"),
	webRule("line_music_jp", "LINE MUSIC", "日本服务", "日本", "https://music.line.me/webapp/today", "检测 LINE MUSIC 网页入口；不声称曲库解锁。"),
	webRule("etax_jp", "日本 e-Tax", "日本服务", "日本", "https://www.e-tax.nta.go.jp/", "只检测公共入口，不尝试登录或访问个人资料。"),

	webRule("coupang_kr", "Coupang 韩国", "其他地区", "韩国", "https://www.coupang.com/", "检测韩国 Coupang 网页入口。"),
	webRule("gov_uk", "GOV.UK", "其他地区", "英国", "https://www.gov.uk/", "英国公共服务网络锚点。"),
	webRule("itv_uk", "ITVX 英国", "其他地区", "英国", "https://www.itv.com/watch", "检测 ITVX 网页入口；不声称播放解锁。"),
	webRule("spiegel_de", "DER SPIEGEL 德国", "其他地区", "德国", "https://www.spiegel.de/", "德国媒体网络锚点。"),
	webRule("bahn_de", "Deutsche Bahn", "其他地区", "德国", "https://www.bahn.de/", "德国铁路网络锚点。"),
	webRule("gov_sg", "Singapore Government", "其他地区", "新加坡", "https://www.gov.sg/", "新加坡政府网络锚点。"),
	webRule("straits_sg", "The Straits Times", "其他地区", "新加坡", "https://www.straitstimes.com/", "新加坡媒体网络锚点。"),
	webRule("abc_au", "ABC Australia", "其他地区", "澳大利亚", "https://www.abc.net.au/", "澳大利亚公共媒体网络锚点。"),
	webRule("ptt_tw", "PTT 台湾", "其他地区", "台湾", "https://www.ptt.cc/bbs/index.html", "台湾社区网络锚点。"),
	webRule("yahoo_tw", "Yahoo 台湾", "其他地区", "台湾", "https://tw.yahoo.com/", "台湾门户网络锚点。"),
	webRule("mytv_hk", "myTV SUPER 香港", "其他地区", "香港", "https://www.mytvsuper.com/", "检测网页入口；不声称播放解锁。"),
	webRule("viu_web", "Viu", "其他地区", "香港/亚洲", "https://www.viu.com/", "检测网页入口；不声称播放解锁。"),
	webRule("nowe_hk", "Now E 香港", "其他地区", "香港", "https://www.nowe.com/", "检测网页入口；不声称播放解锁。"),
	webRule("kktv_tw", "KKTV 台湾（已结束营运）", "其他地区", "台湾", "https://www.kktv.me/sunset", "官方已结束营运，保留目录和旧历史，不再将停运页作为节点可用性检测。"),
	ipokRule("ipok_ipv4", "IPv4"),
	ipokRule("ipok_ipv6", "IPv6"),
	{
		ServiceID: netflixIPv4Service, SupersededBy: "netflix_unlock", Name: "Netflix 片目检查（经节点 DNS · IPv4）", RuleVersion: 2,
		Category: "流媒体解锁", Region: "全球", ResultKind: "streaming_unlock",
		Description: "可选：经同一被测节点向Cloudflare查询一次IPv4，再检查片目；额外DNS响应最多16KiB，原域名和证书校验保留，不代表播放。",
		TargetURL:   "https://www.netflix.com/title/81280792 + /title/70143836", Method: http.MethodGet,
		SuccessCriterion: "单次经节点DoH A查询，本轮固定同一IPv4并记录TTL，不跨轮缓存；原Host/SNI校验两部片目，仅同主机HTTPS跳转；不同于节点默认解析，不混为同一方法，每片目最多1MiB；识别明确国家限制提示，不验证播放",
		RedirectPolicy:   "follow_same_host", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: 1024 * 1024,
	},
}

func webRule(id, name, category, region, target, description string) Rule {
	resultKind := "regional_reachability"
	if region == "全球" || category == "基础网络" || category == "开发服务" || category == "AI 与平台" {
		resultKind = "web_access"
	}
	if category == "开发服务" || category == "基础网络" {
		resultKind = "endpoint_reachability"
	}
	return Rule{
		ServiceID: id, Name: name, Category: category, Region: region, ResultKind: resultKind, Description: description,
		RuleVersion: 4, TargetURL: target, Method: http.MethodGet,
		SuccessCriterion: "最多跟随3次同服务HTTPS跳转，最终2xx且无挑战标记才记入口可达；跨服务跳转单列；不等同于账号、支付或播放解锁",
		RedirectPolicy:   "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	}
}

func streamingRule(id, name, region, target, description, criterion string) Rule {
	category := "流媒体解锁"
	if region == "日本" {
		category = "日本解锁"
	}
	return Rule{
		ServiceID: id, Name: name, Category: category, Region: region, ResultKind: "streaming_unlock", Description: description,
		RuleVersion: 2, TargetURL: target, Method: http.MethodGet, SuccessCriterion: criterion,
		RedirectPolicy: "probe_specific", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	}
}

func ping0FamilyRule(family string) Rule {
	id, host := "ping0_ipv4", "ipv4.ping0.cc"
	if family == "IPv6" {
		id, host = "ping0_ipv6", "ipv6.ping0.cc"
	}
	return Rule{ServiceID: id, Name: "Ping0 " + family + " 风险画像", RuleVersion: 1, Category: "出口与 IP", Region: "全球", ResultKind: "ip_quality", Description: "独立查询所选节点的 " + family + " 出口与 Ping0 风险分，不借用另一地址族的结果。", TargetURL: "https://" + host + "/geo/json", Method: http.MethodGet, SuccessCriterion: "HTTP 200，JSON出口必须属于指定地址族；风险分按源站原值展示，缺失不记0", RedirectPolicy: "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: 8 * 1024, Accept: "application/json"}
}

func Catalog() []Rule {
	result := make([]Rule, 0, len(catalog))
	for _, rule := range catalog {
		// Keep old snapshots resolvable through RuleFor, outside new plans.
		if strings.HasPrefix(rule.ServiceID, "ipok_") || rule.ServiceID == "ping0_ip_quality" || rule.ServiceID == "microsoft_connect" || rule.ServiceID == "firefox_portal" {
			continue
		}
		result = append(result, rule)
	}
	return append(append(result, ping0FamilyRule("IPv4"), ping0FamilyRule("IPv6")), resourceRules()...)
}

func RuleFor(serviceID string) (Rule, bool) {
	serviceID = strings.TrimSpace(serviceID)
	if serviceID == "ping0_ipv4" {
		return ping0FamilyRule("IPv4"), true
	}
	if serviceID == "ping0_ipv6" {
		return ping0FamilyRule("IPv6"), true
	}
	for _, rule := range append(append([]Rule{}, catalog...), resourceRules()...) {
		if rule.ServiceID == serviceID {
			return rule, true
		}
	}
	return Rule{}, false
}

type Result struct {
	Model        string            `json:"model,omitempty"`
	RequestCount int               `json:"request_count,omitempty"`
	Summary      string            `json:"summary,omitempty"`
	Details      map[string]string `json:"details,omitempty"`
	Outcome      string            `json:"outcome"`
	HTTPStatus   *int              `json:"http_status,omitempty"`
	BytesRead    int64             `json:"bytes_read"`
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   time.Time         `json:"finished_at"`
	DurationMs   int64             `json:"duration_ms"`
	FailurePhase string            `json:"failure_phase,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
}

type ClientFactory func(node monitor.MonitoredNode, timeout time.Duration) (*http.Client, error)

// Checker issues a bounded probe through the selected node's isolated proxy
// path. Most rules make one request; explicitly versioned composite rules may
// make a small fixed number. It has no cookie jar or browser session state.
type Checker struct {
	ReputationStore  ReputationStore
	AntigravityToken string `json:"-"`
	ClientFactory    ClientFactory
	DriftSamples     int
}

func (c Checker) Check(ctx context.Context, node monitor.MonitoredNode, rule Rule, timeout time.Duration) (final Result) {
	startedAt := time.Now().UTC()
	var pathSnapshot func() speedtester.DownloadNetworkPath
	defer func() {
		if final.Details == nil {
			final.Details = map[string]string{}
		}
		// Specialized rules may flatten transport errors. Restore a typed local
		// constraint without discarding any earlier HTTP writes in a composite.
		if final.Details["physical_constraint_failure"] == "true" && final.HTTPStatus == nil {
			final.Outcome = "not_executed"
			if final.Details["http_requests_written"] != "0" {
				final.Outcome = "unknown"
			}
			final.FailurePhase = "physical_binding"
			final.ErrorMessage = "物理出口约束未核实；未取得完整服务结论"
		}
		if final.RequestCount == 0 || final.Outcome == "not_executed" {
			// No transport was entered: an exhausted/cancelled preparation
			// budget is not evidence of a node network timeout.
			if final.Outcome == "timed_out" || final.Outcome == "cancelled" {
				why := "budget_exhausted_before_network"
				if final.Outcome == "cancelled" {
					why = "cancelled_before_network"
				}
				final.Outcome, final.FailurePhase = "not_executed", "preparation"
				final.Details["not_executed_reason"] = why
			}
			final.Details["execution_status"] = "not_executed"
		} else {
			final.Details["execution_status"] = "executed"
		}
		final.Details["conclusion"] = serviceEvidenceConclusion(final)
		final.Details["evidence_node_identity"] = node.NodeIdentityKey
		final.Details["evidence_config_revision"] = node.ConfigRevisionKey
		final.Details["evidence_protocol"] = node.Type
		final.Details["evidence_target"] = rule.TargetURL
		final.Details["egress_verification"] = "unverified_default_node_proxy_route"
		if pathSnapshot != nil {
			path := pathSnapshot()
			encoded, _ := json.Marshal(path)
			final.Details["network_path"] = string(encoded)
			final.Details["egress_verification"] = "physical_binding_unverified_route_unobserved"
			if path.SocketBindVerified {
				final.Details["egress_verification"] = "physical_binding_verified_route_unobserved"
			}
		}
		final.Details["evidence_observed_at"] = final.FinishedAt.Format(time.RFC3339Nano)
		final.Details["evidence_valid_until"] = final.FinishedAt.Add(5 * time.Minute).Format(time.RFC3339Nano)
		final.Details["evidence_reuse"] = "disabled_exit_not_verified"
	}()
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaximumTimeout {
		timeout = MaximumTimeout
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if ctx.Err() != nil {
		return finish(unexecutedPreparation(ctx.Err()), startedAt)
	}
	if c.DriftSamples < 0 || c.DriftSamples > 10 {
		return finish(Result{Outcome: "not_executed", FailurePhase: "request_setup", ErrorMessage: "服务观察次数必须在 1–10 次范围内；未发出请求"}, startedAt)
	}
	if rule.ServiceID == "kktv_tw" {
		return finish(Result{Outcome: "not_executed", FailurePhase: "service_retired", Summary: "KKTV 已结束营运", ErrorMessage: "官方已结束营运；保留历史，不发起节点检测请求", Details: map[string]string{"not_executed_reason": "service_retired", "reference_url": "https://events.kktv.me/tw/faq-card-177"}}, startedAt)
	}

	if rule.ServiceID == "antigravity" && strings.TrimSpace(c.AntigravityToken) == "" {
		return finish(Result{Outcome: "credentials_required", FailurePhase: "credentials", ErrorMessage: "请先在设置中绑定 Google 凭据；未发出检测请求"}, startedAt)
	}
	// IPOK admission reads durable cooldown/quota before any proxy/DNS setup.
	if rule.ServiceID == "ipok_ipv4" || rule.ServiceID == "ipok_ipv6" {
		if c.ReputationStore == nil {
			return finish(Result{Outcome: "not_executed", FailurePhase: "local_storage", ErrorMessage: "IPOK 预算存储未初始化；未发请求"}, startedAt)
		}
		allowed, budgetErr := c.ReputationStore.CheckIPReputationBudget(ctx, time.Now().UTC())
		if budgetErr != nil {
			return finish(Result{Outcome: "not_executed", FailurePhase: "local_storage", ErrorMessage: "IPOK 持久化预算预检失败；未发请求"}, startedAt)
		}
		if !allowed {
			return finish(Result{Outcome: "not_executed", FailurePhase: "provider_quota", ErrorMessage: "IPOK 本地配额或冷却禁止查询；未发起出口观察", Details: map[string]string{"not_executed_reason": "ipok_budget_preflight_blocked"}}, startedAt)
		}
	}
	var client *http.Client
	var err error
	var cleanup func()
	if c.ClientFactory != nil {
		client, err = c.ClientFactory(node, timeout)
	} else {
		autoFamily := rule.ServiceID != "ipok_ipv4" && rule.ServiceID != "ipok_ipv6" && rule.ServiceID != "antigravity"
		client, pathSnapshot, cleanup, err = defaultClientFactory(ctx, node, timeout, autoFamily, serviceSocketBudget(rule, c.DriftSamples))
		if cleanup != nil {
			defer cleanup()
		}
	}
	if ctx.Err() != nil {
		return finish(unexecutedPreparation(ctx.Err()), startedAt)
	}
	if err != nil {
		if pathSnapshot != nil {
			return finish(Result{Outcome: "not_executed", FailurePhase: "physical_preflight", ErrorMessage: "物理出口准备未完成；未发出服务请求"}, startedAt)
		}
		return finish(Result{Outcome: "transport_error", FailurePhase: "proxy_setup", ErrorMessage: "无法建立节点隔离代理连接"}, startedAt)
	}
	if client == nil {
		return finish(Result{Outcome: "transport_error", FailurePhase: "proxy_setup", ErrorMessage: "无法建立节点隔离代理连接"}, startedAt)
	}
	// A supplied factory can share a client; never install attempt state on it.
	localClient := *client
	client = &localClient
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	phases := &requestPhases{}
	client.Transport = phases.wrap(client.Transport, c.ClientFactory == nil)
	if c.ClientFactory == nil {
		// Telemetry owns a cloned transport; close its pool as well as the
		// original physical client/proxy, especially after cancellation.
		defer client.CloseIdleConnections()
	}
	defer phases.attach(&final)
	if host := selectedNodeIPv4Host(rule); host != "" {
		resolved, evidence, failure := prepareTargetIPv4(ctx, client, phases, host)
		defer func() { evidence.attach(&final) }()
		if failure != nil {
			return finish(*failure, startedAt)
		}
		client = resolved
		defer client.CloseIdleConnections()
	}

	res1 := c.checkOnce(ctx, client, node, rule, startedAt)
	driftTarget := c.DriftSamples
	if driftTarget <= 1 || rule.ServiceID == "ipok_ipv4" || rule.ServiceID == "ipok_ipv6" ||
		res1.Outcome == "credentials_required" || res1.Outcome == "not_executed" || res1.Outcome == "cancelled" {
		if driftTarget == 1 {
			recordSingleServiceSample(&res1)
		}
		final = res1
		return final
	}

	extractIPAndColo := func(r Result) (string, string) {
		ip := r.Details["ip"]
		colo := r.Details["cloudflare_colo"]
		if colo == "" {
			colo = r.Details["cf_colo"]
		}
		return ip, colo
	}

	type sampleEntry struct {
		mode       string
		outcome    string
		httpStatus *int
		ip         string
		colo       string
		res        Result
	}

	ip1, colo1 := extractIPAndColo(res1)
	samples := []sampleEntry{
		{mode: "fresh", outcome: res1.Outcome, httpStatus: res1.HTTPStatus, ip: ip1, colo: colo1, res: res1},
	}

	for i := 2; i <= driftTarget; i++ {
		if samples[len(samples)-1].outcome == "rate_limited" {
			break
		}
		if ctx.Err() != nil {
			break
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 400*time.Millisecond {
			break
		}
		var mode string
		if i%2 == 0 {
			mode = "keepalive_requested"
		} else {
			mode = "close_http_idle_requested"
			if client != nil {
				client.CloseIdleConnections()
			}
		}

		probeStart := time.Now().UTC()
		subRes := c.checkOnce(ctx, client, node, rule, probeStart)
		subIP, subColo := extractIPAndColo(subRes)
		samples = append(samples, sampleEntry{
			mode:       mode,
			outcome:    subRes.Outcome,
			httpStatus: subRes.HTTPStatus,
			ip:         subIP,
			colo:       subColo,
			res:        subRes,
		})
		res1.RequestCount += subRes.RequestCount
		res1.BytesRead += subRes.BytesRead
		if subRes.Outcome == "cancelled" || subRes.Outcome == "not_executed" || subRes.Outcome == "credentials_required" {
			break
		}
	}

	totalSamples := len(samples)
	successSamples, failureSamples, unknownSamples, cancelledSamples, notExecutedSamples := 0, 0, 0, 0, 0
	ipSamples := 0
	var observedIPs, observedColos, observedOutcomes []string
	seenIPs, seenColos, seenOutcomes := map[string]bool{}, map[string]bool{}, map[string]bool{}
	keepaliveCount, idleCloseCount := 0, 0
	var firstUnknown *Result
	var limitedSample *Result

	for _, sample := range samples {
		if sample.outcome == "rate_limited" && limitedSample == nil {
			r := sample.res
			limitedSample = &r
		}
		if sample.mode == "keepalive_requested" {
			keepaliveCount++
		} else if sample.mode == "close_http_idle_requested" {
			idleCloseCount++
		}
		if ip := net.ParseIP(sample.ip); ip != nil {
			ipSamples++
			canonical := ip.String()
			if !seenIPs[canonical] {
				seenIPs[canonical] = true
				observedIPs = append(observedIPs, canonical)
			}
		}
		if sample.colo != "" && !seenColos[sample.colo] {
			seenColos[sample.colo] = true
			observedColos = append(observedColos, sample.colo)
		}
		if sample.outcome != "" && !seenOutcomes[sample.outcome] {
			seenOutcomes[sample.outcome] = true
			observedOutcomes = append(observedOutcomes, sample.outcome)
		}
		switch {
		case isSuccessOutcome(sample.outcome):
			successSamples++
		case isFailureOutcome(sample.res):
			failureSamples++
		case sample.outcome == "cancelled" || sample.outcome == "user_cancelled" || sample.outcome == "interrupted":
			cancelledSamples++
		case sample.outcome == "not_executed" || sample.outcome == "credentials_required":
			notExecutedSamples++
		default:
			unknownSamples++
			if firstUnknown == nil {
				r := sample.res
				firstUnknown = &r
			}
		}
	}

	unstartedSamples := max(0, driftTarget-totalSamples)
	confirmedSamples := successSamples + failureSamples
	incomplete := unknownSamples+cancelledSamples+notExecutedSamples+unstartedSamples > 0
	ipDrift := len(observedIPs) > 1
	flapping := successSamples > 0 && failureSamples > 0
	if res1.Details == nil {
		res1.Details = map[string]string{}
	}
	details := res1.Details
	if limitedSample != nil {
		details["sampling_stop_reason"] = "rate_limited"
		for _, key := range []string{"rate_limit_source", "retry_after_seconds", "retry_after_at", "rate_limit_reset_at"} {
			if value := limitedSample.Details[key]; value != "" {
				details[key] = value
			}
		}
	}
	details["sample_semantics_version"] = "2"
	details["sample_planned"] = fmt.Sprint(driftTarget)
	details["sample_total"] = fmt.Sprint(totalSamples)
	details["sample_success"] = fmt.Sprint(successSamples)
	details["sample_failure"] = fmt.Sprint(failureSamples)
	details["sample_unknown"] = fmt.Sprint(unknownSamples)
	details["sample_cancelled"] = fmt.Sprint(cancelledSamples)
	details["sample_not_executed"] = fmt.Sprint(notExecutedSamples)
	details["sample_unstarted"] = fmt.Sprint(unstartedSamples)
	details["sample_confirmed"] = fmt.Sprint(confirmedSamples)
	details["sample_completion"] = "complete"
	if incomplete {
		details["sample_completion"] = "partial"
	}
	details["sample_success_rate"] = "未知"
	if confirmedSamples > 0 {
		details["sample_success_rate"] = fmt.Sprintf("%d%%", successSamples*100/confirmedSamples)
	}
	details["sample_success_rate_scope"] = "confirmed_samples_only"
	details["keepalive_requested_probes"] = fmt.Sprint(keepaliveCount)
	details["idle_close_requested_probes"] = fmt.Sprint(idleCloseCount)
	details["ip_drift_detected"] = "unknown"
	details["exit_observation_status"] = "unobserved"
	switch {
	case ipDrift:
		details["ip_drift_detected"] = "true"
		details["exit_observation_status"] = "ip_drift_observed"
	case ipSamples >= 2:
		details["ip_drift_detected"] = "false"
		details["exit_observation_status"] = "same_ip_observed"
	case ipSamples == 1:
		details["exit_observation_status"] = "single_ip_observation"
	}
	details["ip_observation_samples"] = fmt.Sprint(ipSamples)
	details["colo_variation_detected"] = fmt.Sprint(len(observedColos) > 1)
	details["flapping_detected"] = fmt.Sprint(flapping)
	if len(observedIPs) > 0 {
		details["observed_ips"] = strings.Join(observedIPs, ", ")
	}
	if len(observedColos) > 0 {
		details["observed_colos"] = strings.Join(observedColos, ", ")
	}
	details["observed_outcomes"] = strings.Join(observedOutcomes, ", ")

	counts := fmt.Sprintf("%d通过 / %d判据未通过 / %d未确认 / %d取消 / %d未执行 / %d未启动", successSamples, failureSamples, unknownSamples, cancelledSamples, notExecutedSamples, unstartedSamples)
	switch {
	case ctx.Err() == context.Canceled || cancelledSamples > 0:
		res1.Outcome, res1.FailurePhase = "cancelled", "cancelled"
		res1.Summary = "多次检测已取消；已取得的样本保留"
		res1.ErrorMessage = "检测已取消；取消和未启动样本不算服务失败"
	case flapping:
		res1.Outcome, res1.FailurePhase = "flapping", "drift_validation"
		res1.Summary = "检测结论不一致；同时取得通过与判据未通过的证据"
		res1.ErrorMessage = "多次检测结论有差异；不能仅凭结论差异判定出口 IP 漂移"
	case limitedSample != nil:
		res1.Outcome, res1.FailurePhase = "rate_limited", limitedSample.FailurePhase
		res1.HTTPStatus = limitedSample.HTTPStatus
		res1.Summary = "目标限流，已停止本次后续采样；不判为节点故障"
		res1.ErrorMessage = limitedSample.ErrorMessage
	case incomplete:
		res1.Outcome, res1.FailurePhase = "unknown", "sampling_incomplete"
		res1.Summary = "多次检测尚未确认；部分样本未知或未完成"
		res1.ErrorMessage = "未确认、取消和未执行样本不计为服务失败"
		if firstUnknown != nil {
			res1.ErrorMessage = firstUnknown.ErrorMessage
			details["first_unconfirmed_outcome"] = firstUnknown.Outcome
			details["first_unconfirmed_phase"] = firstUnknown.FailurePhase
		}
	case successSamples == totalSamples:
		res1.Summary = fmt.Sprintf("%d/%d 次探针规则一致通过", totalSamples, driftTarget)
	default:
		res1.Summary = fmt.Sprintf("%d/%d 次探针判据未通过", failureSamples, driftTarget)
	}
	exitNote := "未观察出口 IP，不能判断是否变化"
	if ipDrift {
		exitNote = "观察到多个出口 IP；不代表物理流向已验证"
	} else if ipSamples > 0 {
		exitNote = "已观察出口 IP；仅代表已取得的样本，不保证出口稳定"
	}
	details["drift_summary"] = counts + " · " + exitNote
	res1 = finish(res1, startedAt)
	final = res1
	return final
}

// An explicitly selected single observation keeps its original outcome/phase;
// it does not claim repeated stability or turn an unknown into a zero score.
func recordSingleServiceSample(r *Result) {
	if r.Details == nil {
		r.Details = map[string]string{}
	}
	d := r.Details
	for _, key := range []string{"sample_success", "sample_failure", "sample_unknown", "sample_cancelled", "sample_not_executed", "sample_unstarted", "sample_confirmed"} {
		d[key] = "0"
	}
	switch {
	case isSuccessOutcome(r.Outcome):
		d["sample_success"], d["sample_confirmed"] = "1", "1"
	case isFailureOutcome(*r):
		d["sample_failure"], d["sample_confirmed"] = "1", "1"
	case r.Outcome == "cancelled" || r.Outcome == "user_cancelled" || r.Outcome == "interrupted":
		d["sample_cancelled"] = "1"
	case r.Outcome == "not_executed" || r.Outcome == "credentials_required":
		d["sample_not_executed"] = "1"
	default:
		d["sample_unknown"] = "1"
	}
	d["sample_semantics_version"], d["sample_planned"], d["sample_total"] = "2", "1", "1"
	d["sample_completion"], d["sample_success_rate"] = "partial", "未知"
	if d["sample_confirmed"] == "1" {
		d["sample_completion"] = "complete"
		if d["sample_success"] == "1" {
			d["sample_success_rate"] = "100%"
		} else {
			d["sample_success_rate"] = "0%"
		}
	}
	d["sample_success_rate_scope"] = "confirmed_samples_only"
	d["ip_drift_detected"] = "unknown"
}

func (c Checker) checkOnce(ctx context.Context, client *http.Client, node monitor.MonitoredNode, rule Rule, startedAt time.Time) (final Result) {

	if strings.HasPrefix(rule.ServiceID, "resource_") {
		return checkResource(ctx, client, rule, startedAt)
	}
	if rule.ServiceID == "antigravity" {
		return checkAntigravityObserved(ctx, client, c.AntigravityToken, startedAt)
	}
	switch rule.ServiceID {
	case "ipok_ipv4", "ipok_ipv6":
		return c.checkIPOK(ctx, client, rule, startedAt)
	case "exit_ipv4", "exit_ipv6":
		return checkExitAddress(ctx, client, rule, startedAt)
	case "cloudflare_trace":
		return checkCloudflareTrace(ctx, client, rule, startedAt)
	case "ping0_ip_quality", "ping0_ipv4", "ping0_ipv6":
		return checkPing0(ctx, client, rule, startedAt)
	case "ippure_ip_quality":
		return checkIPPure(ctx, client, rule, startedAt)
	case "netflix_unlock", netflixIPv4Service:
		return checkNetflix(ctx, client, rule, startedAt)
	case "abema_unlock":
		return checkAbema(ctx, client, rule, startedAt)
	case "fod_unlock":
		return checkFOD(ctx, client, rule, startedAt)
	case "youtube_premium":
		return checkYouTubePremium(ctx, client, rule, startedAt)
	case "prime_video":
		return checkPrimeVideo(ctx, client, rule, startedAt)
	case "hulu_jp_unlock":
		return checkHuluJapan(ctx, client, rule, startedAt)
	case "cloudflare_doh":
		return checkCloudflareDoH(ctx, client, rule, startedAt)
	case "google_search":
		return checkGoogleSearch(ctx, client, rule, startedAt)
	}
	client, redirectEvidence := serviceRedirectClient(client, rule)
	defer func() { redirectEvidence.attach(&final) }()
	request, err := http.NewRequestWithContext(ctx, rule.Method, rule.TargetURL, nil)
	if err != nil {
		return finish(Result{Outcome: "transport_error", FailurePhase: "request_setup", ErrorMessage: "无法创建固定服务请求"}, startedAt)
	}
	ua := "clash-speedtest"
	if rule.ResultKind == "web_access" || rule.ResultKind == "regional_reachability" || strings.HasSuffix(rule.ServiceID, "_web") {
		ua = browserUserAgent
	}
	request.Header.Set("User-Agent", ua)
	if rule.Accept != "" {
		request.Header.Set("Accept", rule.Accept)
	}
	if rule.APIVersionHeader != "" {
		request.Header.Set("X-GitHub-Api-Version", rule.APIVersionHeader)
	}

	response, err := client.Do(request)
	if err != nil {
		var pathErr *speedtester.PhysicalPathError
		if errors.As(err, &pathErr) {
			return finish(Result{Outcome: "not_executed", FailurePhase: "physical_binding", ErrorMessage: "物理出口约束未核实；未取得服务结论"}, startedAt)
		}
		result := Result{}
		classifyRequestError(&result, ctx, err)
		return finish(result, startedAt)
	}
	defer response.Body.Close()

	status := response.StatusCode
	result := Result{HTTPStatus: &status, RequestCount: 1, Details: map[string]string{}}
	if ray := response.Header.Get("CF-Ray"); ray != "" {
		result.Details["cf_ray"] = ray
		if parts := strings.Split(ray, "-"); len(parts) > 1 {
			result.Details["cf_colo"] = strings.TrimSpace(parts[1])
		}
	}
	if serviceRateLimit(&result, response, rule.ServiceID == "github_api_root") {
		if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
			result.Details["challenge_evidence"] = "cf-mitigated header on quota response"
		}
		return finish(result, startedAt)
	}
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Details = map[string]string{"challenge_evidence": "cf-mitigated header"}
		result.Outcome = "challenge"
		result.Summary = "Cloudflare 要求浏览器验证"
		result.ErrorMessage = "目标返回 Cloudflare Challenge；网络已连通，但无头请求未通过网页验证"
		return finish(result, startedAt)
	}
	if status >= 300 && status < 400 {
		if isReachabilityRule(rule) {
			result.Outcome = "redirect"
			result.Summary = "入口返回跳转，未验证最终页面"
			result.Details = map[string]string{"redirect_to": serviceEvidenceURL(response.Header.Get("Location"))}
			return finish(result, startedAt)
		}
		result.Outcome = "redirect"
		result.FailurePhase = "http_status"
		result.ErrorMessage = "服务返回重定向；本规则不跟随重定向"
		return finish(result, startedAt)
	}
	if (rule.ServiceID == "docker_registry" || rule.ServiceID == "xai_api") && status == http.StatusUnauthorized {
		result.Outcome = "reachable"
		result.Summary = "服务与鉴权入口可达"
		return finish(result, startedAt)
	}
	if isReachabilityRule(rule) {
		bodyLimit := int64(8192)
		if rule.ServiceID == "peacock_web" {
			bodyLimit = MaximumResponseBytes
		}
		if rule.Method == http.MethodHead {
			bodyLimit = 0
			result.Details["response_body_policy"] = "not_read_HEAD"
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, bodyLimit))
		result.BytesRead = int64(len(body))
		if readErr != nil {
			classifyRequestError(&result, ctx, atRequestPhase(readErr, "response_read"))
			return finish(result, startedAt)
		}
		if rule.ServiceID == "grok_web" {
			// This is a bounded prefix of the one response, never a new probe.
			result.Details = map[string]string{"body_prefix_sha256": fmt.Sprintf("%x", sha256.Sum256(body)), "body_evidence_limit": "8192 bytes; prefix only"}
			if classifyPageChallenge(&result, body) {
				return finish(result, startedAt)
			}
			if status < 200 || status >= 300 {
				result.Outcome, result.FailurePhase, result.ErrorMessage = "http_rejected", "http_status", fmt.Sprintf("入口返回 HTTP %d；未发现明确挑战标记", status)
				return finish(result, startedAt)
			}
		} else if classifyPageChallenge(&result, body) {
			return finish(result, startedAt)
		}

		if status < 200 || status >= 300 {
			result.Outcome, result.FailurePhase = "http_rejected", "http_status"
			result.ErrorMessage = fmt.Sprintf("入口返回 HTTP %d；未发现明确挑战标记", status)
			return finish(result, startedAt)
		}

		if rule.ServiceID == "abema_jp" && abemaErrorDocument(body) {
			result.Outcome, result.FailurePhase = "unknown", "page_error"
			result.Summary = "ABEMA 已响应，但返回错误页"
			result.ErrorMessage = "HTTP 200页面标题明确为ABEMA错误页；未确认正常入口或播放，不能推断节点断线或地区封锁"
			result.Details["page_error_evidence"] = "abema_error_title"
			result.Details["body_prefix_sha256"] = fmt.Sprintf("%x", sha256.Sum256(body))
			result.Details["http_reachable"] = "true"
			return finish(result, startedAt)
		}
		if rule.ServiceID == "peacock_web" && response.Request != nil && response.Request.URL != nil && strings.EqualFold(strings.TrimSuffix(response.Request.URL.Path, "/"), "/unavailable") {
			result.Details["playback_verified"] = "false"
			if strings.Contains(strings.ToLower(string(body)), "not yet available in your territory") {
				result.Outcome = "region_blocked"
				result.Summary = "Peacock 页面明确提示当前地区尚未提供服务；未验证播放"
				result.Details["restriction_evidence"] = "unavailable_path_and_explicit_territory_notice"
			} else {
				result.Outcome, result.FailurePhase = "unknown", "page_document"
				result.ErrorMessage = "Peacock 进入 unavailable 页面，但读取范围内没有明确地区说明，不能确认服务可用"
			}
			return finish(result, startedAt)
		}
		result.Outcome = "reachable"
		result.Summary = "入口响应可达；未验证登录、聊天或播放"
		return finish(result, startedAt)
	}

	if rule.ServiceID == "cloudflare_204" || rule.ServiceID == "google_204" || rule.ServiceID == "xiaomi_204" {
		if status == http.StatusNoContent {
			result.Outcome = "matched"
			return finish(result, startedAt)
		}
		result.Outcome = "http_rejected"
		result.FailurePhase = "http_status"
		result.ErrorMessage = fmt.Sprintf("服务返回 HTTP %d；判据要求 HTTP 204", status)
		return finish(result, startedAt)
	}

	if status != http.StatusOK {
		result.Outcome = "http_rejected"
		result.FailurePhase = "http_status"
		result.ErrorMessage = fmt.Sprintf("服务返回 HTTP %d；判据要求 HTTP 200", status)
		return finish(result, startedAt)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, MaximumResponseBytes+1))
	result.BytesRead = int64(min(len(body), MaximumResponseBytes))
	if readErr != nil {
		classifyRequestError(&result, ctx, atRequestPhase(readErr, "response_read"))
		return finish(result, startedAt)
	}
	if len(body) > MaximumResponseBytes {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "response_limit"
		result.ErrorMessage = "响应达到 64 KiB 读取上限，无法按规则确认"
		return finish(result, startedAt)
	}
	if expected, ok := fixedBodyExpectation(rule.ServiceID); ok {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(expected)) {
			result.Outcome = "matched"
			result.Summary = "固定连通性响应符合预期"
			return finish(result, startedAt)
		}
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "response_body"
		result.ErrorMessage = "响应正文未包含固定连通性标记"
		return finish(result, startedAt)
	}
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "content_type"
		result.ErrorMessage = "响应内容类型不符合 JSON 判据"
		return finish(result, startedAt)
	}
	var root struct {
		CurrentUserURL string `json:"current_user_url"`
		RepositoryURL  string `json:"repository_url"`
	}
	if json.Unmarshal(body, &root) != nil || root.CurrentUserURL == "" || root.RepositoryURL == "" {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "root_document"
		result.ErrorMessage = "响应未包含符合规则的 API 根索引"
		return finish(result, startedAt)
	}
	result.Outcome = "matched"
	return finish(result, startedAt)
}

func isReachabilityRule(rule Rule) bool {
	switch rule.ResultKind {
	case "web_access", "regional_reachability", "endpoint_reachability":
		return true
	default:
		return false
	}
}

func fixedBodyExpectation(serviceID string) (string, bool) {
	switch serviceID {
	case "apple_captive":
		return "Success", true
	case "microsoft_connect":
		return "Microsoft Connect Test", true
	case "firefox_portal":
		return "success", true
	default:
		return "", false
	}
}

func finish(result Result, startedAt time.Time) Result {
	result.StartedAt = startedAt.UTC()
	result.FinishedAt = time.Now().UTC()
	result.DurationMs = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	return result
}

func unexecutedPreparation(err error) Result {
	why := "budget_exhausted_before_network"
	if err == context.Canceled {
		why = "cancelled_before_network"
	}
	return Result{Outcome: "not_executed", FailurePhase: "preparation", ErrorMessage: "准备阶段原请求预算已耗尽或取消；未发起网络探测", Details: map[string]string{"not_executed_reason": why}}
}

func isSuccessOutcome(outcome string) bool {
	switch outcome {
	case "matched", "unlocked", "reachable", "profiled":
		return true
	default:
		return false
	}
}

// Only observed business/criterion rejection counts as a failed sample.
// Transport errors, quotas, authentication needs and probe limits stay unknown.
// Keep this evidence boundary aligned with frontend serviceAvailability.
func isFailureOutcome(r Result) bool {
	switch r.Outcome {
	case "region_blocked", "region_limited", "originals_only", "service_rejected", "challenge":
		return true
	case "http_rejected":
		return r.HTTPStatus != nil && *r.HTTPStatus >= 400 && *r.HTTPStatus < 500 && *r.HTTPStatus != 401 && *r.HTTPStatus != 429
	case "criteria_mismatch":
		switch r.FailurePhase {
		case "response_limit", "response_evidence_limit", "json_document", "response_body":
			return false
		}
		return r.HTTPStatus != nil
	default:
		return false
	}
}

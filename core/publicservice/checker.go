package publicservice

import (
	"context"
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
)

const (
	DefaultTimeout       = 10 * time.Second
	MaximumTimeout       = 30 * time.Second
	MaximumResponseBytes = 64 * 1024
	RuleVersion          = 1
)

// Rule is an immutable entry in the public-service catalog. The catalog is
// deliberately code-owned; callers cannot supply an arbitrary target URL.
type Rule struct {
	ServiceID        string `json:"service_id"`
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
		ServiceID: "ippure_ip_quality", Name: "IPPure 纯净度画像", RuleVersion: 2,
		Category: "出口与 IP", Region: "全球", ResultKind: "ip_quality", Description: "读取 IPPure 的欺诈分、住宅属性与原生/广播判断。", BatchDefault: true,
		TargetURL: "https://my.ippure.com/v1/info", Method: http.MethodGet,
		SuccessCriterion: "HTTP 200 且 JSON 包含出口 IP；欺诈分按 IPPure 原值独立展示",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: 8 * 1024, Accept: "application/json",
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
		ServiceID: "chatgpt_web", Name: "ChatGPT 网页与挑战", RuleVersion: 2,
		Category: "AI 服务", Region: "全球", ResultKind: "web_access", Description: "区分网页可达、Cloudflare 挑战和 HTTP 拒绝；不登录账号。",
		TargetURL: "https://chatgpt.com/", Method: http.MethodGet,
		SuccessCriterion: "收到网页响应；Cloudflare 挑战单独展示，网页可达不代表账号可用",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "netflix_unlock", Name: "Netflix 片目与地区检查", RuleVersion: 2,
		Category: "流媒体解锁", Region: "全球", ResultKind: "streaming_unlock", Description: "比较版权片与宽松片页面，不把登录页算通过；不代表实际播放解锁。",
		TargetURL: "https://www.netflix.com/title/81280792 + /title/70143836", Method: http.MethodGet,
		SuccessCriterion: "按两部测试片目的页面响应分档；规则会随 Netflix 片库变化，结果不读取播放分片",
		RedirectPolicy:   "follow_same_service", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "tver_jp", Name: "TVer 日本网页可达性", RuleVersion: 1,
		Category: "日本服务", Region: "日本", ResultKind: "regional_reachability", Description: "检测 TVer 网页响应；不把首页可达等同于可播放。",
		TargetURL: "https://tver.jp/", Method: http.MethodGet, SuccessCriterion: "收到 2xx/3xx 网页响应；仅表示网页可达，不证明播放解锁",
		RedirectPolicy: "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "abema_jp", Name: "ABEMA 日本网页可达性", RuleVersion: 1,
		Category: "日本服务", Region: "日本", ResultKind: "regional_reachability", Description: "检测 ABEMA 网页响应；不把首页可达等同于可播放。",
		TargetURL: "https://abema.tv/", Method: http.MethodGet, SuccessCriterion: "收到 2xx/3xx 网页响应；仅表示网页可达，不证明播放解锁",
		RedirectPolicy: "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "nhk_plus_jp", Name: "NHK Plus 日本网页可达性", RuleVersion: 1,
		Category: "日本服务", Region: "日本", ResultKind: "regional_reachability", Description: "检测 NHK Plus 网页响应；不使用账号。",
		TargetURL: "https://plus.nhk.jp/", Method: http.MethodGet, SuccessCriterion: "收到 2xx/3xx 网页响应；仅表示网页可达，不证明播放解锁",
		RedirectPolicy: "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "unext_jp", Name: "U-NEXT 日本网页可达性", RuleVersion: 1,
		Category: "日本服务", Region: "日本", ResultKind: "regional_reachability", Description: "检测 U-NEXT 网页响应；不使用账号。",
		TargetURL: "https://www.video.unext.jp/", Method: http.MethodGet, SuccessCriterion: "收到 2xx/3xx 网页响应；仅表示网页可达，不证明播放解锁",
		RedirectPolicy: "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "bbc_iplayer_uk", Name: "BBC iPlayer 英国网页可达性", RuleVersion: 1,
		Category: "其他地区", Region: "英国", ResultKind: "regional_reachability", Description: "检测 BBC iPlayer 网页响应；不把首页可达等同于播放解锁。",
		TargetURL: "https://www.bbc.co.uk/iplayer", Method: http.MethodGet, SuccessCriterion: "收到 2xx/3xx 网页响应；仅表示网页可达，不证明播放解锁",
		RedirectPolicy: "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "naver_kr", Name: "Naver 韩国网页可达性", RuleVersion: 1,
		Category: "其他地区", Region: "韩国", ResultKind: "regional_reachability", Description: "检测韩国 Naver 门户网页响应。",
		TargetURL: "https://www.naver.com/", Method: http.MethodGet, SuccessCriterion: "收到 2xx/3xx 网页响应；仅表示网页可达",
		RedirectPolicy: "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	streamingRule("abema_unlock", "ABEMA 日本地区判定", "日本", "https://api.abema.io/v1/ip/check?device=android", "读取 ABEMA 官方 IP 检查响应中的地区代码。", "isoCountryCode=JP 判定日本完整入口；其他地区显示海外档位"),
	streamingRule("fod_unlock", "FOD 日本地区判定", "日本", "https://geocontrol1.stream.ne.jp/fod-geo/check.xml?time=1624504256", "读取 FOD 地区控制端点，不下载视频。", "XML FLAG TYPE=true 判定地区允许"),
	streamingRule("hulu_jp_unlock", "Hulu Japan 入口限制检查", "日本", "https://id.hulu.jp/", "识别明确的 restrict 跳转；首页可达不代表地区或播放解锁。", "明确 restrict 跳转记为受限；200 仅记网页可达；403 不推断为地区封锁"),
	streamingRule("youtube_premium", "YouTube Premium 地区判定", "全球", "https://www.youtube.com/premium", "读取 Premium 页面公开地区与可用性标记。", "区分 Premium 可用、所在地区不可用及页面结构未知"),
	streamingRule("prime_video", "Prime Video 地区判定", "全球", "https://www.primevideo.com/", "读取 Prime Video 首页公开的 territory 与限制标记。", "currentTerritory 存在且 isServiceRestricted 明确为 false 才通过地区判据，不代表播放"),
	{
		ServiceID: "cloudflare_doh", Name: "Cloudflare DNS over HTTPS", Category: "基础网络", Region: "全球",
		ResultKind: "connectivity", Description: "验证 DNS 响应成功且包含解析结果。", RuleVersion: 2,
		TargetURL: "https://cloudflare-dns.com/dns-query?name=cloudflare.com&type=A", Method: http.MethodGet,
		SuccessCriterion: "HTTP 200、DNS Status=0 且 Answer 非空", RedirectPolicy: "do_not_follow",
		TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},

	webRule("fastly_web", "Fastly 边缘网络", "基础网络", "全球", "https://www.fastly.com/", "确认节点能否到达 Fastly 官网入口。"),
	webRule("aws_web", "AWS / CloudFront 入口", "基础网络", "全球", "https://aws.amazon.com/", "确认节点能否到达 AWS 全球入口。"),
	webRule("xiaomi_204", "小米国内网络对照", "基础网络", "中国", "https://connect.rom.miui.com/generate_204", "国内连通性对照，用于区分全球目标异常与整网异常。"),

	webRule("github_web", "GitHub 网页", "开发服务", "全球", "https://github.com/", "检测 GitHub 网页入口。"),
	webRule("github_codeload", "GitHub Codeload", "开发服务", "全球", "https://codeload.github.com/octocat/Hello-World/zip/refs/heads/master", "只确认归档下载入口响应，不读取归档正文。"),
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
	webRule("peacock_web", "Peacock 网页入口", "美国服务", "美国", "https://www.peacocktv.com/", "仅检测网页入口；不声称播放解锁。"),
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
	webRule("kktv_tw", "KKTV 台湾", "其他地区", "台湾", "https://www.kktv.me/", "检测网页入口；不声称播放解锁。"),
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
		RuleVersion: 2, TargetURL: target, Method: http.MethodGet,
		SuccessCriterion: "2xx 且未识别到挑战页记为入口可达；3xx 单独记为跳转；不等同于账号、支付或播放解锁",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
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

func Catalog() []Rule {
	result := make([]Rule, len(catalog))
	copy(result, catalog)
	return result
}

func RuleFor(serviceID string) (Rule, bool) {
	for _, rule := range catalog {
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
	AntigravityToken string `json:"-"`
	ClientFactory    ClientFactory
}

func (c Checker) Check(ctx context.Context, node monitor.MonitoredNode, rule Rule, timeout time.Duration) (final Result) {
	startedAt := time.Now().UTC()
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaximumTimeout {
		timeout = MaximumTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if rule.ServiceID == "antigravity" && strings.TrimSpace(c.AntigravityToken) == "" {
		return finish(Result{Outcome: "credentials_required", FailurePhase: "credentials", ErrorMessage: "请先在设置中绑定 Google 凭据；未发出检测请求"}, startedAt)
	}
	factory := c.ClientFactory
	if factory == nil {
		factory = defaultClientFactory
	}
	client, err := factory(node, timeout)
	if err != nil {
		return finish(Result{Outcome: "transport_error", FailurePhase: "proxy_setup", ErrorMessage: "无法建立节点隔离代理连接"}, startedAt)
	}
	if client == nil {
		return finish(Result{Outcome: "transport_error", FailurePhase: "proxy_setup", ErrorMessage: "无法建立节点隔离代理连接"}, startedAt)
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	phases := &requestPhases{}
	client.Transport = phases.wrap(client.Transport, c.ClientFactory == nil)
	defer phases.attach(&final)

	if rule.ServiceID == "antigravity" {
		return checkAntigravityObserved(ctx, client, c.AntigravityToken, startedAt)
	}
	switch rule.ServiceID {
	case "exit_ipv4", "exit_ipv6":
		return checkIPify(ctx, client, rule, startedAt)
	case "cloudflare_trace":
		return checkCloudflareTrace(ctx, client, rule, startedAt)
	case "ping0_ip_quality":
		return checkPing0(ctx, client, rule, startedAt)
	case "ippure_ip_quality":
		return checkIPPure(ctx, client, rule, startedAt)
	case "netflix_unlock":
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
	request, err := http.NewRequestWithContext(ctx, rule.Method, rule.TargetURL, nil)
	if err != nil {
		return finish(Result{Outcome: "transport_error", FailurePhase: "request_setup", ErrorMessage: "无法创建固定服务请求"}, startedAt)
	}
	request.Header.Set("User-Agent", "clash-speedtest")
	if rule.Accept != "" {
		request.Header.Set("Accept", rule.Accept)
	}
	if rule.APIVersionHeader != "" {
		request.Header.Set("X-GitHub-Api-Version", rule.APIVersionHeader)
	}

	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() == context.Canceled || err == context.Canceled {
			return finish(Result{Outcome: "cancelled", FailurePhase: "cancelled", ErrorMessage: "检测已取消"}, startedAt)
		}
		if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			return finish(Result{Outcome: "timed_out", FailurePhase: "timeout", ErrorMessage: "请求超过设定超时"}, startedAt)
		}
		if networkErr, ok := err.(net.Error); ok && networkErr.Timeout() {
			return finish(Result{Outcome: "timed_out", FailurePhase: "timeout", ErrorMessage: "请求超过设定超时"}, startedAt)
		}
		return finish(Result{Outcome: "transport_error", FailurePhase: "transport", ErrorMessage: "节点代理或 HTTP 传输失败；具体阶段未知"}, startedAt)
	}
	defer response.Body.Close()

	status := response.StatusCode
	result := Result{HTTPStatus: &status, RequestCount: 1}
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Outcome = "challenge"
		result.Summary = "Cloudflare 要求浏览器验证"
		result.ErrorMessage = "目标返回 Cloudflare Challenge；网络已连通，但无头请求未通过网页验证"
		return finish(result, startedAt)
	}
	if status >= 300 && status < 400 {
		if isReachabilityRule(rule) {
			result.Outcome = "redirect"
			result.Summary = "入口返回跳转，未验证最终页面"
			result.Details = map[string]string{"redirect_to": response.Header.Get("Location")}
			return finish(result, startedAt)
		}
		result.Outcome = "redirect"
		result.FailurePhase = "http_status"
		result.ErrorMessage = "服务返回重定向；本规则不跟随重定向"
		return finish(result, startedAt)
	}
	if status == http.StatusTooManyRequests || (rule.ServiceID == "github_api_root" && status == http.StatusForbidden && strings.TrimSpace(response.Header.Get("X-RateLimit-Remaining")) == "0") {
		result.Outcome = "rate_limited"
		result.FailurePhase = "http_status"
		result.ErrorMessage = "响应状态或限流头表明目标限制了请求"
		return finish(result, startedAt)
	}
	if (rule.ServiceID == "docker_registry" || rule.ServiceID == "xai_api") && status == http.StatusUnauthorized {
		result.Outcome = "reachable"
		result.Summary = "服务与鉴权入口可达"
		return finish(result, startedAt)
	}
	if isReachabilityRule(rule) && status >= 200 && status < 300 {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 8192))
		result.BytesRead = int64(len(body))
		if readErr != nil {
			classifyRequestError(&result, ctx, readErr)
			return finish(result, startedAt)
		}
		if pageHasChallenge(body) {
			result.Outcome, result.Summary = "challenge", "网页要求浏览器验证，未验证功能可用"
			return finish(result, startedAt)
		}
		result.Outcome = "reachable"
		result.Summary = "入口响应可达；未验证登录、聊天或播放"
		return finish(result, startedAt)
	}

	if rule.ServiceID == "cloudflare_204" || rule.ServiceID == "google_204" {
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
	body, readErr := io.ReadAll(io.LimitReader(response.Body, MaximumResponseBytes))
	result.BytesRead = int64(len(body))
	if readErr != nil {
		var networkErr net.Error
		switch {
		case ctx.Err() == context.Canceled || errors.Is(readErr, context.Canceled):
			result.Outcome = "cancelled"
			result.FailurePhase = "cancelled"
			result.ErrorMessage = "检测已取消"
		case ctx.Err() == context.DeadlineExceeded || errors.Is(readErr, context.DeadlineExceeded) || (errors.As(readErr, &networkErr) && networkErr.Timeout()):
			result.Outcome = "timed_out"
			result.FailurePhase = "timeout"
			result.ErrorMessage = "请求超过设定超时"
		default:
			result.Outcome = "transport_error"
			result.FailurePhase = "response_read"
			result.ErrorMessage = "读取响应时发生传输错误"
		}
		return finish(result, startedAt)
	}
	if len(body) >= MaximumResponseBytes {
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

func defaultClientFactory(node monitor.MonitoredNode, timeout time.Duration) (*http.Client, error) {
	return monitor.NewDefaultNodeDialer().CreateClient(node, timeout)
}

func finish(result Result, startedAt time.Time) Result {
	result.StartedAt = startedAt.UTC()
	result.FinishedAt = time.Now().UTC()
	result.DurationMs = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	return result
}

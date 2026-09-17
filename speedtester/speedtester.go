package speedtester

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/provider"
	"github.com/metacubex/mihomo/constant"
)

type Config struct {
	ConfigPaths      string
	FilterRegex      string
	BlockRegex       string
	ServerURL        string
	DownloadSize     int
	UploadSize       int
	Timeout          time.Duration
	Concurrent       int
	MaxLatency       time.Duration
	MaxPacketLoss    float64
	MinDownloadSpeed float64
	MinUploadSpeed   float64
	Mode             SpeedMode
	Metrics          MetricSet
	Duration         time.Duration
	Rounds           int
	OutputPath       string
	UserAgent        string // optional; empty means use default (mihomo kernel UA)
	// AntigravityToken is an OAuth access token used only by the Antigravity
	// eligibility probe. Google applies the region gate after authentication, so
	// without a token the probe can only report "unknown" - never a verdict.
	AntigravityToken string
}

const DefaultSpeedServer = "https://speed.cloudflare.com"

type serverMode int

const (
	serverModeDownloadServer serverMode = iota
	serverModeDirectDownload
)

// defaultFetchConfigUA returns the default User-Agent (mihomo kernel format) when none is set.
func defaultFetchConfigUA() string {
	return constant.MihomoName + "/" + constant.Version
}

func (st *SpeedTester) fetchConfigUA() string {
	if st.config.UserAgent != "" {
		return st.config.UserAgent
	}
	return defaultFetchConfigUA()
}

func (st *SpeedTester) fetchHTTPConfig(targetURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", st.fetchConfigUA())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

type serverTarget struct {
	mode        serverMode
	baseURL     string
	downloadURL string
}

type SpeedTester struct {
	config           *Config
	blockedNodes     []string
	blockedNodeCount int
	serverMode       serverMode
	serverBaseURL    string
	downloadURL      string
	mode             SpeedMode
	metrics          MetricSet
}

func New(config *Config) (*SpeedTester, error) {
	if config.Concurrent <= 0 {
		config.Concurrent = 1
	}
	if config.DownloadSize < 0 {
		config.DownloadSize = 100 * 1024 * 1024
	}
	if config.UploadSize < 0 {
		config.UploadSize = 10 * 1024 * 1024
	}
	mode := config.Mode
	if mode == "" {
		mode = SpeedModeDownload
	}
	metrics := config.Metrics
	if metrics.IsZero() {
		metrics = MetricsFromMode(mode)
	}
	mode = metrics.ToSpeedMode()
	if strings.TrimSpace(config.ServerURL) == "" {
		config.ServerURL = DefaultSpeedServer
	}
	target, err := resolveServerTarget(config.ServerURL)
	if err != nil {
		return nil, err
	}
	if metrics.Upload && config.UploadSize <= 0 {
		return nil, fmt.Errorf("upload size must be positive when upload testing is enabled")
	}
	if metrics.Upload && target.mode == serverModeDirectDownload {
		fallback, fallbackErr := resolveServerTarget(DefaultSpeedServer)
		if fallbackErr != nil {
			return nil, fallbackErr
		}
		target = fallback
	} else if target.mode == serverModeDirectDownload && mode == SpeedModeFull {
		mode = SpeedModeDownload
		metrics.Upload = false
	}
	config.Mode = mode
	config.Metrics = metrics

	// Automatically bind to the physical network interface so that proxy dials
	// bypass any active local TUN adapter (e.g. Clash Verge, Sing-box).
	_ = BindPhysicalInterface()

	return &SpeedTester{
		config:        config,
		serverMode:    target.mode,
		serverBaseURL: target.baseURL,
		downloadURL:   target.downloadURL,
		mode:          mode,
		metrics:       metrics,
	}, nil
}

func (st *SpeedTester) Mode() SpeedMode {
	return st.mode
}

func (st *SpeedTester) Metrics() MetricSet {
	if st.metrics.IsZero() {
		return MetricsFromMode(st.mode)
	}
	return st.metrics
}

func resolveServerTarget(rawURL string) (*serverTarget, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, fmt.Errorf("server url is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse server url %q failed: %w", rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("server url %q must include scheme and host", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("server url %q must use http or https scheme, got %q", rawURL, parsed.Scheme)
	}
	path := strings.TrimSpace(parsed.Path)
	hasPath := strings.Trim(path, "/") != ""
	hasQuery := parsed.RawQuery != ""
	hasFragment := parsed.Fragment != ""
	if !hasPath && !hasQuery && !hasFragment {
		return &serverTarget{
			mode:    serverModeDownloadServer,
			baseURL: strings.TrimRight(trimmed, "/"),
		}, nil
	}
	return &serverTarget{
		mode:        serverModeDirectDownload,
		downloadURL: trimmed,
	}, nil
}

type CProxy struct {
	constant.Proxy
	Config map[string]any
}

type RawConfig struct {
	Providers map[string]map[string]any `yaml:"proxy-providers"`
	Proxies   []map[string]any          `yaml:"proxies"`
}

func (st *SpeedTester) LoadProxies() (map[string]*CProxy, error) {
	allProxies := make(map[string]*CProxy)
	st.blockedNodes = make([]string, 0)
	st.blockedNodeCount = 0

	for configPath := range strings.SplitSeq(st.config.ConfigPaths, ",") {
		var body []byte
		var err error
		if strings.HasPrefix(configPath, "http") {
			body, err = st.fetchHTTPConfig(strings.TrimSpace(configPath))
			if err != nil {
				log.Printf("failed to fetch config: %s", err)
				continue
			}
		} else {
			body, err = os.ReadFile(configPath)
		}
		if err != nil {
			log.Printf("failed to read config: %s", err)
			continue
		}

		rawCfg, err := parseConfigBody(body)
		if err != nil {
			return nil, fmt.Errorf("unable to parse config at path %s: %w", configPath, err)
		}
		proxies := make(map[string]*CProxy)
		proxiesConfig := rawCfg.Proxies
		providersConfig := rawCfg.Providers

		for i, config := range proxiesConfig {
			proxy, err := adapter.ParseProxy(config)
			if err != nil {
				return nil, fmt.Errorf("proxy %d: %w", i, err)
			}

			if _, exist := proxies[proxy.Name()]; exist {
				return nil, fmt.Errorf("proxy %s is the duplicate name", proxy.Name())
			}
			proxies[proxy.Name()] = &CProxy{Proxy: proxy, Config: config}
		}
		for name, config := range providersConfig {
			if name == provider.ReservedName {
				return nil, fmt.Errorf("can not defined a provider called `%s`", provider.ReservedName)
			}
			pd, err := provider.ParseProxyProvider(name, config)
			if err != nil {
				return nil, fmt.Errorf("parse proxy provider %s error: %w", name, err)
			}
			if err := pd.Initial(); err != nil {
				log.Printf("initial proxy provider %s error: %s", pd.Name(), err)
				continue
			}

			body, err = st.fetchHTTPConfig(config["url"].(string))
			if err != nil {
				log.Printf("failed to fetch config: %s", err)
				continue
			}
			pdRawCfg, err := parseConfigBody(body)
			if err != nil {
				return nil, fmt.Errorf("unable to parse provider config %s: %w", name, err)
			}
			pdProxies := make(map[string]map[string]any)
			for _, pdProxy := range pdRawCfg.Proxies {
				if pdProxy["name"] == nil || pdProxy["server"] == nil {
					continue
				}
				pdProxies[pdProxy["name"].(string)] = pdProxy
			}
			for _, proxy := range pd.Proxies() {
				proxies[fmt.Sprintf("[%s] %s", name, proxy.Name())] = &CProxy{
					Proxy:  proxy,
					Config: pdProxies[proxy.Name()],
				}
			}
		}
		for k, p := range proxies {
			switch p.Type() {
			case constant.Shadowsocks, constant.ShadowsocksR, constant.Snell, constant.Socks5, constant.Http,
				constant.Vmess, constant.Vless, constant.Trojan, constant.Hysteria, constant.Hysteria2,
				constant.WireGuard, constant.Tuic, constant.Ssh, constant.Mieru, constant.AnyTLS, constant.Sudoku:
			default:
				continue
			}
			if server, ok := p.Config["server"]; ok {
				p.Config["server"] = convertMappedIPv6ToIPv4(server.(string))
			}
			if _, ok := allProxies[k]; !ok {
				allProxies[k] = p
			}
		}
	}

	filterRegexp := regexp.MustCompile(st.config.FilterRegex)
	var blockKeywords []string
	if st.config.BlockRegex != "" {
		for _, keyword := range strings.Split(st.config.BlockRegex, "|") {
			keyword = strings.TrimSpace(keyword)
			if keyword != "" {
				blockKeywords = append(blockKeywords, strings.ToLower(keyword))
			}
		}
	}

	filteredProxies := make(map[string]*CProxy)
	for name := range allProxies {
		shouldBlock := false
		if len(blockKeywords) > 0 {
			lowerName := strings.ToLower(name)
			for _, keyword := range blockKeywords {
				if strings.Contains(lowerName, keyword) {
					shouldBlock = true
					break
				}
			}
		}

		if shouldBlock {
			continue
		}
		if filterRegexp.MatchString(name) {
			filteredProxies[name] = allProxies[name]
		}
	}
	return deduplicateProxiesByServerPort(filteredProxies), nil
}

func deduplicateProxiesByServerPort(proxies map[string]*CProxy) map[string]*CProxy {
	if len(proxies) < 2 {
		return proxies
	}

	deduplicated := make(map[string]*CProxy, len(proxies))
	seen := make(map[string]struct{}, len(proxies))
	for name, proxy := range proxies {
		key, ok := buildProxyServerPortKey(proxy)
		if ok {
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		deduplicated[name] = proxy
	}
	return deduplicated
}

func buildProxyServerPortKey(proxy *CProxy) (string, bool) {
	if proxy == nil || proxy.Config == nil {
		return "", false
	}
	serverValue, serverOK := proxy.Config["server"]
	portValue, portOK := proxy.Config["port"]
	if !serverOK || !portOK {
		return "", false
	}
	server := strings.TrimSpace(fmt.Sprintf("%v", serverValue))
	if server == "" {
		return "", false
	}
	port := strings.TrimSpace(fmt.Sprintf("%v", portValue))
	if port == "" {
		return "", false
	}
	return fmt.Sprintf("%s:%s", server, port), true
}

func (st *SpeedTester) TestSingle(name string, proxy *CProxy, emit func(*Result) bool) *Result {
	var last *Result
	st.testProxyEmit(name, proxy, func(result *Result) bool {
		last = result
		if emit != nil {
			return emit(result)
		}
		return true
	})
	return last
}

func (st *SpeedTester) TestProxies(proxies map[string]*CProxy, tester func(result *Result)) {
	st.TestProxiesUntil(proxies, func(result *Result) bool {
		tester(result)
		return true
	})
}

func (st *SpeedTester) TestProxiesUntil(proxies map[string]*CProxy, tester func(result *Result) bool) {
	names := make([]string, 0, len(proxies))
	for name := range proxies {
		names = append(names, name)
	}
	sort.Strings(names)

	var deadline time.Time
	if st.config != nil && st.config.Duration > 0 {
		deadline = time.Now().Add(st.config.Duration)
	}
	targetRounds := 1
	if st.config != nil && st.config.Rounds > 0 {
		targetRounds = st.config.Rounds
	}
	currentRound := 0

	for {
		currentRound++
		for _, name := range names {
			if !deadline.IsZero() && !time.Now().Before(deadline) {
				return
			}
			if !st.testProxyEmit(name, proxies[name], tester) {
				return
			}
		}
		if deadline.IsZero() && currentRound >= targetRounds {
			return
		}
	}
}

func (st *SpeedTester) Rounds() int {
	if st.config != nil && st.config.Rounds > 0 {
		return st.config.Rounds
	}
	return 1
}

type LatencySample struct {
	Seq       int           `json:"seq"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
	LatencyMs int64         `json:"latency_ms"`
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
}

type Ping0Info struct {
	IP        string `json:"ip"`
	Location  string `json:"location"`
	Country   string `json:"country"`
	ASN       string `json:"asn"`
	Org       string `json:"org"`
	IsIDC     bool   `json:"is_idc"`
	RiskScore int    `json:"risk_score"` // 0-100 (ping0 风控值)
}

type IPPureInfo struct {
	IP            string `json:"ip"`
	ASN           int    `json:"asn"`
	ASOrg         string `json:"as_org"`
	Country       string `json:"country"`
	CountryCode   string `json:"country_code"`
	City          string `json:"city"`
	FraudScore    int    `json:"fraud_score"`    // 0-100 (IPPure 系数)
	IsResidential bool   `json:"is_residential"` // true: 住宅, false: 机房
	IsBroadcast   bool   `json:"is_broadcast"`   // true: 广播IP, false: 原生IP
}

type IPInfo struct {
	IP          string      `json:"ip"`
	ASN         string      `json:"asn"`
	ISP         string      `json:"isp"`
	IPType      string      `json:"ip_type"`     // "住宅家宽", "机房IDC"
	OriginType  string      `json:"origin_type"` // "原生IP", "广播IP"
	RiskScore   int         `json:"risk_score"`  // 0-100 (ping0.cc 风控分)
	FraudScore  int         `json:"fraud_score"` // 0-100 (ippure.com 欺诈/纯净度系数)
	Location    string      `json:"location"`    // e.g. "新加坡 新加坡"
	Country     string      `json:"country"`
	CountryCode string      `json:"country_code"`
	Ping0       *Ping0Info  `json:"ping0,omitempty"`
	IPPure      *IPPureInfo `json:"ippure,omitempty"`
}

type StabilityInfo struct {
	TotalProbes   int      `json:"total_probes"`
	SuccessProbes int      `json:"success_probes"`
	StabilityRate float64  `json:"stability_rate"` // e.g. 66.7
	Flapping      bool     `json:"flapping"`       // true if inconsistent results (偶发送中/负载均衡漂移)
	ExitIPs       []string `json:"exit_ips"`       // unique IPs detected across probe rounds
	BlockedIPs    []string `json:"blocked_ips,omitempty"` // IPs identified as blocked/CN by Google
	FlapReason    string   `json:"flap_reason,omitempty"`
	GoogleTTFBMs  int64    `json:"google_ttfb_ms"`        // average Google API interaction latency
	GoogleMinTTFB int64    `json:"google_min_ttfb_ms"`
	GoogleMaxTTFB int64    `json:"google_max_ttfb_ms"`
	LatencyGrade  string   `json:"latency_grade,omitempty"` // "fast", "medium", "slow", "laggy"
}

type Result struct {
	ProxyName         string          `json:"proxy_name"`
	ProxyType         string          `json:"proxy_type"`
	ProxyConfig       map[string]any  `json:"proxy_config"`
	Latency           time.Duration   `json:"latency"`
	Jitter            time.Duration   `json:"jitter"`
	PacketLoss        float64         `json:"packet_loss"`
	LatencySamples    []LatencySample `json:"latency_samples,omitempty"`
	DownloadSize      float64         `json:"download_size"`
	DownloadTime      time.Duration   `json:"download_time"`
	DownloadSpeed     float64         `json:"download_speed"`
	DownloadError     string          `json:"download_error"`
	UploadSize        float64         `json:"upload_size"`
	UploadTime        time.Duration   `json:"upload_time"`
	UploadSpeed       float64         `json:"upload_speed"`
	UploadError       string          `json:"upload_error"`
	AntigravityStatus string          `json:"antigravity_status,omitempty"`
	AntigravityDetail string          `json:"antigravity_detail,omitempty"`
	GoogleTTFBMs      int64           `json:"google_ttfb_ms,omitempty"`
	ExitCountry       string          `json:"exit_country,omitempty"`
	ExitCountryCode   string          `json:"exit_country_code,omitempty"`
	IPInfo            *IPInfo         `json:"ip_info,omitempty"`
	Stability         *StabilityInfo  `json:"stability,omitempty"`
}

func (r *Result) FormatDownloadSpeed() string {
	if r.DownloadError != "" {
		return r.DownloadError
	}
	return formatSpeed(r.DownloadSpeed)
}

func (r *Result) FormatDownloadSpeedValue() string {
	return formatSpeed(r.DownloadSpeed)
}

func (r *Result) FormatLatency() string {
	if r.Latency == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%dms", r.Latency.Milliseconds())
}

func (r *Result) FormatJitter() string {
	if r.Jitter == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%dms", r.Jitter.Milliseconds())
}

func (r *Result) FormatPacketLoss() string {
	return fmt.Sprintf("%.1f%%", r.PacketLoss)
}

func (r *Result) FormatUploadSpeed() string {
	if r.UploadError != "" {
		return r.UploadError
	}
	return formatSpeed(r.UploadSpeed)
}

func (r *Result) FormatUploadSpeedValue() string {
	return formatSpeed(r.UploadSpeed)
}

func (r *Result) FormatDownloadError() string {
	if r.DownloadError == "" {
		return "N/A"
	}
	return r.DownloadError
}

func (r *Result) FormatUploadError() string {
	if r.UploadError == "" {
		return "N/A"
	}
	return r.UploadError
}

func (r *Result) FormatAntigravity() string {
	if r == nil {
		return "N/A"
	}
	switch r.AntigravityStatus {
	case AntigravityAvailable:
		return "可用"
	case AntigravityFlapping:
		return "偶发送中"
	case AntigravityBlocked:
		return "地区不可用"
	case AntigravityUnreachable:
		return "节点不通"
	case AntigravityAuthFailed:
		return "凭证失效"
	case AntigravityUnknown:
		return "未知"
	default:
		return "N/A"
	}
}

func (r *Result) FormatExitCountry() string {
	if r == nil {
		return "N/A"
	}
	code := strings.TrimSpace(r.ExitCountryCode)
	name := strings.TrimSpace(r.ExitCountry)
	switch {
	case code != "" && name != "":
		return code + " " + name
	case code != "":
		return code
	case name != "":
		return name
	default:
		return "N/A"
	}
}

func formatSpeed(bytesPerSecond float64) string {
	if bytesPerSecond == 0 {
		return "N/A"
	}
	units := []string{"B/s", "KB/s", "MB/s", "GB/s", "TB/s"}
	unit := 0
	speed := bytesPerSecond
	for speed >= 1024 && unit < len(units)-1 {
		speed /= 1024
		unit++
	}
	return fmt.Sprintf("%.2f%s", speed, units[unit])
}

func (st *SpeedTester) testProxy(name string, proxy *CProxy) *Result {
	var last *Result
	st.testProxyEmit(name, proxy, func(result *Result) bool {
		last = result
		return true
	})
	return last
}

func (r *Result) snapshot() *Result {
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}

func (st *SpeedTester) testProxyEmit(name string, proxy *CProxy, emit func(*Result) bool) bool {
	if proxy == nil {
		return true
	}
	metrics := st.Metrics()
	result := &Result{
		ProxyName:   name,
		ProxyType:   proxy.Type().String(),
		ProxyConfig: proxy.Config,
	}
	emitOrContinue := func() bool {
		if emit == nil {
			return true
		}
		return emit(result.snapshot())
	}

	if metrics.Latency && metrics.Antigravity {
		var wg sync.WaitGroup
		wg.Add(2)
		var latRes *latencyResult
		var agStatus, agCountry, agCountryCode, agDetail string
		var agIPInfo *IPInfo
		var agStability *StabilityInfo

		go func() {
			defer wg.Done()
			pingCount := 6
			if st.config != nil && st.config.Duration > 0 {
				pingCount = 3
			}
			latRes = st.testLatency(proxy, st.config.MaxLatency, pingCount)
		}()

		go func() {
			defer wg.Done()
			agStatus, agCountry, agCountryCode, agDetail, agIPInfo, agStability = st.testAntigravity(proxy)
		}()

		wg.Wait()

		if latRes != nil {
			result.Latency = latRes.avgLatency
			result.Jitter = latRes.jitter
			result.PacketLoss = latRes.packetLoss
			result.LatencySamples = latRes.samples
		}
		result.AntigravityStatus = agStatus
		result.AntigravityDetail = agDetail
		result.ExitCountry = agCountry
		result.ExitCountryCode = agCountryCode
		result.IPInfo = agIPInfo
		result.Stability = agStability
		if agStability != nil && agStability.GoogleTTFBMs > 0 {
			result.GoogleTTFBMs = agStability.GoogleTTFBMs
		}

		if !emitOrContinue() {
			return false
		}

		deadNode := result.PacketLoss == 100
		if deadNode && !metrics.Download && !metrics.Upload {
			return true
		}
		if !deadNode {
			if st.config.OutputPath != "" && st.config.MaxPacketLoss < 100 && latRes != nil && latRes.packetLoss > st.config.MaxPacketLoss {
				return true
			}
			if st.config.OutputPath != "" && st.config.MaxLatency > 0 && latRes != nil && latRes.avgLatency > st.config.MaxLatency {
				return true
			}
		}
		if deadNode {
			metrics.Download = false
			metrics.Upload = false
		}
	} else {
		if metrics.Latency {
			pingCount := 6
			if st.config != nil && st.config.Duration > 0 {
				pingCount = 3
			}
			latencyResult := st.testLatency(proxy, st.config.MaxLatency, pingCount)
			result.Latency = latencyResult.avgLatency
			result.Jitter = latencyResult.jitter
			result.PacketLoss = latencyResult.packetLoss
			result.LatencySamples = latencyResult.samples
			if !emitOrContinue() {
				return false
			}
			deadNode := result.PacketLoss == 100
			if deadNode && !metrics.Download && !metrics.Upload && !metrics.Antigravity {
				return true
			}
			if !deadNode {
				if st.config.OutputPath != "" && st.config.MaxPacketLoss < 100 && latencyResult.packetLoss > st.config.MaxPacketLoss && !metrics.Antigravity {
					return true
				}
				if st.config.OutputPath != "" && st.config.MaxLatency > 0 && latencyResult.avgLatency > st.config.MaxLatency && !metrics.Antigravity {
					return true
				}
			}
			if deadNode {
				metrics.Download = false
				metrics.Upload = false
			}
		}

		if metrics.Antigravity {
			status, country, countryCode, detail, ipInfo, stab := st.testAntigravity(proxy)
			result.AntigravityStatus = status
			result.AntigravityDetail = detail
			result.ExitCountry = country
			result.ExitCountryCode = countryCode
			result.IPInfo = ipInfo
			result.Stability = stab
			if stab != nil && stab.GoogleTTFBMs > 0 {
				result.GoogleTTFBMs = stab.GoogleTTFBMs
			}
			if !emitOrContinue() {
				return false
			}
		}
	}

	if metrics.Download {
		downloadChunkSize := st.config.DownloadSize / st.config.Concurrent
		if downloadChunkSize > 0 {
			downloadSummary := newTransferSummary()
			downloadResults := make(chan *downloadResult, st.config.Concurrent)
			var wg sync.WaitGroup
			for i := 0; i < st.config.Concurrent; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					downloadResults <- st.testDownload(proxy, downloadChunkSize, st.config.Timeout)
				}()
			}
			wg.Wait()
			for range st.config.Concurrent {
				if dr := <-downloadResults; dr != nil {
					downloadSummary.add(dr)
				}
			}
			close(downloadResults)
			result.DownloadSize, result.DownloadTime, result.DownloadSpeed, result.DownloadError = applyTransferSummary(downloadSummary)
			if !emitOrContinue() {
				return false
			}
			if st.config.OutputPath != "" && st.config.MinDownloadSpeed > 0 && result.DownloadSpeed < st.config.MinDownloadSpeed {
				return true
			}
		}
	}

	if metrics.Upload {
		uploadChunkSize := st.config.UploadSize / st.config.Concurrent
		if uploadChunkSize > 0 {
			uploadSummary := newTransferSummary()
			uploadResults := make(chan *downloadResult, st.config.Concurrent)
			var wg sync.WaitGroup
			for i := 0; i < st.config.Concurrent; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					uploadResults <- st.testUpload(proxy, uploadChunkSize, st.config.Timeout)
				}()
			}
			wg.Wait()
			for i := 0; i < st.config.Concurrent; i++ {
				if ur := <-uploadResults; ur != nil {
					uploadSummary.add(ur)
				}
			}
			close(uploadResults)
			result.UploadSize, result.UploadTime, result.UploadSpeed, result.UploadError = applyTransferSummary(uploadSummary)
			if !emitOrContinue() {
				return false
			}
		}
	}

	return true
}

type latencyResult struct {
	avgLatency time.Duration
	jitter     time.Duration
	packetLoss float64
	samples    []LatencySample
}

func (st *SpeedTester) testLatency(proxy constant.Proxy, minLatency time.Duration, pingCount int) *latencyResult {
	if pingCount <= 0 {
		pingCount = 6
	}
	client := st.createClient(proxy, minLatency)
	defer client.CloseIdleConnections()

	latencies := make([]time.Duration, 0, pingCount)
	samples := make([]LatencySample, 0, pingCount)
	failedPings := 0
	probeURL := st.probeURL()

	for i := 0; i < pingCount; i++ {
		time.Sleep(100 * time.Millisecond)

		start := time.Now()
		req, err := http.NewRequest(http.MethodGet, probeURL, nil)
		if err != nil {
			failedPings++
			samples = append(samples, LatencySample{
				Seq:       i + 1,
				Timestamp: start,
				Success:   false,
				Error:     err.Error(),
			})
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			failedPings++
			samples = append(samples, LatencySample{
				Seq:       i + 1,
				Timestamp: start,
				Success:   false,
				Error:     err.Error(),
			})
			continue
		}
		_, _ = io.CopyN(io.Discard, resp.Body, 1)
		resp.Body.Close()
		dur := time.Since(start)
		latencies = append(latencies, dur)
		samples = append(samples, LatencySample{
			Seq:       i + 1,
			Timestamp: start,
			Duration:  dur,
			LatencyMs: dur.Milliseconds(),
			Success:   true,
		})
	}

	stats := calculateLatencyStats(latencies, failedPings, pingCount)
	stats.samples = samples
	return stats
}

func (st *SpeedTester) probeURL() string {
	if st.serverMode == serverModeDirectDownload && st.downloadURL != "" {
		return st.downloadURL
	}
	if st.serverBaseURL != "" {
		return fmt.Sprintf("%s/__down?bytes=1", st.serverBaseURL)
	}
	return st.downloadURL
}

type downloadResult struct {
	error    string
	bytes    int64
	duration time.Duration
}

type transferSummary struct {
	totalBytes    int64
	totalDuration time.Duration
	successCount  int
	errors        []string
	errorSeen     map[string]struct{}
}

func applyTransferSummary(summary *transferSummary) (float64, time.Duration, float64, string) {
	if summary == nil {
		return 0, 0, 0, ""
	}
	var size float64
	var duration time.Duration
	var speed float64
	var errorMessage string
	if summary.successCount > 0 {
		size = float64(summary.totalBytes)
		duration = summary.averageDuration()
		if duration > 0 {
			speed = float64(summary.totalBytes) / duration.Seconds()
		}
	}
	if len(summary.errors) > 0 {
		errorMessage = strings.Join(summary.errors, "; ")
		// If any transfer error is reported, treat the speed as zero.
		speed = 0
	}
	return size, duration, speed, errorMessage
}

func newTransferSummary() *transferSummary {
	return &transferSummary{
		errorSeen: make(map[string]struct{}),
	}
}

func (s *transferSummary) add(result *downloadResult) {
	if result == nil {
		return
	}
	if result.error != "" {
		s.appendError(result.error)
		return
	}
	s.totalBytes += result.bytes
	s.totalDuration += result.duration
	s.successCount++
}

func (s *transferSummary) appendError(message string) {
	if message == "" {
		return
	}
	if s.errorSeen == nil {
		s.errorSeen = make(map[string]struct{})
	}
	if _, exists := s.errorSeen[message]; exists {
		return
	}
	s.errorSeen[message] = struct{}{}
	s.errors = append(s.errors, message)
}

func (s *transferSummary) averageDuration() time.Duration {
	if s.successCount == 0 {
		return 0
	}
	return s.totalDuration / time.Duration(s.successCount)
}

func (st *SpeedTester) testDownload(proxy constant.Proxy, size int, timeout time.Duration) *downloadResult {
	client := st.createClient(proxy, timeout)
	defer client.CloseIdleConnections()

	start := time.Now()
	var downloadURL string
	if st.serverMode == serverModeDirectDownload {
		downloadURL = st.downloadURL
	} else {
		downloadURL = fmt.Sprintf("%s/__down?bytes=%d", st.serverBaseURL, size)
	}

	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return &downloadResult{
			error: fmt.Sprintf("create download request for %s failed: %v", downloadURL, err),
		}
	}
	if st.serverMode == serverModeDirectDownload && size > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", size-1))
	}
	resp, err := client.Do(req)
	if err != nil {
		return &downloadResult{
			error: fmt.Sprintf("download request to %s failed: %v, spent %s", downloadURL, err, time.Since(start)),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return &downloadResult{
			error: fmt.Sprintf("download response from %s returned %s, spent %s", downloadURL, resp.Status, time.Since(start)),
		}
	}

	downloadBytes, _ := io.Copy(io.Discard, resp.Body)
	return &downloadResult{
		bytes:    downloadBytes,
		duration: time.Since(start),
	}
}

func (st *SpeedTester) testUpload(proxy constant.Proxy, size int, timeout time.Duration) *downloadResult {
	client := st.createClient(proxy, timeout)
	defer client.CloseIdleConnections()

	reader := NewZeroReader(size)
	uploadURL := fmt.Sprintf("%s/__up", st.serverBaseURL)

	start := time.Now()
	resp, err := client.Post(
		uploadURL,
		"application/octet-stream",
		reader,
	)
	if err != nil {
		return &downloadResult{
			error: fmt.Sprintf("upload request to %s failed: %v, spent %s", uploadURL, err, time.Since(start)),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &downloadResult{
			error: fmt.Sprintf("upload response from %s returned %s, spent %s", uploadURL, resp.Status, time.Since(start)),
		}
	}

	return &downloadResult{
		bytes:    reader.WrittenBytes(),
		duration: time.Since(start),
	}
}

func (st *SpeedTester) createClient(proxy constant.Proxy, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				var u16Port uint16
				if port, err := strconv.ParseUint(port, 10, 16); err == nil {
					u16Port = uint16(port)
				}
				return proxy.DialContext(ctx, &constant.Metadata{
					Host:    host,
					DstPort: u16Port,
				})
			},
		},
	}
}

func calculateLatencyStats(latencies []time.Duration, failedPings, pingCount int) *latencyResult {
	if pingCount <= 0 {
		pingCount = 6
	}
	result := &latencyResult{
		packetLoss: float64(failedPings) / float64(pingCount) * 100,
	}

	if len(latencies) == 0 {
		return result
	}

	// 计算平均延迟
	var total time.Duration
	for _, l := range latencies {
		total += l
	}
	result.avgLatency = total / time.Duration(len(latencies))

	// 计算抖动
	var variance float64
	for _, l := range latencies {
		diff := float64(l - result.avgLatency)
		variance += diff * diff
	}
	variance /= float64(len(latencies))
	result.jitter = time.Duration(math.Sqrt(variance))

	return result
}

func convertMappedIPv6ToIPv4(server string) string {
	ip := net.ParseIP(server)
	if ip == nil {
		return server
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.String()
	}
	return server
}

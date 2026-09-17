package speedtester

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/core/auth"
	"github.com/metacubex/mihomo/constant"
)

const (
	AntigravityAvailable   = "available"
	AntigravityFlapping    = "flapping"
	AntigravityBlocked     = "blocked"
	AntigravityUnreachable = "unreachable"
	AntigravityAuthFailed  = "auth_failed"
	AntigravityUnknown     = "unknown"
)

const (
	// antigravityDailyURL is the authoritative internal endpoint used by Google Antigravity.
	antigravityDailyURL = "https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse"
	// antigravityProdURL is the fallback internal Cloud Code endpoint.
	antigravityProdURL = "https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse"
	// antigravityUserInfoURL is Google's authoritative user & IP region identity inspection endpoint.
	// It directly returns Google's internal geofence classification of the client IP (e.g. {"regionCode":"CN"}, {"regionCode":"HK"}).
	antigravityUserInfoURL = "https://daily-cloudcode-pa.googleapis.com/v1internal:fetchUserInfo"
	// antigravityProbeBody mimics Antigravity's real internal model call (gemini-3.8-flash-low).
	antigravityProbeBody = `{"project":"default","model":"gemini-3.8-flash-low","request":{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}}`
)

// ParseAntigravityToken extracts a bearer token from user-supplied text.
// It accepts a bare token, or a pasted header line such as
// "Authorization: Bearer ya29....". Blank lines and #-comments are skipped.
func ParseAntigravityToken(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(strings.ToLower(line), "authorization:"); idx >= 0 {
			line = line[idx+len("authorization:"):]
		}
		line = strings.TrimSpace(line)
		if len(line) > 7 && strings.EqualFold(line[:7], "bearer ") {
			line = strings.TrimSpace(line[7:])
		}
		return line
	}
	return ""
}

// ReadAntigravityTokenFile loads an OAuth access token from a file.
func ReadAntigravityTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := ParseAntigravityToken(string(data))
	if token == "" {
		return "", fmt.Errorf("no token found in %s", path)
	}
	return token, nil
}

// classifyAntigravityResponse maps an HTTP response to an availability verdict.
func classifyAntigravityResponse(statusCode int, body []byte) (status, detail string) {
	return classifyAntigravityResponseWithRegion(statusCode, body, "")
}

// classifyAntigravityResponseWithRegion maps an HTTP response and Google's detected region to an availability verdict.
//
// Google evaluates the region gate before checking license/subscription.
// - If the exit IP is blocked: Google ESF returns HTTP 400 with "User location is not supported"
//   or "FAILED_PRECONDITION".
// - If the exit IP is in a supported region: Google allows the request through. If the user
//   has no active enterprise license, Google returns HTTP 403 with "SUBSCRIPTION_REQUIRED".
//   This proves authentication succeeded AND the region is fully supported!
// - If credentials are completely invalid/expired: Google returns HTTP 401 UNAUTHENTICATED.
func classifyAntigravityResponseWithRegion(statusCode int, body []byte, googleRegion string) (status, detail string) {
	text := string(body)
	lower := strings.ToLower(text)

	// 1. Authoritative region verdict from Google's response.
	isLocationBlocked := strings.Contains(text, "User location is not supported") ||
		(strings.Contains(text, "FAILED_PRECONDITION") && (strings.Contains(lower, "location") || strings.Contains(lower, "region") || strings.Contains(lower, "country") || strings.Contains(lower, "api use"))) ||
		(statusCode == http.StatusBadRequest && (strings.Contains(lower, "user location") || strings.Contains(lower, "location is not supported")))

	if isLocationBlocked {
		if googleRegion == "CN" {
			return AntigravityBlocked, "出口送中 (Google出口识别为 CN, 限制访问)"
		}
		if googleRegion == "HK" {
			return AntigravityBlocked, "地区不支持 (Google出口识别为 HK, 限制访问)"
		}
		if googleRegion != "" {
			return AntigravityBlocked, fmt.Sprintf("地区不支持 (Google出口识别为 %s)", googleRegion)
		}
		return AntigravityBlocked, "地区不支持 (Google限制该出口)"
	}

	// If Google's internal geofence strictly identifies exit IP as China, Hong Kong or Macau,
	// Google's API strictly disallows access.
	if googleRegion == "CN" {
		return AntigravityBlocked, "出口送中 (Google出口识别为 CN, 限制访问)"
	}
	if googleRegion == "HK" {
		return AntigravityBlocked, "地区不支持 (Google出口识别为 HK, 限制访问)"
	}
	if googleRegion == "MO" {
		return AntigravityBlocked, "地区不支持 (Google出口识别为 MO, 限制访问)"
	}

	if statusCode <= 0 {
		return AntigravityUnreachable, firstLine(text)
	}
	if statusCode >= 200 && statusCode < 300 {
		if googleRegion != "" {
			return AntigravityAvailable, fmt.Sprintf("官方可用 (Google出口: %s)", googleRegion)
		}
		return AntigravityAvailable, "官方可用 (模型响应正常)"
	}
	if statusCode == http.StatusTooManyRequests || strings.Contains(text, "RESOURCE_EXHAUSTED") {
		if googleRegion != "" {
			return AntigravityAvailable, fmt.Sprintf("官方可用 (已达限额, 出口: %s)", googleRegion)
		}
		return AntigravityAvailable, "官方可用 (已达限额)"
	}
	if statusCode == http.StatusForbidden {
		if strings.Contains(text, "SUBSCRIPTION_REQUIRED") {
			if googleRegion != "" {
				return AntigravityAvailable, fmt.Sprintf("官方可用 (需许可, 出口: %s)", googleRegion)
			}
			return AntigravityAvailable, "官方可用 (需企业许可)"
		}
		if strings.Contains(text, "PERMISSION_DENIED") || strings.Contains(text, "cloudaicompanion.googleapis.com") || strings.Contains(text, "RESOURCE_PROJECT_INVALID") {
			if googleRegion != "" {
				return AntigravityAvailable, fmt.Sprintf("官方可用 (服务可达, 出口: %s)", googleRegion)
			}
			return AntigravityAvailable, "官方可用 (服务可达)"
		}
		return AntigravityAvailable, "官方可用 (服务可达)"
	}
	if statusCode == http.StatusUnauthorized {
		return AntigravityAuthFailed, "凭据未授权或已过期"
	}
	if statusCode >= 500 {
		return AntigravityUnreachable, fmt.Sprintf("HTTP %d", statusCode)
	}
	// Any other 4xx (not found, invalid argument)
	return AntigravityUnknown, firstLine(text)
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if i := strings.IndexAny(text, "\r\n"); i >= 0 {
		return strings.TrimSpace(text[:i])
	}
	if len(text) > 160 {
		return text[:160]
	}
	return text
}

func (st *SpeedTester) testAntigravity(proxy constant.Proxy) (status, country, countryCode, detail string, ipInfo *IPInfo, stab *StabilityInfo) {
	timeout := st.config.Timeout
	if timeout < 10*time.Second {
		timeout = 10 * time.Second
	}
	client := st.createClient(proxy, timeout)
	defer client.CloseIdleConnections()

	// 1. Probe IP attributes & purity (ping0.cc / ip-api)
	ipInfo = probeIPAttributes(client)
	if ipInfo != nil {
		if ipInfo.Country != "" {
			country = ipInfo.Country
		}
		if ipInfo.CountryCode != "" {
			countryCode = ipInfo.CountryCode
		}
	}

	if countryCode == "" {
		country, countryCode = lookupExitCountry(client)
	}
	if countryCode == "" {
		countryCode = parseCountryFromName(proxy.Name())
	}

	token := st.config.AntigravityToken
	if token == "" {
		if autoToken, _, err := auth.TryAutoDetectAntigravityToken(); err == nil && autoToken != "" {
			token = autoToken
		}
	}
	if token == "" {
		return AntigravityUnknown, country, countryCode, "需要绑定 Google 凭据以测试 Antigravity", ipInfo, nil
	}

	// 2. Multi-round stability probe against Google Antigravity to detect IP drift and intermittent '送中'
	status, country, countryCode, detail, stab = probeAntigravityStability(client, token, country, countryCode, ipInfo)
	return status, country, countryCode, detail, ipInfo, stab
}

func probeAntigravityStability(client *http.Client, token, currentCountry, currentCountryCode string, initialIPInfo *IPInfo) (status, country, countryCode, detail string, stab *StabilityInfo) {
	country = currentCountry
	countryCode = currentCountryCode

	rounds := 8
	stab = &StabilityInfo{
		TotalProbes: rounds,
		ExitIPs:     make([]string, 0, rounds),
		BlockedIPs:  make([]string, 0),
	}

	type probeRoundResult struct {
		idx          int
		status       string
		googleRegion string
		detail       string
		exitIP       string
		ttfbMs       int64
	}

	results := make([]probeRoundResult, 0, rounds)
	var mu sync.Mutex
	var wg sync.WaitGroup

	seenIPs := make(map[string]struct{})
	blockedIPMap := make(map[string]struct{})
	if initialIPInfo != nil && initialIPInfo.IP != "" {
		seenIPs[initialIPInfo.IP] = struct{}{}
		stab.ExitIPs = append(stab.ExitIPs, initialIPInfo.IP)
	}

	for i := 0; i < rounds; i++ {
		wg.Add(1)
		go func(roundIdx int) {
			defer wg.Done()

			// Slightly stagger connections by 25ms to force upstream load balancers to distribute
			if roundIdx > 0 {
				time.Sleep(time.Duration(roundIdx*25) * time.Millisecond)
			}

			// Clone transport with DisableKeepAlives = true so each probe opens a fresh TCP dial
			roundClient := &http.Client{
				Timeout: client.Timeout,
			}
			if tr, ok := client.Transport.(*http.Transport); ok {
				cloned := tr.Clone()
				cloned.DisableKeepAlives = true
				roundClient.Transport = cloned
			} else {
				roundClient.Transport = client.Transport
			}

			roundIP := lookupExitIPQuick(roundClient)
			rStatus, rRegion, rDetail, rTTFB := probeAntigravity(roundClient, token)

			mu.Lock()
			defer mu.Unlock()

			if roundIP != "" {
				if _, exists := seenIPs[roundIP]; !exists {
					seenIPs[roundIP] = struct{}{}
					stab.ExitIPs = append(stab.ExitIPs, roundIP)
				}
			}

			if rStatus == AntigravityBlocked || rRegion == "CN" || strings.Contains(rDetail, "送中") || strings.Contains(rDetail, "不支持") {
				if roundIP != "" {
					if _, bExists := blockedIPMap[roundIP]; !bExists {
						blockedIPMap[roundIP] = struct{}{}
						stab.BlockedIPs = append(stab.BlockedIPs, roundIP)
					}
				}
			}

			results = append(results, probeRoundResult{
				idx:          roundIdx,
				status:       rStatus,
				googleRegion: rRegion,
				detail:       rDetail,
				exitIP:       roundIP,
				ttfbMs:       rTTFB,
			})
		}(i)
	}

	wg.Wait()

	// Analyze aggregate results
	hasCN := false
	hasAvailable := false
	hasBlocked := false
	var lastRegion string
	var validTTFBs []int64

	for _, r := range results {
		if r.googleRegion != "" {
			lastRegion = r.googleRegion
		}
		if r.googleRegion == "CN" || strings.Contains(r.detail, "送中") {
			hasCN = true
		}
		if r.status == AntigravityAvailable {
			hasAvailable = true
			stab.SuccessProbes++
			validTTFBs = append(validTTFBs, r.ttfbMs)
		} else if r.status == AntigravityBlocked {
			hasBlocked = true
		}
	}

	stab.StabilityRate = math.Round((float64(stab.SuccessProbes)/float64(rounds))*1000) / 10

	// Calculate TTFB latency metrics
	if len(validTTFBs) > 0 {
		var minTTFB int64 = 999999
		var maxTTFB int64 = 0
		var sumTTFB int64 = 0
		for _, t := range validTTFBs {
			sumTTFB += t
			if t < minTTFB {
				minTTFB = t
			}
			if t > maxTTFB {
				maxTTFB = t
			}
		}
		stab.GoogleTTFBMs = sumTTFB / int64(len(validTTFBs))
		stab.GoogleMinTTFB = minTTFB
		stab.GoogleMaxTTFB = maxTTFB

		switch {
		case stab.GoogleTTFBMs < 500:
			stab.LatencyGrade = "fast"
		case stab.GoogleTTFBMs <= 1200:
			stab.LatencyGrade = "medium"
		case stab.GoogleTTFBMs <= 2500:
			stab.LatencyGrade = "slow"
		default:
			stab.LatencyGrade = "laggy"
		}
	}

	// 1. Check for Flapping / Intermittent Sent to China (负载均衡漂移)
	if (hasAvailable && hasBlocked) || (hasAvailable && hasCN) || (stab.SuccessProbes > 0 && stab.SuccessProbes < rounds) {
		stab.Flapping = true
		status = AntigravityFlapping

		blockedCount := rounds - stab.SuccessProbes
		if len(stab.BlockedIPs) > 0 {
			country = fmt.Sprintf("%s (负载均衡偶发送中)", country)
			stab.FlapReason = fmt.Sprintf("负载均衡多出口漂移: %d次并发抽样中 %d次被阻断，发现 %d 个出口 IP [%s]，受阻出口: [%s]",
				rounds, blockedCount, len(stab.ExitIPs), strings.Join(stab.ExitIPs, ", "), strings.Join(stab.BlockedIPs, ", "))
			detail = fmt.Sprintf("⚠️ 负载均衡偶发送中 (%d/%d次通过, 受阻出口: %s)",
				stab.SuccessProbes, rounds, strings.Join(stab.BlockedIPs, ", "))
		} else if hasCN {
			country = fmt.Sprintf("%s (偶发送中)", country)
			stab.FlapReason = fmt.Sprintf("出口IP池存在漂移，部分出口被Google判定为CN (检出 %d 个不同IP)", len(stab.ExitIPs))
			detail = fmt.Sprintf("⚠️ 偶发送中/出口漂移 (%d/%d次通过, 检出 %d 个出口IP)",
				stab.SuccessProbes, rounds, len(stab.ExitIPs))
		} else {
			stab.FlapReason = fmt.Sprintf("部分出口连接受阻 (稳定率 %.0f%%, 检出 %d 个不同IP)", stab.StabilityRate, len(stab.ExitIPs))
			detail = fmt.Sprintf("⚠️ 负载均衡波动 (%d/%d次通过, 稳定率 %.0f%%)",
				stab.SuccessProbes, rounds, stab.StabilityRate)
		}

		if stab.GoogleTTFBMs > 0 {
			detail += fmt.Sprintf(" · 响应: %dms", stab.GoogleTTFBMs)
			if stab.LatencyGrade == "slow" || stab.LatencyGrade == "laggy" {
				detail += " (延迟偏高易卡顿)"
			}
		}
		return status, country, countryCode, detail, stab
	}

	// 2. All 8 probes passed
	if stab.SuccessProbes == rounds {
		status = AntigravityAvailable
		if lastRegion != "" {
			if countryCode == "" {
				countryCode = lastRegion
			}
			detail = fmt.Sprintf("官方可用 (%d/%d 抽样极稳, Google出口: %s", stab.SuccessProbes, rounds, lastRegion)
		} else {
			detail = fmt.Sprintf("官方可用 (%d/%d 抽样极稳", stab.SuccessProbes, rounds)
		}
		if stab.GoogleTTFBMs > 0 {
			detail += fmt.Sprintf(", 响应: %dms)", stab.GoogleTTFBMs)
			if stab.LatencyGrade == "slow" || stab.LatencyGrade == "laggy" {
				detail += " ⚠️Google响应较慢"
			}
		} else {
			detail += ")"
		}
		return status, country, countryCode, detail, stab
	}

	// 3. All rounds failed
	lastRes := results[len(results)-1]
	status = lastRes.status
	detail = lastRes.detail
	if lastRegion != "" {
		if countryCode == "" {
			countryCode = lastRegion
		}
		if (lastRegion == "CN" || strings.Contains(lastRes.detail, "送中")) && countryCode != "CN" {
			country = fmt.Sprintf("%s (送中)", country)
		}
		detail = fmt.Sprintf("持续受限/送中 (Google出口: %s, %d/%d 次抽样均被拦截)", lastRegion, rounds, rounds)
	}
	return status, country, countryCode, detail, stab
}

func probeIPAttributes(client *http.Client) *IPInfo {
	var wg sync.WaitGroup
	var ping0Res *Ping0Info
	var ippureRes *IPPureInfo
	var mu sync.Mutex

	wg.Add(2)

	// Probe 1: ping0.cc (Geo, IDC, Risk Score)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ping0.cc/geo/json", nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return
		}

		var p struct {
			IP       string `json:"ip"`
			Location string `json:"location"`
			Country  string `json:"country"`
			City     string `json:"city"`
			ASN      string `json:"asn"`
			Org      string `json:"org"`
			IsIDC    bool   `json:"isidc"`
			IPRisk   int    `json:"iprisk"`
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if err != nil || json.Unmarshal(body, &p) != nil || p.IP == "" {
			return
		}

		loc := p.Location
		if loc == "" {
			loc = strings.TrimSpace(p.Country + " " + p.City)
		}
		mu.Lock()
		ping0Res = &Ping0Info{
			IP:        p.IP,
			Location:  loc,
			Country:   p.Country,
			ASN:       p.ASN,
			Org:       p.Org,
			IsIDC:     p.IsIDC,
			RiskScore: p.IPRisk,
		}
		mu.Unlock()
	}()

	// Probe 2: ippure.com (my.ippure.com/v1/info: Fraud Score, Residential, Broadcast)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://my.ippure.com/v1/info", nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return
		}

		var ipr struct {
			IP            string `json:"ip"`
			ASN           int    `json:"asn"`
			ASOrg         string `json:"asOrganization"`
			Country       string `json:"country"`
			CountryCode   string `json:"countryCode"`
			City          string `json:"city"`
			FraudScore    int    `json:"fraudScore"`
			IsResidential bool   `json:"isResidential"`
			IsBroadcast   bool   `json:"isBroadcast"`
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if err != nil || json.Unmarshal(body, &ipr) != nil || ipr.IP == "" {
			return
		}

		mu.Lock()
		ippureRes = &IPPureInfo{
			IP:            ipr.IP,
			ASN:           ipr.ASN,
			ASOrg:         ipr.ASOrg,
			Country:       ipr.Country,
			CountryCode:   ipr.CountryCode,
			City:          ipr.City,
			FraudScore:    ipr.FraudScore,
			IsResidential: ipr.IsResidential,
			IsBroadcast:   ipr.IsBroadcast,
		}
		mu.Unlock()
	}()

	wg.Wait()

	// If both primary services failed, perform fallback to ip-api.com
	if ping0Res == nil && ippureRes == nil {
		return probeIPAttributesFallback(client)
	}

	info := &IPInfo{
		Ping0:  ping0Res,
		IPPure: ippureRes,
	}

	// Synthesize unified fields from both authoritative services
	if ping0Res != nil {
		info.IP = ping0Res.IP
		info.ASN = ping0Res.ASN
		info.ISP = ping0Res.Org
		info.Location = ping0Res.Location
		info.Country = ping0Res.Country
		info.RiskScore = ping0Res.RiskScore
		if ping0Res.IsIDC {
			info.IPType = "机房IDC"
		} else {
			info.IPType = "住宅家宽"
		}
	}

	if ippureRes != nil {
		if info.IP == "" {
			info.IP = ippureRes.IP
		}
		if info.ASN == "" && ippureRes.ASN > 0 {
			info.ASN = fmt.Sprintf("AS%d", ippureRes.ASN)
		}
		if info.ISP == "" {
			info.ISP = ippureRes.ASOrg
		}
		if info.Location == "" {
			info.Location = strings.TrimSpace(ippureRes.Country + " " + ippureRes.City)
		}
		if info.Country == "" {
			info.Country = ippureRes.Country
		}
		if info.CountryCode == "" {
			info.CountryCode = ippureRes.CountryCode
		}
		info.FraudScore = ippureRes.FraudScore

		// Prioritize residential if either indicates residential
		if ippureRes.IsResidential {
			info.IPType = "住宅家宽"
		} else if info.IPType == "" {
			info.IPType = "机房IDC"
		}

		// Origin type directly from IPPure's broadcast detection
		if ippureRes.IsBroadcast {
			info.OriginType = "广播IP"
		} else {
			info.OriginType = "原生IP"
		}
	} else if info.IPType == "住宅家宽" {
		info.OriginType = "原生IP"
	} else {
		info.OriginType = "机房IP"
	}

	return info
}

func probeIPAttributesFallback(client *http.Client) *IPInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://ip-api.com/json/?fields=status,country,countryCode,city,isp,org,as,query,hosting", nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var p2 struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		City        string `json:"city"`
		ISP         string `json:"isp"`
		Org         string `json:"org"`
		AS          string `json:"as"`
		Query       string `json:"query"`
		Hosting     bool   `json:"hosting"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if json.Unmarshal(body, &p2) != nil || p2.Status != "success" {
		return nil
	}

	ipType := "机房IDC"
	originType := "广播IP"
	if !p2.Hosting {
		ipType = "住宅家宽"
		originType = "原生IP"
	}
	asn := p2.AS
	if idx := strings.Index(asn, " "); idx > 0 {
		asn = asn[:idx]
	}
	isp := p2.ISP
	if isp == "" {
		isp = p2.Org
	}
	return &IPInfo{
		IP:          p2.Query,
		ASN:         asn,
		ISP:         isp,
		IPType:      ipType,
		OriginType:  originType,
		RiskScore:   20,
		FraudScore:  20,
		Location:    strings.TrimSpace(p2.Country + " " + p2.City),
		Country:     p2.Country,
		CountryCode: p2.CountryCode,
	}
}

func lookupExitIPQuick(client *http.Client) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ip=") {
			return strings.TrimPrefix(line, "ip=")
		}
	}
	return ""
}

// fetchGoogleRegion queries Google's authoritative internal geofence endpoint (fetchUserInfo).
// It returns Google's internal 2-letter ISO region code for the exit IP (e.g. "CN", "HK", "SG", "US").
func fetchGoogleRegion(client *http.Client, token string) string {
	req, err := http.NewRequest(http.MethodPost, antigravityUserInfoURL, bytes.NewBufferString("{}"))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/2.14.0")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return ""
	}
	var info struct {
		RegionCode string `json:"regionCode"`
	}
	if err := json.Unmarshal(body, &info); err == nil {
		return strings.TrimSpace(strings.ToUpper(info.RegionCode))
	}
	return ""
}

// probeAntigravity asks Google about the exit's eligibility using caller credentials.
// It dual-checks fetchUserInfo and streamGenerateContent on daily-cloudcode-pa.
func probeAntigravity(client *http.Client, token string) (status, googleRegion, detail string, ttfbMs int64) {
	if token == "" {
		return AntigravityUnknown, "", "需要绑定 Google 凭据以测试 Antigravity", 0
	}

	// 1. Live Google internal geofence check
	googleRegion = fetchGoogleRegion(client, token)

	// 2. Live model stream generation check
	endpoints := []struct {
		url    string
		accept string
	}{
		{antigravityDailyURL, "text/event-stream"},
		{antigravityProdURL, "text/event-stream"},
	}

	status, detail = AntigravityUnreachable, ""
	for _, ep := range endpoints {
		s, d, reached, ttfb := probeAntigravityURL(client, ep.url, ep.accept, token, googleRegion)
		if reached {
			return s, googleRegion, d, ttfb
		}
		status, detail = s, d
	}
	return status, googleRegion, detail, 0
}

func probeAntigravityURL(client *http.Client, endpoint, accept, token, googleRegion string) (status, detail string, reached bool, ttfbMs int64) {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(antigravityProbeBody))
	if err != nil {
		return AntigravityUnknown, err.Error(), true, 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "antigravity/2.14.0")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return AntigravityUnreachable, err.Error(), false, 0
	}
	defer resp.Body.Close()
	ttfbMs = time.Since(start).Milliseconds()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	status, detail = classifyAntigravityResponseWithRegion(resp.StatusCode, body, googleRegion)
	return status, detail, true, ttfbMs
}

func parseCountryFlag(s string) string {
	runes := []rune(s)
	for i := 0; i+1 < len(runes); i++ {
		if runes[i] >= 0x1F1E6 && runes[i] <= 0x1F1FF &&
			runes[i+1] >= 0x1F1E6 && runes[i+1] <= 0x1F1FF {
			c1 := byte('A' + (runes[i] - 0x1F1E6))
			c2 := byte('A' + (runes[i+1] - 0x1F1E6))
			return string([]byte{c1, c2})
		}
	}
	return ""
}

func parseCountryFromName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "台湾") || strings.Contains(lower, "taiwan") || strings.Contains(lower, "tw"):
		return "TW"
	case strings.Contains(lower, "香港") || strings.Contains(lower, "hong kong") || strings.Contains(lower, "hongkong") || strings.Contains(lower, "hk"):
		return "HK"
	case strings.Contains(lower, "澳门") || strings.Contains(lower, "macau") || strings.Contains(lower, "mo"):
		return "MO"
	case strings.Contains(lower, "日本") || strings.Contains(lower, "japan") || strings.Contains(lower, "jp"):
		return "JP"
	case strings.Contains(lower, "新加坡") || strings.Contains(lower, "singapore") || strings.Contains(lower, "sg"):
		return "SG"
	case strings.Contains(lower, "美国") || strings.Contains(lower, "united states") || strings.Contains(lower, "us"):
		return "US"
	case strings.Contains(lower, "中国") || strings.Contains(lower, "china") || strings.Contains(lower, "cn"):
		return "CN"
	case strings.Contains(lower, "俄罗斯") || strings.Contains(lower, "russia") || strings.Contains(lower, "ru"):
		return "RU"
	}
	if flag := parseCountryFlag(name); flag != "" {
		return flag
	}
	return ""
}

func lookupExitCountry(client *http.Client) (country, countryCode string) {
	// 1. Primary provider: ip-api.com
	country, countryCode = lookupExitCountryIPAPI(client)
	if countryCode != "" {
		return country, countryCode
	}

	// 2. Fallback provider: Cloudflare trace (fast, global Anycast, virtually never fails)
	countryCode = lookupExitCountryCloudflare(client)
	if countryCode != "" {
		return "", countryCode
	}

	// 3. Fallback provider: ip.sb
	return lookupExitCountryIPSB(client)
}

func lookupExitCountryIPAPI(client *http.Client) (country, countryCode string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://ip-api.com/json/?fields=status,country,countryCode", nil)
	if err != nil {
		return "", ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return "", ""
	}
	var payload struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	if payload.Status != "" && payload.Status != "success" {
		return "", ""
	}
	return payload.Country, strings.ToUpper(payload.CountryCode)
}

func lookupExitCountryCloudflare(client *http.Client) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "loc=") {
			return strings.ToUpper(strings.TrimPrefix(line, "loc="))
		}
	}
	return ""
}

func lookupExitCountryIPSB(client *http.Client) (country, countryCode string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ip.sb/geoip", nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return "", ""
	}
	var payload struct {
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	return payload.Country, strings.ToUpper(payload.CountryCode)
}

// AntigravityRank returns a sort rank for the given status (lower is better: available first).
func AntigravityRank(status string) int {
	switch status {
	case AntigravityAvailable:
		return 0
	case AntigravityFlapping:
		return 1
	case AntigravityBlocked:
		return 2
	case AntigravityUnreachable:
		return 3
	case AntigravityAuthFailed:
		return 4
	case AntigravityUnknown:
		return 5
	default:
		return 6
	}
}

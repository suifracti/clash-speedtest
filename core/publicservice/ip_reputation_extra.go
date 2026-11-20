package publicservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func ipQueryRule() Rule {
	return Rule{
		ServiceID:        "ipquery_ip_quality",
		Name:             "IPQuery 风险画像",
		Category:         "IP 风险与纯净度",
		Region:           "全球",
		ResultKind:       "ip_quality",
		RuleVersion:      1,
		Description:      "独立查询 IPQuery 的出口地址、机房/VPN/代理/移动标记与 0–100 风险分，用于与 IPPure、Ping0、IPLogs 横向对照。",
		TargetURL:        "https://api.ipquery.io/?format=json",
		Method:           http.MethodGet,
		SuccessCriterion: "HTTP 200 且返回有效出口 IP 及 0–100 风险分；按源站原值独立展示，不代表业务解锁",
		RedirectPolicy:   "do_not_follow",
		TimeoutSeconds:   int(DefaultTimeout.Seconds()),
		MaximumBodyBytes: 16 * 1024,
		Accept:           "application/json",
	}
}

func ipLogsRule() Rule {
	return Rule{
		ServiceID:        "iplogs_ip_quality",
		Name:             "IPLogs 风险画像",
		Category:         "IP 风险与纯净度",
		Region:           "全球",
		ResultKind:       "ip_quality",
		RuleVersion:      1,
		Description:      "独立查询 IPLogs 的 VPN/代理/托管机房判定与 0–100 综合风险分，用于多站交叉比对出口纯净度。",
		TargetURL:        "https://iplogs.com/v1/check",
		Method:           http.MethodPost,
		SuccessCriterion: "HTTP 200 且返回有效出口 IP 与 0–100 风险分；按源站原值独立展示，不代表业务解锁",
		RedirectPolicy:   "do_not_follow",
		TimeoutSeconds:   int(DefaultTimeout.Seconds()),
		MaximumBodyBytes: 32 * 1024,
		Accept:           "application/json",
	}
}

func checkIPQuery(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes, rule.Accept)
	if !ok {
		return finish(result, started)
	}
	var payload struct {
		IP  string `json:"ip"`
		ISP struct {
			ASN string `json:"asn"`
			Org string `json:"org"`
			ISP string `json:"isp"`
		} `json:"isp"`
		Location struct {
			Country     string `json:"country"`
			CountryCode string `json:"country_code"`
			City        string `json:"city"`
			State       string `json:"state"`
		} `json:"location"`
		Risk struct {
			IsMobile     *bool `json:"is_mobile"`
			IsVPN        *bool `json:"is_vpn"`
			IsTor        *bool `json:"is_tor"`
			IsProxy      *bool `json:"is_proxy"`
			IsDatacenter *bool `json:"is_datacenter"`
			RiskScore    *int  `json:"risk_score"`
		} `json:"risk"`
	}
	if json.Unmarshal(body, &payload) != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "json_document", "IPQuery 响应不是可识别的 JSON"
		return finish(result, started)
	}
	parsedIP := net.ParseIP(strings.TrimSpace(payload.IP))
	if parsedIP == nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "json_document", "IPQuery 响应未包含可识别的出口 IP"
		return finish(result, started)
	}
	if payload.Risk.RiskScore == nil || *payload.Risk.RiskScore < 0 || *payload.Risk.RiskScore > 100 {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "risk_score", "IPQuery 风险分缺失或不在 0–100 范围"
		return finish(result, started)
	}
	score := *payload.Risk.RiskScore
	ipStr := parsedIP.String()
	family := "IPv6"
	if parsedIP.To4() != nil {
		family = "IPv4"
	}
	ipType := "非 IDC（不等于家宽）"
	switch {
	case payload.Risk.IsDatacenter != nil && *payload.Risk.IsDatacenter:
		ipType = "机房 IDC"
	case payload.Risk.IsMobile != nil && *payload.Risk.IsMobile:
		ipType = "移动网络"
	}
	cc := strings.ToUpper(strings.TrimSpace(payload.Location.CountryCode))
	loc := strings.TrimSpace(payload.Location.Country + " " + payload.Location.City)
	org := strings.TrimSpace(payload.ISP.Org)
	if org == "" {
		org = strings.TrimSpace(payload.ISP.ISP)
	}
	result.Outcome = "profiled"
	result.Summary = fmt.Sprintf("IPQuery 风险分 %d · %s", score, ipType)
	result.Details = map[string]string{
		"ip":             ipStr,
		"ip_family":      family,
		"country_code":   cc,
		"country":        strings.TrimSpace(payload.Location.Country),
		"location":       loc,
		"asn":            cleanASNString(payload.ISP.ASN),
		"organization":   org,
		"isp":            strings.TrimSpace(payload.ISP.ISP),
		"ip_type":        ipType,
		"risk_score":     strconv.Itoa(score),
		"risk_scale_max": "100",
		"risk_direction": "lower_is_better",
		"provider":       "IPQuery",
		"source_url":     rule.TargetURL,
		"query_at":       started.UTC().Format(time.RFC3339Nano),
		"exit_scope":     "此检测目标看到的节点出口；不同信誉库标准独立，不代表业务解锁",
	}
	attachBoolFlag(result.Details, "is_datacenter", payload.Risk.IsDatacenter)
	attachBoolFlag(result.Details, "is_mobile", payload.Risk.IsMobile)
	attachBoolFlag(result.Details, "is_vpn", payload.Risk.IsVPN)
	attachBoolFlag(result.Details, "is_proxy", payload.Risk.IsProxy)
	attachBoolFlag(result.Details, "is_tor", payload.Risk.IsTor)
	return finish(result, started)
}

func checkIPLogs(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result := Result{}
	if client == nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "proxy_setup", "无法建立节点隔离代理连接"
		return finish(result, started)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rule.TargetURL, bytes.NewBufferString("{}"))
	if err != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "request_setup", "无法构造 IPLogs 请求"
		return finish(result, started)
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	result.RequestCount = 1
	resp, err := client.Do(req)
	if err != nil {
		classifyRequestError(&result, ctx, err)
		return finish(result, started)
	}
	defer resp.Body.Close()
	code := resp.StatusCode
	result.HTTPStatus = &code
	if serviceRateLimit(&result, resp, false) {
		return finish(result, started)
	}
	limit := rule.MaximumBodyBytes
	if limit <= 0 || limit > MaximumResponseBytes {
		limit = 32 * 1024
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	result.BytesRead = int64(min(len(body), limit))
	if readErr != nil {
		classifyRequestError(&result, ctx, atRequestPhase(readErr, "response_read"))
		return finish(result, started)
	}
	if len(body) > limit {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "response_limit", "IPLogs 响应超过读取上限"
		return finish(result, started)
	}
	if code != http.StatusOK {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "http_rejected", "http_status", fmt.Sprintf("IPLogs 返回 HTTP %d", code)
		return finish(result, started)
	}
	var payload struct {
		IP         string   `json:"ip"`
		Score      *float64 `json:"score"`
		Verdict    string   `json:"verdict"`
		Reputation string   `json:"reputation"`
		ProxyType  string   `json:"proxy_type"`
		IsVPN      *bool    `json:"is_vpn"`
		IsProxy    *bool    `json:"is_proxy"`
		IsTor      *bool    `json:"is_tor"`
		IsRelay    *bool    `json:"is_relay"`
		IsHosting  *bool    `json:"is_hosting"`
		Country    string   `json:"country"`
		CountryCC  string   `json:"country_code"`
		City       string   `json:"city"`
		ASN        string   `json:"asn"`
		Org        string   `json:"org"`
		IPInfo     *struct {
			IP          string `json:"ip"`
			ASN         string `json:"asn"`
			Org         string `json:"org"`
			ISP         string `json:"isp"`
			Country     string `json:"country"`
			CountryCode string `json:"country_code"`
			City        string `json:"city"`
			Type        string `json:"type"`
		} `json:"ip_info"`
	}
	if json.Unmarshal(body, &payload) != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "json_document", "IPLogs 响应不是可识别的 JSON"
		return finish(result, started)
	}
	rawIP := strings.TrimSpace(payload.IP)
	if rawIP == "" && payload.IPInfo != nil {
		rawIP = strings.TrimSpace(payload.IPInfo.IP)
	}
	parsedIP := net.ParseIP(rawIP)
	if parsedIP == nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "json_document", "IPLogs 响应未包含可识别的出口 IP"
		return finish(result, started)
	}
	if payload.Score == nil || math.IsNaN(*payload.Score) || math.IsInf(*payload.Score, 0) || *payload.Score < 0 || *payload.Score > 100 {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "risk_score", "IPLogs 风险分缺失或超出有效范围"
		return finish(result, started)
	}
	// IPLogs returns score on a 0.0–1.0 scale (0 = clean, 1.0 = high risk);
	// normalize cleanly to 0–100 integer risk scale.
	rawScore := *payload.Score
	score := int(math.Round(rawScore * 100))
	if rawScore > 1 {
		score = int(math.Round(rawScore))
	}
	score = max(0, min(100, score))

	ipStr := parsedIP.String()
	family := "IPv6"
	if parsedIP.To4() != nil {
		family = "IPv4"
	}
	infoType := ""
	country := strings.TrimSpace(payload.Country)
	cc := strings.ToUpper(strings.TrimSpace(payload.CountryCC))
	city := strings.TrimSpace(payload.City)
	asn := strings.TrimSpace(payload.ASN)
	org := strings.TrimSpace(payload.Org)
	isp := ""
	if payload.IPInfo != nil {
		infoType = strings.ToLower(strings.TrimSpace(payload.IPInfo.Type))
		if country == "" {
			country = strings.TrimSpace(payload.IPInfo.Country)
		}
		if cc == "" {
			cc = strings.ToUpper(strings.TrimSpace(payload.IPInfo.CountryCode))
		}
		if city == "" {
			city = strings.TrimSpace(payload.IPInfo.City)
		}
		if asn == "" {
			asn = strings.TrimSpace(payload.IPInfo.ASN)
		}
		if org == "" {
			org = strings.TrimSpace(payload.IPInfo.Org)
		}
		isp = strings.TrimSpace(payload.IPInfo.ISP)
	}
	if org == "" {
		org = isp
	}
	ipType := "非 IDC（不等于家宽）"
	switch {
	case (payload.IsHosting != nil && *payload.IsHosting) || strings.Contains(infoType, "datacenter") || strings.Contains(infoType, "hosting") || strings.EqualFold(payload.ProxyType, "datacenter"):
		ipType = "机房 IDC"
	case strings.Contains(infoType, "mobile") || strings.Contains(infoType, "cellular"):
		ipType = "移动网络"
	case strings.Contains(infoType, "residential") || strings.Contains(infoType, "isp"):
		ipType = "住宅（第三方判断）"
	}
	tier := strings.TrimSpace(payload.Verdict)
	if payload.Reputation != "" && payload.Reputation != tier {
		if tier != "" {
			tier += " / " + strings.TrimSpace(payload.Reputation)
		} else {
			tier = strings.TrimSpace(payload.Reputation)
		}
	}
	result.Outcome = "profiled"
	result.Summary = fmt.Sprintf("IPLogs 风险分 %d · %s", score, ipType)
	result.Details = map[string]string{
		"ip":             ipStr,
		"ip_family":      family,
		"country_code":   cc,
		"country":        country,
		"location":       strings.TrimSpace(country + " " + city),
		"asn":            cleanASNString(asn),
		"organization":   org,
		"isp":            isp,
		"ip_type":        ipType,
		"risk_score":     strconv.Itoa(score),
		"risk_tier":      tier,
		"risk_scale_max": "100",
		"risk_direction": "lower_is_better",
		"provider":       "IPLogs",
		"source_url":     rule.TargetURL,
		"query_at":       started.UTC().Format(time.RFC3339Nano),
		"exit_scope":     "此检测目标看到的节点出口；不同信誉库标准独立，不代表业务解锁",
	}
	attachBoolFlag(result.Details, "is_hosting", payload.IsHosting)
	attachBoolFlag(result.Details, "is_vpn", payload.IsVPN)
	attachBoolFlag(result.Details, "is_proxy", payload.IsProxy)
	attachBoolFlag(result.Details, "is_tor", payload.IsTor)
	attachBoolFlag(result.Details, "is_relay", payload.IsRelay)
	return finish(result, started)
}

func cleanASNString(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	upper := strings.ToUpper(s)
	if strings.HasPrefix(upper, "AS") {
		digits := strings.TrimPrefix(upper, "AS")
		if _, err := strconv.ParseUint(digits, 10, 32); err == nil {
			return "AS" + digits
		}
	}
	if _, err := strconv.ParseUint(s, 10, 32); err == nil {
		return "AS" + s
	}
	return s
}

func attachBoolFlag(details map[string]string, key string, val *bool) {
	if val != nil {
		details[key] = strconv.FormatBool(*val)
	}
}

package publicservice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Store reserves quota durably before an upstream lookup. A missing/failing
// store fails closed; process restarts cannot reset the daily request budget.
type ReputationStore interface {
	CheckIPReputationBudget(context.Context, time.Time) (bool, error)
	LoadIPReputation(context.Context, string, time.Time) (string, error)
	SaveIPReputation(context.Context, string, string, time.Time) error
	ReserveIPReputation(context.Context, time.Time) (bool, error)
	CoolIPReputation(context.Context, time.Time) error
}

var reputationLookupMu sync.Mutex

func ipokRule(id, family string) Rule {
	return Rule{ServiceID: id, Name: "IPOK " + family + " 画像与信誉", Category: "IP 画像与信誉", Region: "全球", ResultKind: "ip_quality", RuleVersion: 1, TargetURL: "https://ipok.io/api/ip", Method: http.MethodGet, Description: "按需：经被测节点确认 " + family + " 出口，再查询该 IP；缓存和厂商风险独立展示，不代表业务解锁。", SuccessCriterion: "所选节点的 ipify 出口与指定地址族一致，IPOK v1 geo.ip 与查询地址一致；仅表示第三方 IP 画像", RedirectPolicy: "do_not_follow", TimeoutSeconds: 10, MaximumBodyBytes: MaximumResponseBytes}
}
func publicAddress(raw string) (netip.Addr, bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil || a.Is4In6() || !a.IsGlobalUnicast() || a.IsPrivate() {
		return a, false
	}
	for _, rawPrefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(rawPrefix).Contains(a) {
			return a, false
		}
	}
	return a, true
}
func (c Checker) checkIPOK(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	family, id := "IPv4", "exit_ipv4"
	if rule.ServiceID == "ipok_ipv6" {
		family, id = "IPv6", "exit_ipv6"
	}
	if c.ReputationStore == nil {
		return finish(Result{Outcome: "not_executed", FailurePhase: "local_storage", ErrorMessage: "IPOK 额度/缓存存储未初始化；未发请求"}, started)
	}
	discoveryRule, _ := RuleFor(id)
	discovery := checkIPify(ctx, client, discoveryRule, started)
	if discovery.Outcome != "profiled" {
		discovery.Details = map[string]string{"provider": "IPOK", "ip_family": family, "failure_scope": "selected_node_exit_observation"}
		return discovery
	}
	address, ok := publicAddress(discovery.Details["ip"])
	if !ok || address.Is4() != (family == "IPv4") {
		return finish(Result{Outcome: "unknown", RequestCount: discovery.RequestCount, BytesRead: discovery.BytesRead, FailurePhase: "ip_family", ErrorMessage: "被测节点的指定地址族公网出口未确认；未查询信誉源"}, started)
	}
	ip := address.String()
	cacheKey := "ipok-v1|" + family + "|" + ip
	// Serialize cache misses across nodes. Discovery still runs on each node;
	// shared vendor cache never substitutes for node exit observation.
	reputationLookupMu.Lock()
	defer reputationLookupMu.Unlock()
	if ctx.Err() != nil {
		r := Result{RequestCount: discovery.RequestCount, BytesRead: discovery.BytesRead}
		classifyRequestError(&r, ctx, ctx.Err())
		return finish(r, started)
	}
	now := time.Now().UTC()
	cached, err := c.ReputationStore.LoadIPReputation(ctx, cacheKey, now)
	if err != nil {
		return finish(Result{Outcome: "unknown", RequestCount: discovery.RequestCount, FailurePhase: "local_storage", ErrorMessage: "缓存读取失败；未继续查询源站"}, started)
	}
	decorate := func(r Result, hit bool) Result {
		if r.Details == nil {
			r.Details = map[string]string{}
		}
		r.Details["ip"] = ip
		r.Details["ip_family"] = family
		r.Details["provider"] = "IPOK"
		r.Details["source_url"] = "https://ipok.io/"
		r.Details["exit_observed_at"] = discovery.FinishedAt.Format(time.RFC3339Nano)
		r.Details["exit_observation_target"] = discoveryRule.TargetURL
		r.Details["exit_scope"] = "仅此节点的指定地址族 ipify 出口；不保证其它检测目标使用同一出口"
		r.Details["provider_api_version"] = "1"
		r.Details["cache_status"] = "miss"
		if hit {
			r.Details["cache_status"] = "hit"
		}
		r.Details["service_availability"] = "not_verified"
		return finish(r, started)
	}
	if cached != "" {
		var r Result
		if json.Unmarshal([]byte(cached), &r) == nil && r.Outcome == "profiled" && r.Details["ip"] == ip && r.Details["ip_family"] == family {
			r.RequestCount = discovery.RequestCount
			r.BytesRead = discovery.BytesRead
			return decorate(r, true)
		}
	}
	reserved, err := c.ReputationStore.ReserveIPReputation(ctx, now)
	if err != nil || !reserved {
		return decorate(Result{Outcome: "unknown", RequestCount: discovery.RequestCount, BytesRead: discovery.BytesRead, FailurePhase: "provider_quota", ErrorMessage: "IPOK 本地额度/冷却受限（全局最多15次/UTC日，间隔至少2秒）；没有发送信誉查询"}, false)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rule.TargetURL+"?ip="+url.QueryEscape(ip), nil)
	if err != nil {
		return decorate(Result{Outcome: "unknown", RequestCount: discovery.RequestCount, FailurePhase: "request_setup", ErrorMessage: err.Error()}, false)
	}
	request.Header.Set("User-Agent", "Clash-SpeedTest/1 IP-Reputation")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Version", "1")
	result := Result{RequestCount: discovery.RequestCount + 1, BytesRead: discovery.BytesRead}
	response, err := client.Do(request)
	if err != nil {
		classifyRequestError(&result, ctx, err)
		return decorate(result, false)
	}
	defer response.Body.Close()
	status := response.StatusCode
	result.HTTPStatus = &status
	if status != http.StatusOK {
		result.Outcome = "http_rejected"
		result.FailurePhase = "provider_http"
		result.ErrorMessage = fmt.Sprintf("IPOK HTTP %d（信誉源响应，不是节点失效）", status)
		if status == 429 {
			result.Outcome = "rate_limited"
			wait := 30 * time.Minute
			if n, e := strconv.Atoi(response.Header.Get("Retry-After")); e == nil && time.Duration(n)*time.Second > wait {
				wait = time.Duration(n) * time.Second
			} else if at, e := http.ParseTime(response.Header.Get("Retry-After")); e == nil && at.Sub(now) > wait {
				wait = at.Sub(now)
			}
			if err := c.ReputationStore.CoolIPReputation(ctx, now.Add(wait)); err != nil {
				result.ErrorMessage += "；冷却保存失败，须核查存储"
			}
		}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaximumResponseBytes+1))
	result.BytesRead += int64(min(len(body), MaximumResponseBytes))
	result.Details = ipokResponseEvidence(response, body, err == nil)
	if status != http.StatusOK {
		return decorate(result, false)
	}
	if err != nil {
		classifyRequestError(&result, ctx, err)
		return decorate(result, false)
	}
	if len(body) > MaximumResponseBytes {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "response_limit"
		result.ErrorMessage = "IPOK 响应超过64KiB"
		return decorate(result, false)
	}
	type profileDocument struct {
		APIVersion string `json:"apiVersion"`
		Geo        struct {
			IP      string          `json:"ip"`
			ASN     json.RawMessage `json:"asn"`
			ASName  string          `json:"asName"`
			ISP     string          `json:"isp"`
			Country string          `json:"country"`
		} `json:"geo"`
		Risk    *int            `json:"risk"`
		IPType  string          `json:"ipType"`
		Signals json.RawMessage `json:"signals"`
		Sources json.RawMessage `json:"sources"`
		Quota   json.RawMessage `json:"quota"`
	}
	var document profileDocument
	if json.Unmarshal(body, &document) != nil || document.APIVersion != "1" || document.Geo.IP != ip || document.Risk != nil && (*document.Risk < 0 || *document.Risk > 100) {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "provider_document"
		result.ErrorMessage = "IPOK v1 响应、查询IP或风险字段不匹配；未保存为有效画像"
		return decorate(result, false)
	}
	// Persist only the same allowlisted projection used by response evidence.
	var sanitized profileDocument
	if json.Unmarshal([]byte(result.Details["provider_response_body"]), &sanitized) != nil || sanitized.APIVersion != "1" || sanitized.Geo.IP != ip {
		result.Outcome, result.FailurePhase = "criteria_mismatch", "response_evidence_limit"
		result.ErrorMessage = "响应无法在隐私与正文上限内保留有效画像证据"
		return decorate(result, false)
	}
	document = sanitized
	risk := "未提供"
	if document.Risk != nil {
		risk = strconv.Itoa(*document.Risk)
	}
	result.Outcome = "profiled"
	result.Summary = "IPOK 独立风险分 " + risk + " · " + family + " · 不代表服务可用"
	profileDetails := map[string]string{"risk_score": risk, "asn": cleanASN(document.Geo.ASN), "organization": document.Geo.ASName, "isp": document.Geo.ISP, "country": document.Geo.Country, "ip_type": document.IPType, "risk_flags": string(document.Signals), "source_observations": string(document.Sources), "provider_quota": string(document.Quota), "query_at": now.Format(time.RFC3339Nano), "cache_valid_until": now.Add(24 * time.Hour).Format(time.RFC3339Nano), "data_freshness": "sources.observedAt 保留；缺失为未知。x-ipok-upstream 单独保留，不假定实时；本地缓存命中不刷新厂商查询时间", "provider_result_json": result.Details["provider_response_body"]}
	for k, v := range profileDetails {
		result.Details[k] = v
	}
	result = decorate(result, false)
	// Respect an explicitly exhausted source quota even if another client used it.
	var quota struct {
		Arbitrary *struct {
			Remaining *int  `json:"remaining"`
			Resets    int64 `json:"resetsInSeconds"`
		} `json:"arbitraryLookup"`
	}
	if json.Unmarshal(document.Quota, &quota) == nil && quota.Arbitrary != nil && quota.Arbitrary.Remaining != nil && *quota.Arbitrary.Remaining == 0 {
		wait := time.Duration(quota.Arbitrary.Resets) * time.Second
		if wait <= 0 {
			nextDay := now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
			wait = nextDay.Sub(now)
		}
		if err := c.ReputationStore.CoolIPReputation(ctx, now.Add(wait)); err != nil {
			result.Details["quota_save_error"] = "源站零剩余额度的冷却保存失败"
		}
	}
	result.Details["lookup_target"] = rule.TargetURL + "?ip=" + url.QueryEscape(ip)
	data, _ := json.Marshal(result)
	if err := c.ReputationStore.SaveIPReputation(ctx, cacheKey, string(data), now); err != nil {
		result.Details["cache_save_error"] = "缓存保存失败；本次结果仍由原有工作台保存流程处理"
	}
	return result
}

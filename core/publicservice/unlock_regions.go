package publicservice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// Public browser SDK identifier used by Disney’s no-account device flow; not a user credential.
	disneyRegionAuth = "Bearer ZGlzbmV5JmJyb3dzZXImMS4wLjA.Cu56AgSfBTDag5NiRA81oLHkDZfu5L3CKadnefEAY84"
	disneyDeviceURL  = "https://disney.api.edge.bamgrid.com/devices"
	disneyTokenURL   = "https://disney.api.edge.bamgrid.com/token"
	disneyGraphURL   = "https://disney.api.edge.bamgrid.com/graph/v1/device/graphql"
	spotifyRegionURL = "https://spclient.wg.spotify.com/signup/public/v1/account"
	tiktokRegionURL  = "https://www.tiktok.com/"
	unextRegionURL   = "https://cc.unext.jp/?zxuid=1b64984ecc23&zxemp=29854391&operationName=cosmo_getPlaylistUrl&variables=%7B%22code%22%3A%22ED00440417%22%2C%22playMode%22%3A%22caption%22%2C%22bitrateLow%22%3A192%2C%22bitrateHigh%22%3Anull%2C%22validationOnly%22%3Atrue%7D&extensions=%7B%22persistedQuery%22%3A%7B%22version%22%3A1%2C%22sha256Hash%22%3A%22a2309e22a6819ff747cf9a389dd78db35fa3c386fac1d53461061ba20fa44e34%22%7D%7D"
	dmmRegionURL     = "https://tv.dmm.com/vod/"
	radikoRegionURL  = "https://radiko.jp/"
)

var (
	regionCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)
	radikoAreaPattern = regexp.MustCompile(`(?i)(?:window\.)?["']?area[_]?id["']?\s*[:=]\s*["'](JP[0-9]{2}|OUT)["']`)
	tiktokBootstrap   = regexp.MustCompile(`(?s)<script[^>]+id=["']__UNIVERSAL_DATA_FOR_REHYDRATION__["'][^>]*>(.*?)</script>`)
)

// regionUnlockRules is kept separate so the catalog owner can merge these
// entries without coupling this probe implementation to checker.go.
func regionUnlockRules() []Rule {
	return []Rule{
		regionUnlockRule("disney_unlock", "Disney+ 地区判定", "流媒体", "全球", disneyDeviceURL, http.MethodPost, "Disney+ 会话返回国家代码及 supported-location 地区判据"),
		regionUnlockRule("spotify_unlock", "Spotify 地区判定", "流媒体", "全球", spotifyRegionURL, http.MethodPost, "Spotify 注册校验接口返回 country 与 is_country_launched；不提交邮箱或密码，不注册账号"),
		regionUnlockRule("tiktok_unlock", "TikTok 地区判定", "流媒体", "全球", tiktokRegionURL, http.MethodGet, "TikTok 首页启动数据提供 region；普通网页响应不算解锁"),
		regionUnlockRule("unext_unlock", "U-NEXT 日本地区判定", "日本", "日本", unextRegionURL, http.MethodGet, "U-NEXT 播放校验返回地区判定状态；validationOnly 不取播放清单"),
		regionUnlockRule("dmm_unlock", "DMM TV 日本地区判定", "日本", "日本", dmmRegionURL, http.MethodGet, "DMM TV 地区拒绝页或服务数据中的明确地区字段"),
		regionUnlockRule("radiko_unlock", "radiko 日本地区判定", "日本", "日本", radikoRegionURL, http.MethodGet, "radiko 页面当前 areaId 为日本区域或明确显示区域限制"),
	}
}

func regionUnlockRule(id, name, category, region, target, method, criterion string) Rule {
	rule := Rule{
		ServiceID: id, Name: name, Category: category, Region: region, ResultKind: "unlock",
		RuleVersion: 1, Description: "使用服务自身返回的地区字段或明确地区拒绝；不验证账号、订阅或播放。",
		TargetURL: target, Method: method, SuccessCriterion: criterion, RedirectPolicy: "probe_specific",
		TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	}
	if id == "tiktok_unlock" {
		rule.MaximumBodyBytes = MaximumWebDocumentBytes
	}
	if id == "disney_unlock" || id == "spotify_unlock" || id == "tiktok_unlock" {
		rule.RuleVersion = 2
	}
	return rule
}

func checkRegionUnlock(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	switch rule.ServiceID {
	case "disney_unlock":
		return checkDisneyRegion(ctx, client, rule, started)
	case "spotify_unlock":
		return checkSpotifyRegion(ctx, client, rule, started)
	case "tiktok_unlock":
		return checkTikTokRegion(ctx, client, rule, started)
	case "unext_unlock":
		return checkUNextRegion(ctx, client, rule, started)
	case "dmm_unlock":
		return checkDMMRegion(ctx, client, rule, started)
	case "radiko_unlock":
		return checkRadikoRegion(ctx, client, rule, started)
	default:
		return finish(Result{Outcome: "unknown", FailurePhase: "rule", ErrorMessage: "未识别的地区解锁规则"}, started)
	}
}

func checkDisneyRegion(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result := Result{Details: map[string]string{}}
	device, err := json.Marshal(map[string]any{
		"deviceFamily": "browser", "applicationRuntime": "chrome", "deviceProfile": "windows", "attributes": map[string]any{},
	})
	if err != nil {
		return finish(Result{Outcome: "unknown", FailurePhase: "request_setup", ErrorMessage: "无法准备 Disney+ 地区请求"}, started)
	}
	body, part, ok := regionRequest(ctx, client, http.MethodPost, rule.TargetURL, "application/json", device, rule.MaximumBodyBytes)
	addRegionRequest(&result, part)
	if !ok {
		return finishRegionFailure(result, body, "forbidden-location", "Disney+ 当前地区不受支持", started)
	}
	var deviceData struct {
		Assertion string `json:"assertion"`
	}
	if json.Unmarshal(body, &deviceData) != nil || deviceData.Assertion == "" {
		return regionUnknown(result, "region_device", "Disney+ 未返回设备地区令牌", started)
	}
	form := url.Values{
		"grant_type":         {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"latitude":           {"0"},
		"longitude":          {"0"},
		"platform":           {"browser"},
		"subject_token":      {deviceData.Assertion},
		"subject_token_type": {"urn:bamtech:params:oauth:token-type:device"},
	}
	body, part, ok = regionRequest(ctx, client, http.MethodPost, disneyTokenURL, "application/x-www-form-urlencoded", []byte(form.Encode()), rule.MaximumBodyBytes)
	addRegionRequest(&result, part)
	if !ok {
		return finishRegionFailure(result, body, "forbidden-location", "Disney+ 当前地区不受支持", started)
	}
	var tokenData struct {
		RefreshToken string `json:"refresh_token"`
	}
	if json.Unmarshal(body, &tokenData) != nil || tokenData.RefreshToken == "" {
		return regionUnknown(result, "region_token", "Disney+ 未返回地区会话", started)
	}
	query, _ := json.Marshal(map[string]any{
		"query":     "mutation refreshToken($input: RefreshTokenInput!) { refreshToken(refreshToken: $input) { activeSession { sessionId } } }",
		"variables": map[string]any{"input": map[string]string{"refreshToken": tokenData.RefreshToken}},
	})
	body, part, ok = regionRequest(ctx, client, http.MethodPost, disneyGraphURL, "application/json", query, rule.MaximumBodyBytes)
	addRegionRequest(&result, part)
	if !ok {
		return finishRegionFailure(result, body, "forbidden-location", "Disney+ 当前地区不受支持", started)
	}
	var session map[string]any
	if json.Unmarshal(body, &session) != nil {
		return regionUnknown(result, "region_session", "Disney+ 地区会话无法识别", started)
	}
	region := jsonStringField(session, "countryCode")
	supported, hasSupport := jsonBoolField(session, "inSupportedLocation")
	if validRegionCode(region) {
		region = strings.ToUpper(region)
		result.Details["service_region"] = region
	}
	if !validRegionCode(region) || !hasSupport {
		return regionUnknown(result, "region_session", "Disney+ 未提供完整地区判据", started)
	}
	if supported {
		result.Outcome, result.Summary = "unlocked", "Disney+ 服务地区判定通过；未验证账号或播放"
		return finish(result, started)
	}
	result.Outcome, result.Summary = "region_blocked", "Disney+ 当前地区暂不支持"
	result.ErrorMessage = result.Summary
	return finish(result, started)
}

func checkSpotifyRegion(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	// Deliberately omit email, password and agreement: this only reads signup validation geography.
	form := url.Values{"birth_day": {"11"}, "birth_month": {"11"}, "birth_year": {"2000"}, "platform": {"www"}, "key": {"a1e486e2729f46d6bb368d6b2bcda326"}, "identifier_token": {"invalid-region-check"}}
	body, result, ok := regionRequest(ctx, client, http.MethodPost, rule.TargetURL, "application/x-www-form-urlencoded", []byte(form.Encode()), rule.MaximumBodyBytes)
	if !ok {
		return finishPageResult(result, "Spotify 注册地区接口未取得结论", started)
	}
	var data struct {
		Status   int               `json:"status"`
		Country  string            `json:"country"`
		Launched *bool             `json:"is_country_launched"`
		Errors   map[string]string `json:"errors"`
	}
	if json.Unmarshal(body, &data) != nil {
		return regionUnknown(result, "region_document", "Spotify 注册地区响应无法识别", started)
	}
	result.Details = map[string]string{"service_region": strings.ToUpper(data.Country)}
	switch {
	case data.Status == 320 && strings.Contains(strings.ToLower(data.Errors["generic_error"]), "proxy"):
		result.Outcome, result.Summary = "service_rejected", "Spotify 拒绝代理 IP 注册"
	case data.Status == 320 || data.Status == 120 || (data.Launched != nil && !*data.Launched):
		result.Outcome, result.Summary = "region_blocked", "Spotify 当前国家未开放注册"
	case data.Status == 311 && validRegionCode(data.Country) && data.Launched != nil && *data.Launched:
		result.Outcome, result.Summary = "unlocked", "Spotify 注册地区可用；未注册账号或验证播放"
	default:
		return regionUnknown(result, "region_document", "Spotify 未返回完整注册地区判据", started)
	}
	return finish(result, started)
}

func checkTikTokRegion(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, page, ok := readPage(ctx, client, rule.TargetURL, []string{"tiktok.com"}, map[string]string{"Accept-Language": "en-US,en;q=0.9"}, rule.MaximumBodyBytes)
	if !ok {
		return finishPageResult(result, "TikTok 地区数据未能读取", started)
	}
	text := strings.ToLower(string(page.body))
	if strings.Contains(text, "not available in your region") || strings.Contains(text, "tiktok is not available") {
		result.Outcome, result.Summary = "region_blocked", "TikTok 明确提示当前地区不可用"
		return finish(result, started)
	}
	match := tiktokBootstrap.FindSubmatch(page.body)
	if len(match) != 2 {
		// TikTok also serves a compact JSON page state outside the hydration script.
		region := firstRegexGroup(regexp.MustCompile(`"region"\s*:\s*"([A-Z]{2})"`), string(page.body))
		if validRegionCode(region) {
			result.Details = map[string]string{"service_region": region, "region_field": "region"}
			result.Outcome, result.Summary = "unlocked", "TikTok 网页地区判定通过；未验证账号或推荐内容"
			return finish(result, started)
		}
		return regionUnknown(result, "region_document", "TikTok 首页没有地区启动数据", started)
	}
	var document map[string]any
	if json.Unmarshal(match[1], &document) != nil {
		return regionUnknown(result, "region_document", "TikTok 地区启动数据无法识别", started)
	}
	scope, _ := document["__DEFAULT_SCOPE__"].(map[string]any)
	app, _ := scope["webapp.app-context"].(map[string]any)
	region, _ := app["region"].(string)
	if !validRegionCode(region) {
		return regionUnknown(result, "region_document", "TikTok 启动数据没有有效地区代码", started)
	}
	result.Outcome, result.Summary = "unlocked", "TikTok 网页服务地区判定通过；未验证账号或推荐内容"
	result.Details = map[string]string{"service_region": strings.ToUpper(region)}
	return finish(result, started)
}

func checkUNextRegion(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	body, result, ok := regionRequest(ctx, client, http.MethodGet, rule.TargetURL, "", nil, rule.MaximumBodyBytes)
	if !ok {
		return finishPageResult(result, "U-NEXT 地区接口未提供判定状态", started)
	}
	var response map[string]any
	if json.Unmarshal(body, &response) != nil {
		return regionUnknown(result, "region_document", "U-NEXT 地区响应无法识别", started)
	}
	status := jsonNumberField(response, "resultStatus", "result_status")
	switch status {
	case "467":
		result.Outcome, result.Summary = "region_blocked", "U-NEXT 判定当前访问不在日本地区"
		result.ErrorMessage = result.Summary
	case "200", "475":
		result.Outcome, result.Summary = "unlocked", "U-NEXT 日本地区判定通过；未读取播放清单"
		result.Details = map[string]string{"service_region": "JP"}
	default:
		return regionUnknown(result, "region_status", "U-NEXT 未返回已知地区状态", started)
	}
	return finish(result, started)
}

func checkDMMRegion(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, page, ok := readPage(ctx, client, rule.TargetURL, []string{"dmm.com"}, nil, rule.MaximumBodyBytes)
	if !ok {
		return finishPageResult(result, "DMM TV 地区状态未确认", started)
	}
	text := strings.ToLower(string(page.body))
	if strings.Contains(strings.ToLower(page.finalURL), "not-available-in-your-region") || strings.Contains(text, "お住まいの地域からはご利用になれません") || strings.Contains(text, "not available in your region") {
		result.Outcome, result.Summary = "region_blocked", "DMM TV 明确拒绝当前地区"
		result.ErrorMessage = result.Summary
		return finish(result, started)
	}
	region := jsonRegionField(page.body, "serviceRegion", "countryCode", "country_code")
	if strings.EqualFold(region, "JP") {
		result.Outcome, result.Summary = "unlocked", "DMM TV 日本地区判定通过；未验证登录或播放"
		result.Details = map[string]string{"service_region": "JP"}
		return finish(result, started)
	}
	return regionUnknown(result, "region_document", "DMM TV 页面未提供明确地区字段", started)
}

func checkRadikoRegion(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, page, ok := readPage(ctx, client, rule.TargetURL, []string{"radiko.jp"}, nil, rule.MaximumBodyBytes)
	if !ok {
		return finishPageResult(result, "radiko 地区状态未确认", started)
	}
	text := strings.ToLower(string(page.body))
	if strings.Contains(text, "日本国内限定のサービス") || strings.Contains(text, "service is available only in japan") {
		result.Outcome, result.Summary = "region_blocked", "radiko 判定当前访问不在日本地区"
		result.ErrorMessage = result.Summary
		return finish(result, started)
	}
	match := radikoAreaPattern.FindSubmatch(page.body)
	if len(match) == 2 && strings.HasPrefix(strings.ToUpper(string(match[1])), "JP") {
		result.Outcome, result.Summary = "unlocked", "radiko 日本地区判定通过"
		result.Details = map[string]string{"service_region": "JP"}
		return finish(result, started)
	}
	if len(match) == 2 && strings.EqualFold(string(match[1]), "OUT") {
		result.Outcome, result.Summary = "region_blocked", "radiko 判定当前访问不在日本地区"
		result.ErrorMessage = result.Summary
		return finish(result, started)
	}
	return regionUnknown(result, "region_document", "radiko 页面未提供当前 areaId", started)
}

func regionRequest(ctx context.Context, client *http.Client, method, target, contentType string, body []byte, maximum int) ([]byte, Result, bool) {
	result := Result{RequestCount: 1}
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(string(body)))
	if err != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "request_setup", "无法创建地区请求"
		return nil, result, false
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if strings.Contains(target, "disney.api.edge.bamgrid.com") {
		auth := disneyRegionAuth
		if target == disneyGraphURL {
			auth = strings.TrimPrefix(auth, "Bearer ")
		}
		request.Header.Set("Authorization", auth)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	clone := *client
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := clone.Do(request)
	if err != nil {
		classifyRequestError(&result, ctx, err)
		return nil, result, false
	}
	defer response.Body.Close()
	status := response.StatusCode
	result.HTTPStatus = &status
	if serviceRateLimit(&result, response, false) {
		return nil, result, false
	}
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Outcome, result.Summary, result.ErrorMessage = "challenge", "服务要求浏览器验证", "地区接口返回网页挑战，未确认服务能力"
		return nil, result, false
	}
	if maximum <= 0 || maximum > MaximumResponseBytes {
		maximum = MaximumResponseBytes
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(maximum)+1))
	result.BytesRead = int64(min(len(data), maximum))
	if err != nil {
		classifyRequestError(&result, ctx, atRequestPhase(err, "response_read"))
		return nil, result, false
	}
	if len(data) > maximum {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "response_evidence_limit", "地区响应超过读取上限"
		return data[:maximum], result, false
	}
	if classifyPageChallenge(&result, data) {
		return data, result, false
	}
	if status < 200 || status >= 300 {
		if status == 467 && target == unextRegionURL {
			return data, result, true
		}
		result.Outcome, result.FailurePhase = "unknown", "http_status"
		result.ErrorMessage = fmt.Sprintf("地区接口返回 HTTP %d，未取得服务结论", status)
		if status == http.StatusUnauthorized {
			result.FailurePhase, result.ErrorMessage = "authentication", "地区接口要求鉴权，未取得服务结论"
		}
		return data, result, false
	}
	return data, result, true
}

func addRegionRequest(total *Result, part Result) {
	total.RequestCount += part.RequestCount
	total.BytesRead += part.BytesRead
	if part.HTTPStatus != nil {
		total.HTTPStatus = part.HTTPStatus
	}
	if part.Outcome != "" {
		total.Outcome, total.FailurePhase, total.ErrorMessage, total.Summary = part.Outcome, part.FailurePhase, part.ErrorMessage, part.Summary
	}
	for key, value := range part.Details {
		if total.Details == nil {
			total.Details = map[string]string{}
		}
		total.Details[key] = value
	}
}

func finishRegionFailure(result Result, body []byte, marker, message string, started time.Time) Result {
	if strings.Contains(strings.ToLower(string(body)), strings.ToLower(marker)) {
		result.Outcome, result.Summary, result.ErrorMessage = "region_blocked", message, message
	}
	return finish(result, started)
}

func finishPageResult(result Result, message string, started time.Time) Result {
	if result.Outcome == "http_rejected" {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "http_status", message
		if result.HTTPStatus != nil && *result.HTTPStatus == http.StatusUnauthorized {
			result.Outcome, result.FailurePhase, result.ErrorMessage = "authentication", "authentication", "服务要求鉴权，未取得地区结论"
		}
	}
	return finish(result, started)
}

func regionUnknown(result Result, phase, message string, started time.Time) Result {
	result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", phase, message
	return finish(result, started)
}

func validRegionCode(value string) bool { return regionCodePattern.MatchString(strings.ToUpper(value)) }

func jsonRegionField(body []byte, keys ...string) string {
	var document any
	if json.Unmarshal(body, &document) != nil {
		return ""
	}
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	var find func(any) string
	find = func(value any) string {
		switch item := value.(type) {
		case map[string]any:
			for key, child := range item {
				if allowed[key] {
					if code, ok := child.(string); ok && validRegionCode(code) {
						return strings.ToUpper(code)
					}
				}
			}
			for _, child := range item {
				if code := find(child); code != "" {
					return code
				}
			}
		case []any:
			for _, child := range item {
				if code := find(child); code != "" {
					return code
				}
			}
		}
		return ""
	}
	return find(document)
}

func jsonStringField(value any, key string) string {
	switch item := value.(type) {
	case map[string]any:
		if text, ok := item[key].(string); ok {
			return text
		}
		for _, child := range item {
			if text := jsonStringField(child, key); text != "" {
				return text
			}
		}
	case []any:
		for _, child := range item {
			if text := jsonStringField(child, key); text != "" {
				return text
			}
		}
	}
	return ""
}

func jsonBoolField(value any, key string) (bool, bool) {
	switch item := value.(type) {
	case map[string]any:
		if result, ok := item[key].(bool); ok {
			return result, true
		}
		for _, child := range item {
			if result, ok := jsonBoolField(child, key); ok {
				return result, true
			}
		}
	case []any:
		for _, child := range item {
			if result, ok := jsonBoolField(child, key); ok {
				return result, true
			}
		}
	}
	return false, false
}

func jsonNumberField(value any, keys ...string) string {
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	var find func(any) string
	find = func(value any) string {
		switch item := value.(type) {
		case map[string]any:
			for key, child := range item {
				if allowed[key] {
					switch number := child.(type) {
					case float64:
						return fmt.Sprintf("%.0f", number)
					case string:
						return number
					}
				}
			}
			for _, child := range item {
				if number := find(child); number != "" {
					return number
				}
			}
		case []any:
			for _, child := range item {
				if number := find(child); number != "" {
					return number
				}
			}
		}
		return ""
	}
	return find(value)
}

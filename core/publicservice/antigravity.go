package publicservice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const antigravityBase = "https://daily-cloudcode-pa.googleapis.com/v1internal:"

// Observe a separate, credential-free endpoint alongside the model request.
// It is an exit observation, never proof of the IP seen by Google's service.
func checkAntigravityObserved(ctx context.Context, client *http.Client, token string, started time.Time) Result {
	observed := make(chan Result, 1)
	go func() {
		exitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		traceClient := *client
		traceClient.Jar = nil
		observed <- checkCloudflareTrace(exitCtx, &traceClient, Rule{TargetURL: "https://www.cloudflare.com/cdn-cgi/trace", MaximumBodyBytes: 8192}, started)
	}()
	r := checkAntigravity(ctx, client, token, started)
	if ctx.Err() == nil && (r.Outcome == "matched" || r.Outcome == "region_blocked" || r.Outcome == "permission_denied" || r.Outcome == "unknown" || r.Outcome == "service_rejected") {
		if r.Details == nil {
			r.Details = map[string]string{}
		}
		googleRegion, geoBlocked, geoReqs, geoBytes, geoErr := fetchAntigravityGoogleRegion(ctx, client, token)
		r.RequestCount += geoReqs
		r.BytesRead += geoBytes
		if googleRegion != "" {
			r.Details["google_region"] = googleRegion
			r.Details["service_region"] = googleRegion
		}
		if geoBlocked {
			r.Outcome = "region_blocked"
			r.FailurePhase = "fetchUserInfo"
			r.Details["antigravity_progress"] = "Google 内部地区判定受限"
			switch {
			case googleRegion == "CN":
				r.ErrorMessage = "Google 内部地区判定为 CN（送中），限制使用（不以生成接口 200 误判可用）"
			case googleRegion != "":
				r.ErrorMessage = "Google 内部地区判定为 " + googleRegion + "，限制使用（不以生成接口 200 误判可用）"
			case geoErr != "":
				r.ErrorMessage = geoErr
			default:
				r.ErrorMessage = "Google 内部地区判定当前出口不支持"
			}
		}
	}
	exit := <-observed
	if r.Details == nil {
		r.Details = map[string]string{}
	}
	r.RequestCount += exit.RequestCount
	r.BytesRead += exit.BytesRead
	r.Details["exit_observation"] = "独立 Cloudflare 请求；不代表 Google 实际看到的出口"
	if exit.Outcome == "profiled" {
		for _, key := range []string{"ip", "country_code", "cloudflare_colo"} {
			r.Details[key] = exit.Details[key]
		}
	} else {
		r.Details["exit_observation_state"] = "未获取到出口；不影响模型判定"
	}
	if r.Outcome == "matched" {
		r.Summary, r.ErrorMessage = r.ErrorMessage, ""
	}
	return r
}

func fetchAntigravityGoogleRegion(ctx context.Context, client *http.Client, token string) (regionCode string, blocked bool, reqCount int, bytesRead int64, errMsg string) {
	if client == nil || strings.TrimSpace(token) == "" {
		return "", false, 0, 0, ""
	}
	geoCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(geoCtx, http.MethodPost, antigravityBase+"fetchUserInfo", bytes.NewBufferString("{}"))
	if err != nil {
		return "", false, 0, 0, ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "antigravity/2.14.0")
	reqCount = 1
	resp, err := client.Do(req)
	if err != nil {
		return "", false, reqCount, 0, ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	bytesRead = int64(len(body))
	if err != nil && len(body) == 0 {
		return "", false, reqCount, bytesRead, ""
	}
	lower := strings.ToLower(string(body))
	if resp.StatusCode >= 400 && (strings.Contains(lower, "user location is not supported") || strings.Contains(lower, "location is not supported for the api")) {
		return "", true, reqCount, bytesRead, "Google 内部地区接口拒绝当前出口"
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, reqCount, bytesRead, ""
	}
	var info struct {
		RegionCode string `json:"regionCode"`
	}
	if json.Unmarshal(body, &info) == nil {
		code := strings.ToUpper(strings.TrimSpace(info.RegionCode))
		if len(code) == 2 {
			regionCode = code
			if code == "CN" || code == "HK" || code == "MO" {
				return regionCode, true, reqCount, bytesRead, ""
			}
		}
	}
	return regionCode, false, reqCount, bytesRead, ""
}

// checkAntigravity uses the existing internal service route, not a public API
// guarantee. Unexpected schemas fail closed. Only a model content response is
// success; authentication, permission and quota responses are not availability.
func checkAntigravity(ctx context.Context, client *http.Client, token string, started time.Time) Result {
	r := Result{Details: map[string]string{"antigravity_progress": "尚未确认账号项目"}}
	end := func(outcome, phase, message string) Result {
		r.Outcome, r.FailurePhase, r.ErrorMessage = outcome, phase, message
		return finish(r, started)
	}
	call := func(step string, payload any) ([]byte, bool) {
		r.FailurePhase = step
		r.HTTPStatus = nil // A later pre-header failure has no response status of its own.
		encoded, err := json.Marshal(payload)
		if err != nil {
			r.Outcome, r.ErrorMessage = "unknown", "无法构造检测请求"
			return nil, false
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, antigravityBase+step, bytes.NewReader(encoded))
		if err != nil {
			r.Outcome, r.ErrorMessage = "unknown", "无法构造检测请求"
			return nil, false
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("User-Agent", "antigravity/2.14.0")
		r.RequestCount++
		resp, err := client.Do(req)
		if err != nil {
			antigravityReadError(&r, ctx, err)
			return nil, false
		}
		defer resp.Body.Close()
		status := resp.StatusCode
		r.HTTPStatus = &status
		if serviceRateLimit(&r, resp, false) {
			// Keep the account/catalog/model step visible. The HTTP quota is
			// already known; a slow error body must not turn it into a timeout.
			r.FailurePhase = step
			return nil, false
		}
		// The model catalog is substantially larger than an ordinary probe
		// response. Keep a bounded cap, but do not misclassify a valid catalog
		// as unavailable just because it exceeds the generic 64 KiB limit.
		maximum := 1024 * 1024
		if step == "fetchAvailableModels" {
			maximum = 4 * 1024 * 1024
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maximum)+1))
		r.BytesRead += int64(min(len(body), maximum))
		if err != nil {
			antigravityReadError(&r, ctx, atRequestPhase(err, "response_read"))
			return nil, false
		}
		if len(body) > maximum {
			r.Outcome, r.ErrorMessage = "unknown", "服务响应过大，未能完整读取；这不代表节点不可用"
			return nil, false
		}
		var envelope struct {
			Error struct {
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &envelope)
		lower := strings.ToLower(envelope.Error.Message)
		switch {
		case status >= 400 && (strings.Contains(lower, "user location is not supported") || strings.Contains(lower, "location is not supported for the api")):
			r.Outcome, r.ErrorMessage = "region_blocked", "服务明确返回当前地区不支持；不据此推断出口国家"
		case status == 401 || envelope.Error.Status == "UNAUTHENTICATED":
			r.Outcome, r.ErrorMessage = "auth_failed", "Google 凭据无效或已过期，请在设置中重新绑定"
		case status == 429 || envelope.Error.Status == "RESOURCE_EXHAUSTED":
			r.Outcome, r.ErrorMessage = "rate_limited", "账号额度或请求频率受限，当前未验证模型可用"
		case status == 403 || envelope.Error.Status == "PERMISSION_DENIED":
			r.Outcome, r.ErrorMessage = "permission_denied", "账号、订阅或项目权限不足；不能据此判断节点或地区不可用"
		case status >= 300 && status < 400:
			r.Outcome, r.ErrorMessage = "redirect", "服务返回重定向，未携带凭据跟随"
		case status != 200 || envelope.Error.Status != "" || envelope.Error.Message != "":
			r.Outcome, r.ErrorMessage = "unknown", "服务未接受当前检测请求，可能是接口或请求格式发生变化"
		default:
			return body, true
		}
		return nil, false
	}

	// No fabricated project ID or model name: both must be returned by the service.
	body, ok := call("loadCodeAssist", map[string]any{"metadata": map[string]string{"ideType": "ANTIGRAVITY"}})
	if !ok {
		return finish(r, started)
	}
	var config struct {
		Project json.RawMessage `json:"cloudaicompanionProject"`
	}
	if json.Unmarshal(body, &config) != nil {
		return end("unknown", "loadCodeAssist", "账号配置响应无法识别")
	}
	var project string
	if json.Unmarshal(config.Project, &project) != nil {
		var object struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(config.Project, &object)
		project = object.ID
	}
	if strings.TrimSpace(project) == "" {
		return end("setup_required", "loadCodeAssist", "账号尚未返回可用项目，请先在 Antigravity 完成登录与开通")
	}
	r.Details["antigravity_progress"] = "账号项目已返回，尚未确认可用模型"

	body, ok = call("fetchAvailableModels", map[string]string{"project": project})
	if !ok {
		return finish(r, started)
	}
	var listing struct {
		Models map[string]json.RawMessage `json:"models"`
	}
	if json.Unmarshal(body, &listing) != nil || len(listing.Models) == 0 {
		return end("unknown", "fetchAvailableModels", "未获取到可识别的模型列表，不能确认可用性")
	}
	models := make([]string, 0)
	for id := range listing.Models {
		if strings.HasPrefix(id, "gemini-") && strings.Contains(id, "flash") && len(id) <= 128 {
			models = append(models, id)
		}
	}
	sort.Strings(models)
	if len(models) == 0 {
		return end("setup_required", "fetchAvailableModels", "账号未提供轻量 Gemini Flash 模型，本次不改用高成本模型")
	}
	r.Model = models[0]
	r.Details["checked_model"] = r.Model
	r.Details["models_listed"] = strconv.Itoa(len(models))
	r.Details["all_models_count"] = strconv.Itoa(len(listing.Models))
	r.Details["antigravity_progress"] = "轻量模型已列出，尚未收到真实回答"
	body, ok = call("streamGenerateContent?alt=sse", map[string]any{
		"project": project, "model": r.Model,
		"request": map[string]any{
			"contents":         []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": "Reply with only the word READY."}}}},
			"generationConfig": map[string]any{"maxOutputTokens": 16},
		},
	})
	if !ok {
		return finish(r, started)
	}
	replyText, refusalOutcome, refusalMsg := antigravityInspectModelResponse(body)
	if refusalOutcome != "" {
		return end(refusalOutcome, "model_response", refusalMsg)
	}
	if replyText == "" {
		return end("unknown", "model_response", "收到了响应，但没有可确认的模型输出；不标记可用")
	}
	r.Details["reply_preview"] = truncatePreview(replyText, 48)
	r.Details["antigravity_progress"] = "真实模型已回答"
	return end("matched", "", "本次通过所选节点收到模型输出；仅代表当前账号、模型与测试时刻可用")
}

func antigravityReadError(r *Result, ctx context.Context, err error) {
	step := r.FailurePhase
	classifyRequestError(r, ctx, err)
	// Keep the model workflow step in the existing field; the precise request
	// stage and timeout origin are separate Details shared with other probes.
	r.FailurePhase = step
}

func antigravityHasContent(body []byte) bool {
	reply, outcome, _ := antigravityInspectModelResponse(body)
	return outcome == "" && reply != ""
}

func truncatePreview(s string, maxRunes int) string {
	s = strings.Join(strings.Fields(s), " ")
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	out := make([]rune, 0, maxRunes+1)
	for _, r := range s {
		if len(out) >= maxRunes {
			break
		}
		out = append(out, r)
	}
	return string(out) + "…"
}

func antigravityBodyRegionRefusal(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range []string{
		"user location is not supported",
		"location is not supported for the api",
		"not available in your country",
		"not available in your region",
		"not supported in your region",
		"country is not supported",
		"unsupported region",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func antigravityExtractText(body []byte) string {
	reply, outcome, _ := antigravityInspectModelResponse(body)
	if outcome != "" {
		return ""
	}
	return reply
}

func antigravityInspectModelResponse(body []byte) (reply string, refusalOutcome string, refusalMsg string) {
	type candidate struct {
		FinishReason string `json:"finishReason"`
		Content      struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
	}
	type feedback struct {
		BlockReason string `json:"blockReason"`
	}
	type response struct {
		Candidates     []candidate     `json:"candidates"`
		PromptFeedback *feedback       `json:"promptFeedback"`
		Error          json.RawMessage `json:"error"`
	}
	blockMarker := ""
	inspectBlock := func(fb *feedback, cands []candidate) {
		if fb != nil && strings.TrimSpace(fb.BlockReason) != "" && blockMarker == "" {
			blockMarker = strings.TrimSpace(fb.BlockReason)
		}
		for _, c := range cands {
			reason := strings.ToUpper(strings.TrimSpace(c.FinishReason))
			switch reason {
			case "SAFETY", "RECITATION", "PROHIBITED_CONTENT", "BLOCKLIST", "SPII":
				if blockMarker == "" {
					blockMarker = reason
				}
			}
		}
	}
	extract := func(data []byte) (string, bool) {
		var item struct {
			Response       response        `json:"response"`
			Candidates     []candidate     `json:"candidates"`
			PromptFeedback *feedback       `json:"promptFeedback"`
			Error          json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &item) != nil || len(item.Error) > 0 || len(item.Response.Error) > 0 {
			return "", false
		}
		inspectBlock(item.PromptFeedback, item.Candidates)
		inspectBlock(item.Response.PromptFeedback, item.Response.Candidates)
		var sb strings.Builder
		for _, c := range append(item.Response.Candidates, item.Candidates...) {
			for _, p := range c.Content.Parts {
				if !p.Thought && strings.TrimSpace(p.Text) != "" {
					sb.WriteString(p.Text)
				}
			}
		}
		text := strings.TrimSpace(sb.String())
		return text, text != ""
	}
	if antigravityBodyRegionRefusal(string(body)) {
		return "", "region_blocked", "模型接口正文返回当前地区不支持"
	}
	if text, ok := extract(body); ok {
		return text, "", ""
	}
	var sseText strings.Builder
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			if string(data) == "[DONE]" {
				continue
			}
			var item map[string]json.RawMessage
			if json.Unmarshal(data, &item) != nil || item["error"] != nil {
				return "", "", ""
			}
			var nested response
			if json.Unmarshal(item["response"], &nested) == nil && len(nested.Error) > 0 {
				return "", "", ""
			}
			if part, ok := extract(data); ok {
				sseText.WriteString(part)
			}
		}
	}
	final := strings.TrimSpace(sseText.String())
	if final != "" && antigravityBodyRegionRefusal(final) {
		return "", "region_blocked", "模型回复正文提示当前地区不支持"
	}
	if final == "" && blockMarker != "" {
		return "", "service_rejected", "模型请求被服务策略拦截（" + blockMarker + "）"
	}
	return final, "", ""
}

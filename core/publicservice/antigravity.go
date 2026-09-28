package publicservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
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
		// The model catalog is substantially larger than an ordinary probe
		// response. Keep a bounded cap, but do not misclassify a valid catalog
		// as unavailable just because it exceeds the generic 64 KiB limit.
		maximum := MaximumResponseBytes
		if step == "fetchAvailableModels" {
			maximum = 512 * 1024
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maximum)+1))
		r.BytesRead += int64(min(len(body), maximum))
		if err != nil {
			antigravityReadError(&r, ctx, err)
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
	r.Details["antigravity_progress"] = "轻量模型已列出，尚未收到真实回答"
	body, ok = call("streamGenerateContent?alt=sse", map[string]any{
		"project": project, "model": r.Model,
		"request": map[string]any{
			"contents":         []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": "Reply OK."}}}},
			"generationConfig": map[string]any{"maxOutputTokens": 16},
		},
	})
	if !ok {
		return finish(r, started)
	}
	if !antigravityHasContent(body) {
		return end("unknown", "model_response", "收到了响应，但没有可确认的模型输出；不标记可用")
	}
	r.Details["antigravity_progress"] = "真实模型已回答"
	return end("matched", "", "本次通过所选节点收到模型输出；仅代表当前账号、模型与测试时刻可用")
}

func antigravityReadError(r *Result, ctx context.Context, err error) {
	var ne net.Error
	switch {
	case ctx.Err() == context.Canceled || errors.Is(err, context.Canceled):
		r.Outcome, r.ErrorMessage = "cancelled", "检测已取消"
	case ctx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		r.Outcome, r.ErrorMessage = "timed_out", "检测超过总时长上限"
	default:
		r.Outcome, r.ErrorMessage = "transport_error", "所选节点的服务连接或响应读取失败"
	}
}

func antigravityHasContent(body []byte) bool {
	type candidate struct {
		Content struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
	}
	type response struct {
		Candidates []candidate     `json:"candidates"`
		Error      json.RawMessage `json:"error"`
	}
	inspect := func(data []byte) bool {
		var item struct {
			Response   response        `json:"response"`
			Candidates []candidate     `json:"candidates"`
			Error      json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &item) != nil || len(item.Error) > 0 || len(item.Response.Error) > 0 {
			return false
		}
		for _, c := range append(item.Response.Candidates, item.Candidates...) {
			for _, p := range c.Content.Parts {
				if !p.Thought && strings.TrimSpace(p.Text) != "" {
					return true
				}
			}
		}
		return false
	}
	if inspect(body) {
		return true
	}
	found := false
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			if string(data) == "[DONE]" {
				continue
			}
			var item map[string]json.RawMessage
			if json.Unmarshal(data, &item) != nil || item["error"] != nil {
				return false
			}
			var nested response
			if json.Unmarshal(item["response"], &nested) == nil && len(nested.Error) > 0 {
				return false
			}
			found = inspect(data) || found
		}
	}
	return found
}

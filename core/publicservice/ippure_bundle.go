package publicservice

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
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

const ippureCFURL = "https://cf.999831.xyz/api/cf-summary"
const ippureCFGateway = ippureWebAPIBase + "/api/gateway/cf"

type ippureOriginTransport struct{ base http.RoundTripper }

func (t ippureOriginTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	if req.URL.Host == "api.123169.xyz" {
		clone.Header.Set("Origin", "https://ippure.com")
		clone.Header.Set("Referer", "https://ippure.com/")
	}
	return t.base.RoundTrip(clone)
}
func (t ippureOriginTransport) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// Match the site's normal signed API request: SHA256(x-t) AES-CBC key,
// random IV followed by PKCS7-padded ciphertext. No browser cookies are copied.
func ippureEncryptedBody(request *http.Request, raw []byte, xt string) error {
	key := sha256.Sum256([]byte(xt))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	pad := aes.BlockSize - len(raw)%aes.BlockSize
	plain := append(append([]byte(nil), raw...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	body := make([]byte, aes.BlockSize+len(plain))
	if _, err = rand.Read(body[:aes.BlockSize]); err != nil {
		return err
	}
	cipher.NewCBCEncrypter(block, body[:aes.BlockSize]).CryptBlocks(body[aes.BlockSize:], plain)
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.GetBody = nil
	return nil
}

// At most one replay for the site's initial nonce response; other failures stop.
func ippureExtraExchange(ctx context.Context, client *http.Client, method, target string, payload []byte) (Result, []byte, bool) {
	state := newIPPureNonceState()
	local := ippureWebClient(client, state, true)
	total := Result{}
	for pass := 0; pass < 2; pass++ {
		part := Result{RequestCount: 1}
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
		if err != nil {
			return part, nil, false
		}
		req.Header.Set("User-Agent", browserUserAgent)
		req.Header.Set("Accept", "application/json")
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/octet-stream")
		}
		response, err := local.Do(req)
		if err != nil {
			classifyRequestError(&part, ctx, err)
			ippureAccumulate(&total, part)
			ippureCarryFailure(&total, part)
			return total, nil, false
		}
		code := response.StatusCode
		part.HTTPStatus = &code
		body, readErr := io.ReadAll(io.LimitReader(response.Body, ippureWebBodyLimit+1))
		response.Body.Close()
		part.BytesRead = int64(min(len(body), ippureWebBodyLimit))
		if serviceRateLimit(&part, response, false) {
			ippureAccumulate(&total, part)
			ippureCarryFailure(&total, part)
			return total, nil, false
		}
		if ippureHasCFChallengeMarker(response.Header) {
			part.Outcome = "challenge"
			part.ErrorMessage = "验证页拦截"
		} else if code != 200 {
			part.Outcome = "http_rejected"
			part.ErrorMessage = fmt.Sprintf("HTTP %d", code)
		} else if readErr != nil {
			classifyRequestError(&part, ctx, readErr)
		} else if len(body) > ippureWebBodyLimit {
			ippureUnknown(&part, "response_limit", "响应超过读取上限")
		}
		ippureAccumulate(&total, part)
		if part.Outcome != "" {
			ippureCarryFailure(&total, part)
			return total, nil, false
		}
		hasNonce, protocolError, rechallenged := state.status()
		if protocolError != "" || rechallenged {
			ippureChallenge(&total)
			return total, nil, false
		}
		if pass == 0 && hasNonce {
			if !state.firstNonceResponseAllowsReplay(body, true) {
				ippureUnknown(&total, "nonce_protocol", "签名响应格式无法识别")
				return total, nil, false
			}
			continue
		}
		return total, body, true
	}
	ippureChallenge(&total)
	return total, nil, false
}
func ippureExtraStatus(r Result) string {
	if r.HTTPStatus != nil && *r.HTTPStatus != 200 {
		return fmt.Sprintf("HTTP %d", *r.HTTPStatus)
	}
	switch r.Outcome {
	case "cancelled":
		return "已取消"
	case "timed_out":
		return "查询超时"
	case "challenge":
		return "验证页拦截"
	case "rate_limited":
		return "目标限流"
	}
	return "未取得结果"
}

func checkIPPureBundle(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	local := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	local.Transport = ippureOriginTransport{base}
	result := checkIPPureWeb(ctx, &local, rule, started)
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	d := result.Details
	d["profile_bundle_version"] = "1"
	d["cf_status"] = "未查询"
	d["asn_traffic_status"] = "未查询"
	// Each sub-result keeps its own scope/time. Its failure cannot overwrite the
	// IPPure score, HTTP status, or outcome of the primary lookup.
	add := func(part Result) { result.RequestCount += part.RequestCount; result.BytesRead += part.BytesRead }
	if ctx.Err() != nil {
		return finish(result, started)
	}
	cfAt := time.Now().UTC()
	d["cf_query_at"] = cfAt.Format(time.RFC3339Nano)
	d["cf_source_url"] = "https://ippure.com/cloudflare"
	d["cf_scope"] = "本次App请求环境，受出口与客户端特征影响，不等于浏览器结果"
	part, payload, ok := boundedGET(ctx, &local, ippureCFURL, ippureWebBodyLimit, "application/octet-stream")
	add(part)
	asn := strings.TrimPrefix(strings.ToUpper(d["asn"]), "AS")
	if ok {
		part, body, valid := ippureExtraExchange(ctx, &local, http.MethodPost, ippureCFGateway, payload)
		add(part)
		if valid {
			var doc struct {
				IP   string          `json:"ip"`
				Risk *int            `json:"risk_score"`
				ASN  json.RawMessage `json:"asn"`
			}
			if json.Unmarshal(body, &doc) == nil && net.ParseIP(doc.IP) != nil && doc.Risk != nil && *doc.Risk >= 0 && *doc.Risk <= 100 {
				d["cf_risk_score"] = strconv.Itoa(*doc.Risk)
				d["cf_status"] = "已取得"
				d["cf_ip"] = doc.IP
				d["cf_ip_family"] = "IPv6"
				if net.ParseIP(doc.IP).To4() != nil {
					d["cf_ip_family"] = "IPv4"
				}
				if asn == "" {
					asn = strings.TrimPrefix(cleanASN(doc.ASN), "AS")
				}
			} else {
				d["cf_status"] = "未返回有效系数"
			}
		} else {
			d["cf_status"] = ippureExtraStatus(part)
		}
	} else {
		d["cf_status"] = ippureExtraStatus(part)
	}
	asnNumber, err := strconv.ParseUint(asn, 10, 32)
	if err != nil || asnNumber == 0 {
		d["asn_traffic_status"] = "未取得ASN"
		return finish(result, started)
	}
	d["asn_traffic_asn"] = "AS" + asn
	d["asn_traffic_scope"] = "整个ASN的统计，并非当前节点的流量比例"
	d["asn_traffic_query_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	d["asn_traffic_source_url"] = "https://ippure.com/guide/ASN-Human-Bot-Ratio"
	if ctx.Err() != nil {
		d["asn_traffic_status"] = "本次时限已到"
		return finish(result, started)
	}
	part, body, ok := ippureExtraExchange(ctx, &local, http.MethodGet, ippureWebAPIBase+"/api/info/asn/botclass/"+asn, nil)
	add(part)
	if !ok {
		d["asn_traffic_status"] = ippureExtraStatus(part)
		return finish(result, started)
	}
	var doc struct {
		OK   bool `json:"ok"`
		Data struct {
			Human *float64 `json:"human"`
			Bot   *float64 `json:"bot"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &doc) != nil || !doc.OK || doc.Data.Human == nil || doc.Data.Bot == nil || *doc.Data.Human < 0 || *doc.Data.Human > 100 || *doc.Data.Bot < 0 || *doc.Data.Bot > 100 || math.Abs(*doc.Data.Human+*doc.Data.Bot-100) > 0.1 {
		d["asn_traffic_status"] = "统计数据不足"
		return finish(result, started)
	}
	d["asn_human_percent"] = strconv.FormatFloat(*doc.Data.Human, 'f', 2, 64)
	d["asn_bot_percent"] = strconv.FormatFloat(*doc.Data.Bot, 'f', 2, 64)
	d["asn_traffic_status"] = "已取得"
	return finish(result, started)
}

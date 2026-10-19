package publicservice

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/faceair/clash-speedtest/core/monitor"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIPPureBundleGroupsThreeIndependentMetrics(t *testing.T) {
	counts := map[string]int{}
	payload := []byte{0xff, 0xfe, 0, 0xe2, 0x82, 0, 1}
	key := "ephemeral-bundle-key"
	checker := ippureChecker(func(req *http.Request) (*http.Response, error) {
		path := req.URL.Path
		counts[path]++
		if req.URL.Host == "api.ipify.org" {
			return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8"}`), nil
		}
		if req.URL.Host == "api.123169.xyz" && (req.Header.Get("Origin") != "https://ippure.com" || req.Header.Get("Referer") != "https://ippure.com/") {
			t.Fatal("missing API origin context")
		}
		nonce := func() *http.Response {
			r := fixtureResponse(req, 200, "text/plain;charset=UTF-8", `{"ok":false}`)
			r.Header.Set("x-k", key)
			r.Header.Set("x-t", strconv.FormatInt(time.Now().UnixMilli(), 10))
			return r
		}
		switch path {
		case "/api/info/ip-basic/8.8.8.8":
			if counts[path] == 1 {
				return nonce(), nil
			}
			ippureSignedTimestamp(t, req, key, req.URL.String())
			return ippureJSONResponse(req, 200, `{"ok":true,"data":{"ip":"8.8.8.8","asn":{"number":15169}}}`), nil
		case "/api/info/ip-risk/8.8.8.8":
			return ippureJSONResponse(req, 200, `{"ok":true,"data":{"ip":"8.8.8.8","risk_score":0}}`), nil
		case "/api/cf-summary":
			return fixtureResponse(req, 200, "application/octet-stream", string(payload)), nil
		case "/api/gateway/cf":
			if counts[path] == 1 {
				return nonce(), nil
			}
			body, _ := io.ReadAll(req.Body)
			xt := req.Header.Get("x-t")
			k := sha256.Sum256([]byte(xt))
			block, _ := aes.NewCipher(k[:])
			if len(body) < 32 || len(body)%16 != 0 {
				t.Fatal("invalid encrypted gateway body")
			}
			plain := make([]byte, len(body)-16)
			cipher.NewCBCDecrypter(block, body[:16]).CryptBlocks(plain, body[16:])
			pad := int(plain[len(plain)-1])
			plain = plain[:len(plain)-pad]
			if !bytes.Equal(plain, payload) {
				t.Fatal("gateway payload changed")
			}
			ts := strings.SplitN(xt, "-", 2)[0]
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write([]byte("POST-" + req.URL.String() + "-" + "\ufffd\ufffd\x00\ufffd\x00\x01" + "-" + ts))
			if xt != ts+"-"+hex.EncodeToString(mac.Sum(nil)) {
				t.Fatal("gateway signature not using original browser UTF-8 payload")
			}
			return ippureJSONResponse(req, 200, `{"ip":"2001:4860::8888","risk_score":61,"asn":15169}`), nil
		case "/api/info/asn/botclass/15169":
			if counts[path] == 1 {
				return nonce(), nil
			}
			ippureSignedTimestamp(t, req, key, req.URL.String())
			return ippureJSONResponse(req, 200, `{"ok":true,"data":{"human":5.78,"bot":94.22}}`), nil
		default:
			t.Fatalf("unexpected request %s", req.URL)
			return nil, nil
		}
	})
	rule, _ := RuleFor("ippure_ip_quality")
	rule.RuleVersion = 6
	r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "profiled" || r.Details["fraud_score"] != "0" || r.Details["cf_risk_score"] != "61" || r.Details["cf_ip_family"] != "IPv6" || r.Details["asn_human_percent"] != "5.78" || r.Details["asn_bot_percent"] != "94.22" || r.RequestCount != 9 {
		t.Fatalf("incomplete bundle: %+v", r)
	}
	b, _ := json.Marshal(r)
	if bytes.Contains(b, []byte(key)) {
		t.Fatal("nonce persisted")
	}
}

func TestIPPureBundleDoesNotLoseRiskWhenExtraSourcesReject(t *testing.T) {
	checker := ippureChecker(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.ipify.org" {
			return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8"}`), nil
		}
		if req.URL.Host == "my.ippure.com" {
			return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8","fraudScore":20,"asn":15169}`), nil
		}
		if req.URL.Host == "cf.999831.xyz" {
			return fixtureResponse(req, 200, "application/octet-stream", "opaque"), nil
		}
		return fixtureResponse(req, 403, "text/plain", "403"), nil
	})
	rule, _ := RuleFor("ippure_ip_quality")
	rule.RuleVersion = 6
	r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "profiled" || r.Details["fraud_score"] != "20" || r.HTTPStatus == nil || *r.HTTPStatus != 200 || r.Details["cf_risk_score"] != "" || r.Details["asn_human_percent"] != "" || !strings.Contains(r.Details["cf_status"], "403") || !strings.Contains(r.Details["asn_traffic_status"], "403") {
		t.Fatalf("extras overwrote main result or invented scores: %+v", r)
	}
}

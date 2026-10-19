package publicservice

import (
	"context"
	"crypto/x509"
	"errors"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIPPureV7UsesOfficialRiskAndAcceptsNormalNonceRefresh(t *testing.T) {
	counts := map[string]int{}
	checker := ippureChecker(func(req *http.Request) (*http.Response, error) {
		counts[req.URL.Path]++
		switch req.URL.Host + req.URL.Path {
		case "my.ippure.com/v1/info":
			return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8","fraudScore":25,"asn":15169}`), nil
		case "cf.999831.xyz/api/cf-summary":
			return fixtureResponse(req, 200, "application/octet-stream", "opaque"), nil
		case "api.123169.xyz/api/gateway/cf":
			if counts[req.URL.Path] <= 2 {
				r := fixtureResponse(req, 200, "text/plain", `{"ok":false}`)
				r.Header.Set("x-k", "normal-key-"+strconv.Itoa(counts[req.URL.Path]))
				r.Header.Set("x-t", strconv.FormatInt(time.Now().UnixMilli(), 10))
				return r, nil
			}
			return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8","risk_score":99,"asn":15169}`), nil
		case "api.123169.xyz/api/info/asn/botclass/15169":
			return ippureJSONResponse(req, 200, `{"ok":true,"data":{"human":5.78,"bot":94.22}}`), nil
		default:
			t.Fatalf("unexpected request %s", req.URL)
			return nil, nil
		}
	})
	rule, _ := RuleFor("ippure_ip_quality")
	rule.RuleVersion = 7
	r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "profiled" || r.Details["fraud_score"] != "25" || r.Details["cf_risk_score"] != "99" || r.Details["asn_human_percent"] != "5.78" || counts["/api/gateway/cf"] != 3 {
		t.Fatalf("independent bundle failed: %+v", r)
	}
}

func TestIPPureV7KeepsPrimaryAndExplainsUnfinishedExtras(t *testing.T) {
	checker := ippureChecker(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "my.ippure.com" {
			return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8","fraudScore":0,"asn":15169}`), nil
		}
		if strings.Contains(req.URL.Path, "cf-summary") {
			return fixtureResponse(req, 503, "text/plain", "unavailable"), nil
		}
		return fixtureResponse(req, 403, "text/plain", "denied"), nil
	})
	rule, _ := RuleFor("ippure_ip_quality")
	rule.RuleVersion = 7
	r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Details["fraud_score"] != "0" || r.Details["cf_status"] == "未查询" || r.Details["asn_traffic_status"] == "未查询" {
		t.Fatal(r)
	}
}

func TestIPPureV7NonceRotationStopsAtWebsiteLimit(t *testing.T) {
	state := newIPPureNonceState()
	state.normalRefreshLimit = 3
	calls := 0
	client := &http.Client{Transport: resourceTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		r := fixtureResponse(req, 200, "text/plain", `{"ok":false}`)
		r.Header.Set("x-k", "rotating-"+strconv.Itoa(calls))
		r.Header.Set("x-t", strconv.FormatInt(time.Now().UnixMilli(), 10))
		return r, nil
	})}
	r, _, ok := ippureExtraExchange(context.Background(), client, http.MethodPost, ippureCFGateway, []byte("payload"), state)
	if ok || calls != 5 || r.FailurePhase != "nonce_challenge" {
		t.Fatalf("unbounded replay: %d %+v", calls, r)
	}
}

func TestIPPureBrowserTransportKeepsNodeDialAndCertificateValidation(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("untrusted certificate reached HTTP") }))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	called := false
	tr := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		called = true
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	client := &http.Client{Transport: ippureBrowserSession(tr), Timeout: time.Second}
	defer client.CloseIdleConnections()
	_, err := client.Get(ippureWebAPIBase + "/api/gateway/cf")
	var certErr x509.UnknownAuthorityError
	if !called || err == nil || !errors.As(err, &certErr) || !tr.DisableKeepAlives {
		t.Fatalf("dial/certificate/original transport changed: called=%v err=%v", called, err)
	}
}

package publicservice

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestIPPureV7UsesOfficialRiskAndAcceptsNormalNonceRefresh(t *testing.T) {
	counts := map[string]int{}
	var mu sync.Mutex
	checker := ippureChecker(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
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
	for _, tc := range []struct {
		name, certHost string
		trusted        bool
	}{
		{"untrusted", "api.123169.xyz", false},
		{"trusted", "api.123169.xyz", true},
		{"wrong_hostname", "example.com", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{tc.certHost}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
			der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
			if err != nil {
				t.Fatal(err)
			}
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				t.Fatal(err)
			}
			// Explicit roots avoid platform-specific system verifier/error ordering.
			roots := x509.NewCertPool()
			if tc.trusted {
				roots.AddCert(cert)
			}
			var dialCalls, httpCalls atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpCalls.Add(1)
				if r.ProtoMajor != 2 || r.Host != "api.123169.xyz" || r.TLS.ServerName != "api.123169.xyz" {
					t.Error("HTTP/2 or original target identity was lost")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			server.EnableHTTP2 = true
			server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
			server.StartTLS()
			defer server.Close()
			tr := &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{RootCAs: roots}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				dialCalls.Add(1)
				if address != "api.123169.xyz:443" {
					return nil, errors.New("unexpected fixture destination")
				}
				// Public host is request metadata only: every socket goes to loopback.
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			}}
			client := &http.Client{Transport: ippureBrowserSession(tr), Timeout: 2 * time.Second}
			defer client.CloseIdleConnections()
			if !ippureUsesBrowserSession(client.Transport) {
				t.Fatal("browser session not installed")
			}
			resp, err := client.Get(ippureWebAPIBase + "/api/gateway/cf")
			if resp != nil {
				resp.Body.Close()
			}
			var authorityErr x509.UnknownAuthorityError
			var hostnameErr x509.HostnameError
			switch tc.name {
			case "untrusted":
				if !errors.As(err, &authorityErr) {
					t.Fatalf("expected unknown authority: %v", err)
				}
			case "wrong_hostname":
				if !errors.As(err, &hostnameErr) {
					t.Fatalf("expected hostname rejection: %v", err)
				}
			default:
				if err != nil || resp.StatusCode != http.StatusNoContent {
					t.Fatalf("trusted local TLS failed: %v", err)
				}
			}
			wantHTTP := int32(0)
			if tc.name == "trusted" {
				wantHTTP = 1
			}
			if dialCalls.Load() != 1 || httpCalls.Load() != wantHTTP || !tr.DisableKeepAlives {
				t.Fatalf("dial/HTTP/original transport changed: dial=%d HTTP=%d", dialCalls.Load(), httpCalls.Load())
			}
		})
	}
}

func TestIPPureV7KeepsCFWhenPrimaryUsesWholeDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	checker := ippureChecker(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "my.ippure.com":
			<-req.Context().Done()
			return nil, req.Context().Err()
		case "cf.999831.xyz":
			return fixtureResponse(req, 200, "application/octet-stream", "opaque"), nil
		default:
			return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8","risk_score":98,"asn":15169}`), nil
		}
	})
	rule, _ := RuleFor("ippure_ip_quality")
	r := checker.Check(ctx, monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Details["cf_risk_score"] != "98" || r.Details["cf_status"] != "已取得" || r.Details["fraud_score"] != "" {
		t.Fatalf("completed CF result lost: %+v", r)
	}
}

func TestIPPureV7FallsBackToWebForFreshCFIPv4Only(t *testing.T) {
	checker := ippureChecker(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host + req.URL.Path {
		case "my.ippure.com/v1/info":
			return fixtureResponse(req, 403, "text/html", "blocked"), nil
		case "cf.999831.xyz/api/cf-summary":
			return fixtureResponse(req, 200, "application/octet-stream", "opaque"), nil
		case "api.123169.xyz/api/gateway/cf":
			return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8","risk_score":98}`), nil
		case "api.123169.xyz/api/info/ip-basic/8.8.8.8":
			return ippureJSONResponse(req, 200, `{"ok":true,"data":{"ip":"8.8.8.8","asn":{"number":15169}}}`), nil
		case "api.123169.xyz/api/info/ip-risk/8.8.8.8":
			return ippureJSONResponse(req, 200, `{"ok":true,"data":{"ip":"8.8.8.8","risk_score":68}}`), nil
		case "api.123169.xyz/api/info/asn/botclass/15169":
			return ippureJSONResponse(req, 200, `{"ok":true,"data":{"human":5.78,"bot":94.22}}`), nil
		default:
			t.Fatalf("unexpected URL %s", req.URL)
			return nil, nil
		}
	})
	rule, _ := RuleFor("ippure_ip_quality")
	r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "profiled" || r.Details["fraud_score"] != "68" || r.Details["cf_risk_score"] != "98" || r.Details["asn_human_percent"] != "5.78" {
		t.Fatal(r)
	}
}

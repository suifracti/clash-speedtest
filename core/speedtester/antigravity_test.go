package speedtester

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestClassifyAntigravityResponse(t *testing.T) {
	blockedBody := []byte(`{
  "error": {
    "code": 400,
    "message": "User location is not supported for the API use.",
    "status": "FAILED_PRECONDITION"
  }
}`)

	subscriptionRequiredBody := []byte(`[{
  "error": {
    "code": 403,
    "message": "You do not have a valid license of this product.",
    "status": "PERMISSION_DENIED",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.ErrorInfo",
        "reason": "SUBSCRIPTION_REQUIRED",
        "domain": "cloudaicompanion.googleapis.com"
      }
    ]
  }
}]`)

	invalidArgumentBody := []byte(`[{
  "error": {
    "code": 400,
    "message": "Invalid argument",
    "status": "INVALID_ARGUMENT"
  }
}]`)

	cases := []struct {
		name       string
		statusCode int
		body       []byte
		want       string
	}{
		{"region rejected", http.StatusBadRequest, blockedBody, AntigravityBlocked},
		{"region rejected over sse", http.StatusBadRequest, []byte("data: " + string(blockedBody)), AntigravityBlocked},
		{"region rejected with 403", http.StatusForbidden, blockedBody, AntigravityBlocked},
		{"subscription required means location allowed", http.StatusForbidden, subscriptionRequiredBody, AntigravityAvailable},
		{"invalid argument is not available", http.StatusBadRequest, invalidArgumentBody, AntigravityUnknown},
		{"permission denied means location allowed", http.StatusForbidden,
			[]byte(`{"error":{"code":403,"message":"The caller does not have permission","status":"PERMISSION_DENIED"}}`), AntigravityAvailable},
		{"anonymous probe must not read as available", http.StatusUnauthorized,
			[]byte(`{"error":{"code":401,"status":"UNAUTHENTICATED"}}`), AntigravityAuthFailed},
		{"expired token", http.StatusUnauthorized,
			[]byte(`{"error":{"code":401,"message":"Request had invalid authentication credentials."}}`), AntigravityAuthFailed},
		{"forbidden service reachable", http.StatusForbidden,
			[]byte(`{"error":{"code":403,"message":"Access Denied"}}`), AntigravityAvailable},
		{"transport failure", 0, []byte("dial tcp timeout"), AntigravityUnreachable},
		{"upstream error", http.StatusServiceUnavailable, nil, AntigravityUnreachable},
		{"authenticated success", http.StatusOK,
			[]byte(`{"cloudaicompanionProject":"demo"}`), AntigravityAvailable},
		{"rate limited proves location allowed", http.StatusTooManyRequests,
			[]byte(`{"error":{"status":"RESOURCE_EXHAUSTED"}}`), AntigravityAvailable},
		{"unexpected 4xx is not a verdict", http.StatusNotFound, nil, AntigravityUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, detail := classifyAntigravityResponse(tc.statusCode, tc.body)
			if got != tc.want {
				t.Fatalf("classify(%d) = %s (%s), want %s", tc.statusCode, got, detail, tc.want)
			}
		})
	}

	// Tests with Google's authoritative region detection
	regionCases := []struct {
		name         string
		statusCode   int
		body         []byte
		googleRegion string
		wantStatus   string
		wantDetailSub string
	}{
		{"HK region blocked", http.StatusBadRequest, blockedBody, "HK", AntigravityBlocked, "HK"},
		{"CN sent to china blocked", http.StatusBadRequest, blockedBody, "CN", AntigravityBlocked, "CN"},
		{"CN sent to china even with unexpected status", http.StatusOK, []byte(`{}`), "CN", AntigravityBlocked, "送中"},
		{"SG supported region available", http.StatusOK, []byte(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}}`), "SG", AntigravityAvailable, "SG"},
		{"US supported region with subscription required", http.StatusForbidden, subscriptionRequiredBody, "US", AntigravityAvailable, "US"},
	}

	for _, tc := range regionCases {
		t.Run(tc.name, func(t *testing.T) {
			got, detail := classifyAntigravityResponseWithRegion(tc.statusCode, tc.body, tc.googleRegion)
			if got != tc.wantStatus {
				t.Fatalf("classifyWithRegion(%s) status = %s, want %s (detail: %s)", tc.googleRegion, got, tc.wantStatus, detail)
			}
			if !strings.Contains(detail, tc.wantDetailSub) {
				t.Fatalf("classifyWithRegion(%s) detail = %s, want substring %q", tc.googleRegion, detail, tc.wantDetailSub)
			}
		})
	}
}

func TestParseCountryFromName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"🇭🇰香港专线01|BGP|住宅IP", "HK"},
		{"香港高速03|BGP|CMCU", "HK"},
		{"🇨🇳台湾专线01|BGP|流媒体", "TW"},
		{"台湾优质 02", "TW"},
		{"🇯🇵日本专线01|BGP|流媒体", "JP"},
		{"🇸🇬新加坡高速01", "SG"},
		{"🇺🇸美国02|流媒体", "US"},
	}
	for _, tc := range cases {
		got := parseCountryFromName(tc.name)
		if got != tc.want {
			t.Errorf("parseCountryFromName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestParseAntigravityToken(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"bare token", "ya29.abc123\n", "ya29.abc123"},
		{"authorization header", "Authorization: Bearer ya29.abc123", "ya29.abc123"},
		{"lowercase header", "authorization: bearer ya29.abc123", "ya29.abc123"},
		{"comments and blank lines skipped", "# note\n\nAuthorization: Bearer ya29.abc123\n", "ya29.abc123"},
		{"crlf line endings", "Authorization: Bearer ya29.abc123\r\n", "ya29.abc123"},
		{"empty input", "\n\n", ""},
		{"only comments", "# nothing here\n", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseAntigravityToken(tc.raw); got != tc.want {
				t.Fatalf("ParseAntigravityToken(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestFormatAntigravityCoversEveryStatus(t *testing.T) {
	cases := map[string]string{
		AntigravityAvailable:   "可用",
		AntigravityFlapping:    "偶发送中",
		AntigravityBlocked:     "地区不可用",
		AntigravityUnreachable: "节点不通",
		AntigravityAuthFailed:  "凭证失效",
		AntigravityUnknown:     "未知",
	}
	for status, want := range cases {
		r := &Result{AntigravityStatus: status}
		if got := r.FormatAntigravity(); got != want {
			t.Fatalf("FormatAntigravity(%s) = %s, want %s", status, got, want)
		}
	}
}

func TestAntigravityRank(t *testing.T) {
	if AntigravityRank(AntigravityAvailable) >= AntigravityRank(AntigravityFlapping) {
		t.Errorf("Available should rank higher than Flapping")
	}
	if AntigravityRank(AntigravityFlapping) >= AntigravityRank(AntigravityBlocked) {
		t.Errorf("Flapping should rank higher than Blocked")
	}
}

func TestDualIPAttributesParsing(t *testing.T) {
	p0 := &Ping0Info{
		IP:        "1.1.1.1",
		Location:  "新加坡 新加坡",
		Country:   "新加坡",
		ASN:       "AS13335",
		Org:       "Cloudflare",
		IsIDC:     true,
		RiskScore: 25,
	}

	ipr := &IPPureInfo{
		IP:            "1.1.1.1",
		ASN:           13335,
		ASOrg:         "Cloudflare, Inc.",
		Country:       "Singapore",
		CountryCode:   "SG",
		City:          "Singapore",
		FraudScore:    59,
		IsResidential: false,
		IsBroadcast:   true,
	}

	info := &IPInfo{
		IP:          p0.IP,
		ASN:         p0.ASN,
		ISP:         p0.Org,
		IPType:      "机房IDC",
		OriginType:  "广播IP",
		RiskScore:   p0.RiskScore,
		FraudScore:  ipr.FraudScore,
		Location:    p0.Location,
		Country:     p0.Country,
		CountryCode: ipr.CountryCode,
		Ping0:       p0,
		IPPure:      ipr,
	}

	if info.Ping0 == nil || info.IPPure == nil {
		t.Fatal("expected both Ping0 and IPPure to be present")
	}
	if info.RiskScore != 25 {
		t.Fatalf("expected ping0 risk score 25, got %d", info.RiskScore)
	}
	if info.FraudScore != 59 {
		t.Fatalf("expected ippure fraud score 59, got %d", info.FraudScore)
	}
	if info.OriginType != "广播IP" {
		t.Fatalf("expected origin type 广播IP, got %s", info.OriginType)
	}
}

type mockFlappingRoundTripper struct {
	mu          sync.Mutex
	traceCount  int
	userCount   int
	streamCount int
}

func (m *mockFlappingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	urlStr := req.URL.String()

	// 1. Trace request
	if strings.Contains(urlStr, "cdn-cgi/trace") {
		m.traceCount++
		ip := "103.21.244.1"
		if m.traceCount%2 == 0 {
			ip = "103.21.244.2"
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("fl=123\nh=cloudflare.com\nip=" + ip + "\nts=123\nloc=SG\n")),
			Header:     make(http.Header),
		}, nil
	}

	// 2. Google fetchUserInfo request
	if strings.Contains(urlStr, "fetchUserInfo") {
		m.userCount++
		region := "SG"
		if m.userCount%2 == 0 {
			region = "CN"
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"regionCode":"` + region + `"}`)),
			Header:     make(http.Header),
		}, nil
	}

	// 3. Google streamGenerateContent request
	if strings.Contains(urlStr, "streamGenerateContent") {
		m.streamCount++
		if m.streamCount%2 == 0 {
			// Sent to China / location not supported
			body := `{"error":{"code":400,"message":"User location is not supported for the API use.","status":"FAILED_PRECONDITION"}}`
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}
		// Success
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`data: {"response":{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}}`)),
			Header:     make(http.Header),
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
	}, nil
}

func TestProbeAntigravityStabilityMock(t *testing.T) {
	mockClient := &http.Client{
		Transport: &mockFlappingRoundTripper{},
	}

	status, country, countryCode, detail, stab := probeAntigravityStability(
		mockClient,
		"ya29.test-mock-token",
		"新加坡",
		"SG",
		&IPInfo{IP: "103.21.244.1"},
	)

	if status != AntigravityFlapping {
		t.Fatalf("expected status %s, got %s (detail: %s)", AntigravityFlapping, status, detail)
	}
	if !stab.Flapping {
		t.Fatalf("expected stab.Flapping to be true")
	}
	if stab.TotalProbes != 8 {
		t.Fatalf("expected 8 total probes, got %d", stab.TotalProbes)
	}
	if len(stab.ExitIPs) < 2 {
		t.Fatalf("expected at least 2 distinct exit IPs, got %v", stab.ExitIPs)
	}
	if len(stab.BlockedIPs) == 0 {
		t.Fatalf("expected at least 1 blocked IP, got %v", stab.BlockedIPs)
	}
	if stab.LatencyGrade == "" {
		t.Fatalf("expected non-empty LatencyGrade, got empty")
	}
	if !strings.Contains(detail, "负载均衡") && !strings.Contains(detail, "偶发送中") {
		t.Fatalf("expected detail to mention 负载均衡/偶发送中, got %s", detail)
	}
	t.Logf("Flapping detail: %s, country: %s, countryCode: %s, TTFB: %dms (grade: %s)", detail, country, countryCode, stab.GoogleTTFBMs, stab.LatencyGrade)
}



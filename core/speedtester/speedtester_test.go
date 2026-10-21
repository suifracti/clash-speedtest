package speedtester

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTransferSummaryAdd(t *testing.T) {
	summary := newTransferSummary()
	summary.add(nil)

	errorMessage := "download request to https://example.com/__down?bytes=1 failed: boom"
	summary.add(&downloadResult{error: errorMessage})
	if summary.successCount != 0 {
		t.Fatalf("expected successCount to remain 0, got %d", summary.successCount)
	}
	if len(summary.errors) != 1 {
		t.Fatalf("expected 1 error message, got %d", len(summary.errors))
	}
	if summary.errors[0] != errorMessage {
		t.Fatalf("expected error message %q, got %q", errorMessage, summary.errors[0])
	}

	summary.add(&downloadResult{error: errorMessage})
	if len(summary.errors) != 1 {
		t.Fatalf("expected duplicate errors to be deduplicated, got %d", len(summary.errors))
	}

	summary.add(&downloadResult{bytes: 100, duration: time.Second})
	summary.add(&downloadResult{bytes: 50, duration: 2 * time.Second})

	if summary.successCount != 2 {
		t.Fatalf("expected successCount to be 2, got %d", summary.successCount)
	}
	if summary.totalBytes != 150 {
		t.Fatalf("expected totalBytes to be 150, got %d", summary.totalBytes)
	}
	if summary.totalDuration != 3*time.Second {
		t.Fatalf("expected totalDuration to be 3s, got %v", summary.totalDuration)
	}
	if summary.averageDuration() != 1500*time.Millisecond {
		t.Fatalf("expected averageDuration to be 1.5s, got %v", summary.averageDuration())
	}
}

func TestResultFormatErrors(t *testing.T) {
	result := &Result{}
	if result.FormatDownloadError() != "N/A" {
		t.Fatalf("expected empty download error to format as N/A, got %q", result.FormatDownloadError())
	}
	if result.FormatUploadError() != "N/A" {
		t.Fatalf("expected empty upload error to format as N/A, got %q", result.FormatUploadError())
	}

	result.DownloadError = "download failed: timeout"
	result.UploadError = "upload failed: status 500"
	if result.FormatDownloadError() != result.DownloadError {
		t.Fatalf("expected download error to pass through, got %q", result.FormatDownloadError())
	}
	if result.FormatUploadError() != result.UploadError {
		t.Fatalf("expected upload error to pass through, got %q", result.FormatUploadError())
	}

	result.DownloadSpeed = 1024
	result.UploadSpeed = 2048
	if result.FormatDownloadSpeed() != result.DownloadError {
		t.Fatalf("expected download speed to prefer error string, got %q", result.FormatDownloadSpeed())
	}
	if result.FormatUploadSpeed() != result.UploadError {
		t.Fatalf("expected upload speed to prefer error string, got %q", result.FormatUploadSpeed())
	}
	if result.FormatDownloadSpeedValue() == result.DownloadError {
		t.Fatalf("expected download speed value to ignore error string")
	}
	if result.FormatUploadSpeedValue() == result.UploadError {
		t.Fatalf("expected upload speed value to ignore error string")
	}
}

func TestDeduplicateProxies(t *testing.T) {
	t.Run("deduplicate by server and port", func(t *testing.T) {
		proxies := map[string]*CProxy{
			"proxy-a": {Config: map[string]any{"server": "1.1.1.1", "port": 443}},
			"proxy-b": {Config: map[string]any{"server": "1.1.1.1", "port": "443"}},
			"proxy-c": {Config: map[string]any{"server": "2.2.2.2", "port": float64(443)}},
		}

		results := deduplicateProxiesByServerPort(proxies)
		if len(results) != 2 {
			t.Fatalf("expected 2 proxies after deduplication, got %d", len(results))
		}
		if _, ok := results["proxy-c"]; !ok {
			t.Fatalf("expected proxy-c to remain")
		}
		duplicateCount := 0
		if _, ok := results["proxy-a"]; ok {
			duplicateCount++
		}
		if _, ok := results["proxy-b"]; ok {
			duplicateCount++
		}
		if duplicateCount != 1 {
			t.Fatalf("expected exactly one duplicate proxy to remain, got %d", duplicateCount)
		}
		for i := 0; i < 32; i++ {
			stable := deduplicateProxiesByServerPort(proxies)
			if stable["proxy-a"] != proxies["proxy-a"] || stable["proxy-b"] != nil {
				t.Fatal("the first named alias must remain stable across repeated reads")
			}
		}
	})

	t.Run("mapped ipv6 and ipv4 are deduplicated after normalization", func(t *testing.T) {
		proxies := map[string]*CProxy{
			"proxy-a": {Config: map[string]any{"server": convertMappedIPv6ToIPv4("::ffff:1.1.1.1"), "port": 443}},
			"proxy-b": {Config: map[string]any{"server": "1.1.1.1", "port": 443}},
		}

		results := deduplicateProxiesByServerPort(proxies)
		if len(results) != 1 {
			t.Fatalf("expected 1 proxy after deduplication, got %d", len(results))
		}
	})

	t.Run("missing server or port is not deduplicated", func(t *testing.T) {
		proxies := map[string]*CProxy{
			"proxy-a": {Config: map[string]any{"server": "1.1.1.1"}},
			"proxy-b": {Config: map[string]any{"server": "1.1.1.1", "port": 80}},
			"proxy-c": {Config: map[string]any{"port": 80}},
		}

		results := deduplicateProxiesByServerPort(proxies)
		if len(results) != 3 {
			t.Fatalf("expected proxies to remain when server or port missing, got %d", len(results))
		}
	})
}

func TestSubscriptionInfoAliasDoesNotHideRealNode(t *testing.T) {
	config := func(name, password string) *CProxy {
		return &CProxy{Config: map[string]any{"name": name, "type": "ss", "server": "192.0.2.1", "port": 443, "cipher": "aes-128-gcm", "password": password}}
	}
	real := "🇯🇵日本高速01|CTCU|0.5x"
	quota, expiry := "剩余流量：10 GB", "套餐到期：长期有效"
	proxies := map[string]*CProxy{real: config(real, "fixture"), "🇯🇵日本高速99|同配置别名": config("🇯🇵日本高速99|同配置别名", "fixture"), quota: config(quota, "fixture"), expiry: config(expiry, "fixture"), "different credentials": config("different credentials", "other")}
	got := deduplicateProxiesByServerPort(proxies)
	if got[real] == nil || got[quota] == nil || got[expiry] == nil || got["different credentials"] == nil || len(got) != 4 {
		names := make([]string, 0, len(got))
		for name := range got {
			names = append(names, name)
		}
		t.Fatalf("real node, information and credential variants must remain distinct; got %v", names)
	}
}

func TestSafeServerNormalization(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    any
		expected any
	}{
		{"mapped ipv6", "::ffff:192.0.2.1", "192.0.2.1"},
		{"bracketed mapped ipv6", "[::ffff:192.0.2.1]", "192.0.2.1"},
		{"bracketed ipv6", "[2001:db8::1]", "[2001:db8::1]"},
		{"regular ipv4", "192.0.2.1", "192.0.2.1"},
		{"domain", "example.com", "example.com"},
		{"int server safe", 12345, 12345},
		{"nil server safe", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &CProxy{
				Config: map[string]any{"server": tc.input, "port": 443},
			}
			if server, ok := p.Config["server"]; ok && server != nil {
				if s, ok := server.(string); ok {
					p.Config["server"] = convertMappedIPv6ToIPv4(s)
				}
			}
			if p.Config["server"] != tc.expected {
				t.Fatalf("got server %v, want %v", p.Config["server"], tc.expected)
			}
		})
	}
}

func TestEquivalentProxyConfigurationIgnoresInternalMarkers(t *testing.T) {
	a := &CProxy{
		Config: map[string]any{
			"name":                 "node-a",
			"type":                 "ss",
			"server":               "192.0.2.1",
			"port":                 443,
			"cipher":               "aes-128-gcm",
			"password":             "pass",
			"_cst_generated_ws_ua": true,
		},
	}
	b := &CProxy{
		Config: map[string]any{
			"name":     "node-b",
			"type":     "ss",
			"server":   "192.0.2.1",
			"port":     443,
			"cipher":   "aes-128-gcm",
			"password": "pass",
		},
	}
	if !EquivalentProxyConfiguration(a, b) {
		t.Fatal("expected proxies with and without internal _cst_generated_ws_ua to be equivalent")
	}
}

func TestEquivalentProxyConfigurationComparesNestedYAMLMaps(t *testing.T) {
	alias := func(name, path string) *CProxy {
		return &CProxy{Config: map[string]any{
			"name": name, "type": "vless", "server": "cf.example.com", "port": 443, "uuid": "u",
			"network": "ws",
			"ws-opts": map[any]any{"path": path, "headers": map[any]any{"Host": "cf.example.com"}},
		}}
	}
	if !EquivalentProxyConfiguration(alias("node-a", "/same"), alias("node-b", "/same")) {
		t.Fatal("expected aliases with identical nested YAML config to be equivalent")
	}
	if EquivalentProxyConfiguration(alias("node-a", "/same"), alias("node-b", "/other")) {
		t.Fatal("expected different ws paths to be non-equivalent")
	}
}

func TestLatencyTargetURLNormalization(t *testing.T) {
	for _, tc := range []struct {
		input   string
		wantURL string
		wantErr bool
	}{
		{"cloudflare", "https://speed.cloudflare.com/__down?bytes=1", false},
		{"Cloudflare", "https://speed.cloudflare.com/__down?bytes=1", false},
		{"  CLOUDFLARE  ", "https://speed.cloudflare.com/__down?bytes=1", false},
		{"google", "https://www.gstatic.com/generate_204", false},
		{" Google ", "https://www.gstatic.com/generate_204", false},
		{"ALL", "multi://latency-four-v1", false},
		{"", "https://speed.cloudflare.com/__down?bytes=1", false},
		{"unknown_xyz", "", true},
	} {
		url, err := LatencyTargetURL(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("LatencyTargetURL(%q) expected error, got nil", tc.input)
		} else if !tc.wantErr && err != nil {
			t.Errorf("LatencyTargetURL(%q) unexpected error: %v", tc.input, err)
		} else if !tc.wantErr && url != tc.wantURL {
			t.Errorf("LatencyTargetURL(%q) = %q, want %q", tc.input, url, tc.wantURL)
		}
	}
}

func TestCalculateLatencyStatsBoundaries(t *testing.T) {
	// Negative failed pings clamp to 0
	r1 := calculateLatencyStats([]time.Duration{10 * time.Millisecond}, -1, 5)
	if r1.packetLoss != 0 {
		t.Fatalf("expected 0 packet loss for negative failed pings, got %f", r1.packetLoss)
	}

	// Failed pings greater than pingCount clamp to pingCount (100% loss)
	r2 := calculateLatencyStats(nil, 10, 5)
	if r2.packetLoss != 100 {
		t.Fatalf("expected 100 packet loss for excessive failed pings, got %f", r2.packetLoss)
	}

	// Non-positive pingCount defaults to 6
	r3 := calculateLatencyStats(nil, 3, 0)
	if r3.packetLoss != 50 {
		t.Fatalf("expected 50 packet loss when pingCount defaults to 6 (3/6), got %f", r3.packetLoss)
	}
}

func TestCreateClientNilProxyDefense(t *testing.T) {
	st := &SpeedTester{}
	client := st.createClient(nil, time.Second)
	if client == nil || client.Transport == nil {
		t.Fatal("expected client and transport to be initialized")
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr.DialContext == nil {
		t.Fatal("expected http.Transport with DialContext")
	}
	_, err := tr.DialContext(context.Background(), "tcp", "1.1.1.1:80")
	if err == nil || !strings.Contains(err.Error(), "proxy is nil") {
		t.Fatalf("expected 'proxy is nil' error, got %v", err)
	}
}

func TestSpeedTesterNilSafety(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Fatal("expected New(nil) to return error, got nil")
	}

	var st *SpeedTester = nil

	if mode := st.Mode(); mode != SpeedModeDownload {
		t.Fatalf("expected Mode() on nil to return SpeedModeDownload, got %v", mode)
	}
	if metrics := st.Metrics(); !metrics.Download {
		t.Fatalf("expected Metrics() on nil to have Download enabled, got %+v", metrics)
	}
	if rounds := st.Rounds(); rounds != 1 {
		t.Fatalf("expected Rounds() on nil to be 1, got %d", rounds)
	}
	if target := st.LatencyProbeTarget(); target != "" {
		t.Fatalf("expected LatencyProbeTarget() on nil to be empty, got %q", target)
	}
	if probe := st.probeURL(); probe != "" {
		t.Fatalf("expected probeURL() on nil to be empty, got %q", probe)
	}
	if ua := st.fetchConfigUA(); ua == "" {
		t.Fatalf("expected fetchConfigUA() on nil to return default UA")
	}
	if _, err := st.fetchHTTPConfig("https://example.com"); err == nil {
		t.Fatal("expected fetchHTTPConfig on nil to return error")
	}
	if _, err := st.LoadProxies(); err == nil {
		t.Fatal("expected LoadProxies on nil to return error")
	}
	if res := st.TestSingle("test", nil, nil); res != nil {
		t.Fatalf("expected TestSingle on nil to return nil, got %v", res)
	}
	if res := st.TestSingleContext(context.Background(), "test", nil, nil); res != nil {
		t.Fatalf("expected TestSingleContext on nil to return nil, got %v", res)
	}
	// TestProxies and TestProxiesUntil should safely return without panic
	st.TestProxies(nil, nil)
	st.TestProxiesUntil(nil, nil)

	if res := st.testProxy("test", nil); res != nil {
		t.Fatalf("expected testProxy on nil to return nil, got %v", res)
	}
	if ok := st.testProxyEmit("test", nil, nil); ok {
		t.Fatal("expected testProxyEmit on nil to return false")
	}
	if ok := st.testProxyEmitContext(context.Background(), "test", nil, nil); ok {
		t.Fatal("expected testProxyEmitContext on nil to return false")
	}

	latRes := st.testLatency(nil, 0, 0)
	if latRes == nil || latRes.packetLoss != 100 {
		t.Fatalf("expected 100%% packet loss for testLatency on nil, got %+v", latRes)
	}
	latResCtx := st.testLatencyContext(context.Background(), nil, 0, 0)
	if latResCtx == nil || latResCtx.packetLoss != 100 {
		t.Fatalf("expected 100%% packet loss for testLatencyContext on nil, got %+v", latResCtx)
	}
	latResClient := st.testLatencyWithClient(nil, 0)
	if latResClient == nil || latResClient.packetLoss != 100 {
		t.Fatalf("expected 100%% packet loss for testLatencyWithClient on nil, got %+v", latResClient)
	}
	latResClientCtx := st.testLatencyWithClientContext(context.Background(), nil, 0)
	if latResClientCtx == nil || latResClientCtx.packetLoss != 100 {
		t.Fatalf("expected 100%% packet loss for testLatencyWithClientContext on nil, got %+v", latResClientCtx)
	}

	dr := st.testDownload(nil, 1024, time.Second)
	if dr == nil || dr.error == "" {
		t.Fatalf("expected testDownload on nil to return error, got %+v", dr)
	}
	ur := st.testUpload(nil, 1024, time.Second)
	if ur == nil || ur.error == "" {
		t.Fatalf("expected testUpload on nil to return error, got %+v", ur)
	}

	status, _, _, detail, _, _ := st.testAntigravity(nil)
	if status != AntigravityUnknown || detail == "" {
		t.Fatalf("expected testAntigravity on nil to return AntigravityUnknown, got status=%s detail=%s", status, detail)
	}

	physRes := st.TestSinglePhysicalContext(context.Background(), "test", nil, nil)
	if physRes != nil {
		t.Fatalf("expected TestSinglePhysicalContext on nil to return nil, got %v", physRes)
	}

	// Defense against uninitialized config struct
	emptyST := &SpeedTester{}
	if _, err := emptyST.LoadProxies(); err == nil {
		t.Fatal("expected LoadProxies on empty SpeedTester to return error")
	}
	if rounds := emptyST.Rounds(); rounds != 1 {
		t.Fatalf("expected Rounds() on empty SpeedTester to be 1, got %d", rounds)
	}
	if ok := emptyST.testProxyEmitContext(context.Background(), "test", nil, nil); ok {
		t.Fatal("expected testProxyEmitContext on empty SpeedTester to return false")
	}
	agStatus, _, _, _, _, _ := emptyST.testAntigravity(nil)
	if agStatus != AntigravityUnknown {
		t.Fatalf("expected testAntigravity on empty SpeedTester to return AntigravityUnknown, got %s", agStatus)
	}
	physResEmpty := emptyST.TestSinglePhysicalContext(context.Background(), "test", nil, nil)
	if physResEmpty != nil {
		t.Fatalf("expected nil result from physical test on nil proxy, got %v", physResEmpty)
	}
}

func TestResultNilFormatting(t *testing.T) {
	var r *Result = nil

	if got := r.FormatDownloadSpeed(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatDownloadSpeedValue(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatLatency(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatJitter(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatPacketLoss(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatUploadSpeed(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatUploadSpeedValue(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatDownloadError(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatUploadError(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatAntigravity(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
	if got := r.FormatExitCountry(); got != "N/A" {
		t.Fatalf("expected 'N/A', got %q", got)
	}
}

func TestTransferSummaryNilSafety(t *testing.T) {
	var s *transferSummary
	// None of these should panic
	s.add(nil)
	s.add(&downloadResult{bytes: 100})
	s.appendError("boom")
	if avg := s.averageDuration(); avg != 0 {
		t.Fatalf("expected 0 average duration on nil transferSummary, got %v", avg)
	}

	size, duration, speed, err := applyTransferSummary(nil)
	if size != 0 || duration != 0 || speed != 0 || err != "" {
		t.Fatalf("expected zeroes from applyTransferSummary(nil), got size=%f duration=%v speed=%f err=%q", size, duration, speed, err)
	}
}

func TestMarkDownloadPhaseNilContext(t *testing.T) {
	// Should not panic on nil context or context without phase hook
	markDownloadPhase(nil, "phase1")
	markDownloadPhase(context.Background(), "phase2")
}

func TestUniqueProxyNameNilMap(t *testing.T) {
	if got := uniqueProxyName(nil, "my-node"); got != "my-node" {
		t.Fatalf("expected 'my-node', got %q", got)
	}
	if got := uniqueProxyName(nil, ""); got != "proxy" {
		t.Fatalf("expected 'proxy', got %q", got)
	}
}

func TestOpenPhysicalProbeClientNilPreparer(t *testing.T) {
	_, _, cleanup, err := openPhysicalProbeClient(context.Background(), nil, time.Second, 1, nil)
	defer cleanup()
	if err == nil || !strings.Contains(err.Error(), "physical probe preparer is nil") {
		t.Fatalf("expected 'physical probe preparer is nil' error, got %v", err)
	}
}


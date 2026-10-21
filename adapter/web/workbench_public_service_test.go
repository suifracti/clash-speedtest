package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/publicservice"
)

func TestWebPublicServiceCatalogUsesSharedApplicationDirectory(t *testing.T) {
	root := t.TempDir()
	server, err := NewServer(ServerConfig{
		ProfilePaths: profiles.Paths{Dir: filepath.Join(root, "profiles")},
		HistoryDir:   filepath.Join(root, "history"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/workbench/public-service-catalog", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog response status %d: %s", rec.Code, rec.Body.String())
	}
	var rules []publicservice.Rule
	if err := json.Unmarshal(rec.Body.Bytes(), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) < 16 || rules[0].ServiceID != "chatgpt_unlock" || rules[1].ServiceID != "claude_unlock" || rules[2].ServiceID != "gemini_unlock" {
		t.Fatalf("unexpected public-service directory: %+v", rules)
	}
	if !reflect.DeepEqual(rules, publicservice.Catalog()) {
		t.Fatal("Web catalog differs from the shared application directory")
	}
	byID := make(map[string]publicservice.Rule, len(rules))
	for _, rule := range rules {
		byID[rule.ServiceID] = rule
	}
	for _, retired := range []string{"ipok_ipv4", "ipok_ipv6", "ping0_ip_quality", "microsoft_connect", "firefox_portal"} {
		if _, exists := byID[retired]; exists {
			t.Errorf("retired service %s reappeared in new plans", retired)
		}
	}
	for _, id := range []string{"ping0_ipv4", "ping0_ipv6", "chatgpt_unlock", "claude_unlock"} {
		if _, exists := byID[id]; !exists {
			t.Errorf("current service %s is missing from the Web catalog", id)
		}
	}

	if rules[4].Category == "" || rules[4].ResultKind == "" {
		t.Fatalf("expanded catalog metadata is missing: %+v", rules[4])
	}
}

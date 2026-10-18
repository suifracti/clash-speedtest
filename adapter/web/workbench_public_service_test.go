package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	if len(rules) < 16 || rules[0].ServiceID != "cloudflare_204" || rules[1].ServiceID != "google_204" || rules[2].ServiceID != "github_api_root" || rules[3].ServiceID != "antigravity" {
		t.Fatalf("unexpected public-service directory: %+v", rules)
	}
	if len(rules) != 85 || rules[82].ServiceID != "ipok_ipv4" || rules[83].ServiceID != "ipok_ipv6" || rules[82].BatchDefault || rules[83].BatchDefault {
		t.Fatal("new on-demand rules must be appended without moving the original catalog")
	}

	if rules[84].ServiceID != "netflix_unlock_ipv4" || rules[84].SupersededBy != "netflix_unlock" || rules[84].BatchDefault {
		t.Fatal("temporary Netflix alias must be appended and marked history-only")
	}
	if rules[4].Category == "" || rules[4].ResultKind == "" {
		t.Fatalf("expanded catalog metadata is missing: %+v", rules[4])
	}
}

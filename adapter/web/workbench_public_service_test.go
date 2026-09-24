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
	if len(rules) != 3 || rules[0].ServiceID != "cloudflare_204" || rules[1].ServiceID != "google_204" || rules[2].ServiceID != "github_api_root" {
		t.Fatalf("unexpected public-service directory: %+v", rules)
	}
}

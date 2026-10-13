package web

import (
	"github.com/faceair/clash-speedtest/core/profiles"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportScopeForwardedToApplication(t *testing.T) {
	root := t.TempDir()
	s, err := NewServer(ServerConfig{Port: 0, ProfilePaths: profiles.Paths{Dir: filepath.Join(root, "profiles")}, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.AppService().InitializeEmptyProfileStore(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/workbench/export-clash", strings.NewReader(`{"selections":[{"profile_id":"missing","node_key":"node","node_identity_key":"id","config_revision_key":"rev"}]}`))
	w := httptest.NewRecorder()
	s.handleExportWorkbenchClash(w, r)
	if w.Code != 400 || strings.Contains(w.Body.String(), "至少一个") {
		t.Fatalf("scope discarded by adapter: %d %s", w.Code, w.Body.String())
	}
}

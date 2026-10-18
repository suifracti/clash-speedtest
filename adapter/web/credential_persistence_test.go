package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
)

func TestWebTokenSaveReportsDiskFailureWithoutDailyWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	paths, _ := appdata.Resolve(t.TempDir())
	if err := os.Mkdir(filepath.Join(paths.DataRoot, ".antigravity_token"), 0700); err != nil {
		t.Fatal(err)
	}
	svc := application.NewAppServiceWithOptions(nil, paths, nil, application.Options{NoAutoCredentials: true})
	defer svc.Close()
	server := &Server{app: svc}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/antigravity/token", strings.NewReader(`{"token":"ya29.local-value"}`))
	server.handleAntigravityToken(recorder, req)
	if recorder.Code != 500 || svc.GetAntigravityToken() != "" {
		t.Fatalf("status=%d; failed save claimed success", recorder.Code)
	}
	if _, err := os.Stat(filepath.Join(home, ".antigravity_token")); !os.IsNotExist(err) {
		t.Fatal("failed save touched daily HOME")
	}
}

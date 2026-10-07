package web

import (
	"bytes"
	"encoding/json"
	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPeriodicRoutesRequireAuthAndValidateConfig(t *testing.T) {
	paths, err := appdata.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(ServerConfig{AppPaths: paths, WebPassword: "fixture", AppOptions: application.Options{NoAutoCredentials: true}})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	handler := srv.buildHandler()
	for _, method := range []string{"GET", "PUT"} {
		r := httptest.NewRequest(method, "http://127.0.0.1:8999/api/periodic-sampling", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("%s accepted unauthenticated: %d", method, w.Code)
		}
	}
	for _, route := range []struct{ method, path string }{{"GET", "/api/measurement-rounds"}, {"POST", "/api/measurement-rounds"}, {"POST", "/api/measurement-rounds/manual-fixture/finish"}, {"POST", "/api/periodic-sampling/sample"}, {"POST", "/api/periodic-sampling/pause-measurements"}, {"GET", "/api/periodic-sampling/checks"}} {
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, httptest.NewRequest(route.method, "http://127.0.0.1:8999"+route.path, nil))
		if out.Code != 401 {
			t.Fatalf("sample/checks accepted unauthenticated: %d", out.Code)
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "http://127.0.0.1:8999/api/auth/login", bytes.NewBufferString(`{"password":"fixture"}`)))
	var auth struct {
		Token string `json:"token"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &auth); err != nil || auth.Token == "" {
		t.Fatal("fixture login failed")
	}
	call := func(method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8999/api/periodic-sampling", bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+auth.Token)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, r)
		return out
	}
	if w = call("GET", ""); w.Code != http.StatusOK {
		t.Fatalf("get status: %d", w.Code)
	}
	if w = call("PUT", `{"enabled":false,"latency_interval_seconds":120,"service_interval_seconds":300,"download_interval_seconds":3600,"download_mib":10,"include_antigravity":true}`); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	if w = call("PUT", `{"enabled":true,"download_mib":0}`); w.Code != 400 {
		t.Fatalf("invalid config accepted: %d", w.Code)
	}
	var status application.PeriodicSamplingStatus
	w = call("GET", "")
	if err = json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Config.DownloadMiB != 10 || status.Running {
		t.Fatal("rejected config changed state")
	}
}

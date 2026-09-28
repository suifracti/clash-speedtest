package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthStatusAndLogin(t *testing.T) {
	srv, err := NewServer(ServerConfig{
		WebPassword: "secret-password",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	handler := srv.buildHandler()

	// 1. Unauthenticated request to /api/auth/status
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8999/api/auth/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var status struct {
		AuthRequired  bool `json:"auth_required"`
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.AuthRequired || status.Authenticated {
		t.Fatalf("expected auth_required=true, authenticated=false, got %+v", status)
	}

	// 2. Unauthenticated request to protected endpoint
	reqProtected := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8999/api/monitor/runs", nil)
	recProtected := httptest.NewRecorder()
	handler.ServeHTTP(recProtected, reqProtected)
	if recProtected.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for protected endpoint, got %d", recProtected.Code)
	}

	// 3. Login with wrong password
	wrongBody, _ := json.Marshal(map[string]string{"password": "wrong"})
	reqWrong := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8999/api/auth/login", bytes.NewReader(wrongBody))
	recWrong := httptest.NewRecorder()
	handler.ServeHTTP(recWrong, reqWrong)
	if recWrong.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", recWrong.Code)
	}

	// 4. Login with correct password
	loginBody, _ := json.Marshal(map[string]string{"password": "secret-password"})
	reqLogin := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8999/api/auth/login", bytes.NewReader(loginBody))
	recLogin := httptest.NewRecorder()
	handler.ServeHTTP(recLogin, reqLogin)
	if recLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 for correct login, got %d", recLogin.Code)
	}
	var loginResp struct {
		Authenticated bool   `json:"authenticated"`
		Token         string `json:"token"`
	}
	if err := json.Unmarshal(recLogin.Body.Bytes(), &loginResp); err != nil {
		t.Fatal(err)
	}
	if !loginResp.Authenticated || loginResp.Token == "" {
		t.Fatalf("expected authenticated=true and non-empty token, got %+v", loginResp)
	}

	// 5. Access protected endpoint with Bearer token
	reqAuth := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8999/api/monitor/runs", nil)
	reqAuth.Header.Set("Authorization", "Bearer "+loginResp.Token)
	recAuth := httptest.NewRecorder()
	handler.ServeHTTP(recAuth, reqAuth)
	if recAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 with Bearer token, got %d: %s", recAuth.Code, recAuth.Body.String())
	}
}

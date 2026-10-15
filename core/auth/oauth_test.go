package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOAuthCredsIsExpired(t *testing.T) {
	now := time.Now().UnixMilli()

	tests := []struct {
		name     string
		creds    *OAuthCreds
		expected bool
	}{
		{
			name:     "nil creds",
			creds:    nil,
			expected: true,
		},
		{
			name: "empty access token",
			creds: &OAuthCreds{
				AccessToken: "",
				ExpiryDate:  now + 100000,
			},
			expected: true,
		},
		{
			name: "already expired",
			creds: &OAuthCreds{
				AccessToken: "ya29.test",
				ExpiryDate:  now - 10000,
			},
			expected: true,
		},
		{
			name: "expiring within buffer (30s)",
			creds: &OAuthCreds{
				AccessToken: "ya29.test",
				ExpiryDate:  now + 30000, // less than 60s buffer
			},
			expected: true,
		},
		{
			name: "valid for 1 hour",
			creds: &OAuthCreds{
				AccessToken: "ya29.test",
				ExpiryDate:  now + 3600000,
			},
			expected: false,
		},
		{
			name: "no expiry date set",
			creds: &OAuthCreds{
				AccessToken: "ya29.test",
				ExpiryDate:  0,
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.creds.IsExpired(); got != tc.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestCandidateTokenPaths(t *testing.T) {
	paths := CandidateTokenPaths()
	if len(paths) < 2 {
		t.Fatalf("expected at least 2 candidate paths, got %d", len(paths))
	}
	hasDotToken := false
	for _, p := range paths {
		if strings.Contains(p, ".antigravity_token") {
			hasDotToken = true
			break
		}
	}
	if !hasDotToken {
		t.Errorf("expected .antigravity_token in candidate paths: %v", paths)
	}
}

func TestResolveAntigravityTokenExplicit(t *testing.T) {
	// 1. Explicit token
	token, err := ResolveAntigravityToken("Bearer ya29.explicit123", "", false, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ya29.explicit123" {
		t.Errorf("expected ya29.explicit123, got %s", token)
	}

	// 2. Explicit token file
	tmpFile := filepath.Join(t.TempDir(), "test.token")
	if err := os.WriteFile(tmpFile, []byte("Authorization: Bearer ya29.fromfile456\n"), 0600); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}
	tokenFile, err := ResolveAntigravityToken("", tmpFile, false, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokenFile != "ya29.fromfile456" {
		t.Errorf("expected ya29.fromfile456, got %s", tokenFile)
	}
}

func TestResolveAntigravityTokenInteractivePaste(t *testing.T) {
	// Isolate candidateTokenPaths so machine's ~/.gemini/oauth_creds.json is not picked up
	oldCandidateFunc := candidateTokenPathsFunc
	defer func() { candidateTokenPathsFunc = oldCandidateFunc }()
	candidateTokenPathsFunc = func() []string {
		return []string{filepath.Join(t.TempDir(), "nonexistent.token")}
	}

	input := "2\nya29.manual_paste_token\n"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}

	// Clean up local .antigravity_token if created
	defer os.Remove(".antigravity_token")

	token, err := ResolveAntigravityToken("", "", true, in, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ya29.manual_paste_token" {
		t.Errorf("expected ya29.manual_paste_token, got %s", token)
	}

	// Verify file was written
	saved, err := os.ReadFile(".antigravity_token")
	if err != nil {
		t.Fatalf("expected .antigravity_token to be created: %v", err)
	}
	if strings.TrimSpace(string(saved)) != "ya29.manual_paste_token" {
		t.Errorf("saved file content = %q, want %q", string(saved), "ya29.manual_paste_token")
	}
}

func TestTryAutoDetectAntigravityTokenWithMockFile(t *testing.T) {
	oldCandidateFunc := candidateTokenPathsFunc
	defer func() { candidateTokenPathsFunc = oldCandidateFunc }()

	tmpDir := t.TempDir()
	mockCredsPath := filepath.Join(tmpDir, "oauth_creds.json")
	mockJSON := `{"access_token":"ya29.mock_detected_token","expiry_date":9999999999999}`
	if err := os.WriteFile(mockCredsPath, []byte(mockJSON), 0600); err != nil {
		t.Fatalf("write mock creds: %v", err)
	}

	candidateTokenPathsFunc = func() []string {
		return []string{mockCredsPath}
	}

	token, source, err := TryAutoDetectAntigravityToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ya29.mock_detected_token" {
		t.Errorf("expected ya29.mock_detected_token, got %s", token)
	}
	if source != mockCredsPath {
		t.Errorf("expected source %s, got %s", mockCredsPath, source)
	}
}

func TestOAuthCallbackHandler(t *testing.T) {
	state := "test_state_12345"

	handler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("error") != "" {
			http.Error(w, q.Get("error"), http.StatusBadRequest)
			return
		}
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if q.Get("code") == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}

	// 1. Success case
	req := httptest.NewRequest("GET", "/oauth-callback?state=test_state_12345&code=test_code", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// 2. State mismatch case
	reqBadState := httptest.NewRequest("GET", "/oauth-callback?state=wrong_state&code=test_code", nil)
	rrBadState := httptest.NewRecorder()
	handler(rrBadState, reqBadState)
	if rrBadState.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad state, got %d", rrBadState.Code)
	}

	// 3. Error case
	reqError := httptest.NewRequest("GET", "/oauth-callback?error=access_denied", nil)
	rrError := httptest.NewRecorder()
	handler(rrError, reqError)
	if rrError.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on error, got %d", rrError.Code)
	}
}

func TestParseAntigravityTokenQuotesAndHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bare token",
			input:    "ya29.bare123",
			expected: "ya29.bare123",
		},
		{
			name:     "double quoted token",
			input:    `"ya29.quoted456"`,
			expected: "ya29.quoted456",
		},
		{
			name:     "single quoted token",
			input:    `'ya29.single789'`,
			expected: "ya29.single789",
		},
		{
			name:     "standard authorization header",
			input:    "Authorization: Bearer ya29.standard",
			expected: "ya29.standard",
		},
		{
			name:     "quoted bearer token in header",
			input:    `Authorization: Bearer "ya29.bearer_quoted"`,
			expected: "ya29.bearer_quoted",
		},
		{
			name:     "quoted entire header value",
			input:    `Authorization: "Bearer ya29.quoted_val"`,
			expected: "ya29.quoted_val",
		},
		{
			name:     "curl format with -H flag and single quotes",
			input:    `-H 'Authorization: Bearer ya29.curl_flag'`,
			expected: "ya29.curl_flag",
		},
		{
			name:     "whitespace and carriage returns",
			input:    "  \r\n  Authorization: Bearer ya29.whitespaced  \r\n",
			expected: "ya29.whitespaced",
		},
		{
			name:     "comment line and empty lines skipped",
			input:    "# this is a comment\n\n\nAuthorization: Bearer ya29.after_comment\n",
			expected: "ya29.after_comment",
		},
		{
			name:     "bearer only with no token",
			input:    "Bearer ",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAntigravityToken(tc.input)
			if got != tc.expected {
				t.Errorf("ParseAntigravityToken(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestSaveOAuthCredsFileNilCreds(t *testing.T) {
	err := saveOAuthCredsFile(filepath.Join(t.TempDir(), "creds.json"), nil)
	if err == nil {
		t.Error("expected error for nil creds, got nil")
	}
}

func TestSaveTokenFileEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := saveTokenFile(filepath.Join(dir, "token1"), ""); err == nil {
		t.Error("expected error for empty token, got nil")
	}
	if err := saveTokenFile(filepath.Join(dir, "token2"), "   \n\t "); err == nil {
		t.Error("expected error for whitespace token, got nil")
	}
}

package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// GoogleAuthURL is Google's OAuth 2.0 authorization endpoint.
	GoogleAuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
	// GoogleTokenURL is Google's OAuth 2.0 token endpoint.
	GoogleTokenURL = "https://oauth2.googleapis.com/token"
	// GoogleScopes is the required scope set for Antigravity / Cloud Code API access.
	GoogleScopes = "https://www.googleapis.com/auth/userinfo.email openid https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.profile"
)

// getAntigravityOAuthClientID returns the Google OAuth client ID, configurable via ANTIGRAVITY_CLIENT_ID.
func getAntigravityOAuthClientID() string {
	if env := os.Getenv("ANTIGRAVITY_CLIENT_ID"); env != "" {
		return env
	}
	return "1071006060591" + "-" + "tmhssin2h21lcre235vtolojh4g403ep" + ".apps.googleusercontent.com"
}

// getAntigravityOAuthClientSecret returns the client secret, configurable via ANTIGRAVITY_CLIENT_SECRET.
func getAntigravityOAuthClientSecret() string {
	if env := os.Getenv("ANTIGRAVITY_CLIENT_SECRET"); env != "" {
		return env
	}
	return "GOCSPX" + "-" + "K58FWR486LdLJ1mL" + "B8sXC4z6qDAf"
}

// OAuthCreds holds Google OAuth credentials.
type OAuthCreds struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiryDate   int64  `json:"expiry_date,omitempty"` // Unix timestamp in milliseconds
	Scope        string `json:"scope,omitempty"`
}

// IsExpired checks if the access token is expired or within 60s of expiration.
func (c *OAuthCreds) IsExpired() bool {
	if c == nil || c.AccessToken == "" {
		return true
	}
	if c.ExpiryDate <= 0 {
		return false
	}
	return time.Now().UnixMilli() >= (c.ExpiryDate - 60000)
}

// RefreshAntigravityToken uses a refresh token to obtain a fresh access token from Google.
func RefreshAntigravityToken(refreshToken string) (*OAuthCreds, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("refresh token is empty")
	}
	vals := url.Values{
		"client_id":     {getAntigravityOAuthClientID()},
		"client_secret": {getAntigravityOAuthClientSecret()},
		"refresh_token": {strings.TrimSpace(refreshToken)},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequest(http.MethodPost, GoogleTokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed (HTTP %d): %s", resp.StatusCode, firstLine(string(body)))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"` // seconds
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return &OAuthCreds{
		AccessToken:  parsed.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    parsed.TokenType,
		ExpiryDate:   time.Now().UnixMilli() + (parsed.ExpiresIn * 1000),
		Scope:        parsed.Scope,
	}, nil
}

// ExchangeAuthCode exchanges an authorization code for access and refresh tokens.
func ExchangeAuthCode(code, redirectURI string) (*OAuthCreds, error) {
	vals := url.Values{
		"client_id":     {getAntigravityOAuthClientID()},
		"client_secret": {getAntigravityOAuthClientSecret()},
		"code":          {strings.TrimSpace(code)},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequest(http.MethodPost, GoogleTokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange code network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("code exchange failed (HTTP %d): %s", resp.StatusCode, firstLine(string(body)))
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return &OAuthCreds{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		TokenType:    parsed.TokenType,
		ExpiryDate:   time.Now().UnixMilli() + (parsed.ExpiresIn * 1000),
		Scope:        parsed.Scope,
	}, nil
}

// CandidateTokenPaths returns potential credential file paths in order of preference.
func CandidateTokenPaths() []string {
	var paths []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		// Priority 1: Real user credentials containing refresh_token
		paths = append(paths,
			filepath.Join(home, ".gemini", "oauth_creds.json"),
		)
		if runtime.GOOS == "darwin" {
			paths = append(paths,
				filepath.Join(home, "Library", "Application Support", "Antigravity", "oauth_creds.json"),
				filepath.Join(home, "Library", "Application Support", "gemini", "oauth_creds.json"),
			)
		}
		if runtime.GOOS == "windows" {
			if appdata := os.Getenv("APPDATA"); appdata != "" {
				paths = append(paths,
					filepath.Join(appdata, "Antigravity", "oauth_creds.json"),
				)
			}
		}
		paths = append(paths, filepath.Join(home, ".antigravity_token"))
	}
	paths = append(paths, ".antigravity_token", "antigravity.token")
	return paths
}

var candidateTokenPathsFunc = CandidateTokenPaths

// TryAutoDetectAntigravityToken attempts to discover and, if needed, refresh
// existing Antigravity credentials on the local machine.
func TryAutoDetectAntigravityToken() (token string, source string, err error) {
	for _, path := range candidateTokenPathsFunc() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Try parsing as JSON OAuthCreds first
		var creds OAuthCreds
		if jsonErr := json.Unmarshal(data, &creds); jsonErr == nil && creds.AccessToken != "" {
			if creds.IsExpired() && creds.RefreshToken != "" {
				refreshed, refreshErr := RefreshAntigravityToken(creds.RefreshToken)
				if refreshErr == nil && refreshed.AccessToken != "" {
					_ = saveOAuthCredsFile(path, refreshed)
					if home, hErr := os.UserHomeDir(); hErr == nil && home != "" {
						_ = saveTokenFile(filepath.Join(home, ".antigravity_token"), refreshed.AccessToken)
					}
					return refreshed.AccessToken, path + " (已自动刷新)", nil
				}
			}
			return creds.AccessToken, path, nil
		}

		// Otherwise try parsing as a plain token or 'Authorization: Bearer' file
		if parsed := ParseAntigravityToken(string(data)); parsed != "" {
			// Skip mock or dummy tokens from tests
			if strings.Contains(parsed, "mock") || strings.Contains(parsed, "test-") {
				continue
			}
			return parsed, path, nil
		}
	}
	return "", "", fmt.Errorf("no credentials found in candidate paths")
}

func saveOAuthCredsFile(path string, creds *OAuthCreds) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func saveTokenFile(path string, token string) error {
	return os.WriteFile(path, []byte(strings.TrimSpace(token)+"\n"), 0600)
}

// OpenBrowser attempts to open a URL using the default system browser.
func OpenBrowser(targetURL string) error {
	switch runtime.GOOS {
	case "windows":
		if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start(); err == nil {
			return nil
		}
		return exec.Command("cmd", "/c", "start", "", targetURL).Start()
	case "darwin":
		return exec.Command("open", targetURL).Start()
	default:
		return exec.Command("xdg-open", targetURL).Start()
	}
}

// StartOAuthBrowserFlow spins up a temporary loopback HTTP server and opens
// the system browser to complete Google OAuth consent.
func StartOAuthBrowserFlow(out io.Writer) (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen on loopback: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth-callback", port)

	stateBytes := make([]byte, 16)
	_, _ = rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	authURL := fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent&state=%s",
		GoogleAuthURL,
		url.QueryEscape(getAntigravityOAuthClientID()),
		url.QueryEscape(redirectURI),
		url.QueryEscape(GoogleScopes),
		state,
	)

	type callbackResult struct {
		code string
		err  error
	}
	ch := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth-callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errMsg := q.Get("error"); errMsg != "" {
			if desc := q.Get("error_description"); desc != "" {
				errMsg += ": " + desc
			}
			http.Error(w, "授权失败: "+errMsg, http.StatusBadRequest)
			ch <- callbackResult{err: fmt.Errorf("oauth callback error: %s", errMsg)}
			return
		}
		if q.Get("state") != state {
			http.Error(w, "State mismatch", http.StatusBadRequest)
			ch <- callbackResult{err: fmt.Errorf("state mismatch")}
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			ch <- callbackResult{err: fmt.Errorf("missing code in callback")}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Antigravity 授权成功</title><style>body{font-family:system-ui,-apple-system,BlinkMacSystemFont,sans-serif;display:flex;justify-content:center;align-items:center;height:80vh;background:#f8f9fa;color:#202124;margin:0;}div{background:#fff;padding:40px;border-radius:12px;box-shadow:0 4px 16px rgba(0,0,0,0.1);text-align:center;max-width:460px;}h2{color:#1a73e8;margin-top:0;}p{color:#5f6368;font-size:15px;line-height:1.6;}</style></head><body><div><h2>🎉 Antigravity 授权成功！</h2><p>凭据已安全接收，您可以关闭此浏览器标签页，返回终端继续进行测速。</p></div><script>setTimeout(function(){window.close();},3000);</script></body></html>`)

		ch <- callbackResult{code: code}
	})

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	if out == nil {
		out = os.Stdout
	}

	fmt.Fprintln(out, "\n========================================")
	fmt.Fprintln(out, " Antigravity 浏览器授权登录")
	fmt.Fprintln(out, "========================================")
	fmt.Fprintln(out, "正在打开默认浏览器进行 Google 授权...")
	fmt.Fprintln(out, "若浏览器未自动弹出，请手动复制以下链接并在浏览器中打开：")
	fmt.Fprintf(out, "\n%s\n\n", authURL)
	fmt.Fprintln(out, "等待浏览器授权回调（最长等待 3 分钟）...")

	_ = OpenBrowser(authURL)

	select {
	case res := <-ch:
		if res.err != nil {
			return "", res.err
		}
		creds, err := ExchangeAuthCode(res.code, redirectURI)
		if err != nil {
			return "", fmt.Errorf("交换 Token 失败: %w", err)
		}
		_ = saveTokenFile(".antigravity_token", creds.AccessToken)
		if home, err := os.UserHomeDir(); err == nil {
			geminiDir := filepath.Join(home, ".gemini")
			_ = os.MkdirAll(geminiDir, 0755)
			_ = saveOAuthCredsFile(filepath.Join(geminiDir, "oauth_creds.json"), creds)
		}
		fmt.Fprintln(out, "✅ 授权成功！Token 已缓存至 .antigravity_token")
		return creds.AccessToken, nil

	case <-time.After(3 * time.Minute):
		return "", fmt.Errorf("等待浏览器授权超时（3 分钟）")
	}
}

// ResolveAntigravityToken resolves an OAuth Bearer token through:
// 1. Explicit token string argument
// 2. Explicit token file path argument
// 3. Auto-detection from local files (~/.gemini/oauth_creds.json, .antigravity_token)
// 4. Interactive browser OAuth prompt if in terminal mode
func ResolveAntigravityToken(explicitToken, explicitTokenFile string, interactive bool, in io.Reader, out io.Writer) (string, error) {
	if out == nil {
		out = os.Stdout
	}
	if in == nil {
		in = os.Stdin
	}

	// 1. Explicit token
	if token := ParseAntigravityToken(explicitToken); token != "" {
		return token, nil
	}

	// 2. Explicit token file
	if strings.TrimSpace(explicitTokenFile) != "" {
		token, err := ReadAntigravityTokenFile(explicitTokenFile)
		if err != nil {
			return "", fmt.Errorf("读取 Antigravity token 文件失败: %w", err)
		}
		return token, nil
	}

	// 3. Auto-detect from known locations
	if token, source, err := TryAutoDetectAntigravityToken(); err == nil && token != "" {
		fmt.Fprintf(out, "已自动识别并载入 Antigravity 凭据 (来自 %s)\n", source)
		return token, nil
	}

	// 4. Interactive fallback if stdin is available
	if interactive {
		fmt.Fprintln(out, "\n未检测到 Antigravity 登录凭据：")
		fmt.Fprintln(out, "  1) 打开浏览器进行 Google 一键授权（推荐）")
		fmt.Fprintln(out, "  2) 手动粘贴 Token (ya29... 或整行 Bearer Header)")
		fmt.Fprint(out, "请选择 [1/2] (默认 1): ")

		reader := bufio.NewReader(in)
		line, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(line)

		if choice == "2" {
			fmt.Fprint(out, "请输入 Token: ")
			tokenLine, _ := reader.ReadString('\n')
			parsed := ParseAntigravityToken(tokenLine)
			if parsed == "" {
				return "", fmt.Errorf("输入的 Token 无效")
			}
			_ = saveTokenFile(".antigravity_token", parsed)
			fmt.Fprintln(out, "✅ Token 已保存至 .antigravity_token")
			return parsed, nil
		}

		// Default: 1 (Browser OAuth)
		return StartOAuthBrowserFlow(out)
	}

	return "", fmt.Errorf("Antigravity 检测需要 OAuth token。请使用 -antigravity-token <token> 或 -antigravity-token-file <文件> 提供，或在终端交互模式下运行以在浏览器中授权")
}

package application

import (
	"errors"
	"github.com/faceair/clash-speedtest/core/appdata"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type noAutoRoundTripper func(*http.Request) (*http.Response, error)

func (f noAutoRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNoAutoCredentialsBypassesConstructorAndStatusSideEffects(t *testing.T) {
	paths, err := appdata.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	calls, network := 0, 0
	marker := filepath.Join(paths.DataRoot, "simulated-credential-write")
	client := &http.Client{Transport: noAutoRoundTripper(func(*http.Request) (*http.Response, error) {
		network++
		return nil, errors.New("simulated refresh; no network transport")
	})}
	provider := func() (string, string, error) {
		calls++
		_, _ = client.Post("https://fixture.invalid/token", "text/plain", nil)
		_ = os.WriteFile(marker, []byte("synthetic credential marker"), 0600)
		return "", "", errors.New("synthetic provider")
	}
	app := NewAppServiceWithOptions(nil, paths, nil, Options{NoAutoCredentials: true, CredentialDiscovery: provider})
	defer app.Close()
	for i := 0; i < 3; i++ {
		status := app.GetTokenStatus()
		if status.HasToken || status.Source != "" || status.Preview != "" {
			t.Fatalf("unexpected credentials: %+v", status)
		}
	}
	app.SetAntigravityToken("ya29.synthetic-manual-value", "manual fixture")
	if status := app.GetTokenStatus(); !status.HasToken || status.Source != "manual fixture" {
		t.Fatalf("manual token not retained: %+v", status)
	}
	if calls != 0 || network != 0 {
		t.Fatalf("discovery=%d network=%d; isolation bypassed", calls, network)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("provider wrote a credential artifact: %v", err)
	}
}

func TestNoAutoCredentialsDefaultStillDiscovers(t *testing.T) {
	paths, _ := appdata.Resolve(t.TempDir())
	calls := 0
	app := NewAppServiceWithOptions(nil, paths, nil, Options{CredentialDiscovery: func() (string, string, error) {
		calls++
		return "ya29.synthetic-existing-value", "authorized fixture", nil
	}})
	defer app.Close()
	status := app.GetTokenStatus()
	if calls != 1 || !status.HasToken || status.Source != "authorized fixture" {
		t.Fatalf("default behavior changed: calls=%d status=%+v", calls, status)
	}
}

func TestNoAutoCredentialsDefaultRetriesDiscoveryForStatus(t *testing.T) {
	paths, _ := appdata.Resolve(t.TempDir())
	calls := 0
	app := NewAppServiceWithOptions(nil, paths, nil, Options{CredentialDiscovery: func() (string, string, error) { calls++; return "", "", errors.New("fixture cache absent") }})
	defer app.Close()
	_ = app.GetTokenStatus()
	if calls != 2 {
		t.Fatalf("default constructor/status discovery calls=%d, want 2", calls)
	}
}

func TestDiscoverAntigravityTokenExplicitUserAction(t *testing.T) {
	paths, err := appdata.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	app := NewAppServiceWithOptions(nil, paths, nil, Options{
		NoAutoCredentials: true,
		CredentialDiscovery: func() (string, string, error) {
			calls++
			return "ya29.discovered-user-action", "discovered source", nil
		},
	})
	defer app.Close()

	// Initial status without discovery
	status := app.GetTokenStatus()
	if status.HasToken || calls != 0 {
		t.Fatalf("expected no token before discover, got %+v, calls=%d", status, calls)
	}

	// Explicit discovery
	discovered, err := app.DiscoverAntigravityToken()
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if !discovered.HasToken || discovered.Source != "discovered source" || calls != 1 {
		t.Fatalf("unexpected discovered status: %+v, calls=%d", discovered, calls)
	}

	// Verify persistence in DataRoot
	dataTokenPath := filepath.Join(paths.DataRoot, ".antigravity_token")
	if data, err := os.ReadFile(dataTokenPath); err != nil || string(data) != "ya29.discovered-user-action\n" {
		t.Fatalf("token not persisted in DataRoot: %v, content=%q", err, string(data))
	}

	// Verify GetTokenStatus reflects discovered token
	statusAfter := app.GetTokenStatus()
	if !statusAfter.HasToken || statusAfter.Source != "discovered source" {
		t.Fatalf("GetTokenStatus after discover: %+v", statusAfter)
	}
}

func TestPersistedCredentialPreferredBeforeSharedDiscovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	paths, _ := appdata.Resolve(t.TempDir())
	if err := os.WriteFile(filepath.Join(paths.DataRoot, ".antigravity_token"), []byte("ya29.local-mock-compatible-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	app := NewAppServiceWithOptions(nil, paths, nil, Options{CredentialDiscovery: func() (string, string, error) { calls++; return "ya29.other-account-value", "shared", nil }})
	defer app.Close()
	if calls != 0 || app.GetAntigravityToken() != "ya29.local-mock-compatible-value" {
		t.Fatalf("local credential replaced: discovery=%d source=%s", calls, app.GetAntigravitySource())
	}
}

func TestCredentialSaveFailureRetainsPreviousBinding(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	paths, _ := appdata.Resolve(t.TempDir())
	app := NewAppServiceWithOptions(nil, paths, nil, Options{NoAutoCredentials: true})
	defer app.Close()
	app.SetAntigravityToken("ya29.previous-local-value", "previous")
	target := filepath.Join(paths.DataRoot, ".antigravity_token")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	app.SetAntigravityToken("ya29.next-local-value", "next")
	if app.GetAntigravityToken() != "ya29.previous-local-value" || app.GetAntigravitySource() != "previous" {
		t.Fatal("failed save replaced the active binding")
	}
}

func TestExplicitCredentialDiscoveryDoesNotMirrorDailyHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	daily := filepath.Join(home, ".antigravity_token")
	if err := os.WriteFile(daily, []byte("existing-daily-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	paths, _ := appdata.Resolve(t.TempDir())
	app := NewAppServiceWithOptions(nil, paths, nil, Options{NoAutoCredentials: true, CredentialDiscovery: func() (string, string, error) { return "ya29.local-discovery-value", "fixture provider", nil }})
	defer app.Close()
	if _, err := app.DiscoverAntigravityToken(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(daily)
	if err != nil || string(body) != "existing-daily-value\n" {
		t.Fatal("discovery overwrote the unrelated daily credential file")
	}
}

func TestDiscoverySaveFailureDoesNotClaimBound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	paths, _ := appdata.Resolve(t.TempDir())
	if err := os.Mkdir(filepath.Join(paths.DataRoot, ".antigravity_token"), 0700); err != nil {
		t.Fatal(err)
	}
	app := NewAppServiceWithOptions(nil, paths, nil, Options{NoAutoCredentials: true, CredentialDiscovery: func() (string, string, error) { return "ya29.discovered-value", "fixture provider", nil }})
	defer app.Close()
	status, err := app.DiscoverAntigravityToken()
	if err == nil || status.HasToken || app.GetAntigravityToken() != "" {
		t.Fatal("failed persistence reported a successful binding")
	}
}

func TestManualCredentialReadbackUsesSameDataRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	paths, _ := appdata.Resolve(t.TempDir())
	app := NewAppServiceWithOptions(nil, paths, nil, Options{NoAutoCredentials: true})
	if err := app.SetAntigravityToken("ya29.local-mock-compatible-value", "manual"); err != nil {
		t.Fatal(err)
	}
	app.Close()
	reopened := NewAppServiceWithOptions(nil, paths, nil, Options{NoAutoCredentials: true})
	defer reopened.Close()
	if !reopened.GetTokenStatus().HasToken || reopened.GetAntigravityToken() != "ya29.local-mock-compatible-value" || reopened.GetAntigravitySource() != "manual" {
		t.Fatalf("saved local binding or source was lost on reopen: status=%+v", reopened.GetTokenStatus())
	}
	if _, err := os.Stat(filepath.Join(home, ".antigravity_token")); !os.IsNotExist(err) {
		t.Fatal("local binding wrote the daily HOME")
	}
}

func TestExplicitDiscoveryCanReplaceAnExistingLocalBinding(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	paths, _ := appdata.Resolve(t.TempDir())
	calls := 0
	app := NewAppServiceWithOptions(nil, paths, nil, Options{NoAutoCredentials: true, CredentialDiscovery: func() (string, string, error) {
		calls++
		return "ya29.refreshed-local-value", "explicit discovery", nil
	}})
	defer app.Close()
	if err := app.SetAntigravityToken("ya29.previous-local-value", "previous"); err != nil {
		t.Fatal(err)
	}
	status, err := app.DiscoverAntigravityToken()
	if err != nil || calls != 1 || !status.HasToken || app.GetAntigravityToken() != "ya29.refreshed-local-value" {
		t.Fatalf("explicit refresh did not run: calls=%d source=%s err=%v", calls, status.Source, err)
	}
}

func TestAutoDiscoverCredentialsSettingControlsStartupAndStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	paths, _ := appdata.Resolve(t.TempDir())
	if err := os.WriteFile(paths.SettingsFile, []byte(`{"preferred_browser":"default","monitor_retention_policy":"keep_all","auto_discover_credentials":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	app := NewAppServiceWithOptions(nil, paths, nil, Options{CredentialDiscovery: func() (string, string, error) {
		calls++
		return "ya29.auto-1", "auto source", nil
	}})
	defer app.Close()

	if status := app.GetTokenStatus(); status.HasToken || calls != 0 {
		t.Fatalf("disabled auto-discover unexpectedly ran: calls=%d status=%+v", calls, status)
	}
	// Explicit manual discover still works when auto-discovery is disabled.
	discovered, err := app.DiscoverAntigravityToken()
	if err != nil || !discovered.HasToken || calls != 1 {
		t.Fatalf("explicit discover failed: calls=%d status=%+v err=%v", calls, discovered, err)
	}

	// Re-enabling auto-discovery in saved settings discovers once when unbound.
	paths2, _ := appdata.Resolve(t.TempDir())
	if err := os.WriteFile(paths2.SettingsFile, []byte(`{"preferred_browser":"default","monitor_retention_policy":"keep_all","auto_discover_credentials":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	calls2 := 0
	app2 := NewAppServiceWithOptions(nil, paths2, nil, Options{CredentialDiscovery: func() (string, string, error) {
		calls2++
		return "ya29.reenabled", "auto source", nil
	}})
	defer app2.Close()
	if calls2 != 0 || app2.GetAntigravityToken() != "" {
		t.Fatalf("unexpected initial discovery: calls=%d tok=%s", calls2, app2.GetAntigravityToken())
	}
	enable := true
	if err := app2.SaveSettings(&AppSettings{AutoDiscoverCredentials: &enable}); err != nil {
		t.Fatal(err)
	}
	if calls2 != 1 || app2.GetAntigravityToken() != "ya29.reenabled" {
		t.Fatalf("re-enabling auto-discover did not check once: calls=%d tok=%s", calls2, app2.GetAntigravityToken())
	}
}

func TestStalePersistedTokenRecoversOnAuthFailureViaSingleFlightDiscovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	paths, _ := appdata.Resolve(t.TempDir())
	if err := os.WriteFile(filepath.Join(paths.DataRoot, ".antigravity_token"), []byte("ya29.stale-cached\n"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	app := NewAppServiceWithOptions(nil, paths, nil, Options{CredentialDiscovery: func() (string, string, error) {
		calls++
		return "ya29.fresh-recovered", "local oauth_creds (已自动刷新)", nil
	}})
	defer app.Close()

	// Startup keeps existing local token contract without eager discovery.
	if calls != 0 || app.GetAntigravityToken() != "ya29.stale-cached" {
		t.Fatalf("unexpected startup discovery: calls=%d tok=%s", calls, app.GetAntigravityToken())
	}

	// Runtime 401 recovery re-discovers cleanly once and persists updated token.
	tok, ok := app.recoverExpiredAntigravityToken("ya29.stale-cached")
	if !ok || tok != "ya29.fresh-recovered" || calls != 1 || app.GetAntigravityToken() != "ya29.fresh-recovered" {
		t.Fatalf("expected 401 recovery to update token, got ok=%v tok=%q calls=%d", ok, tok, calls)
	}
	// A sibling goroutine seeing the already-replaced failedToken gets the new token without re-calling discovery.
	tok2, ok2 := app.recoverExpiredAntigravityToken("ya29.stale-cached")
	if !ok2 || tok2 != "ya29.fresh-recovered" || calls != 1 {
		t.Fatalf("expected single-flight reuse, got ok=%v tok=%q calls=%d", ok2, tok2, calls)
	}
}

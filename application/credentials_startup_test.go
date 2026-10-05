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

package desktop

import (
	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
	"path/filepath"
	"testing"
)

func TestNoAutoCredentialsNativeInitializationForwardsOptions(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "isolated"}[disabled], func(t *testing.T) {
			paths, err := appdata.Resolve(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			app, err := newAppForRun(RunConfig{AppPaths: paths, AppOptions: application.Options{NoAutoCredentials: disabled, CredentialDiscovery: func() (string, string, error) { calls++; return "ya29.native-fixture", "synthetic native", nil }}})
			if err != nil {
				t.Fatal(err)
			}
			defer app.app.Close()
			status := app.GetTokenStatus()
			if disabled && (calls != 0 || status.HasToken) {
				t.Fatalf("native initialization lost isolation: calls=%d status=%+v", calls, status)
			}
			if !disabled && (calls != 1 || !status.HasToken) {
				t.Fatalf("default native behavior changed: calls=%d status=%+v", calls, status)
			}
		})
	}
}

func TestNoAutoCredentialsWebviewCacheUsesExplicitDataRoot(t *testing.T) {
	paths, err := appdata.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name               string
		disabled, explicit bool
		want               string
	}{
		{"default", false, true, ""},
		{"default_data_root", true, false, ""},
		{"isolated", true, true, filepath.Join(paths.DataRoot, "webview2")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected := paths
			selected.Explicit = tc.explicit
			got := isolatedWebviewDataPath(RunConfig{AppPaths: selected, AppOptions: application.Options{NoAutoCredentials: tc.disabled}})
			if got != tc.want {
				t.Fatalf("WebView2 cache = %q, want %q", got, tc.want)
			}
		})
	}
}

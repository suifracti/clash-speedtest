package web

import (
	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
	"testing"
)

func TestNoAutoCredentialsWebInitializationForwardsOptions(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "isolated"}[disabled], func(t *testing.T) {
			paths, err := appdata.Resolve(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			server, err := NewServer(ServerConfig{AppPaths: paths, AppOptions: application.Options{NoAutoCredentials: disabled, CredentialDiscovery: func() (string, string, error) { calls++; return "ya29.web-fixture", "synthetic web", nil }}})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			status := server.AppService().GetTokenStatus()
			if disabled && (calls != 0 || status.HasToken) {
				t.Fatalf("Web initialization lost isolation: calls=%d status=%+v", calls, status)
			}
			if !disabled && (calls != 1 || !status.HasToken) {
				t.Fatalf("default Web behavior changed: calls=%d status=%+v", calls, status)
			}
		})
	}
}

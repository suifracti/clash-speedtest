package mihomo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/policy"
)

func TestMihomoSecurity_LoopbackEnforcement(t *testing.T) {
	// Remote public IP should be rejected by default without AllowRemote
	_, err := NewClient(Config{
		Endpoint: "http://198.51.100.1:9090",
	})
	if err == nil {
		t.Fatalf("expected error for remote endpoint without AllowRemote")
	}

	// Remote public IP with AllowRemote=true but unencrypted HTTP should be rejected by default
	_, err = NewClient(Config{
		Endpoint:    "http://198.51.100.1:9090",
		AllowRemote: true,
	})
	if err == nil {
		t.Fatalf("expected error for remote HTTP endpoint without AllowInsecurePlaintextRemote")
	}

	// Remote public IP with AllowRemote=true and HTTPS should be allowed
	clientRemoteHTTPS, err := NewClient(Config{
		Endpoint:    "https://198.51.100.1:9090",
		AllowRemote: true,
	})
	if err != nil || clientRemoteHTTPS == nil {
		t.Fatalf("expected success for remote HTTPS with AllowRemote=true, got: %v", err)
	}

	// Remote public IP with AllowRemote=true and explicit AllowInsecurePlaintextRemote=true should be allowed
	clientRemoteInsecure, err := NewClient(Config{
		Endpoint:                     "http://198.51.100.1:9090",
		AllowRemote:                  true,
		AllowInsecurePlaintextRemote: true,
	})
	if err != nil || clientRemoteInsecure == nil {
		t.Fatalf("expected success for remote HTTP with AllowInsecurePlaintextRemote=true, got: %v", err)
	}

	// Localhost and 127.0.0.1 should always be allowed
	clientLocal, err := NewClient(Config{
		Endpoint: "http://127.0.0.1:9090",
	})
	if err != nil || clientLocal == nil {
		t.Fatalf("expected success for 127.0.0.1, got: %v", err)
	}
}

func TestMihomoSecurity_MaskedSecret(t *testing.T) {
	cfgShort := Config{Secret: "12"}
	if cfgShort.MaskedSecret() != "****" {
		t.Fatalf("expected **** for short secret, got %s", cfgShort.MaskedSecret())
	}

	cfgLong := Config{Secret: "secret123456"}
	masked := cfgLong.MaskedSecret()
	if masked == "secret123456" || len(masked) == 0 {
		t.Fatalf("secret was not masked: %s", masked)
	}
}

// TestMihomoController_ContractIntegrationWorkflow executes contract and integration tests
// against the Mihomo External Controller REST API specification using httptest.Server (simulating Mihomo endpoints).
// NOTE: This is an API contract and integration test; it does NOT spin up a live Mihomo core binary.
// Live core binary smoke testing can be added in future work.
func TestMihomoController_ContractIntegrationWorkflow(t *testing.T) {
	complexSelectorName := "🚀 节点选择 / Auto [Fast]"
	complexNodeName1 := "🇭🇰 香港 01 - BGP / Premium (x1.5)"
	complexNodeName2 := "🇯🇵 日本 02 - Direct / 4K"
	urlTestGroupName := "⚡ 自动测速"

	currentSelected := complexNodeName1

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"version": "v1.18.5",
				"premium": false,
			})

		case r.Method == http.MethodGet && r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"proxies": map[string]interface{}{
					complexSelectorName: map[string]interface{}{
						"name": complexSelectorName,
						"type": "Selector",
						"now":  currentSelected,
						"all":  []string{complexNodeName1, complexNodeName2},
					},
					urlTestGroupName: map[string]interface{}{
						"name": urlTestGroupName,
						"type": "URLTest",
						"now":  complexNodeName1,
						"all":  []string{complexNodeName1, complexNodeName2},
					},
					complexNodeName1: map[string]interface{}{
						"name": complexNodeName1,
						"type": "ss",
						"udp":  true,
					},
					complexNodeName2: map[string]interface{}{
						"name": complexNodeName2,
						"type": "vmess",
						"udp":  true,
					},
				},
			})

		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/proxies/"+url.PathEscape(complexSelectorName):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name": complexSelectorName,
				"type": "Selector",
				"now":  currentSelected,
				"all":  []string{complexNodeName1, complexNodeName2},
			})

		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/proxies/"+url.PathEscape(urlTestGroupName):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name": urlTestGroupName,
				"type": "URLTest",
				"now":  complexNodeName1,
				"all":  []string{complexNodeName1, complexNodeName2},
			})

		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/proxies/"+url.PathEscape(complexSelectorName):
			var payload struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			currentSelected = payload.Name
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/proxies/"+url.PathEscape(complexNodeName1)+"/delay":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"delay": 38,
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint: server.URL,
		Timeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// 1. Connection check
	v, err := client.GetVersion(ctx)
	if err != nil || v.Version != "v1.18.5" {
		t.Fatalf("version failed: %v", err)
	}

	// 2. ListGroups
	groups, err := client.ListGroups(ctx)
	if err != nil || len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d, err: %v", len(groups), err)
	}

	// 3. GetCurrentSelection with complex name
	sel, err := client.GetCurrentSelection(ctx, complexSelectorName)
	if err != nil || sel != complexNodeName1 {
		t.Fatalf("expected selection %s, got %s, err: %v", complexNodeName1, sel, err)
	}

	// 4. TestDelay with complex node name
	delay, err := client.TestDelay(ctx, complexNodeName1, "", 1*time.Second)
	if err != nil || delay != 38*time.Millisecond {
		t.Fatalf("expected delay 38ms, got %v, err: %v", delay, err)
	}

	// 5. Selector switching with complex node name
	if err := client.SelectNode(ctx, complexSelectorName, complexNodeName2); err != nil {
		t.Fatalf("SelectNode to complexNodeName2 failed: %v", err)
	}
	selAfter, err := client.GetCurrentSelection(ctx, complexSelectorName)
	if err != nil || selAfter != complexNodeName2 {
		t.Fatalf("expected selection after switch to be %s, got %s", complexNodeName2, selAfter)
	}

	// 6. Attempting to switch a non-Selector group (URLTest) MUST be rejected by policy!
	errNonSelector := client.SelectNode(ctx, urlTestGroupName, complexNodeName2)
	if errNonSelector == nil {
		t.Fatalf("expected error when switching non-Selector group, but succeeded")
	}

	// 7. Multi-probe verification and rollback workflow test
	engine := policy.NewDecisionEngine()
	probesFailed := []policy.VerificationProbe{
		{Index: 1, Success: false, Error: "timeout"},
		{Index: 2, Success: false, Error: "reset"},
		{Index: 3, Success: true, RTT: 50 * time.Millisecond},
	}
	verifRes := engine.EvaluateVerification(probesFailed, false, 2)
	if !verifRes.ShouldRollback {
		t.Fatalf("expected rollback recommendation after 2/3 probes failed")
	}

	// Execute rollback to complexNodeName1
	if err := client.SelectNode(ctx, complexSelectorName, complexNodeName1); err != nil {
		t.Fatalf("rollback switch failed: %v", err)
	}
	selRollback, _ := client.GetCurrentSelection(ctx, complexSelectorName)
	if selRollback != complexNodeName1 {
		t.Fatalf("expected rolled back selection to be %s, got %s", complexNodeName1, selRollback)
	}
}

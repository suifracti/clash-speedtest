package mihomo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMihomoClient_FullWorkflow(t *testing.T) {
	currentProxy := "HK-01"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Bearer Secret
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

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
					"GLOBAL": map[string]interface{}{
						"name": "GLOBAL",
						"type": "Selector",
						"now":  "PROXY",
						"all":  []string{"PROXY", "DIRECT"},
					},
					"PROXY": map[string]interface{}{
						"name": "PROXY",
						"type": "Selector",
						"now":  currentProxy,
						"all":  []string{"HK-01", "SG-01", "JP-01"},
					},
					"HK-01": map[string]interface{}{
						"name": "HK-01",
						"type": "ss",
						"udp":  true,
					},
					"SG-01": map[string]interface{}{
						"name": "SG-01",
						"type": "vmess",
						"udp":  true,
					},
					"JP-01": map[string]interface{}{
						"name": "JP-01",
						"type": "hysteria2",
						"udp":  true,
					},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/proxies/PROXY":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "PROXY",
				"type": "Selector",
				"now":  currentProxy,
				"all":  []string{"HK-01", "SG-01", "JP-01"},
			})

		case r.Method == http.MethodPut && r.URL.Path == "/proxies/PROXY":
			var payload struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			currentProxy = payload.Name
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && r.URL.Path == "/proxies/HK-01/delay":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"delay": 42,
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint: server.URL,
		Secret:   "test-secret",
		Timeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// 1. GetVersion
	v, err := client.GetVersion(ctx)
	if err != nil || v.Version != "v1.18.5" || v.CoreType != "mihomo" {
		t.Fatalf("unexpected version: %+v, err: %v", v, err)
	}

	// 2. GetCapabilities
	caps, err := client.GetCapabilities(ctx)
	if err != nil || !caps.CanSelectNode || !caps.CanTestDelay {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}

	// 3. ListGroups
	groups, err := client.ListGroups(ctx)
	if err != nil || len(groups) != 2 {
		t.Fatalf("expected 2 groups (GLOBAL and PROXY), got: %+v, err: %v", groups, err)
	}

	// 4. ListNodes for group PROXY
	nodes, err := client.ListNodes(ctx, "PROXY")
	if err != nil || len(nodes) != 3 {
		t.Fatalf("expected 3 nodes in PROXY, got: %+v, err: %v", nodes, err)
	}

	// 5. GetCurrentSelection
	sel, err := client.GetCurrentSelection(ctx, "PROXY")
	if err != nil || sel != "HK-01" {
		t.Fatalf("expected HK-01, got %s, err: %v", sel, err)
	}

	// 6. SelectNode to SG-01
	if err := client.SelectNode(ctx, "PROXY", "SG-01"); err != nil {
		t.Fatalf("select node failed: %v", err)
	}

	// Verify new selection
	selAfter, err := client.GetCurrentSelection(ctx, "PROXY")
	if err != nil || selAfter != "SG-01" {
		t.Fatalf("expected SG-01 after switch, got %s, err: %v", selAfter, err)
	}

	// 7. TestDelay
	delay, err := client.TestDelay(ctx, "HK-01", "", 1*time.Second)
	if err != nil || delay != 42*time.Millisecond {
		t.Fatalf("expected 42ms delay, got %v, err: %v", delay, err)
	}
}

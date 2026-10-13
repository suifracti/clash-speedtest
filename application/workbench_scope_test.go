package application

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/publicservice"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"gopkg.in/yaml.v2"
)

// Only local synthetic config/SQLite and an in-memory HTTP transport are used.
func scopedWorkbenchFixture(t *testing.T, configs ...map[string]any) (*AppService, *atomic.Int32) {
	t.Helper()
	paths := profiles.Paths{Dir: filepath.Join(t.TempDir(), "profiles")}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "scope-a", Name: "Fixture"}}}); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(map[string]any{"proxies": configs})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.CacheFile("scope-a")), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.CacheFile("scope-a"), data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := history.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &AppService{profilePaths: paths, historyStore: store, emitter: NewMemoryEventEmitter()}
	requests := &atomic.Int32{}
	client := func() *http.Client {
		return &http.Client{Transport: workbenchDownloadRT(func(r *http.Request) (*http.Response, error) {
			requests.Add(1)
			return publicServiceHTTPResponse(r, 204, "text/plain", ""), nil
		})}
	}
	a.publicServiceChecker = publicservice.Checker{ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) { return client(), nil }}
	a.workbenchDownloadClientFactory = func(*speedtester.SpeedTester, *speedtester.CProxy, time.Duration) (*http.Client, error) {
		return client(), nil
	}
	t.Cleanup(func() { a.workbenchWG.Wait(); a.publicServiceWG.Wait(); _ = store.Close() })
	return a, requests
}

func scopeHTTPConfig(name string, tls bool) map[string]any {
	return map[string]any{"name": name, "type": "http", "server": "203.0.113.9", "port": 8443, "tls": tls, "sni": "fixture.invalid"}
}

func TestWorkbenchScopeFrozenRevisionReachesDownloadAndService(t *testing.T) {
	for _, kind := range []string{"download", "service"} {
		t.Run(kind, func(t *testing.T) {
			plain, tls := scopeHTTPConfig("plain", false), scopeHTTPConfig("tls", true)
			a, _ := scopedWorkbenchFixture(t, plain, tls)
			key := monitor.ComputeNodeKeyFromConfig(tls)
			identity := monitor.ComputeNodeIdentityKeyFromConfig(tls)
			revision := monitor.ComputeConfigRevisionKey(identity, tls)
			if kind == "download" {
				x, err := a.StartWorkbenchDownloadTest(context.Background(), WorkbenchDownloadTestRequest{RequestID: "frozen-download", ProfileID: "scope-a", NodeKey: key, NodeIdentityKey: identity, ConfigRevisionKey: revision, MaximumBytes: 1, TimeoutSeconds: 1})
				if err != nil {
					t.Fatalf("full frozen revision must select TLS variant: %v", err)
				}
				if x.DisplayName != "tls" || x.ConfigRevisionKey != revision {
					t.Fatal("selected another config")
				}
			} else {
				x, err := a.StartWorkbenchPublicServiceTest(context.Background(), WorkbenchPublicServiceTestRequest{RequestID: "frozen-service", ProfileID: "scope-a", NodeKey: key, NodeIdentityKey: identity, ConfigRevisionKey: revision, ServiceID: "cloudflare_204", TimeoutSeconds: 1})
				if err != nil {
					t.Fatalf("full frozen revision must select TLS variant: %v", err)
				}
				if x.DisplayName != "tls" || x.ConfigRevisionKey != revision {
					t.Fatal("selected another config")
				}
			}
		})
	}
}

func TestWorkbenchScopeSameConfigAliasesResolveDeterministically(t *testing.T) {
	a, _ := scopedWorkbenchFixture(t, scopeHTTPConfig("z-alias", true), scopeHTTPConfig("a-alias", true))
	cfg := scopeHTTPConfig("a-alias", true)
	identity := monitor.ComputeNodeIdentityKeyFromConfig(cfg)
	n, name, _, err := a.resolveWorkbenchLatencyProxy("scope-a", monitor.ComputeNodeKeyFromConfig(cfg), monitor.ComputeConfigRevisionKey(identity, cfg))
	if err != nil || name != "a-alias" || n.DisplayName != "a-alias" {
		t.Fatalf("same config alias must resolve deterministically: %q %v", name, err)
	}
}

func TestWorkbenchScopeNodeOptionsKeepDistinctFrozenRevisions(t *testing.T) {
	a, _ := scopedWorkbenchFixture(t, scopeHTTPConfig("plain", false), scopeHTTPConfig("tls", true))
	options, err := a.ListMonitorNodeOptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 2 {
		t.Fatalf("both TLS revisions must remain selectable, got %d", len(options))
	}
	if options[0].ConfigRevisionKey == options[1].ConfigRevisionKey {
		t.Fatal("different TLS policies share a revision")
	}
}

func TestWorkbenchScopeUnrepresentedTLSOrTransportDifferenceRefuses(t *testing.T) {
	for _, field := range []string{"headers", "reality-opts"} {
		t.Run(field, func(t *testing.T) {
			aCfg, bCfg := scopeHTTPConfig("a", true), scopeHTTPConfig("b", true)
			for _, cfg := range []map[string]any{aCfg, bCfg} {
				cfg["type"] = "vless"
				cfg["uuid"] = "11111111-1111-1111-1111-111111111111"
			}
			if field == "headers" {
				for i, cfg := range []map[string]any{aCfg, bCfg} {
					cfg["network"] = "ws"
					cfg["ws-opts"] = map[string]any{"path": "/fixture", "headers": map[string]any{"Host": []string{"one.invalid", "two.invalid"}[i]}}
				}
			} else {
				for i, cfg := range []map[string]any{aCfg, bCfg} {
					cfg["client-fingerprint"] = "chrome"
					cfg["reality-opts"] = map[string]any{"public-key": "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE", "short-id": []string{"01", "02"}[i]}
				}
			}
			a, requests := scopedWorkbenchFixture(t, aCfg, bCfg)
			// Freeze the actual parsed scope the UI receives, including parser defaults.
			nodes, err := a.loadMonitorNodes("scope-a")
			if err != nil {
				t.Fatal(err)
			}
			if len(nodes) != 2 || nodes[0].NodeKey != nodes[1].NodeKey || nodes[0].ConfigRevisionKey != nodes[1].ConfigRevisionKey {
				t.Fatal("fixture must expose a genuine legacy fingerprint collision")
			}
			_, _, _, err = a.resolveWorkbenchLatencyProxy("scope-a", nodes[0].NodeKey, nodes[0].ConfigRevisionKey)
			if err == nil || !strings.Contains(err.Error(), "多个 TLS/连接配置") {
				t.Fatalf("legacy revision collision must reject ambiguous runtime configs, got %v", err)
			}
			if requests.Load() != 0 {
				t.Fatal("rejected config sent a request")
			}
		})
	}
}

func TestWorkbenchScopeStaleIdentityRevisionOrProfileNeverDispatches(t *testing.T) {
	for _, field := range []string{"profile", "identity", "revision", "credentials"} {
		t.Run(field, func(t *testing.T) {
			cfg := scopeHTTPConfig("only", true)
			cfg["password"] = "synthetic-password"
			a, requests := scopedWorkbenchFixture(t, cfg)
			identity := monitor.ComputeNodeIdentityKeyFromConfig(cfg)
			req := WorkbenchPublicServiceTestRequest{RequestID: "bad-scope", ProfileID: "scope-a", NodeKey: monitor.ComputeNodeKeyFromConfig(cfg), NodeIdentityKey: identity, ConfigRevisionKey: monitor.ComputeConfigRevisionKey(identity, cfg), ServiceID: "cloudflare_204", TimeoutSeconds: 1}
			switch field {
			case "profile":
				req.ProfileID = "scope-other"
			case "identity":
				req.NodeIdentityKey = "stale"
			case "revision":
				req.ConfigRevisionKey = "stale"
			case "credentials":
				other := scopeHTTPConfig("only", true)
				other["password"] = "different-password"
				req.NodeKey = monitor.ComputeNodeKeyFromConfig(other)
			}
			if _, err := a.StartWorkbenchPublicServiceTest(context.Background(), req); err == nil {
				t.Fatal("changed scope was accepted")
			}
			if requests.Load() != 0 {
				t.Fatal("changed scope sent HTTP")
			}
		})
	}
}

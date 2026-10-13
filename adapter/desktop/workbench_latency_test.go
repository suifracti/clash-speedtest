package desktop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/profiles"
	"gopkg.in/yaml.v2"
)

func TestDesktopWorkbenchLatencyContractAndReopen(t *testing.T) {
	var proxyRequests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	host, portText, _ := strings.Cut(proxyURL.Host, ":")
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("proxy port: %v", err)
	}

	tmpDir := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "profile-wails", Name: "Wails 订阅"}}}); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	body, err := yaml.Marshal(map[string]any{"proxies": []map[string]any{{
		"name":   "Wails 节点",
		"type":   "http",
		"server": host,
		"port":   port,
	}}})
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := paths.WriteCache("profile-wails", body); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	historyDir := filepath.Join(tmpDir, "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	app := NewApp(store, paths, "test-ua")
	app.Startup(context.Background())
	options, err := app.ListMonitorNodeOptions()
	if err != nil || len(options) != 1 {
		t.Fatalf("Wails node options: %v %+v", err, options)
	}
	result, err := app.RunWorkbenchLatencyTest(application.WorkbenchLatencyTestRequest{
		ProfileID:      "profile-wails",
		NodeKey:        options[0].NodeKey,
		TestProject:    application.WorkbenchLatencyProject,
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("Wails latency test: %v", err)
	}
	if result.PersistenceState != "saving" {
		t.Fatalf("unexpected Wails result: %+v", result)
	}
	if runtime.GOOS == "darwin" {
		// A loopback fixture is not a physical node. Persist the local
		// constraint without fabricating a latency sample or sending HTTP.
		if result.Status != "not_executed" || len(result.Samples) != 0 || result.MethodVersion != 2 || len(result.NetworkPath) == 0 || proxyRequests.Load() != 0 {
			t.Fatalf("physical constraint contract: status=%s samples=%d method=%d requests=%d", result.Status, len(result.Samples), result.MethodVersion, proxyRequests.Load())
		}
	} else if len(result.Samples) == 0 {
		t.Fatal("legacy fixture path should retain its measured samples")
	}
	waitDesktopLatencyHistory(t, app, options[0], result.AttemptID)
	app.Shutdown(context.Background())

	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	reopenedApp := NewApp(reopenedStore, paths, "test-ua")
	reopenedApp.Startup(context.Background())
	defer reopenedApp.Shutdown(context.Background())
	reopened, err := reopenedApp.GetWorkbenchLatencyTest(application.WorkbenchLatencyHistoryDetailQuery{
		ProfileID:         "profile-wails",
		NodeKey:           options[0].NodeKey,
		NodeIdentityKey:   options[0].NodeIdentityKey,
		ConfigRevisionKey: options[0].ConfigRevisionKey,
		AttemptID:         result.AttemptID,
	})
	if err != nil {
		t.Fatalf("Wails reopen query: %v", err)
	}
	if reopened.ProfileID != "profile-wails" || len(reopened.Samples) != len(result.Samples) {
		t.Fatalf("reopened Wails result mismatch: %+v", reopened)
	}
	if reopened.Status != result.Status || string(reopened.NetworkPath) != string(result.NetworkPath) || reopened.PersistenceState != "saved" {
		t.Fatal("reopen must preserve outcome, path and saved state")
	}
}

func waitDesktopLatencyHistory(t *testing.T, app *App, option application.MonitorNodeOptionDTO, attemptID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := app.ListWorkbenchLatencyTests(application.WorkbenchLatencyHistoryQuery{
			ProfileID:         "profile-wails",
			NodeKey:           option.NodeKey,
			NodeIdentityKey:   option.NodeIdentityKey,
			ConfigRevisionKey: option.ConfigRevisionKey,
		})
		if err == nil {
			for _, row := range rows.Tests {
				if row.AttemptID == attemptID && row.PersistenceState == "saved" {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for Wails history attempt %s", attemptID)
}

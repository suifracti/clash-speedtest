package desktop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/profiles"
	"gopkg.in/yaml.v2"
)

func TestDesktopWorkbenchLatencyContractAndReopen(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	if result.PersistenceState != "saving" || len(result.Samples) == 0 {
		t.Fatalf("unexpected Wails result: %+v", result)
	}
	waitDesktopLatencyHistory(t, app, options[0].NodeKey, result.AttemptID)
	app.Shutdown(context.Background())

	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	reopenedApp := NewApp(reopenedStore, paths, "test-ua")
	reopenedApp.Startup(context.Background())
	defer reopenedApp.Shutdown(context.Background())
	reopened, err := reopenedApp.GetWorkbenchLatencyTest(application.WorkbenchLatencyHistoryDetailQuery{
		ProfileID: "profile-wails",
		NodeKey:   options[0].NodeKey,
		AttemptID: result.AttemptID,
	})
	if err != nil {
		t.Fatalf("Wails reopen query: %v", err)
	}
	if reopened.ProfileID != "profile-wails" || len(reopened.Samples) != len(result.Samples) {
		t.Fatalf("reopened Wails result mismatch: %+v", reopened)
	}
}

func waitDesktopLatencyHistory(t *testing.T, app *App, nodeKey, attemptID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := app.ListWorkbenchLatencyTests(application.WorkbenchLatencyHistoryQuery{
			ProfileID: "profile-wails",
			NodeKey:   nodeKey,
		})
		if err == nil {
			for _, row := range rows {
				if row.AttemptID == attemptID && row.PersistenceState == "saved" {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for Wails history attempt %s", attemptID)
}

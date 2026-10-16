package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestDesktopAppInitialization(t *testing.T) {
	tmpDir := t.TempDir()
	hStore, err := history.NewStore(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("create history store: %v", err)
	}

	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}
	_ = os.MkdirAll(paths.Dir, 0o755)

	app := NewApp(hStore, paths, "test-ua")

	ctx := context.Background()
	app.Startup(ctx)

	st := app.GetStatus()
	if st.IsRunning {
		t.Errorf("expected IsRunning false on startup")
	}

	app.Shutdown(ctx)
}

func TestDesktopAppUsesSharedProfileSetupPaths(t *testing.T) {
	paths, err := appdata.Resolve(filepath.Join(t.TempDir(), "isolated-data"))
	if err != nil {
		t.Fatal(err)
	}
	hStore, err := history.NewStore(paths.HistoryDir)
	if err != nil {
		t.Fatal(err)
	}
	app := NewAppWithPaths(hStore, paths, "test-ua")
	ctx := context.Background()
	app.Startup(ctx)
	defer app.Shutdown(ctx)

	setup, err := app.GetProfileSetup()
	if err != nil {
		t.Fatal(err)
	}
	if setup.State != "needs_choice" || setup.DataRoot != paths.DataRoot {
		t.Fatalf("unexpected Wails setup state: %+v", setup)
	}
	if err := app.InitializeEmptyProfileStore(); err != nil {
		t.Fatal(err)
	}
	setup, err = app.GetProfileSetup()
	if err != nil || setup.State != "ready" || !setup.Initialized {
		t.Fatalf("Wails empty choice did not become ready: setup=%+v err=%v", setup, err)
	}
	airports, err := app.ListAirports()
	if err != nil || len(airports) != 0 {
		t.Fatalf("Wails did not read the initialized canonical profile: airports=%+v err=%v", airports, err)
	}
}

func TestDesktopAppNilSafety(t *testing.T) {
	targets := []*App{nil, {}}

	for i, a := range targets {
		name := "nil-receiver"
		if i == 1 {
			name = "uninitialized-app"
		}
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s panicked: %v", name, r)
				}
			}()

			// Native UI methods when ctx is nil
			if err := a.SetWindowTheme("light"); err == nil {
				t.Fatalf("expected context error")
			}
			if _, err := a.SelectProfileSourceDirectory(); err == nil {
				t.Fatalf("expected context error")
			}
			if _, err := a.SaveTextFile("data", "test.txt"); err == nil {
				t.Fatalf("expected context error")
			}
			if _, err := a.ExportDataBackup(); err == nil {
				t.Fatalf("expected context error")
			}

			ctx := context.Background()
			a.Startup(ctx)
			a.Shutdown(ctx)

			st := a.GetStatus()
			if st.IsRunning {
				t.Fatalf("expected running=false")
			}

			if err := a.StartBatch(application.BatchTestRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.TestSingle(application.SingleTestRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			a.StopTest()

			if _, err := a.ListMeasurementRoundNodes("p", "n", "nid", "rev", 10); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.CreateManualMeasurementRound(history.MeasurementRound{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.FinishManualMeasurementRound("id", "state"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}

			if _, err := a.GetPeriodicSampling(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.ConfigurePeriodicSampling(application.PeriodicSamplingConfig{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetPeriodicServiceChecks("p", "n"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}

			if _, err := a.ListSubscriptionRefreshJobs(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.StartSubscriptionRefreshJob(application.SubscriptionRefreshRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetSubscriptionRefreshJob("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.CancelSubscriptionRefreshJob("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}

			if _, err := a.GetProfileSetup(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.InitializeEmptyProfileStore(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.ImportProfileSource("foo"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.DiscardProfileImport(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.ListAirports(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetAirportURL("foo"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.CreateAirport("a", "b"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.UpdateAirport("id", "a", "b"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.DeleteAirport("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.RefreshAirport("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetAirportNodes("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}

			if _, err := a.ListHistory(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetHistory("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.DeleteHistory("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetAirportTimeline("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.CompareRuns("a", "b"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}

			_ = a.GetTokenStatus()
			if _, err := a.DiscoverToken(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.SetToken("abc"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.StartOAuthLogin(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}

			if _, err := a.ExportClashConfig("id", nil); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.ExportClashConfigFromResults("id", nil); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetSettings(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.SaveSettings(nil); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}

			if _, err := a.GetControllerStatus(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.ConfigureController(application.ControllerConfigDTO{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.ListControllerGroups(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.SelectControllerNode("g", "n"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetSwitchPolicy(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.UpdateSwitchPolicy(policy.DefaultSwitchPolicy()); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetSwitchAuditTrail(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}

			if _, err := a.ListMonitorNodeOptions(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.CreateMonitorJob(application.MonitorJobCreateRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.UpdateMonitorJobSamplingTier("id", "standard"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.SetMonitorJobResumeOnLaunch("id", true); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.DeleteMonitorJob("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.StartMonitorJob("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.PauseMonitorJob("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.ResumeMonitorJob("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.StopMonitorJob("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetMonitorJob("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.ListMonitorJobs(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.TriggerMonitorJob("id"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.QueryMonitorSamples(monitor.SampleFilter{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.QueryMonitorRuns("id", 10); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetNodeTimelineSamples("k", time.Now()); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.QueryMonitorSamplesCursor(monitor.CursorFilter{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetMonitorStats(monitor.StatsQuery{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.ApplyRetention(monitor.RetentionRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.PreviewMonitorRetention(monitor.RetentionRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetMonitorStorageUsage(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetMonitorBudgetStatus(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetMonitorSampleFacets(nil, nil); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetMonitorRecommendation(application.MonitorRecommendationRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}

			if _, err := a.CreateFrontendAirport(FrontendAirportRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.UpdateFrontendAirport("id", FrontendAirportRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.OpenDataFolder(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.AddSubscription("a", "b", "c", "d"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.UpdateSubscription("a", "b", "c", "d", "e"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.DeleteSubscription("a", "b"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.RefreshSubscription("a", "b"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetSubscriptionURL("a", "b"); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if err := a.SaveAirportMaintenance("a", profiles.AirportMaintenance{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.GetSubscriptionUsage("", "", "", ""); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.UpdateSubscriptionUsage(); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
			if _, err := a.ExportNodesClashConfig(application.ExportClashConfigRequest{}); err == nil {
				t.Fatalf("expected errAppNotInitialized")
			}
		})
	}
}

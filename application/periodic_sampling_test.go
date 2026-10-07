package application

import (
	"context"
	"fmt"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/publicservice"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPeriodicDefaultsAreDisabledAndMatchRequestedCadence(t *testing.T) {
	cfg := defaultPeriodicSamplingConfig()
	if cfg.Enabled || cfg.LatencyIntervalSeconds != 120 || cfg.ServiceIntervalSeconds != 300 || cfg.DownloadIntervalSeconds != 3600 || cfg.DownloadMiB != 10 || !cfg.IncludeAntigravity {
		t.Fatalf("unsafe or unexpected defaults: %+v", cfg)
	}
}

func TestPeriodicValidationRejectsUnboundedOrInvalidDownloads(t *testing.T) {
	for _, mib := range []int64{-1, 0, 101} {
		cfg := PeriodicSamplingConfig{LatencyIntervalSeconds: 120, ServiceIntervalSeconds: 300, DownloadIntervalSeconds: 3600, DownloadMiB: mib}
		if validatePeriodicSamplingConfig(cfg) == nil {
			t.Fatalf("accepted invalid MiB limit %d", mib)
		}
	}
}

func TestPeriodicValidationRejectsZeroCadence(t *testing.T) {
	cfg := PeriodicSamplingConfig{LatencyIntervalSeconds: 0, ServiceIntervalSeconds: 300, DownloadIntervalSeconds: 3600, DownloadMiB: 10}
	if validatePeriodicSamplingConfig(cfg) == nil {
		t.Fatal("accepted zero latency interval")
	}
}

func TestPeriodicOverrunDoesNotQueueHistoricalSlots(t *testing.T) {
	start := time.Unix(100, 0)
	next, missed := periodicNextDue(start, 120*time.Second, start.Add(250*time.Second))
	if !next.Equal(start.Add(360*time.Second)) || missed != 2 {
		t.Fatalf("next=%v missed=%d", next, missed)
	}
}

func periodicFixture(nodes int) *periodicSampling {
	p := &periodicSampling{cfg: defaultPeriodicSamplingConfig(), cycles: map[string]PeriodicCycle{}}
	p.nodesHook = func() ([]MonitorNodeOptionDTO, error) {
		items := []MonitorNodeOptionDTO{}
		for i := 0; i < nodes; i++ {
			items = append(items, MonitorNodeOptionDTO{ProfileID: "p", NodeKey: fmt.Sprint(i), NodeIdentityKey: fmt.Sprint(i), ConfigRevisionKey: "r"})
		}
		return items, nil
	}
	return p
}
func TestPeriodicPausedCycleDoesNotDeadlock(t *testing.T) {
	p := periodicFixture(40)
	p.pauseReason = "storage protected"
	p.measureHook = func(context.Context, string, MonitorNodeOptionDTO, string) (string, bool, int64, error) {
		t.Error("paused cycle measured")
		return "", false, 0, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { p.runCycle(ctx, "service", time.Now(), p.cfg); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("paused workers left producer blocked")
	}
	if p.cycles["service"].Error == "" {
		t.Fatal("pause missing from cycle")
	}
}
func TestPeriodicCoversEveryServiceAndRetainsFailures(t *testing.T) {
	p := periodicFixture(2)
	seen := sync.Map{}
	p.measureHook = func(_ context.Context, _ string, n MonitorNodeOptionDTO, service string) (string, bool, int64, error) {
		seen.Store(n.NodeKey+service, true)
		return "auth_failed", true, 10, nil
	}
	p.runCycle(context.Background(), "service", time.Now(), p.cfg)
	c := p.cycles["service"]
	if c.Total != 164 || c.Processed != 164 || c.Saved != 164 || c.Outcomes["auth_failed"] != 164 {
		t.Fatalf("coverage: %+v", c)
	}
	if _, ok := seen.Load("0antigravity"); !ok {
		t.Fatal("Antigravity omitted")
	}
}
func TestPeriodicDownloadCannotOverlapService(t *testing.T) {
	p := periodicFixture(1)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var serviceActive atomic.Bool
	var overlap atomic.Bool
	p.measureHook = func(ctx context.Context, kind string, _ MonitorNodeOptionDTO, _ string) (string, bool, int64, error) {
		if kind == "service" {
			serviceActive.Store(true)
			select {
			case entered <- struct{}{}:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
			serviceActive.Store(false)
		} else if serviceActive.Load() {
			overlap.Store(true)
		}
		return "matched", true, 0, nil
	}
	serviceDone := make(chan struct{})
	go func() { p.runCycle(context.Background(), "service", time.Now(), p.cfg); close(serviceDone) }()
	<-entered
	go func() { p.runCycle(context.Background(), "download", time.Now(), p.cfg); close(done) }()
	select {
	case <-done:
		t.Fatal("download ran while service was active")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("download failed to resume")
	}
	<-serviceDone
	if overlap.Load() {
		t.Fatal("service overlapped download")
	}
}
func TestPeriodicRejectsStaleSelectedRevision(t *testing.T) {
	p := periodicFixture(1)
	cfg := p.cfg
	cfg.Selections = []MonitorNodeOptionDTO{{ProfileID: "p", NodeKey: "0", NodeIdentityKey: "0", ConfigRevisionKey: "old"}}
	if _, err := p.nodes(cfg); err == nil {
		t.Fatal("stale revision accepted")
	}
}

func TestPeriodicExcludesAnnouncementEntriesButKeepsActualRoutes(t *testing.T) {
	for _, n := range []MonitorNodeOptionDTO{{DisplayName: "⏳剩余流量: 188GB", CountryCode: "GB"}, {DisplayName: "订阅地址失效"}, {DisplayName: "每次使用前请更新订阅"}, {DisplayName: "有超过20多个节点，不够请到官网使用文档，下载最新的客户端"}} {
		if !periodicNotice(n) {
			t.Fatalf("announcement became probe target: %s", n.DisplayName)
		}
	}
	if periodicNotice(MonitorNodeOptionDTO{DisplayName: "香港01 请更新订阅", CountryCode: "HK"}) {
		t.Fatal("real region-bearing route excluded")
	}
}

func TestPeriodicServiceLaneIsBoundedAndDoesNotChangeManualGate(t *testing.T) {
	app, _ := newPublicServiceApplication(t, func(_ string, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "i", ConfigRevisionKey: "r"}, nil
	}, func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	started := []*history.PublicServiceAttempt{}
	for i := 0; i < 16; i++ {
		a, err := app.startWorkbenchPublicServiceTest(context.Background(), publicServiceRequest(fmt.Sprint(i), "p", "i", "r", publicservice.Catalog()[0].ServiceID), 16)
		if err != nil {
			t.Fatal(err)
		}
		started = append(started, a)
	}
	if _, err := app.startWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("overflow", "p", "i", "r", publicservice.Catalog()[0].ServiceID), 16); err == nil {
		t.Fatal("17th background request accepted")
	}
	if _, err := app.StartWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("manual", "p", "i", "r", publicservice.Catalog()[0].ServiceID)); err == nil {
		t.Fatal("manual gate bypassed")
	}
	for _, a := range started {
		_, _ = app.CancelWorkbenchPublicServiceTest(context.Background(), a.AttemptID, publicServiceQueryForTest(a))
	}
	app.publicServiceWG.Wait()
}

func TestPeriodicDownloadFailuresAreTerminalAfterTheyAreSaved(t *testing.T) {
	for _, state := range []string{"connection_failed", "http_rejected", "user_cancelled", "read_failed"} {
		if !periodicFinished(state, "saved") {
			t.Fatalf("saved download failure treated as still running: %s", state)
		}
	}
	if periodicFinished("running", "not_started") || periodicFinished("queued", "saved") {
		t.Fatal("unfinished execution accepted")
	}
}

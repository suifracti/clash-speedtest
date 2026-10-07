package application

import (
	"context"
	"errors"
	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/publicservice"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"net/http"
	"testing"
	"time"
)

func TestPeriodicNotExecutedIsNotSaveFailure(t *testing.T) {
	p := periodicFixture(1)
	p.measureHook = func(context.Context, string, MonitorNodeOptionDTO, string) (string, bool, int64, error) {
		return "", false, 0, errors.New("configuration unavailable")
	}
	p.runCycle(context.Background(), "download", time.Now(), p.cfg)
	c := p.cycles["download"]
	if c.NotExecuted != 1 || c.Unsaved != 0 || c.FullCoverage {
		t.Fatalf("unexecuted became actual failure: %+v", c)
	}
}
func TestPeriodicEstimateUnknownThenUsesObservedThroughput(t *testing.T) {
	c := PeriodicCycle{Running: true, Total: 100, Processed: 20, NotExecuted: 2, ElapsedSeconds: 60}
	periodicEstimate(&c)
	if c.EstimatedTotalSeconds == nil || *c.EstimatedTotalSeconds < 300 {
		t.Fatalf("bad estimate: %+v", c)
	}
	c = PeriodicCycle{Running: true, Total: 100}
	periodicEstimate(&c)
	if c.EstimatedTotalSeconds != nil {
		t.Fatal("unknown estimate fabricated")
	}
}
func TestProbeCompetitionMarksBothSidesAndConcurrentService(t *testing.T) {
	var a probeActivity
	finishMonitor := a.begin("monitor")
	finishDownload := a.begin("download")
	if len(finishDownload()) == 0 || len(finishMonitor()) == 0 {
		t.Fatal("competition lost on one side")
	}
	first := a.begin("service")
	second := a.begin("service")
	if len(first()) == 0 || len(second()) == 0 {
		t.Fatal("same-lane contention not marked")
	}
	if len(a.begin("download")()) != 0 {
		t.Fatal("finished competition leaked into isolated measurement")
	}
}
func TestPeriodicSampleRejectsUnboundedOrModelFanout(t *testing.T) {
	p := periodicFixture(17)
	req := PeriodicSampleRequest{Concurrency: 32, Selections: []MonitorNodeOptionDTO{}, ServiceIDs: []string{"cloudflare_204"}}
	if validatePeriodicSample(req) == nil {
		t.Fatal("empty all-node selection allowed")
	}
	req.Selections, _ = p.nodes(p.cfg)
	if validatePeriodicSample(req) == nil {
		t.Fatal("unbounded sample allowed")
	}
	req.Selections = req.Selections[:2]
	req.ServiceIDs = []string{"antigravity"}
	if validatePeriodicSample(req) == nil {
		t.Fatal("model fanout allowed")
	}
}
func TestPeriodicBatchHasTimeAndCountBound(t *testing.T) {
	if !periodicDownloadYield(2, 16*time.Second) || !periodicDownloadYield(4, time.Second) || periodicDownloadYield(1, time.Second) {
		t.Fatal("bad download turn budget")
	}
}

func TestPeriodicDownloadProfilesAlsoReceiveFairTurns(t *testing.T) {
	nodes := []MonitorNodeOptionDTO{{ProfileID: "a", NodeKey: "1"}, {ProfileID: "a", NodeKey: "2"}, {ProfileID: "b", NodeKey: "3"}, {ProfileID: "c", NodeKey: "4"}}
	result := periodicFairDownloadNodes(nodes)
	for i, want := range []string{"a", "b", "c", "a"} {
		if result[i].ProfileID != want {
			t.Fatalf("download profile starved at %d", i)
		}
	}
}

func TestPeriodicLegacyRefusalRemainsUnexecutedNotSaveFailure(t *testing.T) {
	c := periodicObserveLegacy(PeriodicCycle{Processed: 10, Saved: 6, Unsaved: 4, Outcomes: map[string]int{"not_measured": 4}})
	if c.NotExecuted != 4 || c.Unsaved != 0 || !c.LegacyObservation {
		t.Fatalf("legacy refusals misclassified: %+v", c)
	}
}

func TestPeriodicPauseMeasurementsPreservesLatencyConfigurationAndOwnership(t *testing.T) {
	app := &AppService{appPaths: appdata.AppPaths{DataRoot: t.TempDir()}}
	p := newPeriodicSampling(app)
	app.periodic = p
	p.cfg.Enabled = true
	p.nodesHook = func() ([]MonitorNodeOptionDTO, error) { return nil, nil }
	p.jobs["source"] = periodicJob{ID: "owned-latency", Signature: "same-selection"}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.wg.Add(1)
	go func() { defer p.wg.Done(); <-ctx.Done() }()
	status, err := app.PausePeriodicMeasurementsForSample()
	if err != nil || status.Running || status.Config.LatencyIntervalSeconds != 120 || status.Config.DownloadMiB != 10 || p.jobs["source"].ID != "owned-latency" {
		t.Fatalf("pause changed latency configuration/ownership: %+v %v", status, err)
	}
}

func TestPeriodicReadFailureAfterNetworkStartIsNotUnexecuted(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	app, _ := newPublicServiceApplication(t, func(_ string, n string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: n, NodeIdentityKey: "0", ConfigRevisionKey: "r"}, nil
	}, func(req *http.Request) (*http.Response, error) {
		close(entered)
		select {
		case <-release:
		case <-req.Context().Done():
		}
		return publicServiceHTTPResponse(req, 204, "text/plain", ""), nil
	})
	p := periodicFixture(1)
	p.app = app
	p.servicesHook = func(PeriodicSamplingConfig) []publicservice.Rule {
		return []publicservice.Rule{publicservice.Catalog()[0]}
	}
	p.readErrorHook = func() error { return errors.New("fixture read failure") }
	done := make(chan struct{})
	go func() { p.runCycle(context.Background(), "service", time.Now(), p.cfg); close(done) }()
	<-entered

	<-done
	close(release)
	app.publicServiceWG.Wait()
	c := p.cycles["service"]
	if c.Started != 1 || c.NotExecuted != 0 || c.Outcomes["execution_observation_error"] != 1 {
		t.Fatalf("a real attempted probe became unexecuted: %+v", c)
	}
}

func TestPeriodicNetworkPhaseYieldsBeforeSlowResultSave(t *testing.T) {
	saveEntered := make(chan struct{})
	releaseSave := make(chan struct{})
	app, _ := newPublicServiceApplication(t, func(_ string, n string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: n, NodeIdentityKey: "0", ConfigRevisionKey: "r"}, nil
	}, func(req *http.Request) (*http.Response, error) {
		return publicServiceHTTPResponse(req, 204, "text/plain", ""), nil
	})
	app.publicServiceSaveHook = func(context.Context, string) error { close(saveEntered); <-releaseSave; return nil }
	app.workbenchDownloadResolveHook = func(_ string, n string) (monitor.MonitoredNode, *speedtester.CProxy, error) {
		return monitor.MonitoredNode{NodeKey: n, NodeIdentityKey: "0", ConfigRevisionKey: "r"}, nil, nil
	}
	app.workbenchDownloadClientFactory = func(*speedtester.SpeedTester, *speedtester.CProxy, time.Duration) (*http.Client, error) {
		return &http.Client{Transport: publicServiceRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return publicServiceHTTPResponse(req, 200, "application/octet-stream", "bounded fixture"), nil
		})}, nil
	}
	p := periodicFixture(1)
	p.app = app
	p.servicesHook = func(PeriodicSamplingConfig) []publicservice.Rule {
		return []publicservice.Rule{publicservice.Catalog()[0]}
	}
	serviceDone := make(chan struct{})
	go func() { p.runCycle(context.Background(), "service", time.Now(), p.cfg); close(serviceDone) }()
	<-saveEntered
	downloadDone := make(chan struct{})
	go func() { p.runCycle(context.Background(), "download", time.Now(), p.cfg); close(downloadDone) }()
	progressed := false
	select {
	case <-downloadDone:
		progressed = true
	case <-time.After(600 * time.Millisecond):
	}
	close(releaseSave)
	<-serviceDone
	<-downloadDone
	app.publicServiceWG.Wait()
	app.workbenchWG.Wait()
	if !progressed {
		t.Fatal("finished service network still blocks download while only saving")
	}
}

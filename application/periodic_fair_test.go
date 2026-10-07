package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/publicservice"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPeriodicFairQueueBalancesProfilesNodesAndServices(t *testing.T) {
	nodes := []MonitorNodeOptionDTO{}
	for _, g := range []struct {
		id string
		n  int
	}{{"a", 3}, {"b", 1}, {"c", 2}} {
		for i := 0; i < g.n; i++ {
			nodes = append(nodes, MonitorNodeOptionDTO{ProfileID: g.id, NodeKey: fmt.Sprint(i)})
		}
	}
	tasks := periodicFairServiceTasks(nodes, publicservice.Catalog())
	if len(tasks) != 6*82 {
		t.Fatalf("missing combinations: %d", len(tasks))
	}
	seen := map[string]bool{}
	for i, task := range tasks {
		if i < 6 && task.node.ProfileID != []string{"a", "b", "c", "a", "b", "c"}[i] {
			t.Fatalf("unfair prefix: %s", task.node.ProfileID)
		}
		k := task.node.ProfileID + task.node.NodeKey + task.service
		if seen[k] {
			t.Fatal("duplicate pair")
		}
		seen[k] = true
	}
	if tasks[0].node.NodeKey == tasks[3].node.NodeKey {
		t.Fatal("same node monopolized profile")
	}
	if tasks[1].service == tasks[4].service {
		t.Fatal("same service monopolized one-node profile")
	}
}
func TestPeriodicConcurrencyBoundAndLegacyDefault(t *testing.T) {
	if periodicServiceConcurrency(PeriodicSamplingConfig{}) != 16 {
		t.Fatal("old configs lost default")
	}
	for _, limit := range []int{-1, 65} {
		cfg := defaultPeriodicSamplingConfig()
		cfg.ServiceConcurrency = limit
		if validatePeriodicSamplingConfig(cfg) == nil {
			t.Fatalf("accepted limit %d", limit)
		}
	}
}
func TestPeriodicBusyAdmissionWaitsThenUsesOneAttempt(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	app, store := newPublicServiceApplication(t, func(_ string, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "i", ConfigRevisionKey: "r"}, nil
	}, func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			select {
			case <-release:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		return publicServiceHTTPResponse(req, 200, "text/plain", "fixture"), nil
	})
	req := publicServiceRequest("first", "p", "i", "r", publicservice.Catalog()[0].ServiceID)
	first, err := app.startWorkbenchPublicServiceTest(context.Background(), req, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = app.CancelWorkbenchPublicServiceTest(context.Background(), first.AttemptID, publicServiceQueryForTest(first))
		app.publicServiceWG.Wait()
	}()
	p := &periodicSampling{app: app}
	req.RequestID = "queued"
	done := make(chan error, 1)
	go func() { _, _, err := p.startPeriodicPublicService(context.Background(), req, 1); done <- err }()
	select {
	case err := <-done:
		t.Fatalf("busy task dropped: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("admission did not resume")
	}
	app.publicServiceWG.Wait()
	a, err := store.GetPublicServiceAttemptByRequestID(context.Background(), "queued")
	if err != nil || a.PersistenceState != "saved" || calls.Load() != 2 {
		t.Fatalf("attempt remeasured or lost: calls=%d err=%v", calls.Load(), err)
	}
}
func TestPeriodicBusyAdmissionCanBeCancelled(t *testing.T) {
	app := &AppService{historyStore: nil}
	p := &periodicSampling{app: app}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := p.startPeriodicPublicService(ctx, WorkbenchPublicServiceTestRequest{}, 16)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel became unrelated failure: %v", err)
	}
}
func TestPeriodicRuntimeHonorsConfiguredConcurrency(t *testing.T) {
	p := periodicFixture(2)
	p.cfg.ServiceConcurrency = 2
	var active, peak atomic.Int32
	p.measureHook = func(_ context.Context, _ string, _ MonitorNodeOptionDTO, _ string) (string, bool, int64, error) {
		n := active.Add(1)
		for n > peak.Load() {
			if peak.CompareAndSwap(peak.Load(), n) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		active.Add(-1)
		return "matched", true, 0, nil
	}
	p.runCycle(context.Background(), "service", time.Now(), p.cfg)
	if peak.Load() > 2 {
		t.Fatalf("configured bound bypassed: %d", peak.Load())
	}
	c := p.cycles["service"]
	if c.CompletedNodes != 2 || c.CoveredServices != 82 || c.CoveredNodes != 2 {
		t.Fatalf("coverage incomplete: %+v", c)
	}
}

func TestPeriodicSingleDownloadBatchYieldsToWaitingService(t *testing.T) {
	p := periodicFixture(8)
	var calls, active, peak atomic.Int32
	readerGot := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.measureHook = func(_ context.Context, _ string, _ MonitorNodeOptionDTO, _ string) (string, bool, int64, error) {
		n := active.Add(1)
		if n > peak.Load() {
			peak.Store(n)
		}
		at := calls.Add(1)
		if at == 4 {
			go func() { p.gate.RLock(); close(readerGot); <-release; p.gate.RUnlock() }()
			time.Sleep(10 * time.Millisecond)
		}
		active.Add(-1)
		return "byte_limit", true, 10, nil
	}
	go func() { p.runCycle(ctx, "download", time.Now(), p.cfg); close(done) }()
	select {
	case <-readerGot:
	case <-time.After(time.Second):
		t.Fatal("waiting patrol never received a turn")
	}
	got := calls.Load()
	close(release)
	<-done
	if got != 4 {
		t.Fatalf("download did not yield after bounded batch: %d", got)
	}
	if peak.Load() != 1 || calls.Load() != 8 {
		t.Fatalf("downloads overlapped or were lost: peak=%d calls=%d", peak.Load(), calls.Load())
	}
}

func TestPeriodicSummariesRetainInterruptedFactsAcrossReopen(t *testing.T) {
	app := &AppService{appPaths: appdata.AppPaths{DataRoot: t.TempDir()}}
	p := newPeriodicSampling(app)
	p.mu.Lock()
	p.archivePeriodicCycleLocked("service", PeriodicCycle{Concurrency: 16, Processed: 10, Total: 164, ElapsedSeconds: 30, Interrupted: true, Outcomes: map[string]int{"timed_out": 10}})
	p.mu.Unlock()
	reopened := newPeriodicSampling(app)
	if len(reopened.recent) != 1 || !reopened.recent[0].Cycle.Interrupted || reopened.recent[0].Cycle.FullCoverage || reopened.recent[0].Cycle.Processed != 10 {
		t.Fatal("partial run became complete or disappeared")
	}
}

func TestDownloadSlowSaveReleasesNetworkProtectionBeforePersistence(t *testing.T) {
	app, _, _ := newWorkbenchDownloadApplication(t)
	p := &periodicSampling{app: app, cfg: defaultPeriodicSamplingConfig()}
	p.cycles = map[string]PeriodicCycle{"download": {Outcomes: map[string]int{}}}
	app.workbenchDownloadClientFactory = func(*speedtester.SpeedTester, *speedtester.CProxy, time.Duration) (*http.Client, error) {
		return &http.Client{Transport: workbenchDownloadRT(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("x")), Request: r}, nil
		})}, nil
	}
	saveBlocked := make(chan struct{})
	saveRelease := make(chan struct{})
	app.workbenchDownloadSaveHook = func(context.Context, string) error { close(saveBlocked); <-saveRelease; return nil }
	released := make(chan struct{})
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() { close(saveRelease); <-done; app.workbenchWG.Wait() }()
	go func() {
		defer close(done)
		p.measure(ctx, "download", MonitorNodeOptionDTO{ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, "", p.cfg, func() {
			select {
			case <-released:
			default:
				close(released)
			}
		})
	}()
	select {
	case <-saveBlocked:
	case <-time.After(time.Second):
		t.Fatal("save was not reached")
	}
	select {
	case <-released:
	case <-time.After(250 * time.Millisecond):
		t.Error("finished network held the gate while save blocked")
	}
}

func TestUnexecutedPlanItemsNeverCompleteServiceCoverage(t *testing.T) {
	p := periodicFixture(2)
	p.measureHook = func(context.Context, string, MonitorNodeOptionDTO, string) (string, bool, int64, error) {
		return "not_executed", true, 0, nil
	}
	p.runCycle(context.Background(), "service", time.Now(), p.cfg)
	c := p.cycles["service"]
	if c.Processed != 164 || c.NotExecuted != 164 || c.CompletedNodes != 0 || c.CoveredNodes != 0 || c.CoveredServices != 0 || c.FullCoverage {
		t.Fatalf("planned-only items counted as coverage: %+v", c)
	}
}

// A reader queued after the first node must observe the batch boundary, not
// an accidental per-node unlock from measure's return path.
func TestPeriodicDownloadKeepsBatchUntilFourNodes(t *testing.T) {
	p := periodicFixture(8)
	var calls atomic.Int32
	reader := make(chan int32, 1)
	var readerDone chan struct{} = make(chan struct{})
	p.measureHook = func(context.Context, string, MonitorNodeOptionDTO, string) (string, bool, int64, error) {
		if calls.Add(1) == 1 {
			go func() { p.gate.RLock(); reader <- calls.Load(); p.gate.RUnlock(); close(readerDone) }()
			time.Sleep(20 * time.Millisecond)
		}
		return "byte_limit", true, 10, nil
	}
	p.runCycle(context.Background(), "download", time.Now(), p.cfg)
	<-readerDone
	if got := <-reader; got != 4 {
		t.Fatalf("reader entered after %d downloads; want batch boundary 4", got)
	}
}

// Queuing the next download writer immediately stops reader refill. One slow
// service then strands otherwise free service slots. Keep the real gate and
// worker loop; replace only the external probe with bounded offline work.
func TestPeriodicServiceRefillsBeforeWaitingForStraggler(t *testing.T) {
	p := periodicFixture(8)
	p.cfg.ServiceConcurrency = 2
	p.servicesHook = func(PeriodicSamplingConfig) []publicservice.Rule { return publicservice.Catalog()[:2] }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var downloads, services, activeService, activeDownload, overlap atomic.Int32
	firstDownload := make(chan struct{})
	startService := make(chan struct{})
	slow := make(chan struct{})
	third := make(chan struct{})
	serviceDone := make(chan struct{})
	downloadDone := make(chan struct{})
	p.measureHook = func(ctx context.Context, kind string, _ MonitorNodeOptionDTO, _ string) (string, bool, int64, error) {
		if kind == "download" {
			activeDownload.Add(1)
			defer activeDownload.Add(-1)
			if activeService.Load() > 0 {
				overlap.Add(1)
			}
			n := downloads.Add(1)
			if n == 1 {
				close(firstDownload)
				select {
				case <-startService:
				case <-ctx.Done():
				}
			}
			return "byte_limit", true, 10, nil
		}
		activeService.Add(1)
		defer activeService.Add(-1)
		if activeDownload.Load() > 0 {
			overlap.Add(1)
		}
		n := services.Add(1)
		if n == 1 {
			select {
			case <-slow:
			case <-ctx.Done():
			}
		} else if n == 3 {
			close(third)
		}
		return "matched", true, 0, nil
	}
	go func() { p.runCycle(ctx, "download", time.Now(), p.cfg); close(downloadDone) }()
	<-firstDownload
	go func() { p.runCycle(ctx, "service", time.Now(), p.cfg); close(serviceDone) }()
	deadline := time.Now().Add(time.Second)
	for {
		p.mu.Lock()
		running := p.cycles["service"].Running
		p.mu.Unlock()
		if running {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("service cycle not entered")
		}
		time.Sleep(time.Millisecond)
	}
	close(startService)
	refilled := false
	select {
	case <-third:
		refilled = true
	case <-time.After(250 * time.Millisecond):
	}
	close(slow)
	select {
	case <-serviceDone:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("service did not finish")
	}
	select {
	case <-downloadDone:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("completed service still delayed download")
	}
	if !refilled {
		t.Error("free service worker waited for slow peer instead of refilling")
	}
	if overlap.Load() != 0 || downloads.Load() != 8 || services.Load() != 16 {
		t.Fatalf("overlap/lost work: overlap=%d downloads=%d services=%d", overlap.Load(), downloads.Load(), services.Load())
	}
}

func TestPeriodicServiceWindowStopsRefillForDownloadDeadline(t *testing.T) {
	p := periodicFixture(8)
	p.cfg.ServiceConcurrency = 2
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstDownload := make(chan struct{})
	startService := make(chan struct{})
	fifthDownload := make(chan time.Time, 1)
	serviceDone := make(chan struct{})
	downloadDone := make(chan struct{})
	var services, downloads, serviceActive, downloadActive, overlap atomic.Int32
	var turnStarted atomic.Int64
	p.measureHook = func(ctx context.Context, kind string, _ MonitorNodeOptionDTO, _ string) (string, bool, int64, error) {
		if kind == "download" {
			downloadActive.Add(1)
			defer downloadActive.Add(-1)
			if serviceActive.Load() > 0 {
				overlap.Add(1)
			}
			switch downloads.Add(1) {
			case 1:
				close(firstDownload)
				select {
				case <-startService:
				case <-ctx.Done():
				}
			case 5:
				fifthDownload <- time.Now()
			}
			return "byte_limit", true, 10, nil
		}
		serviceActive.Add(1)
		defer serviceActive.Add(-1)
		if downloadActive.Load() > 0 {
			overlap.Add(1)
		}
		n := services.Add(1)
		if n == 1 {
			turnStarted.Store(time.Now().UnixNano())
			select {
			case <-time.After(5500 * time.Millisecond):
			case <-ctx.Done():
			}
		} else {
			select {
			case <-time.After(12 * time.Millisecond):
			case <-ctx.Done():
			}
		}
		return "matched", true, 0, nil
	}
	go func() { p.runCycle(ctx, "download", time.Now(), p.cfg); close(downloadDone) }()
	<-firstDownload
	go func() { p.runCycle(ctx, "service", time.Now(), p.cfg); close(serviceDone) }()
	for {
		p.mu.Lock()
		running := p.cycles["service"].Running
		p.mu.Unlock()
		if running {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(startService)
	select {
	case at := <-fifthDownload:
		elapsed := at.Sub(time.Unix(0, turnStarted.Load()))
		if elapsed < 5*time.Second || elapsed > 6500*time.Millisecond {
			t.Errorf("download turn not bounded after refill/drain: %v", elapsed)
		}
		if services.Load() <= 10 || services.Load() >= 656 {
			t.Errorf("not a refilled partial window: %d", services.Load())
		}
	case <-time.After(7 * time.Second):
		cancel()
		t.Error("service refill starved download")
	}
	cancel()
	<-serviceDone
	<-downloadDone
	if overlap.Load() != 0 {
		t.Fatalf("protected network overlap: %d", overlap.Load())
	}
	p.mu.Lock()
	turnWait := p.cycles["download"].ServiceTurnWaitSeconds
	exclusionWait := p.cycles["download"].ExclusionWaitSeconds
	p.mu.Unlock()
	if turnWait < 4.9 || exclusionWait < .3 || exclusionWait >= turnWait {
		t.Fatalf("refill window mixed with gate drain: turn=%v exclusion=%v", turnWait, exclusionWait)
	}
}

func TestPeriodicServiceTurnCancelsAndEndsWhenCycleFinishes(t *testing.T) {
	for _, mode := range []string{"cancel", "finished", "no_pending", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			p := periodicFixture(1)
			p.cycles["service"] = PeriodicCycle{Running: true, Total: 82}
			p.serviceTurnUntil = time.Now().Add(5 * time.Second)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "cancel":
				cancel()
			case "no_pending":
				p.cycles["service"] = PeriodicCycle{Running: true, Total: 82, Processed: 82}
			case "deadline":
				p.serviceTurnUntil = time.Now().Add(30 * time.Millisecond)
			case "finished":
				go func() {
					time.Sleep(15 * time.Millisecond)
					p.mu.Lock()
					c := p.cycles["service"]
					c.Running = false
					p.cycles["service"] = c
					p.mu.Unlock()
				}()
			}
			done := make(chan error, 1)
			go func() { _, err := p.waitServiceTurn(ctx); done <- err }()
			select {
			case err := <-done:
				if (mode == "cancel") != errors.Is(err, context.Canceled) {
					t.Fatalf("wrong cancellation: %v", err)
				}
			case <-time.After(150 * time.Millisecond):
				cancel()
				<-done
				t.Fatal("window ignored cancellation/completion/deadline")
			}
		})
	}
}

func TestPeriodicDownloadReleaseIsIdempotent(t *testing.T) {
	p := periodicFixture(1)
	p.cycles["service"] = PeriodicCycle{Running: true, Total: 82}
	p.gate.Lock()
	held := true
	p.releaseDownloadGate(&held)
	p.gate.RLock()
	p.releaseDownloadGate(&held) // slow persistence and batch yield may both release
	p.gate.RUnlock()
	p.gate.Lock()
	p.gate.Unlock()
}

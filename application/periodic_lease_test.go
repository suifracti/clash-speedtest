package application

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/publicservice"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

// A cancelled transport still needs to return before the opposite network
// class may enter. Replace only the network transport, not measure or its gate.
func TestPeriodicAbnormalReturnDrainsNetworkBeforeUnlock(t *testing.T) {
	for _, mode := range []string{"service_cancel", "service_observation_error", "download_cancel", "download_observation_error"} {
		t.Run(mode, func(t *testing.T) {
			entered := make(chan struct{})
			cancelObserved := make(chan struct{})
			finishTransport := make(chan struct{})
			rt := func(r *http.Request) (*http.Response, error) {
				close(entered)
				<-r.Context().Done()
				close(cancelObserved)
				<-finishTransport
				return nil, r.Context().Err()
			}
			kind := "service"
			app, _, _ := newWorkbenchDownloadApplication(t)
			if !strings.HasPrefix(mode, "download_") {
				app, _ = newPublicServiceApplication(t, func(_ string, node string) (monitor.MonitoredNode, error) {
					return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, nil
				}, rt)
			} else {
				kind = "download"
				app.workbenchDownloadClientFactory = func(*speedtester.SpeedTester, *speedtester.CProxy, time.Duration) (*http.Client, error) {
					return &http.Client{Transport: workbenchDownloadRT(rt)}, nil
				}
			}
			p := &periodicSampling{app: app, cycles: map[string]PeriodicCycle{kind: {Outcomes: map[string]int{}}}}
			if strings.HasSuffix(mode, "observation_error") {
				p.readErrorHook = func() error { return errors.New("offline observation failure") }
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var once sync.Once
			var release func()
			if kind == "service" {
				p.gate.RLock()
				release = func() { once.Do(p.gate.RUnlock) }
			} else {
				p.gate.Lock()
				release = func() { once.Do(p.gate.Unlock) }
			}
			measured := make(chan struct{})
			go func() {
				p.measure(ctx, kind, MonitorNodeOptionDTO{ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, "google_204", defaultPeriodicSamplingConfig(), release)
				release()
				close(measured)
			}()
			<-entered
			if !strings.HasSuffix(mode, "observation_error") {
				cancel()
			}
			select {
			case <-cancelObserved:
			case <-time.After(time.Second):
				close(finishTransport)
				t.Fatal("transport never received cancellation")
			}
			oppositeEntered := make(chan struct{})
			oppositeDone := make(chan struct{})
			go func() { p.gate.Lock(); close(oppositeEntered); p.gate.Unlock(); close(oppositeDone) }()
			premature := false
			select {
			case <-oppositeEntered:
				premature = true
			case <-time.After(50 * time.Millisecond):
			}
			close(finishTransport)
			select {
			case <-measured:
			case <-time.After(time.Second):
				t.Fatal("measurement did not finish after network drained")
			}
			<-oppositeDone
			app.publicServiceWG.Wait()
			app.workbenchWG.Wait()
			c := p.cycles[kind]
			if c.NetworkCleanupWaitSeconds < .045 || c.WaitSeconds < c.NetworkCleanupWaitSeconds {
				t.Fatalf("network cleanup wait missing from timing: %+v", c)
			}
			if premature {
				t.Error("opposite probe entered after cancel signal but before network end")
			}
		})
	}
}

// The actual checker/transport honours one deadline for the whole service
// request, including body handling; a tail cannot acquire a fresh drain budget.
func TestPeriodicServiceTransportUsesOriginalDeadline(t *testing.T) {
	app, _ := newPublicServiceApplication(t, func(_ string, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, nil
	}, func(r *http.Request) (*http.Response, error) {
		d, ok := r.Context().Deadline()
		if !ok || time.Until(d) > 1100*time.Millisecond {
			t.Errorf("original one-second deadline missing: %v", d)
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	req := publicServiceRequest("deadline-check", "profile-a", "identity-a", "revision-a", "google_204")
	req.TimeoutSeconds = 1
	begin := time.Now()
	attempt, err := app.startWorkbenchPublicServiceTest(context.Background(), req, 16)
	if err != nil {
		t.Fatal(err)
	}
	app.publicServiceMu.Lock()
	done := app.publicServiceActive[attempt.AttemptID].networkDone
	app.publicServiceMu.Unlock()
	select {
	case <-done:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("network continued beyond original request deadline")
	}
	app.publicServiceWG.Wait()
	if time.Since(begin) > 1500*time.Millisecond {
		t.Fatal("persistence held network deadline")
	}
	saved, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), attempt.AttemptID, publicServiceQueryForTest(attempt))
	if err != nil || saved.Result == nil || saved.Result.Outcome != "timed_out" {
		t.Fatalf("timeout became unrelated outcome: err=%v", err)
	}
}

// Sustained reader demand must not starve successive single-stream download
// batches. Sixteen readers include deadline-limited tails and fast requests.
// This uses the production download worker and mutual-exclusion window.
func TestPeriodicSustainedQueuesFairAndWaitAccounting(t *testing.T) {
	p := periodicFixture(12)
	p.cycles["service"] = PeriodicCycle{Running: true, Concurrency: 16, Total: 100000, Outcomes: map[string]int{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var serviceActive, downloadActive, overlap, serviceCalls, timeouts, downloadCalls atomic.Int32
	var readerWG sync.WaitGroup
	readerWG.Add(16)
	for i := 0; i < 16; i++ {
		go func(i int) {
			defer readerWG.Done()
			for ctx.Err() == nil {
				p.gate.RLock()
				if ctx.Err() != nil {
					p.gate.RUnlock()
					return
				}
				serviceActive.Add(1)
				if downloadActive.Load() > 0 {
					overlap.Add(1)
				}
				serviceCalls.Add(1)
				request, cancelRequest := context.WithTimeout(ctx, 25*time.Millisecond)
				delay := 3 * time.Millisecond
				if i%4 == 0 {
					delay = 50 * time.Millisecond
				}
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-request.Done():
					timer.Stop()
					if request.Err() == context.DeadlineExceeded {
						timeouts.Add(1)
					}
				}
				cancelRequest()
				serviceActive.Add(-1)
				p.gate.RUnlock()
			}
		}(i)
	}
	progress := map[int32]int32{}
	p.measureHook = func(_ context.Context, kind string, _ MonitorNodeOptionDTO, _ string) (string, bool, int64, error) {
		if kind != "download" {
			t.Error("unexpected model/service measure")
		}
		begin := time.Now()
		downloadActive.Add(1)
		if serviceActive.Load() > 0 {
			overlap.Add(1)
		}
		n := downloadCalls.Add(1)
		if n == 5 || n == 9 {
			progress[n] = serviceCalls.Load()
		}
		time.Sleep(5 * time.Millisecond)
		downloadActive.Add(-1)
		p.observeExecution(kind, time.Since(begin).Seconds())
		return "byte_limit", true, 10, nil
	}
	done := make(chan struct{})
	go func() { p.runCycle(ctx, "download", time.Now(), p.cfg); close(done) }()
	select {
	case <-done:
	case <-time.After(12 * time.Second):
		cancel()
		t.Fatal("continuous reader queue starved download")
	}
	cancel()
	readerWG.Wait()
	if overlap.Load() != 0 || downloadCalls.Load() != 12 || timeouts.Load() == 0 || progress[5] == 0 || progress[9] <= progress[5] {
		t.Fatalf("not mutually fair: overlap=%d downloads=%d timeouts=%d services at batches=%v", overlap.Load(), downloadCalls.Load(), timeouts.Load(), progress)
	}
	p.gate.Lock()
	p.gate.Unlock() // cancellation left no batch/read lease behind
	c := p.cycles["download"]
	parts := c.ExclusionWaitSeconds + c.AdmissionWaitSeconds + c.ServiceTurnWaitSeconds + c.NetworkCleanupWaitSeconds
	if math.Abs(parts-c.WaitSeconds) > 1e-6 {
		t.Fatalf("wait categories do not reconcile: sum=%v total=%v", parts, c.WaitSeconds)
	}
	if c.ServiceTurnWaitSeconds < 9.5 || c.ServiceTurnWaitSeconds > 10.5 {
		t.Fatalf("window not bounded under continuous demand: %v", c.ServiceTurnWaitSeconds)
	}
	// Only this single worker's phases can be compared additively with wall time.
	// Producer queue wait overlaps these phases and must not be added again.
	unaccounted := c.ElapsedSeconds - c.WaitSeconds - c.ExecutionSeconds
	if unaccounted < 0 || unaccounted > .25 || c.QueueWaitSeconds < 5 {
		t.Fatalf("wall timing gap or missing producer backpressure: gap=%v queue=%v cycle=%+v", unaccounted, c.QueueWaitSeconds, c)
	}
	t.Logf("offline 16-reader sustained queue: downloads=12 service_admissions=%d timed_out=%d turn_wait=%.3fs lock_wait=%.3fs producer_wait=%.3fs network=%.3fs wall=%.3fs residual=%.4fs", serviceCalls.Load(), timeouts.Load(), c.ServiceTurnWaitSeconds, c.ExclusionWaitSeconds, c.QueueWaitSeconds, c.ExecutionSeconds, c.ElapsedSeconds, unaccounted)
}

func TestPeriodicDownloadBatchObservationErrorReleasesLease(t *testing.T) {
	p := periodicFixture(8)
	var calls atomic.Int32
	p.measureHook = func(context.Context, string, MonitorNodeOptionDTO, string) (string, bool, int64, error) {
		if calls.Add(1) == 4 {
			return "", false, 0, errors.New("offline pre-execution failure")
		}
		return "byte_limit", true, 10, nil
	}
	p.runCycle(context.Background(), "download", time.Now(), p.cfg)
	p.gate.Lock()
	p.gate.Unlock()
	c := p.cycles["download"]
	if calls.Load() != 8 || c.Processed != 8 || c.NotExecuted != 1 || c.FullCoverage {
		t.Fatalf("failed plan item deadlocked or pretended complete: %+v", c)
	}
}

// The configured full service plan is preserved; an offline transport deadline
// or storage error does not authorize shrinking the catalog.
func TestPeriodicFullCatalogSurvivesMixedOfflineOutcomes(t *testing.T) {
	p := periodicFixture(2)
	var calls atomic.Int32
	seen := map[string]bool{}
	var mu sync.Mutex
	p.measureHook = func(ctx context.Context, kind string, _ MonitorNodeOptionDTO, service string) (string, bool, int64, error) {
		mu.Lock()
		seen[service] = true
		mu.Unlock()
		n := calls.Add(1)
		if n%5 == 0 {
			return "timed_out", true, 0, nil
		}
		if n%7 == 0 {
			return "", false, 0, errors.New("offline setup failure")
		}
		return "reachable", true, 0, nil
	}
	p.runCycle(context.Background(), "service", time.Now(), p.cfg)
	c := p.cycles["service"]
	if calls.Load() != 164 || len(seen) != 82 || c.Concurrency != 16 || c.Outcomes["timed_out"] == 0 || c.NotExecuted == 0 || c.FullCoverage {
		t.Fatalf("plan/concurrency/outcome lost: %+v", c)
	}
	if len(publicservice.Catalog()) != 82 {
		t.Fatal("catalog unexpectedly changed")
	}
}

func TestPeriodicAdmissionCannotRenewOriginalRequestDeadline(t *testing.T) {
	enteredFirst := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32
	app, _ := newPublicServiceApplication(t, func(_ string, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, nil
	}, func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(enteredFirst)
			<-releaseFirst
			return publicServiceHTTPResponse(r, 200, "text/plain", ""), nil
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	firstReq := publicServiceRequest("occupy-admission", "profile-a", "identity-a", "revision-a", "google_204")
	firstReq.TimeoutSeconds = 1
	first, err := app.startWorkbenchPublicServiceTest(context.Background(), firstReq, 1)
	if err != nil {
		t.Fatal(err)
	}
	<-enteredFirst
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	go func() { time.Sleep(80 * time.Millisecond); close(releaseFirst) }()
	req := publicServiceRequest("deadline-after-admission", "profile-a", "identity-a", "revision-a", "google_204")
	req.TimeoutSeconds = 1
	p := &periodicSampling{app: app}
	attempt, wait, err := p.startPeriodicPublicService(ctx, req, 1)
	if err != nil {
		t.Fatal(err)
	}
	if wait < .05 {
		t.Fatal("fixture did not exercise admission queue")
	}
	app.publicServiceMu.Lock()
	done := app.publicServiceActive[attempt.AttemptID].networkDone
	app.publicServiceMu.Unlock()
	late := false
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		late = true
		_, _ = app.CancelWorkbenchPublicServiceTest(context.Background(), attempt.AttemptID, publicServiceQueryForTest(attempt))
	}
	app.publicServiceWG.Wait()
	deadline, _ := ctx.Deadline()
	saved, getErr := app.GetWorkbenchPublicServiceAttempt(context.Background(), attempt.AttemptID, publicServiceQueryForTest(attempt))
	if getErr != nil || saved.Result == nil || saved.Result.Details["network_lease_deadline_unix_ns"] != strconv.FormatInt(deadline.UnixNano(), 10) {
		t.Fatalf("original lease deadline absent from saved evidence: %v", getErr)
	}
	_ = first
	if late {
		t.Fatal("admission discarded original deadline and gave tail a new request timeout")
	}
}

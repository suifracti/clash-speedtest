package application

import (
	"context"
	"time"
)

// Estimates include observed queueing and competition; they are never a SLA.
func periodicEstimate(c *PeriodicCycle) {
	c.EstimatedTotalSeconds = nil
	c.RemainingSeconds = nil
	executed := c.Processed - c.NotExecuted
	if !c.Running || executed < 16 || c.ElapsedSeconds < 30 {
		return
	}
	total := c.ElapsedSeconds * float64(c.Total) / float64(executed)
	remaining := total - c.ElapsedSeconds
	if remaining < 0 {
		remaining = 0
	}
	c.EstimatedTotalSeconds = &total
	c.RemainingSeconds = &remaining
}
func periodicDownloadYield(count int, elapsed time.Duration) bool {
	return count >= periodicDownloadBatchSize || elapsed >= 15*time.Second
}
func (p *periodicSampling) adjustActivity(kind string, waiting, active, started int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.cycles[kind]
	c.Waiting += waiting
	c.Active += active
	c.Started += started
	p.cycles[kind] = c
}
func (p *periodicSampling) observeExecution(kind string, seconds float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.cycles[kind]
	c.ExecutionSeconds += seconds
	p.cycles[kind] = c
}

// Legacy summaries lacked execution telemetry. This read model preserves the
// original evidence and only separates typed pre-execution rejections.
func periodicObserveLegacy(c PeriodicCycle) PeriodicCycle {
	if c.Started == 0 && c.Processed > 0 {
		c.LegacyObservation = true
		missing := c.Outcomes["not_measured"]
		if missing > c.NotExecuted {
			c.NotExecuted = missing
			c.Unsaved -= missing
			if c.Unsaved < 0 {
				c.Unsaved = 0
			}
		}
	}
	return c
}

func (p *periodicSampling) observeWaitPart(kind, part string, seconds float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cycles == nil {
		return
	}
	c := p.cycles[kind]
	switch part {
	case "queue":
		c.QueueWaitSeconds += seconds
	case "exclusion":
		c.ExclusionWaitSeconds += seconds
	case "admission":
		c.AdmissionWaitSeconds += seconds
	case "cleanup":
		c.NetworkCleanupWaitSeconds += seconds
	case "service_turn":
		c.ServiceTurnWaitSeconds += seconds
	}
	p.cycles[kind] = c
}

// A queued RWMutex writer blocks new readers. Delay requesting the next writer
// for a bounded refill window, then let existing readers drain normally. The
// mutual exclusion itself remains unchanged: no download/probe network overlap.
const periodicServiceRefillWindow = 5 * time.Second

func (p *periodicSampling) releaseDownloadGate(held *bool) {
	if !*held {
		return
	}
	p.mu.Lock()
	c := p.cycles["service"]
	if c.Running && c.Processed < c.Total {
		p.serviceTurnUntil = time.Now().Add(periodicServiceRefillWindow)
	}
	p.mu.Unlock()
	*held = false
	p.gate.Unlock()
}

func (p *periodicSampling) waitServiceTurn(ctx context.Context) (time.Duration, error) {
	started := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return time.Since(started), err
		}
		p.mu.Lock()
		c := p.cycles["service"]
		until := p.serviceTurnUntil
		pending := c.Running && c.Processed < c.Total
		p.mu.Unlock()
		remaining := time.Until(until)
		if !pending || remaining <= 0 {
			return time.Since(started), nil
		}
		if remaining > 20*time.Millisecond {
			remaining = 20 * time.Millisecond
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			timer.Stop()
			return time.Since(started), ctx.Err()
		case <-timer.C:
		}
	}
}

// This is concurrent worker wait, not an additive wall-clock partition. Network
// completion is signalled before staging/saving. The actual probe retains its
// original timeout and cancellation; a gate is never freed by a guessed timeout.
func (p *periodicSampling) drainPeriodicNetwork(kind string, done <-chan struct{}) {
	if done == nil {
		return
	}
	begin := time.Now()
	<-done
	waited := time.Since(begin).Seconds()
	p.addPeriodicWait(kind, waited)
	p.observeWaitPart(kind, "cleanup", waited)
}

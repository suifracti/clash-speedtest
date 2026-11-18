package speedtester

import (
	"context"
	"slices"
	"sync/atomic"
	"time"
)

const (
	downloadWarmup    = time.Second
	downloadWindow    = time.Second
	downloadTolerance = 0.10
	downloadIdleLimit = 5 * time.Second
)

type downloadStability struct {
	firstByte   time.Time
	windowStart time.Time
	windowBytes int64
	rates       [3]float64
	count       int
	measured    bool
}

func (s *downloadStability) observe(now time.Time, bytes int64) float64 {
	if s.firstByte.IsZero() {
		s.firstByte = now
		return 0
	}
	if now.Sub(s.firstByte) < downloadWarmup {
		return 0
	}
	elapsed := now.Sub(s.windowStart)
	// A long gap cannot provide consecutive one-second observations.
	if s.windowStart.IsZero() || elapsed > 2*downloadWindow {
		s.windowStart, s.windowBytes, s.count = now, bytes, 0
		return 0
	}
	if elapsed < downloadWindow {
		return 0
	}
	speed := float64(bytes-s.windowBytes) * 8 / 1e6 / elapsed.Seconds()
	s.windowStart, s.windowBytes = now, bytes
	s.measured = true
	s.rates = [3]float64{s.rates[1], s.rates[2], speed}
	s.count = min(s.count+1, len(s.rates))
	if s.count < len(s.rates) {
		return 0
	}
	sorted := s.rates
	slices.Sort(sorted[:])
	median := sorted[1]
	if sorted[0] >= median*(1-downloadTolerance) && sorted[2] <= median*(1+downloadTolerance) {
		return median
	}
	return 0
}

func watchDownloadIdle(ctx context.Context, started time.Time, lastByte *atomic.Int64, limit time.Duration, cancel context.CancelCauseFunc) {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			remaining := limit - (time.Since(started) - time.Duration(lastByte.Load()))
			if remaining <= 0 {
				cancel(errDownloadIdle)
				return
			}
			timer.Reset(remaining)
		}
	}
}

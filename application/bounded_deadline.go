package application

import (
	"context"
	"time"

	"github.com/faceair/clash-speedtest/core/speedtester"
)

// Download execution outlives the HTTP request that admits it. The request
// context is accepted to make that boundary explicit, but network work gets a
// fresh task context with its own hard upper bound.
func newWorkbenchDownloadLifecycleContext(_ context.Context, maximum time.Duration) (context.Context, context.CancelFunc) {
	maxLifecycle := 2 * speedtester.MaximumDownloadStreamTimeout
	if maximum <= 0 || maximum > maxLifecycle {
		maximum = maxLifecycle
	}
	return context.WithTimeout(context.Background(), maximum)
}

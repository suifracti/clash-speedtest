//go:build !darwin

package speedtester

import (
	"context"
	"time"
)

func PreparePhysicalAutoProxy(ctx context.Context, p *CProxy, _ time.Duration, sockets int) (*CProxy, *PhysicalDownloadEgress, error) {
	return PreparePhysicalProbeProxy(ctx, p, sockets)
}

func PlanPhysicalAutoDownloadPath(ctx context.Context, p *CProxy) DownloadNetworkPath {
	return PlanPhysicalDownloadPath(ctx, p)
}

//go:build !darwin

package speedtester

import "context"

// Keep other platforms' existing behavior; this round implements Mac only.
func PreparePhysicalProbeProxy(ctx context.Context, p *CProxy, _ int) (*CProxy, *PhysicalDownloadEgress, error) {
	return PreparePhysicalDownloadProxy(ctx, p)
}

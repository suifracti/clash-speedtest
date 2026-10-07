//go:build !darwin

package speedtester

import "context"

type PhysicalDownloadEgress struct{}

func (*PhysicalDownloadEgress) Snapshot() DownloadNetworkPath {
	return DownloadNetworkPath{Method: "legacy_default_binding_unverified"}
}
func PreparePhysicalDownloadProxy(_ context.Context, p *CProxy) (*CProxy, *PhysicalDownloadEgress, error) {
	return p, nil, nil
}

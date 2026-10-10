//go:build !darwin

package speedtester

import "context"

type PhysicalDownloadEgress struct{ failureReason string }

func (e *PhysicalDownloadEgress) Snapshot() DownloadNetworkPath {
	return DownloadNetworkPath{
		Method: "legacy_default_binding_unverified", AddressFamily: "unknown",
		ResolutionSource: "unobserved", TUNEvidence: "packet_route_not_observed",
		FailureReason: e.failureReasonOrDefault(),
	}
}
func PreparePhysicalDownloadProxy(ctx context.Context, p *CProxy) (*CProxy, *PhysicalDownloadEgress, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			reason := "deadline_exceeded"
			if err == context.Canceled {
				reason = "user_cancelled"
			}
			return p, &PhysicalDownloadEgress{failureReason: reason}, err
		}
	}
	return p, &PhysicalDownloadEgress{}, nil
}

func (e *PhysicalDownloadEgress) failureReasonOrDefault() string {
	if e != nil && e.failureReason != "" {
		return e.failureReason
	}
	return "physical_binding_unavailable"
}

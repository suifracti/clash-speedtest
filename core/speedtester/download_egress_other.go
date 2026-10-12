//go:build !darwin

package speedtester

import "context"

type PhysicalDownloadEgress struct{ failureReason string }

func (e *PhysicalDownloadEgress) Snapshot() DownloadNetworkPath {
	return DownloadNetworkPath{
		Method: "legacy_default_binding_unverified", AddressFamily: "unknown",
		AddressSource: "unobserved", TUNEvidence: "packet_route_not_observed",
		FailureReason: e.failureReasonOrDefault(),
	}
}

func PlanPhysicalDownloadPath(ctx context.Context, _ *CProxy) DownloadNetworkPath {
	e := &PhysicalDownloadEgress{}
	if ctx != nil && ctx.Err() != nil {
		e.failureReason = physicalDownloadContextFailure(ctx.Err())
	}
	return e.Snapshot()
}

func physicalDownloadContextFailure(err error) string {
	if err == context.Canceled {
		return "user_cancelled"
	}
	if err == context.DeadlineExceeded {
		return "deadline_exceeded"
	}
	return "physical_binding_unavailable"
}

func PreparePhysicalDownloadProxy(ctx context.Context, p *CProxy) (*CProxy, *PhysicalDownloadEgress, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return p, &PhysicalDownloadEgress{failureReason: physicalDownloadContextFailure(err)}, err
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

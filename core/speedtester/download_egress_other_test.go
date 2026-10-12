//go:build !darwin

package speedtester

import "testing"

func TestLegacyPhysicalPathRemainsExplicitlyUnknown(t *testing.T) {
	path := (&PhysicalDownloadEgress{}).Snapshot()
	if path.Method != "legacy_default_binding_unverified" || path.TUNEvidence != "packet_route_not_observed" || path.SocketBindVerified || path.DNSBindVerified {
		t.Fatalf("non-Darwin legacy path overclaims binding evidence: %+v", path)
	}
}

func TestLegacyPhysicalPathPlanDoesNotClaimVerifiedBinding(t *testing.T) {
	path := PlanPhysicalDownloadPath(nil, nil)
	if path.Method != "legacy_default_binding_unverified" || path.Interface != "" || path.AddressFamily != "unknown" || path.AddressSource != "unobserved" || path.DNSBindVerified || path.SocketBindVerified || path.TUNEvidence != "packet_route_not_observed" {
		t.Fatalf("non-Darwin path plan overclaims binding evidence: %+v", path)
	}
}

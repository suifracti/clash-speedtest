//go:build darwin

package speedtester_test

import (
	"context"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"reflect"
	"testing"
	"time"
)

func TestStrictDNSPathEvidencePersistsWithoutSchemaChange(t *testing.T) {
	for _, p := range []speedtester.DownloadNetworkPath{
		{Method: "physical_socket_v1", AddressFamily: "ipv6", AddressSource: "physical_ipv6_dns", DNSRequests: 2, DNSDialAttempts: 2, DNSBindVerified: true, TUNEvidence: "packet_route_not_observed"},
		{Method: "physical_socket_v1", AddressFamily: "unknown", AddressSource: "unobserved", FailureReason: "a_rcode_nxdomain", DNSRequests: 1, DNSDialAttempts: 1, DNSBindVerified: true, TUNEvidence: "packet_route_not_observed"},
		{Method: "physical_socket_v1", AddressFamily: "unknown", AddressSource: "unobserved", FailureReason: "aaaa_user_cancelled", DNSRequests: 1, DNSDialAttempts: 1, DNSBindVerified: true, TUNEvidence: "packet_route_not_observed"},
	} {
		t.Run(p.AddressSource+"/"+p.FailureReason, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			s, err := history.NewStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			at := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
			a := &history.WorkbenchDownloadAttempt{AttemptID: "fixture-attempt", RequestID: "fixture-request", ProfileID: "fixture-profile", NodeKey: "fixture-node", NodeIdentityKey: "fixture-identity", ConfigRevisionKey: "fixture-revision", Source: history.WorkbenchDownloadSource, RequestedAt: at, ExecutionState: "queued", PersistenceState: "not_started", Rule: history.WorkbenchDownloadRuleSnapshot{RuleVersion: 1, Method: "GET", MaximumBytes: 1000, MaximumDurationNS: int64(time.Second)}}
			if err = s.CreateWorkbenchDownloadAttempt(ctx, a); err != nil {
				t.Fatal(err)
			}
			if err = s.BeginWorkbenchDownloadAttempt(ctx, a.AttemptID, at); err != nil {
				t.Fatal(err)
			}
			// Synthetic offline path fixture, not a claimed successful download.
			m := history.WorkbenchDownloadMeasurement{Outcome: "not_executed", ErrorClass: "physical_path", EndReason: p.FailureReason, StartedAt: at, FinishedAt: at, NetworkPath: &p, Samples: []history.WorkbenchDownloadSample{}}
			if err = s.StageWorkbenchDownloadResult(ctx, a.AttemptID, "not_executed", m); err != nil {
				t.Fatal(err)
			}
			if err = s.CommitWorkbenchDownloadResult(ctx, a.AttemptID); err != nil {
				t.Fatal(err)
			}
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := history.NewStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			got, err := reopened.GetWorkbenchDownloadAttempt(ctx, a.AttemptID, history.WorkbenchDownloadFilter{ProfileID: a.ProfileID, NodeKey: a.NodeKey, NodeIdentityKey: a.NodeIdentityKey, ConfigRevisionKey: a.ConfigRevisionKey})
			if err != nil {
				t.Fatal(err)
			}
			if got.PersistenceState != "saved" || got.Result == nil || !reflect.DeepEqual(got.Result.NetworkPath, &p) || got.Result.BytesRead != 0 || got.ConfigRevisionKey != a.ConfigRevisionKey {
				t.Fatalf("reopen changed evidence: %+v", got)
			}
		})
	}
}

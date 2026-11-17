package history

import (
	"context"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/speedtester"
)

func TestPreviousDownloadRequiresObservedPathAndBudgetVersion(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	at := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	path := speedtester.DownloadNetworkPath{Method: "physical_socket_auto_family_v1", Interface: "en0", DNSMode: "physical_interface_dns_v1", DNSBindVerified: true, SocketBindVerified: true, AddressFamily: "ipv4", AddressSource: "dns"}
	for i, family := range []string{"ipv4", "ipv6"} {
		a := workbenchDownloadFixture(family, family, at.Add(time.Duration(i)*time.Minute))
		a.Rule.RuleVersion = 4
		observed := path
		observed.AddressFamily = family
		if err := store.CreateWorkbenchDownloadAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
		if err := store.BeginWorkbenchDownloadAttempt(ctx, a.AttemptID, a.RequestedAt); err != nil {
			t.Fatal(err)
		}
		if err := store.StageWorkbenchDownloadResult(ctx, a.AttemptID, "completed", WorkbenchDownloadMeasurement{Outcome: "byte_limit", StartedAt: a.RequestedAt, FinishedAt: a.RequestedAt, BytesRead: 1000, NetworkPath: &observed}); err != nil {
			t.Fatal(err)
		}
		if err := store.CommitWorkbenchDownloadResult(ctx, a.AttemptID); err != nil {
			t.Fatal(err)
		}
	}
	current := workbenchDownloadFixture("current", "current", at.Add(2*time.Minute))
	current.Rule.RuleVersion = 4
	current.Result = &WorkbenchDownloadMeasurement{NetworkPath: &path}
	previous, err := store.DB().PreviousWorkbenchDownloadAttempt(ctx, current)
	if err != nil || previous == nil || previous.AttemptID != "ipv4" {
		t.Fatalf("IPv4 comparison selected another path: %+v %v", previous, err)
	}
	path.AddressFamily = "ipv6"
	previous, err = store.DB().PreviousWorkbenchDownloadAttempt(ctx, current)
	if err != nil || previous == nil || previous.AttemptID != "ipv6" {
		t.Fatalf("IPv6 comparison lost matching path: %+v %v", previous, err)
	}
	path.SocketBindVerified = false
	previous, err = store.DB().PreviousWorkbenchDownloadAttempt(ctx, current)
	if err != nil || previous != nil {
		t.Fatalf("unverified run matched verified path: %+v %v", previous, err)
	}
	path.SocketBindVerified = true
	current.Rule.RuleVersion = 3
	previous, err = store.DB().PreviousWorkbenchDownloadAttempt(ctx, current)
	if err != nil || previous != nil {
		t.Fatalf("different timeout policy matched: %+v %v", previous, err)
	}
}

func TestPreviousAttemptUsesFrozenConditions(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	at := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	t.Run("download", func(t *testing.T) {
		current := workbenchDownloadFixture("current", "current", at.Add(2*time.Hour))
		current.FinishedAt = &current.RequestedAt
		for i, id := range []string{"same", "different"} {
			a := workbenchDownloadFixture(id, id, at.Add(time.Duration(i)*time.Hour))
			if i == 1 {
				a.Rule.MaximumBytes++
			}
			if err := store.CreateWorkbenchDownloadAttempt(ctx, a); err != nil {
				t.Fatal(err)
			}
			if err := store.BeginWorkbenchDownloadAttempt(ctx, id, a.RequestedAt); err != nil {
				t.Fatal(err)
			}
			if err := store.StageWorkbenchDownloadResult(ctx, id, "completed", WorkbenchDownloadMeasurement{Outcome: "byte_limit", StartedAt: a.RequestedAt, FinishedAt: a.RequestedAt, BytesRead: 1000, DurationNS: int64(time.Second)}); err != nil {
				t.Fatal(err)
			}
			if err := store.CommitWorkbenchDownloadResult(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
		previous, err := store.DB().PreviousWorkbenchDownloadAttempt(ctx, current)
		if err != nil || previous == nil || previous.AttemptID != "same" {
			t.Fatalf("previous=%+v err=%v", previous, err)
		}
		current.Rule.MaximumBytes = 42
		previous, err = store.DB().PreviousWorkbenchDownloadAttempt(ctx, current)
		if err != nil || previous != nil {
			t.Fatalf("unmatched=%+v err=%v", previous, err)
		}
	})
	t.Run("service", func(t *testing.T) {
		current := publicServiceAttemptFixture("current", "current", "p", "n", "i", "r", "ai", at.Add(2*time.Hour))
		current.FinishedAt = &current.RequestedAt
		for i, id := range []string{"same", "different"} {
			a := publicServiceAttemptFixture(id, id, "p", "n", "i", "r", "ai", at.Add(time.Duration(i)*time.Hour))
			if i == 1 {
				a.Rule.SuccessCriterion = "different"
			}
			if err := store.CreatePublicServiceAttempt(ctx, a); err != nil {
				t.Fatal(err)
			}
			if err := store.BeginPublicServiceAttempt(ctx, id, a.RequestedAt); err != nil {
				t.Fatal(err)
			}
			if err := store.StagePublicServiceResult(ctx, id, "completed", PublicServiceMeasurement{Outcome: "matched", StartedAt: a.RequestedAt, FinishedAt: a.RequestedAt}); err != nil {
				t.Fatal(err)
			}
			if err := store.CommitPublicServiceResult(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
		previous, err := store.DB().PreviousPublicServiceAttempt(ctx, current)
		if err != nil || previous == nil || previous.AttemptID != "same" {
			t.Fatalf("previous=%+v err=%v", previous, err)
		}
		current.Rule.SuccessCriterion = "unmatched"
		previous, err = store.DB().PreviousPublicServiceAttempt(ctx, current)
		if err != nil || previous != nil {
			t.Fatalf("unmatched=%+v err=%v", previous, err)
		}
	})
}

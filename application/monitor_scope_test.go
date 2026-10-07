package application

import (
	"context"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestFrozenMonitorScopeReportsEveryUnresolvedReferenceWithoutNameMatching(t *testing.T) {
	refs := []monitor.MonitorJobNodeReference{{NodeKey: "a", NodeIdentityKey: "i-a", ConfigRevisionKey: "v", DisplayName: "same"}, {NodeKey: "b", NodeIdentityKey: "i-b", ConfigRevisionKey: "v", DisplayName: "same"}, {NodeKey: "c", NodeIdentityKey: "i-c", ConfigRevisionKey: "old", DisplayName: "changed"}}
	current := []monitor.MonitoredNode{{NodeKey: "new", NodeIdentityKey: "new-i", ConfigRevisionKey: "v", DisplayName: "same"}, {NodeKey: "c", NodeIdentityKey: "i-c", ConfigRevisionKey: "new", DisplayName: "changed"}}
	got := monitorResolutionIssues(refs, current)
	if len(got) != 3 || got[0].NodeIdentityKey != "i-a" || got[1].NodeIdentityKey != "i-b" || got[2].ConfigRevisionKey != "old" {
		t.Fatalf("scope issues collapsed or fuzzily rebound: %+v", got)
	}
	if nodes, reason := resolvePersistedMonitorNodes(refs, current); nodes != nil || reason == "" {
		t.Fatal("blocked task partially started")
	}
}

func TestScheduledRoundCannotBeStartedOrFinishedThroughManualAPI(t *testing.T) {
	var calls atomic.Int32
	app, store := newPublicServiceApplication(t, func(profile, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", RawConfig: map[string]any{"type": "http"}}, nil
	}, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return publicServiceHTTPResponse(r, 204, "", ""), nil
	})
	ctx := context.Background()
	req := publicServiceRequest("scheduled-child", "profile-a", "identity-a", "revision-a", "cloudflare_204")
	round := history.MeasurementRound{RoundID: "scheduled-root", TriggerType: "scheduled", StartedAt: time.Now().UTC(), Items: []history.MeasurementRoundItem{{RequestID: req.RequestID, ProfileID: req.ProfileID, NodeKey: req.NodeKey, NodeIdentityKey: req.NodeIdentityKey, ConfigRevisionKey: req.ConfigRevisionKey, Project: "service", ServiceID: req.ServiceID}}}
	if err := store.DB().CreateMeasurementRound(ctx, round); err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartWorkbenchPublicServiceTest(ctx, req); err == nil || calls.Load() != 0 {
		t.Fatal("manual caller started a scheduled child")
	}
	if err := app.FinishManualMeasurementRound(ctx, round.RoundID, "finished"); err == nil {
		t.Fatal("manual caller changed scheduled lifecycle")
	}
	if _, err := app.startWorkbenchPublicServiceTest(context.WithValue(ctx, scheduledRoundContextKey{}, "scheduled"), req, 16); err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	if calls.Load() != 1 {
		t.Fatal("trusted scheduler did not execute once")
	}
}

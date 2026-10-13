package application

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestIPOKRestoredWorkbenchStoreAndHistory(t *testing.T) {
	var calls atomic.Int32
	app, store := newPublicServiceApplication(t, func(_, key string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: key, NodeIdentityKey: "fixture-node", ConfigRevisionKey: "fixture-revision"}, nil
	}, func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		body := `{"apiVersion":"1","geo":{"ip":"8.8.8.8"},"risk":25}`
		if req.URL.Host == "api.ipify.org" {
			body = `{"ip":"8.8.8.8"}`
		}
		return publicServiceHTTPResponse(req, 200, "application/json", body), nil
	})
	a, err := app.StartWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("fixture-ipok", "fixture-profile", "fixture-node", "fixture-revision", "ipok_ipv4"))
	if err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	saved, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), a.AttemptID, publicServiceQueryForTest(a))
	if err != nil {
		t.Fatal(err)
	}
	if saved.PersistenceState != "saved" || saved.Result == nil || saved.Result.Outcome != "profiled" || calls.Load() != 2 {
		t.Fatalf("restored store/result calls=%d saved=%+v", calls.Load(), saved)
	}
	data, err := store.DB().LoadIPReputation(context.Background(), "ipok-v1|IPv4|8.8.8.8", saved.Result.FinishedAt)
	if err != nil || data == "" {
		t.Fatal("cache not persisted", err)
	}
}
func TestIPOKRemainsExplicitlyOnDemand(t *testing.T) {
	for _, include := range []bool{false, true} {
		for _, rule := range periodicServices(PeriodicSamplingConfig{IncludeAntigravity: include}) {
			if rule.ServiceID == "ipok_ipv4" || rule.ServiceID == "ipok_ipv6" {
				t.Fatal("IPOK entered automatic sampling")
			}
		}
	}
}

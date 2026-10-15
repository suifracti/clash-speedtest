package desktop

import (
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestFrontendManagementNilAppDefense(t *testing.T) {
	cases := []struct {
		name string
		app  *App
	}{
		{name: "nil receiver", app: nil},
		{name: "nil app service", app: &App{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.app

			if _, err := a.CreateFrontendAirport(FrontendAirportRequest{}); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("CreateFrontendAirport: unexpected err %v", err)
			}
			if _, err := a.UpdateFrontendAirport("ap-1", FrontendAirportRequest{}); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("UpdateFrontendAirport: unexpected err %v", err)
			}
			if err := a.OpenDataFolder(); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("OpenDataFolder: unexpected err %v", err)
			}
			if _, err := a.AddSubscription("ap-1", "name", "url", "note"); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("AddSubscription: unexpected err %v", err)
			}
			if _, err := a.UpdateSubscription("ap-1", "sub-1", "name", "url", "note"); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("UpdateSubscription: unexpected err %v", err)
			}
			if err := a.DeleteSubscription("ap-1", "sub-1"); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("DeleteSubscription: unexpected err %v", err)
			}
			if _, err := a.RefreshSubscription("ap-1", "sub-1"); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("RefreshSubscription: unexpected err %v", err)
			}
			if _, err := a.GetSubscriptionURL("ap-1", "sub-1"); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("GetSubscriptionURL: unexpected err %v", err)
			}
			if err := a.SaveAirportMaintenance("ap-1", profiles.AirportMaintenance{}); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("SaveAirportMaintenance: unexpected err %v", err)
			}
			if _, err := a.GetSubscriptionUsage("2026-01-01", "2026-01-02", "day", "all"); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("GetSubscriptionUsage: unexpected err %v", err)
			}
			if _, err := a.UpdateSubscriptionUsage(); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("UpdateSubscriptionUsage: unexpected err %v", err)
			}
			if _, err := a.ExportNodesClashConfig(application.ExportClashConfigRequest{}); err == nil || !strings.Contains(err.Error(), "应用服务未初始化") {
				t.Fatalf("ExportNodesClashConfig: unexpected err %v", err)
			}
		})
	}
}

package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

type FrontendAirportRequest struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	WebsiteURL string `json:"website_url"`
	BackupURL  string `json:"backup_url"`
	Note       string `json:"note"`
}

func (a *App) CreateFrontendAirport(req FrontendAirportRequest) (*application.AirportDTO, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.CreateAirport(req.Name, req.URL, a.userAgent, req.WebsiteURL, req.BackupURL, req.Note)
}

func (a *App) UpdateFrontendAirport(id string, req FrontendAirportRequest) (*application.AirportDTO, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.UpdateAirport(id, req.Name, req.URL, a.userAgent, req.WebsiteURL, req.BackupURL, req.Note)
}

func (a *App) OpenDataFolder() error {
	if a == nil || a.app == nil {
		return fmt.Errorf("应用服务未初始化")
	}
	setup, err := a.app.GetProfileSetup()
	if err != nil {
		return err
	}
	info, err := os.Stat(setup.DataRoot)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("数据目录不存在")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", setup.DataRoot)
	case "darwin":
		command = exec.Command("open", setup.DataRoot)
	default:
		command = exec.Command("xdg-open", setup.DataRoot)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("无法打开文件夹，请复制路径手动打开: %w", err)
	}
	go func() { _ = command.Wait() }()
	return nil
}

// Keep subscription management on the same IPC boundary as airport management.
func (a *App) AddSubscription(airportID, name, source, note string) (*application.SubscriptionDTO, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.AddSubscription(airportID, name, source, note, a.userAgent)
}

func (a *App) UpdateSubscription(airportID, subscriptionID, name, source, note string) (*application.SubscriptionDTO, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.UpdateSubscription(airportID, subscriptionID, name, source, note, a.userAgent)
}

func (a *App) DeleteSubscription(airportID, subscriptionID string) error {
	if a == nil || a.app == nil {
		return fmt.Errorf("应用服务未初始化")
	}
	return a.app.DeleteSubscription(airportID, subscriptionID)
}

func (a *App) RefreshSubscription(airportID, subscriptionID string) (*application.SubscriptionDTO, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.RefreshSubscription(airportID, subscriptionID, a.userAgent)
}

func (a *App) GetSubscriptionURL(airportID, subscriptionID string) (string, error) {
	if a == nil || a.app == nil {
		return "", fmt.Errorf("应用服务未初始化")
	}
	return a.app.GetSubscriptionURL(subscriptionID)
}

func (a *App) SaveAirportMaintenance(airportID string, settings profiles.AirportMaintenance) error {
	if a == nil || a.app == nil {
		return fmt.Errorf("应用服务未初始化")
	}
	return a.app.SaveAirportMaintenance(airportID, settings)
}

func (a *App) GetSubscriptionUsage(from, until, period, account string) (*subscriptionusage.Report, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.GetSubscriptionUsage(a.context(), from, until, period, account)
}

func (a *App) GetSubscriptionRefreshState() (*subscriptionusage.RefreshState, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.GetSubscriptionRefreshState(a.context())
}

func (a *App) UpdateSubscriptionUsage() (*subscriptionusage.RefreshState, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.UpdateSubscriptionUsage(a.context(), a.userAgent)
}

func (a *App) ExportNodesClashConfig(request application.ExportClashConfigRequest) (*application.ExportClashConfigResponse, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.ExportNodesClashConfig(a.context(), request)
}

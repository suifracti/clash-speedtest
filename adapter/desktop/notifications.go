package desktop

import (
	"fmt"
	"github.com/faceair/clash-speedtest/core/auth"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) initializeNotifications() error {
	if a == nil || a.ctx == nil {
		return fmt.Errorf("桌面窗口尚未就绪")
	}
	if a.notificationsReady {
		return nil
	}
	if err := wailsruntime.InitializeNotifications(a.ctx); err != nil {
		return err
	}
	a.notificationsReady = true
	return nil
}
func (a *App) EnableLocalNotifications() error {
	if err := a.initializeNotifications(); err != nil {
		return fmt.Errorf("系统未允许 CST 发送通知")
	}
	allowed, err := wailsruntime.RequestNotificationAuthorization(a.ctx)
	if err != nil || !allowed {
		return fmt.Errorf("系统未允许 CST 发送通知")
	}
	return nil
}

func (a *App) OpenSystemNotificationSettings() error {
	return auth.OpenBrowser("x-apple.systempreferences:com.apple.Notifications-Settings.extension")
}
func (a *App) SendLocalNotification(id, title, body string) error {
	if err := a.initializeNotifications(); err != nil {
		return err
	}
	return wailsruntime.SendNotification(a.ctx, wailsruntime.NotificationOptions{ID: id, Title: title, Body: body})
}

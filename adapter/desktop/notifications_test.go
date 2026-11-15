package desktop

import "testing"

func TestNotificationsRequireDesktopContext(t *testing.T) {
	var app *App
	if err := app.EnableLocalNotifications(); err == nil {
		t.Fatal("missing window must be reported")
	}
	if err := app.SendLocalNotification("id", "title", "body"); err == nil {
		t.Fatal("missing window must not report delivery")
	}
	if got := systemNotificationSettingsURI("windows"); got != "ms-settings:notifications" {
		t.Fatalf("windows notification URI = %q", got)
	}
	if got := systemNotificationSettingsURI("darwin"); got != "x-apple.systempreferences:com.apple.Notifications-Settings.extension" {
		t.Fatalf("darwin notification URI = %q", got)
	}
}

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
}

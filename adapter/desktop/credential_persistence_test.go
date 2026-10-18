package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
)

func TestDesktopTokenSaveUsesOwnRootAndReportsFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	daily := filepath.Join(home, ".antigravity_token")
	if err := os.WriteFile(daily, []byte("daily-account\n"), 0600); err != nil {
		t.Fatal(err)
	}
	paths, _ := appdata.Resolve(t.TempDir())
	svc := application.NewAppServiceWithOptions(nil, paths, nil, application.Options{NoAutoCredentials: true})
	defer svc.Close()
	a := &App{app: svc}
	if err := a.SetToken("ya29.local-account"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(daily); string(b) != "daily-account\n" {
		t.Fatal("manual save changed daily HOME credential")
	}
	target := filepath.Join(paths.DataRoot, ".antigravity_token")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.SetToken("ya29.replacement-account"); err == nil {
		t.Fatal("UI binding reported successful save after disk failure")
	}
	if svc.GetAntigravityToken() != "ya29.local-account" {
		t.Fatal("failed save replaced active account")
	}
}

package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestDesktopAppInitialization(t *testing.T) {
	tmpDir := t.TempDir()
	hStore, err := history.NewStore(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("create history store: %v", err)
	}

	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}
	_ = os.MkdirAll(paths.Dir, 0o755)

	app := NewApp(hStore, paths, "test-ua")

	ctx := context.Background()
	app.Startup(ctx)

	st := app.GetStatus()
	if st.IsRunning {
		t.Errorf("expected IsRunning false on startup")
	}

	app.Shutdown(ctx)
}

func TestDesktopAppUsesSharedProfileSetupPaths(t *testing.T) {
	paths, err := appdata.Resolve(filepath.Join(t.TempDir(), "isolated-data"))
	if err != nil {
		t.Fatal(err)
	}
	hStore, err := history.NewStore(paths.HistoryDir)
	if err != nil {
		t.Fatal(err)
	}
	app := NewAppWithPaths(hStore, paths, "test-ua")
	ctx := context.Background()
	app.Startup(ctx)
	defer app.Shutdown(ctx)

	setup, err := app.GetProfileSetup()
	if err != nil {
		t.Fatal(err)
	}
	if setup.State != "needs_choice" || setup.DataRoot != paths.DataRoot {
		t.Fatalf("unexpected Wails setup state: %+v", setup)
	}
	if err := app.InitializeEmptyProfileStore(); err != nil {
		t.Fatal(err)
	}
	setup, err = app.GetProfileSetup()
	if err != nil || setup.State != "ready" || !setup.Initialized {
		t.Fatalf("Wails empty choice did not become ready: setup=%+v err=%v", setup, err)
	}
	airports, err := app.ListAirports()
	if err != nil || len(airports) != 0 {
		t.Fatalf("Wails did not read the initialized canonical profile: airports=%+v err=%v", airports, err)
	}
}

package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

package desktop

import (
	"embed"
	"fmt"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// RunConfig provides initialization parameters for the desktop application.
type RunConfig struct {
	ProfilePaths profiles.Paths
	HistoryDir   string
	UserAgent    string
	Assets       embed.FS
}

// Run starts the Wails desktop application window and event loop.
func Run(cfg RunConfig) error {
	if cfg.ProfilePaths.Dir == "" {
		cfg.ProfilePaths = profiles.DefaultPaths()
	}

	hStore, err := history.NewStore(cfg.HistoryDir)
	if err != nil {
		return fmt.Errorf("init history store: %w", err)
	}

	app := NewApp(hStore, cfg.ProfilePaths, cfg.UserAgent)

	return wails.Run(&options.App{
		Title:     "Clash SpeedTest Pro",
		Width:     1360,
		Height:    900,
		MinWidth:  1080,
		MinHeight: 720,
		AssetServer: &assetserver.Options{
			Assets: cfg.Assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			BackdropType:         windows.Mica,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarDefault(),
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
}

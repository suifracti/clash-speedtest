package desktop

import (
	"embed"
	"fmt"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// RunConfig provides initialization parameters for the desktop application.
type RunConfig struct {
	AppPaths     appdata.AppPaths
	ProfilePaths profiles.Paths
	HistoryDir   string
	UserAgent    string
	Assets       embed.FS
}

// Run starts the Wails desktop application window and event loop.
func Run(cfg RunConfig) error {
	if cfg.AppPaths.ProfileDir != "" {
		cfg.ProfilePaths = profiles.Paths{Dir: cfg.AppPaths.ProfileDir}
		if cfg.HistoryDir == "" {
			cfg.HistoryDir = cfg.AppPaths.HistoryDir
		}
	} else if cfg.ProfilePaths.Dir == "" {
		resolved, err := appdata.Resolve("")
		if err != nil {
			return fmt.Errorf("resolve application paths: %w", err)
		}
		cfg.AppPaths = resolved
		cfg.ProfilePaths = profiles.Paths{Dir: resolved.ProfileDir}
		if cfg.HistoryDir == "" {
			cfg.HistoryDir = resolved.HistoryDir
		}
	} else {
		cfg.AppPaths = appdata.FromLegacy(cfg.ProfilePaths.Dir, cfg.HistoryDir)
	}

	hStore, err := application.OpenHistoryStore(cfg.AppPaths)
	if err != nil {
		return fmt.Errorf("init history store: %w", err)
	}

	app := NewAppWithPaths(hStore, cfg.AppPaths, cfg.UserAgent)

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

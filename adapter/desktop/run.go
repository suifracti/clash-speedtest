package desktop

import (
	"context"
	"embed"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// RunConfig provides initialization parameters for the desktop application.
type RunConfig struct {
	AppPaths     appdata.AppPaths
	ProfilePaths profiles.Paths
	HistoryDir   string
	UserAgent    string
	Assets       embed.FS
	AppOptions   application.Options
}

// Run starts the Wails desktop application window and event loop.
func Run(cfg RunConfig) error {
	app, err := newAppForRun(cfg)
	if err != nil {
		return err
	}
	return runWindow(cfg, app)
}

// newAppForRun is the production initialization boundary before any native window.
func newAppForRun(cfg RunConfig) (*App, error) {
	if cfg.AppPaths.ProfileDir != "" {
		cfg.ProfilePaths = profiles.Paths{Dir: cfg.AppPaths.ProfileDir}
		if cfg.HistoryDir == "" {
			cfg.HistoryDir = cfg.AppPaths.HistoryDir
		}
	} else if cfg.ProfilePaths.Dir == "" {
		resolved, err := appdata.Resolve("")
		if err != nil {
			return nil, fmt.Errorf("resolve application paths: %w", err)
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
		return nil, fmt.Errorf("init history store: %w", err)
	}

	return NewAppWithOptions(hStore, cfg.AppPaths, cfg.UserAgent, cfg.AppOptions), nil
}

// Isolated validation keeps WebView2's session/cache in the explicitly selected
// data directory. Normal startup retains Wails' default profile location.
func isolatedWebviewDataPath(cfg RunConfig) string {
	if cfg.AppOptions.NoAutoCredentials && cfg.AppPaths.Explicit && filepath.IsAbs(cfg.AppPaths.DataRoot) {
		return filepath.Join(cfg.AppPaths.DataRoot, "webview2")
	}
	return ""
}

func runWindow(cfg RunConfig, app *App) error {
	var fitOnce sync.Once
	return wails.Run(&options.App{
		Title:            "SpeedTest · 节点检测",
		Width:            960,
		Height:           640,
		MinWidth:         800,
		MinHeight:        560,
		BackgroundColour: options.NewRGB(245, 247, 251),
		AssetServer: &assetserver.Options{
			Assets: cfg.Assets,
		},
		OnStartup: app.Startup,
		OnDomReady: func(ctx context.Context) {
			fitOnce.Do(func() { fitInitialWindow(ctx) })
		},
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			Theme:                windows.Light,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			BackdropType:         windows.Mica,
			WebviewUserDataPath:  isolatedWebviewDataPath(cfg),
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarDefault(),
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
}

// Wails sizes are logical pixels. Fit the first window within the current
// monitor's work area so the title bar and bottom actions remain reachable at
// Windows display scaling settings. Resizing later remains under user control.
func fitInitialWindow(ctx context.Context) {
	width, height, err := windowWorkArea(ctx)
	if err != nil {
		log.Printf("Could not determine desktop work area: %v", err)
		return
	}
	width, height = min(1360, max(320, width-32)), min(900, max(240, height-32))
	wailsruntime.WindowSetMinSize(ctx, min(800, width), min(560, height))
	wailsruntime.WindowSetSize(ctx, width, height)
	wailsruntime.WindowCenter(ctx)
}

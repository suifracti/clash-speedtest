//go:build !windows

package desktop

import (
	"context"
	"fmt"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func windowWorkArea(ctx context.Context) (int, int, error) {
	screens, err := wailsruntime.ScreenGetAll(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, screen := range screens {
		if screen.IsCurrent && screen.Size.Width > 0 && screen.Size.Height > 0 {
			return screen.Size.Width, screen.Size.Height - 64, nil
		}
	}
	return 0, 0, fmt.Errorf("current monitor unavailable")
}

//go:build windows

package desktop

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The pinned Wails Screen API exposes logical monitor bounds but omits the
// work area (taskbar). Query that area without changing any system DPI setting.
func windowWorkArea(ctx context.Context) (int, int, error) {
	type rect struct{ left, top, right, bottom int32 }
	type monitorInfo struct {
		size          uint32
		monitor, work rect
		flags         uint32
	}
	screens, err := wailsruntime.ScreenGetAll(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, screen := range screens {
		if screen.IsCurrent && screen.Size.Width > 0 && screen.Size.Height > 0 && screen.PhysicalSize.Width > 0 && screen.PhysicalSize.Height > 0 {
			x, y := wailsruntime.WindowGetPosition(ctx)
			logicalWidth, logicalHeight := wailsruntime.WindowGetSize(ctx)
			bounds := rect{
				left: int32(x), top: int32(y),
				right:  int32(x + logicalWidth*screen.PhysicalSize.Width/screen.Size.Width),
				bottom: int32(y + logicalHeight*screen.PhysicalSize.Height/screen.Size.Height),
			}
			user32 := syscall.NewLazyDLL("user32.dll")
			// The largest window intersection identifies the same current monitor
			// as Wails, even when the initial window extends beyond its edge.
			monitor, _, _ := user32.NewProc("MonitorFromRect").Call(uintptr(unsafe.Pointer(&bounds)), 2)
			if monitor == 0 {
				return 0, 0, fmt.Errorf("current monitor unavailable")
			}
			info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
			if ok, _, err := user32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
				return 0, 0, fmt.Errorf("read current work area: %w", err)
			}
			width := int(info.work.right-info.work.left) * screen.Size.Width / screen.PhysicalSize.Width
			height := int(info.work.bottom-info.work.top) * screen.Size.Height / screen.PhysicalSize.Height
			return width, height, nil
		}
	}
	return 0, 0, fmt.Errorf("current monitor scale unavailable")
}

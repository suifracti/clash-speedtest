package desktop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SetWindowTheme keeps Windows decorations and the WebView's loading background
// aligned with the existing frontend appearance preference.
func (a *App) SetWindowTheme(theme string) error {
	if a.ctx == nil {
		return fmt.Errorf("桌面窗口尚未就绪")
	}
	switch theme {
	case "light":
		wailsruntime.WindowSetLightTheme(a.ctx)
		wailsruntime.WindowSetBackgroundColour(a.ctx, 245, 247, 251, 255)
	case "dark":
		wailsruntime.WindowSetDarkTheme(a.ctx)
		wailsruntime.WindowSetBackgroundColour(a.ctx, 17, 21, 30, 255)
	default:
		return fmt.Errorf("不支持的外观设置")
	}
	return nil
}

// A cancelled native dialog returns an empty path; the frontend keeps its
// current source and does not begin an import or claim an export succeeded.
func (a *App) SelectProfileSourceDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("桌面窗口尚未就绪")
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择已有订阅数据目录",
	})
}

func (a *App) SaveTextFile(contents, filename string) (bool, error) {
	if a.ctx == nil {
		return false, fmt.Errorf("桌面窗口尚未就绪")
	}
	filename = filepath.Base(filename)
	if filename == "." || filename == "" {
		filename = "speedtest-export.txt"
	}
	ext := strings.ToLower(filepath.Ext(filename))
	filters := []wailsruntime.FileFilter{{DisplayName: "所有文件", Pattern: "*.*"}}
	if ext != "" {
		filters = append([]wailsruntime.FileFilter{{DisplayName: "导出文件 (" + ext + ")", Pattern: "*" + ext}}, filters...)
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title: "保存导出文件", DefaultFilename: filename, Filters: filters, CanCreateDirectories: true,
	})
	if err != nil || path == "" {
		return false, err
	}
	if err := writeExportFile(path, func(w io.Writer) error {
		_, err := io.WriteString(w, contents)
		return err
	}); err != nil {
		return false, fmt.Errorf("保存导出文件失败: %w", err)
	}
	return true, nil
}

func (a *App) ExportDataBackup() (bool, error) {
	if a.ctx == nil {
		return false, fmt.Errorf("桌面窗口尚未就绪")
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title: "保存数据备份", DefaultFilename: "clash-speedtest-data-" + time.Now().Format("20060102-150405") + ".zip",
		Filters: []wailsruntime.FileFilter{{DisplayName: "数据备份 (*.zip)", Pattern: "*.zip"}}, CanCreateDirectories: true,
	})
	if err != nil || path == "" {
		return false, err
	}
	if err := writeExportFile(path, func(w io.Writer) error { return a.app.ExportDataBackup(a.ctx, w) }); err != nil {
		return false, fmt.Errorf("导出备份失败: %w", err)
	}
	return true, nil
}

// Build beside the destination and replace it only after all writes and close
// succeed. A failed export leaves an existing chosen file intact.
func writeExportFile(path string, write func(io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".speedtest-export-*.tmp")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	if err := write(f); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

//go:build !windows && !darwin

package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/faceair/clash-speedtest/speedtester"
)

// GetAvailableBrowsers lists detectable browsers on Linux/Unix.
func GetAvailableBrowsers() []BrowserInfo {
	chromePath, _ := exec.LookPath("google-chrome")
	chromiumPath, _ := exec.LookPath("chromium")
	edgePath, _ := exec.LookPath("microsoft-edge")

	return []BrowserInfo{
		{
			ID:        "chrome",
			Name:      "Google Chrome (应用模式)",
			Path:      chromePath,
			Installed: chromePath != "",
		},
		{
			ID:        "chromium",
			Name:      "Chromium (应用模式)",
			Path:      chromiumPath,
			Installed: chromiumPath != "",
		},
		{
			ID:        "edge",
			Name:      "Microsoft Edge (应用模式)",
			Path:      edgePath,
			Installed: edgePath != "",
		},
		{
			ID:        "default",
			Name:      "系统默认浏览器 (xdg-open)",
			Path:      "系统默认",
			Installed: true,
		},
	}
}

// LaunchApp opens the URL in Chromium / Chrome app mode or falls back to xdg-open.
func LaunchApp(url string, preferred string) (*exec.Cmd, error) {
	if preferred == "" || preferred == "auto" {
		if settings, err := LoadSettings(); err == nil && settings.PreferredBrowser != "" {
			preferred = settings.PreferredBrowser
		}
	}

	lower := strings.ToLower(strings.TrimSpace(preferred))
	if lower == "default" || lower == "system" {
		return nil, speedtester.OpenBrowser(url)
	}

	candidates := []string{
		"google-chrome",
		"chromium",
		"chromium-browser",
		"microsoft-edge",
	}
	if lower == "chrome" {
		candidates = []string{"google-chrome", "chrome"}
	} else if lower == "edge" {
		candidates = []string{"microsoft-edge"}
	} else if lower == "chromium" {
		candidates = []string{"chromium", "chromium-browser"}
	}

	var browserPath string
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			browserPath = path
			break
		}
	}

	if browserPath == "" {
		return nil, speedtester.OpenBrowser(url)
	}

	tempDir := filepath.Join(os.TempDir(), "clash-speedtest-profile")
	_ = os.MkdirAll(tempDir, 0o755)

	args := []string{
		fmt.Sprintf("--app=%s", url),
		fmt.Sprintf("--user-data-dir=%s", tempDir),
		"--window-size=1280,860",
		"--no-first-run",
		"--no-default-browser-check",
	}

	cmd := exec.Command(browserPath, args...)
	if err := cmd.Start(); err != nil {
		return nil, speedtester.OpenBrowser(url)
	}

	return cmd, nil
}

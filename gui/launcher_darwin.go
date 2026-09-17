//go:build darwin

package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func findDarwinApp(names ...string) string {
	home, _ := os.UserHomeDir()
	for _, name := range names {
		p1 := filepath.Join("/Applications", name+".app")
		if _, err := os.Stat(p1); err == nil {
			return p1
		}
		if home != "" {
			p2 := filepath.Join(home, "Applications", name+".app")
			if _, err := os.Stat(p2); err == nil {
				return p2
			}
		}
	}
	return ""
}

// GetAvailableBrowsers lists detectable browsers on macOS.
func GetAvailableBrowsers() []BrowserInfo {
	zenPath := findDarwinApp("Zen Browser", "Zen", "zen")
	arcPath := findDarwinApp("Arc")
	chromePath := findDarwinApp("Google Chrome")
	edgePath := findDarwinApp("Microsoft Edge")
	bravePath := findDarwinApp("Brave Browser")
	firefoxPath := findDarwinApp("Firefox")

	return []BrowserInfo{
		{
			ID:        "default",
			Name:      "系统默认浏览器 (跟随 Mac 默认设置)",
			Path:      "系统默认 (如 Zen / Safari / Chrome)",
			Installed: true,
		},
		{
			ID:        "zen",
			Name:      "Zen Browser",
			Path:      zenPath,
			Installed: zenPath != "",
		},
		{
			ID:        "chrome",
			Name:      "Google Chrome (应用模式)",
			Path:      chromePath,
			Installed: chromePath != "",
		},
		{
			ID:        "edge",
			Name:      "Microsoft Edge (应用模式)",
			Path:      edgePath,
			Installed: edgePath != "",
		},
		{
			ID:        "arc",
			Name:      "Arc Browser",
			Path:      arcPath,
			Installed: arcPath != "",
		},
		{
			ID:        "brave",
			Name:      "Brave Browser (应用模式)",
			Path:      bravePath,
			Installed: bravePath != "",
		},
		{
			ID:        "firefox",
			Name:      "Mozilla Firefox",
			Path:      firefoxPath,
			Installed: firefoxPath != "",
		},
	}
}

// LaunchApp opens the URL according to preferred browser on macOS.
func LaunchApp(url string, preferred string) (*exec.Cmd, error) {
	if preferred == "" || preferred == "auto" {
		if settings, err := LoadSettings(); err == nil && settings.PreferredBrowser != "" {
			preferred = settings.PreferredBrowser
		}
	}

	lower := strings.ToLower(strings.TrimSpace(preferred))

	// If default or system, open with system default browser (e.g. Zen Browser if set as default)
	if lower == "default" || lower == "system" || lower == "" || lower == "auto" {
		cmd := exec.Command("open", url)
		return cmd, cmd.Start()
	}

	// 1. Zen Browser
	if lower == "zen" || strings.Contains(lower, "zen") {
		zenApp := "Zen Browser"
		if findDarwinApp("Zen") != "" && findDarwinApp("Zen Browser") == "" {
			zenApp = "Zen"
		}
		cmd := exec.Command("open", "-a", zenApp, url)
		if err := cmd.Start(); err == nil {
			return cmd, nil
		}
	}

	// 2. Arc Browser
	if lower == "arc" {
		cmd := exec.Command("open", "-a", "Arc", url)
		if err := cmd.Start(); err == nil {
			return cmd, nil
		}
	}

	// 3. Firefox
	if lower == "firefox" {
		cmd := exec.Command("open", "-a", "Firefox", url)
		if err := cmd.Start(); err == nil {
			return cmd, nil
		}
	}

	// 4. Chrome (Support native app mode if possible)
	if lower == "chrome" || lower == "google chrome" {
		chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(chromePath); err == nil {
			cmd := exec.Command(chromePath, fmt.Sprintf("--app=%s", url), "--window-size=1280,860")
			if err := cmd.Start(); err == nil {
				return cmd, nil
			}
		}
		cmd := exec.Command("open", "-a", "Google Chrome", url)
		if err := cmd.Start(); err == nil {
			return cmd, nil
		}
	}

	// 5. Edge (Support app mode)
	if lower == "edge" || lower == "microsoft edge" {
		edgePath := "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"
		if _, err := os.Stat(edgePath); err == nil {
			cmd := exec.Command(edgePath, fmt.Sprintf("--app=%s", url), "--window-size=1280,860")
			if err := cmd.Start(); err == nil {
				return cmd, nil
			}
		}
		cmd := exec.Command("open", "-a", "Microsoft Edge", url)
		if err := cmd.Start(); err == nil {
			return cmd, nil
		}
	}

	// 6. Arbitrary app name or custom bundle path (e.g. open -a "MyBrowser" url)
	if preferred != "" {
		cmd := exec.Command("open", "-a", preferred, url)
		if err := cmd.Start(); err == nil {
			return cmd, nil
		}
	}

	// Fallback to default
	fallback := exec.Command("open", url)
	return fallback, fallback.Start()
}

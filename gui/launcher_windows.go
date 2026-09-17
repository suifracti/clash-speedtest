//go:build windows

package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/faceair/clash-speedtest/speedtester"
)

func findZen() string {
	candidates := []string{
		`C:\Program Files\Zen Browser\zen.exe`,
		`C:\Program Files (x86)\Zen Browser\zen.exe`,
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates,
			filepath.Join(localAppData, `Zen Browser\zen.exe`),
			filepath.Join(localAppData, `Programs\Zen Browser\zen.exe`),
		)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findChrome() string {
	candidates := []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, `Google\Chrome\Application\chrome.exe`))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findEdge() string {
	candidates := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, `Microsoft\Edge\Application\msedge.exe`))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findBrave() string {
	candidates := []string{
		`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
		`C:\Program Files (x86)\BraveSoftware\Brave-Browser\Application\brave.exe`,
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, `BraveSoftware\Brave-Browser\Application\brave.exe`))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// GetAvailableBrowsers lists detectable browsers on Windows.
func GetAvailableBrowsers() []BrowserInfo {
	zenPath := findZen()
	chromePath := findChrome()
	edgePath := findEdge()
	bravePath := findBrave()

	return []BrowserInfo{
		{
			ID:        "default",
			Name:      "系统默认浏览器 (跟随 Windows 默认设置)",
			Path:      "系统默认",
			Installed: true,
		},
		{
			ID:        "chrome",
			Name:      "Google Chrome (应用模式)",
			Path:      chromePath,
			Installed: chromePath != "",
		},
		{
			ID:        "zen",
			Name:      "Zen Browser",
			Path:      zenPath,
			Installed: zenPath != "",
		},
		{
			ID:        "edge",
			Name:      "Microsoft Edge (应用模式)",
			Path:      edgePath,
			Installed: edgePath != "",
		},
		{
			ID:        "brave",
			Name:      "Brave Browser (应用模式)",
			Path:      bravePath,
			Installed: bravePath != "",
		},
	}
}

// LaunchApp opens the URL according to preferred browser setting on Windows.
func LaunchApp(url string, preferred string) (*exec.Cmd, error) {
	if preferred == "" || preferred == "auto" {
		if settings, err := LoadSettings(); err == nil && settings.PreferredBrowser != "" {
			preferred = settings.PreferredBrowser
		}
	}

	lower := strings.ToLower(strings.TrimSpace(preferred))

	// System default browser (e.g. Zen / Chrome / Edge according to user's Windows default setting)
	if lower == "default" || lower == "system" || lower == "" || lower == "auto" {
		return nil, speedtester.OpenBrowser(url)
	}

	// 1. Zen Browser on Windows
	if lower == "zen" {
		if zen := findZen(); zen != "" {
			cmd := exec.Command(zen, url)
			if err := cmd.Start(); err == nil {
				return cmd, nil
			}
		}
		// Fallback to system default browser if Zen was explicitly picked but not found/failed
		return nil, speedtester.OpenBrowser(url)
	}

	var browserPath string
	if lower == "chrome" {
		browserPath = findChrome()
	} else if lower == "edge" {
		browserPath = findEdge()
	} else if lower == "brave" {
		browserPath = findBrave()
	} else if preferred != "" {
		if _, err := os.Stat(preferred); err == nil {
			browserPath = preferred
		}
	}

	// If preferred browser not installed, fallback to system default
	if browserPath == "" {
		return nil, speedtester.OpenBrowser(url)
	}

	// Try launching in clean app mode (without isolated user-data-dir to avoid ProcessSingleton lock collision)
	cmd := exec.Command(browserPath, fmt.Sprintf("--app=%s", url))
	if err := cmd.Start(); err == nil {
		return cmd, nil
	}

	// Fallback to opening URL normally with the browser executable
	cmd = exec.Command(browserPath, url)
	if err := cmd.Start(); err == nil {
		return cmd, nil
	}

	// Final fallback: System default browser
	return nil, speedtester.OpenBrowser(url)
}


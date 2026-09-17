package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type AppSettings struct {
	PreferredBrowser string `json:"preferred_browser"` // "auto", "chrome", "edge", "default", or custom path
}

var (
	settingsMu sync.RWMutex
)

func defaultSettingsPath() string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".clash-speedtest", "settings.json")
	}
	return filepath.Join(".", ".clash-speedtest", "settings.json")
}

func LoadSettings() (*AppSettings, error) {
	settingsMu.RLock()
	defer settingsMu.RUnlock()

	p := defaultSettingsPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return &AppSettings{
			PreferredBrowser: "default", // Default to system default browser (e.g. Zen / Chrome)
		}, nil
	}

	var s AppSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return &AppSettings{PreferredBrowser: "default"}, nil
	}
	if s.PreferredBrowser == "" {
		s.PreferredBrowser = "default"
	}
	return &s, nil
}

func SaveSettings(s *AppSettings) error {
	if s == nil {
		return nil
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()

	p := defaultSettingsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(p, data, 0o644)
}

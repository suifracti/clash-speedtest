package appdata

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	DataDirEnv = "CLASH_SPEEDTEST_DATA_DIR"
	AppDirName = "ClashSpeedTest"
)

// AppPaths is the single path decision shared by the desktop and local Web
// adapters. ProfileDir is deliberately separate from HistoryDir during the
// PR1 transition: the default history database stays at its established
// location until the later history migration task.
type AppPaths struct {
	DataRoot     string `json:"data_root"`
	ProfileDir   string `json:"profile_dir"`
	HistoryDir   string `json:"history_dir"`
	SettingsFile string `json:"settings_file"`
	Explicit     bool   `json:"explicit"`
}

// Resolve applies the contract's precedence: explicit value, environment,
// then the platform default. It never turns a failed resolution into cwd.
func Resolve(explicit string) (AppPaths, error) {
	raw := strings.TrimSpace(explicit)
	explicitMode := raw != ""
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv(DataDirEnv))
		explicitMode = raw != ""
	}

	if raw != "" && !filepath.IsAbs(raw) {
		return AppPaths{}, fmt.Errorf("data directory must be an absolute path")
	}

	root := raw
	if root == "" {
		var err error
		root, err = defaultDataRoot()
		if err != nil {
			return AppPaths{}, err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return AppPaths{}, fmt.Errorf("resolve data directory: %w", err)
	}
	root = filepath.Clean(root)

	if info, statErr := os.Stat(root); statErr == nil {
		if !info.IsDir() {
			return AppPaths{}, fmt.Errorf("data directory is not a directory")
		}
	} else if !os.IsNotExist(statErr) {
		return AppPaths{}, fmt.Errorf("inspect data directory: %w", statErr)
	}

	paths := AppPaths{
		DataRoot:   root,
		ProfileDir: filepath.Join(root, "profiles"),
		Explicit:   explicitMode,
	}
	if explicitMode {
		paths.HistoryDir = filepath.Join(root, "history")
		paths.SettingsFile = filepath.Join(root, "settings.json")
		return paths, nil
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		if err == nil {
			err = fmt.Errorf("home directory is empty")
		}
		return AppPaths{}, fmt.Errorf("resolve stable user data paths: %w", err)
	}
	legacyRoot := filepath.Join(home, ".clash-speedtest")
	paths.HistoryDir = filepath.Join(legacyRoot, "history")
	paths.SettingsFile = filepath.Join(legacyRoot, "settings.json")
	return paths, nil
}

// FromLegacy adapts test and embedding callers that already provide separate
// profile/history paths. Production startup uses Resolve instead.
func FromLegacy(profileDir, historyDir string) AppPaths {
	profileDir = absoluteOrOriginal(profileDir)
	if historyDir == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			historyDir = filepath.Join(home, ".clash-speedtest", "history")
		} else {
			historyDir = filepath.Join(".", ".clash-speedtest", "history")
		}
	}
	historyDir = absoluteOrOriginal(historyDir)
	settingsFile := filepath.Join(filepath.Dir(historyDir), "settings.json")
	dataRoot := ""
	if profileDir != "" {
		dataRoot = filepath.Dir(profileDir)
	}
	return AppPaths{
		DataRoot:     dataRoot,
		ProfileDir:   profileDir,
		HistoryDir:   historyDir,
		SettingsFile: settingsFile,
	}
}

func defaultDataRoot() (string, error) {
	var base string
	switch runtime.GOOS {
	case "windows":
		// os.UserConfigDir maps to Roaming\AppData on Windows. The contract
		// requires LocalAppData, so prefer the platform-provided environment
		// value and only derive the same location from the home directory.
		base = strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil || home == "" {
				return "", fmt.Errorf("resolve LocalAppData: %w", err)
			}
			base = filepath.Join(home, "AppData", "Local")
		}
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", fmt.Errorf("resolve Application Support: %w", err)
		}
		base = filepath.Join(home, "Library", "Application Support")
	default:
		// Linux and other Unix platforms are kept deterministic without using
		// cwd. This is only a portability fallback; Windows/macOS are the
		// contract's production platforms.
		if config, err := os.UserConfigDir(); err == nil && config != "" {
			base = config
		} else {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil || home == "" {
				return "", fmt.Errorf("resolve user config directory: %w", homeErr)
			}
			base = filepath.Join(home, ".config")
		}
	}
	return filepath.Join(base, AppDirName), nil
}

func absoluteOrOriginal(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

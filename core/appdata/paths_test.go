package appdata

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveDefaultIsIndependentOfWorkingDirectory(t *testing.T) {
	configuredBase := t.TempDir()
	firstCWD := t.TempDir()
	secondCWD := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", configuredBase)
	case "darwin":
		t.Setenv("HOME", configuredBase)
	default:
		t.Setenv("XDG_CONFIG_HOME", configuredBase)
	}
	t.Setenv(DataDirEnv, "")

	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	if err := os.Chdir(firstCWD); err != nil {
		t.Fatal(err)
	}
	first, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(secondCWD); err != nil {
		t.Fatal(err)
	}
	second, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}

	wantRoot := configuredBase
	if runtime.GOOS == "darwin" {
		wantRoot = filepath.Join(wantRoot, "Library", "Application Support")
	}
	wantRoot = filepath.Join(wantRoot, AppDirName)
	if first.DataRoot != wantRoot || second.DataRoot != wantRoot {
		t.Fatalf("default data roots drifted: first=%q second=%q want=%q", first.DataRoot, second.DataRoot, wantRoot)
	}
	if first.ProfileDir != second.ProfileDir || first.HistoryDir != second.HistoryDir {
		t.Fatalf("resolved app paths drifted across cwd: first=%+v second=%+v", first, second)
	}
	wantHistory := filepath.Join(wantRoot, "history")
	if first.HistoryDir != wantHistory || second.HistoryDir != wantHistory {
		t.Fatalf("default history did not use canonical data root: %q / %q", first.HistoryDir, second.HistoryDir)
	}
	if first.SettingsFile != filepath.Join(wantRoot, "settings.json") {
		t.Fatalf("default settings did not use canonical data root: %q", first.SettingsFile)
	}
}

func TestResolveExplicitFlagWinsOverEnvironmentAndRequiresAbsolute(t *testing.T) {
	envRoot := filepath.Join(t.TempDir(), "from-env")
	flagRoot := filepath.Join(t.TempDir(), "from-flag")
	t.Setenv(DataDirEnv, envRoot)

	paths, err := Resolve(flagRoot)
	if err != nil {
		t.Fatal(err)
	}
	if paths.DataRoot != flagRoot || !paths.Explicit {
		t.Fatalf("explicit root did not win: %+v", paths)
	}
	if paths.HistoryDir != filepath.Join(flagRoot, "history") || paths.SettingsFile != filepath.Join(flagRoot, "settings.json") {
		t.Fatalf("explicit data was not fully isolated: %+v", paths)
	}

	if _, err := Resolve("relative-data"); err == nil {
		t.Fatal("relative explicit data directory should be rejected")
	}
}

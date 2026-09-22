package appdata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
)

const (
	MigrationStateReady    = "ready"
	MigrationStatePending  = "pending"
	MigrationStateConflict = "conflict"
	MigrationStateInvalid  = "invalid"
	MigrationStateIsolated = "isolated"
	migrationLockFileName  = ".data-migration.lock"
	migrationStageNameBase = ".history-migration-"
	settingsStageNameBase  = ".settings-migration-"
)

var (
	ErrDataMigrationNotRequired = errors.New("data migration is not required")
	ErrDataMigrationConflict    = errors.New("data migration conflicts with existing canonical data")
	ErrDataMigrationLocked      = errors.New("data migration is already in progress")
	ErrDataMigrationInvalid     = errors.New("data migration source or target is invalid")
)

// MigrationInfo is a safe, non-secret summary used by the profile setup UI.
// It describes candidate locations and counts, but never reads or returns
// subscription/configuration contents.
type MigrationInfo struct {
	State              string `json:"state"`
	SourceHistoryDir   string `json:"source_history_dir,omitempty"`
	TargetHistoryDir   string `json:"target_history_dir"`
	SourceSettingsFile string `json:"source_settings_file,omitempty"`
	TargetSettingsFile string `json:"target_settings_file"`
	SourceHasSQLite    bool   `json:"source_has_sqlite"`
	SourceJSONCount    int    `json:"source_json_count"`
	SourceHasSettings  bool   `json:"source_has_settings"`
	TargetHasSQLite    bool   `json:"target_has_sqlite"`
	TargetJSONCount    int    `json:"target_json_count"`
	TargetHasSettings  bool   `json:"target_has_settings"`
	Error              string `json:"error,omitempty"`
}

// LegacyRoot returns the pre-P0-05 production root. Explicit data roots never
// consult it.
func (p AppPaths) LegacyRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".clash-speedtest")
}

func (p AppPaths) legacyHistoryDir() string {
	root := p.LegacyRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "history")
}

func (p AppPaths) legacySettingsFile() string {
	root := p.LegacyRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "settings.json")
}

// InspectMigration reports whether an explicit user confirmation is needed.
// A valid canonical database wins immediately; old locations are not scanned
// again in that case.
func (p AppPaths) InspectMigration() MigrationInfo {
	info := MigrationInfo{
		State:              MigrationStateReady,
		TargetHistoryDir:   p.HistoryDir,
		TargetSettingsFile: p.SettingsFile,
	}
	if p.Explicit {
		info.State = MigrationStateIsolated
		return info
	}

	targetDB := filepath.Join(p.HistoryDir, "history.db")
	info.TargetHasSQLite = sqliteArtifactExists(targetDB)
	info.TargetJSONCount = countJSON(filepath.Join(p.HistoryDir, "legacy"))
	info.TargetHasSettings = regularFileExists(p.SettingsFile)
	if info.TargetHasSQLite {
		if !regularFileExists(targetDB) {
			info.State = MigrationStateInvalid
			info.Error = "canonical history database is incomplete"
			return info
		}
		if err := history.ValidateDatabase(p.HistoryDir); err != nil {
			info.State = MigrationStateInvalid
			info.Error = fmt.Sprintf("canonical history database is not supported: %v", err)
			return info
		}
		return info
	}

	info.SourceHistoryDir = p.legacyHistoryDir()
	info.SourceSettingsFile = p.legacySettingsFile()
	info.SourceHasSQLite = sqliteArtifactExists(filepath.Join(info.SourceHistoryDir, "history.db"))
	info.SourceJSONCount = countJSON(info.SourceHistoryDir)
	info.SourceHasSettings = regularFileExists(info.SourceSettingsFile)
	sourceHasArtifacts := info.SourceHasSQLite || info.SourceJSONCount > 0 || info.SourceHasSettings
	targetHasArtifacts := info.TargetJSONCount > 0 || info.TargetHasSettings

	if info.SourceHasSQLite {
		sourceDB := filepath.Join(info.SourceHistoryDir, "history.db")
		if !regularFileExists(sourceDB) {
			info.State = MigrationStateInvalid
			info.Error = "legacy history database is incomplete"
			return info
		}
		if err := history.ValidateDatabase(info.SourceHistoryDir); err != nil {
			info.State = MigrationStateInvalid
			info.Error = fmt.Sprintf("legacy history database is not supported: %v", err)
			return info
		}
	}
	if sourceHasArtifacts && targetHasArtifacts {
		info.State = MigrationStateConflict
		info.Error = "canonical and legacy data both exist; choose one source explicitly"
		return info
	}
	if sourceHasArtifacts {
		info.State = MigrationStatePending
	}
	return info
}

// MigrateLegacyData performs the explicitly confirmed migration into a fresh
// canonical root. Source files remain untouched and the target is only made
// visible after staging and validation complete.
func (p AppPaths) MigrateLegacyData(ctx context.Context) error {
	return p.migrateLegacyData(ctx, migrationHooks{backup: history.BackupDatabase})
}

type migrationHooks struct {
	backup func(context.Context, string, string) error
}

func (p AppPaths) migrateLegacyData(ctx context.Context, hooks migrationHooks) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if p.Explicit {
		return fmt.Errorf("%w: explicit data roots are isolated", ErrDataMigrationNotRequired)
	}
	if p.DataRoot == "" || p.HistoryDir == "" || p.SettingsFile == "" {
		return fmt.Errorf("%w: canonical paths are incomplete", ErrDataMigrationInvalid)
	}
	if hooks.backup == nil {
		hooks.backup = history.BackupDatabase
	}
	if err := os.MkdirAll(p.DataRoot, 0o700); err != nil {
		return fmt.Errorf("prepare canonical data root: %w", err)
	}

	lockPath := filepath.Join(p.DataRoot, migrationLockFileName)
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return ErrDataMigrationLocked
		}
		return fmt.Errorf("acquire data migration lock: %w", err)
	}
	_ = lock.Close()
	defer os.Remove(lockPath)

	info := p.InspectMigration()
	if info.State != MigrationStatePending {
		if info.State == MigrationStateConflict {
			return fmt.Errorf("%w: %s", ErrDataMigrationConflict, info.Error)
		}
		if info.Error != "" {
			return fmt.Errorf("%w: %s", ErrDataMigrationInvalid, info.Error)
		}
		return fmt.Errorf("%w: current state is %s", ErrDataMigrationNotRequired, info.State)
	}

	stamp := time.Now().UTC().UnixNano()
	stageDir := filepath.Join(p.DataRoot, fmt.Sprintf("%s%d", migrationStageNameBase, stamp))
	settingsStage := filepath.Join(p.DataRoot, fmt.Sprintf("%s%d.json", settingsStageNameBase, stamp))
	cleanup := func() {
		_ = os.RemoveAll(stageDir)
		_ = os.Remove(settingsStage)
	}
	defer cleanup()
	if err := os.MkdirAll(filepath.Join(stageDir, "legacy"), 0o755); err != nil {
		return fmt.Errorf("create migration staging area: %w", err)
	}

	if info.SourceHasSQLite {
		if err := hooks.backup(ctx, info.SourceHistoryDir, filepath.Join(stageDir, "history.db")); err != nil {
			return fmt.Errorf("backup legacy history: %w", err)
		}
	} else {
		store, err := history.NewStore(stageDir)
		if err != nil {
			return fmt.Errorf("initialize empty canonical history: %w", err)
		}
		if err := store.Close(); err != nil {
			return fmt.Errorf("close staged history: %w", err)
		}
	}
	if err := copyLegacyJSON(info.SourceHistoryDir, filepath.Join(stageDir, "legacy")); err != nil {
		return fmt.Errorf("stage legacy history: %w", err)
	}
	if info.SourceHasSettings {
		if err := copyFile(info.SourceSettingsFile, settingsStage, 0o600); err != nil {
			return fmt.Errorf("stage settings: %w", err)
		}
	}
	if err := history.ValidateDatabase(stageDir); err != nil {
		return fmt.Errorf("validate staged history: %w", err)
	}

	// Do not overwrite or merge any artifact that appeared while staging.
	if canonicalArtifactsExist(p) {
		return ErrDataMigrationConflict
	}
	if err := os.Rename(stageDir, p.HistoryDir); err != nil {
		return fmt.Errorf("publish canonical history: %w", err)
	}
	if settingsStage != "" && regularFileExists(settingsStage) {
		if err := publishNewFile(settingsStage, p.SettingsFile, 0o600); err != nil {
			_ = os.RemoveAll(p.HistoryDir)
			return fmt.Errorf("publish canonical settings: %w", err)
		}
	}
	return nil
}

func canonicalArtifactsExist(p AppPaths) bool {
	return sqliteArtifactExists(filepath.Join(p.HistoryDir, "history.db")) ||
		countJSON(filepath.Join(p.HistoryDir, "legacy")) > 0 ||
		regularFileExists(p.SettingsFile)
}

func sqliteArtifactExists(dbPath string) bool {
	return regularFileExists(dbPath) || regularFileExists(dbPath+"-wal") || regularFileExists(dbPath+"-shm")
}

func regularFileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func countJSON(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			count++
		}
	}
	return count
}

func copyLegacyJSON(sourceDir, targetDir string) error {
	if sourceDir == "" {
		return nil
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := copyFile(filepath.Join(sourceDir, entry.Name()), filepath.Join(targetDir, entry.Name()), info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = os.Remove(target)
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		_ = os.Remove(target)
		return err
	}
	return output.Close()
}

func publishNewFile(staged, target string, mode os.FileMode) error {
	if regularFileExists(target) {
		return fmt.Errorf("target already exists: %s", target)
	}
	return copyFile(staged, target, mode)
}

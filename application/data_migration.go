package application

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
)

// OpenHistoryStore applies the data-root migration state before opening a
// Store. A pending migration with a SQLite source keeps the old source as the
// temporary read/write authority until the user confirms the move. A pending
// JSON/settings-only source intentionally leaves the store nil so startup does
// not create or modify either authority before confirmation.
func OpenHistoryStore(paths appdata.AppPaths) (*history.Store, error) {
	info := paths.InspectMigration()
	switch info.State {
	case appdata.MigrationStatePending:
		if !info.SourceHasSQLite {
			return nil, nil
		}
		return history.NewStore(info.SourceHistoryDir)
	case appdata.MigrationStateConflict:
		return nil, nil
	case appdata.MigrationStateInvalid:
		return nil, fmt.Errorf("data migration is blocked: %s", info.Error)
	case appdata.MigrationStateIsolated, appdata.MigrationStateReady:
		legacyDir := filepath.Join(paths.HistoryDir, "legacy")
		return history.NewStoreWithLegacyDir(paths.HistoryDir, legacyDir)
	default:
		return nil, fmt.Errorf("unknown data migration state %q", info.State)
	}
}

func dataMigrationDTO(info appdata.MigrationInfo) DataMigrationDTO {
	return DataMigrationDTO{
		State:              info.State,
		SourceHistoryDir:   info.SourceHistoryDir,
		TargetHistoryDir:   info.TargetHistoryDir,
		SourceSettingsFile: info.SourceSettingsFile,
		TargetSettingsFile: info.TargetSettingsFile,
		SourceHasSQLite:    info.SourceHasSQLite,
		SourceJSONCount:    info.SourceJSONCount,
		SourceHasSettings:  info.SourceHasSettings,
		TargetHasSQLite:    info.TargetHasSQLite,
		TargetJSONCount:    info.TargetJSONCount,
		TargetHasSettings:  info.TargetHasSettings,
		Error:              info.Error,
	}
}

// GetDataMigration returns a safe migration summary for Web and Wails.
func (s *AppService) GetDataMigration() (DataMigrationDTO, error) {
	return dataMigrationDTO(s.appPaths.InspectMigration()), nil
}

// MigrateLegacyData switches the service from the old production root to the
// canonical root only after the staged migration has committed. Rebuilt
// monitor definitions are stopped/blocked by the normal startup loader; this
// method recovers opted-in running Monitor jobs only after the canonical data
// root is ready and their profile, budget, and capacity gates are rechecked.
func (s *AppService) MigrateLegacyData() error {
	if s.Status().IsRunning {
		return fmt.Errorf("请先停止正在运行的测速任务")
	}
	info := s.appPaths.InspectMigration()
	if info.State != appdata.MigrationStatePending {
		if info.State == appdata.MigrationStateConflict {
			return fmt.Errorf("%w: %s", appdata.ErrDataMigrationConflict, info.Error)
		}
		if info.Error != "" {
			return fmt.Errorf("data migration is blocked: %s", info.Error)
		}
		return fmt.Errorf("data migration is not pending: %s", info.State)
	}
	if err := s.beginPublicServiceStorageTransition(); err != nil {
		return err
	}
	defer s.endPublicServiceStorageTransition()

	s.StopAllMonitorJobs()
	s.latencyPersistenceWG.Wait()
	if s.historyStore != nil {
		if err := s.historyStore.Close(); err != nil {
			return fmt.Errorf("close current history before migration: %w", err)
		}
		s.historyStore = nil
	}

	if err := s.appPaths.MigrateLegacyData(context.Background()); err != nil {
		s.restoreHistoryAfterMigrationFailure()
		return err
	}

	store, err := OpenHistoryStore(s.appPaths)
	if err != nil {
		s.restoreHistoryAfterMigrationFailure()
		return fmt.Errorf("open canonical history after migration: %w", err)
	}
	s.historyStore = store
	s.resetMonitorRuntime()
	if store != nil {
		if err := s.loadPersistedMonitorJobs(); err != nil {
			s.monitorLoadErr = err
			return fmt.Errorf("reload monitor job definitions after migration: %w", err)
		}
	}
	return s.RecoverMonitorJobs(context.Background())
}

func (s *AppService) restoreHistoryAfterMigrationFailure() {
	store, err := OpenHistoryStore(s.appPaths)
	if err != nil {
		s.historyStore = nil
		s.resetMonitorRuntime()
		return
	}
	s.historyStore = store
	s.resetMonitorRuntime()
	state := s.appPaths.InspectMigration().State
	if store != nil && (state == appdata.MigrationStateReady || state == appdata.MigrationStateIsolated) {
		if err := s.loadPersistedMonitorJobs(); err != nil {
			s.monitorLoadErr = err
		}
	}
}

func (s *AppService) resetMonitorRuntime() {
	s.monitorMu.Lock()
	s.monitorSchedulers = make(map[string]*monitor.Scheduler)
	s.monitorRunner = nil
	s.monitorBudget = nil
	s.monitorLoadErr = nil
	if s.historyStore != nil {
		s.monitorBudget = monitor.NewBudgetController(s.historyStore, s.monitorBudgetLimits, nil)
		s.monitorRunner = monitor.NewRunner(monitor.RunnerConfig{Store: s.historyStore, Budget: s.monitorBudget})
	}
	s.monitorMu.Unlock()
	s.monitorRecoveryMu.Lock()
	s.monitorRecoveryStarted = false
	s.monitorRecoveryMu.Unlock()
}

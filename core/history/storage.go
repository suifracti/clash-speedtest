package history

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/faceair/clash-speedtest/core/monitor"
)

// StorageUsage measures only the canonical SQLite files owned by this store.
// File sizes do not claim to measure Monitor rows in isolation because the
// database also contains Workbench history and persisted job definitions.
func (s *Store) StorageUsage(warningBytes, hardBytes int64) (monitor.StorageUsage, error) {
	if s == nil || s.db == nil || s.dir == "" {
		return monitor.StorageUsage{}, fmt.Errorf("history store is not initialized")
	}
	base := filepath.Join(s.dir, "history.db")
	var usage monitor.StorageUsage
	for _, file := range []struct {
		path string
		set  func(int64)
	}{
		{base, func(n int64) { usage.DatabaseBytes = n }},
		{base + "-wal", func(n int64) { usage.WALBytes = n }},
		{base + "-shm", func(n int64) { usage.SharedMemoryBytes = n }},
	} {
		info, err := os.Stat(file.path)
		if os.IsNotExist(err) && file.path != base {
			continue
		}
		if err != nil {
			return monitor.StorageUsage{}, fmt.Errorf("inspect SQLite storage %s: %w", file.path, err)
		}
		if !info.Mode().IsRegular() {
			return monitor.StorageUsage{}, fmt.Errorf("SQLite storage %s is not a regular file", file.path)
		}
		file.set(info.Size())
	}
	usage.TotalBytes = usage.DatabaseBytes + usage.WALBytes + usage.SharedMemoryBytes
	usage.WarningBytes = warningBytes
	usage.HardBytes = hardBytes
	usage.Warning = warningBytes > 0 && usage.TotalBytes >= warningBytes
	usage.Protected = hardBytes > 0 && usage.TotalBytes >= hardBytes
	return usage, nil
}

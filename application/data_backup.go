package application

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ExportDataBackup is shared by Web downloads and native Save dialogs. Preserve
// the existing archive layout and exclusions, but snapshot the live SQLite
// database so committed WAL data is included without copying transient files.
func (s *AppService) ExportDataBackup(ctx context.Context, destination io.Writer) error {
	root := s.appPaths.DataRoot
	if root == "" {
		return fmt.Errorf("数据根目录未配置")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("数据目录不可读取")
	}
	var databasePath, snapshot string
	if s.historyStore != nil && s.historyStore.DB() != nil {
		databasePath = filepath.Join(s.historyStore.Dir(), "history.db")
		rel, err := filepath.Rel(root, databasePath)
		if err != nil {
			return fmt.Errorf("解析历史目录失败: %w", err)
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			temporary, err := os.MkdirTemp("", "speedtest-history-backup-")
			if err != nil {
				return fmt.Errorf("创建历史快照失败: %w", err)
			}
			defer os.RemoveAll(temporary)
			snapshot = filepath.Join(temporary, "history.db")
			if err := s.historyStore.DB().BackupTo(ctx, snapshot); err != nil {
				return fmt.Errorf("保存历史快照失败: %w", err)
			}
		}
	}

	archive := zip.NewWriter(destination)
	defer archive.Close()
	metadata := map[string]any{
		"exported_at":    time.Now().UTC().Format(time.RFC3339),
		"format_version": 1,
		"platform":       runtime.GOOS,
	}
	entry, err := archive.Create("export_meta.json")
	if err != nil {
		return err
	}
	if err := json.NewEncoder(entry).Encode(metadata); err != nil {
		return err
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != root && filepath.Dir(path) == root && strings.EqualFold(info.Name(), "webview2") {
				return filepath.SkipDir
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		base := strings.ToLower(filepath.Base(path))
		if strings.HasSuffix(base, ".lock") || strings.HasSuffix(base, ".tmp") || strings.HasSuffix(base, ".wal") || strings.HasSuffix(base, ".shm") {
			return nil
		}
		if snapshot != "" && (path == databasePath+"-wal" || path == databasePath+"-shm") {
			return nil
		}
		source := path
		if snapshot != "" && path == databasePath {
			source = snapshot
			info, err = os.Stat(source)
			if err != nil {
				return err
			}
		}
		if err := appendBackupFile(archive, filepath.ToSlash(rel), source, info); err != nil {
			return fmt.Errorf("备份 %s 失败: %w", rel, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return archive.Close()
}

func appendBackupFile(archive *zip.Writer, name, path string, info os.FileInfo) error {
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = name
	header.Method = zip.Deflate
	entry, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(entry, f)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

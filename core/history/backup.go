package history

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"modernc.org/sqlite"
)

// BackupDatabase creates a consistent SQLite snapshot of sourceDir at the
// target history.db path. It deliberately uses SQLite's online backup API so
// committed WAL pages are included in the snapshot. The source is opened read-only
// without OpenDB: this entry point must preserve the pre-upgrade schema.
func BackupDatabase(ctx context.Context, sourceDir, targetDBPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sourcePath, err := filepath.Abs(filepath.Join(sourceDir, "history.db"))
	if err != nil {
		return err
	}
	uri := (&url.URL{Scheme: "file", Path: sourcePath, RawQuery: "mode=ro"}).String()
	raw, err := sql.Open("sqlite", uri)
	if err != nil {
		return fmt.Errorf("open source history: %w", err)
	}
	defer raw.Close()
	return (&DB{db: raw}).BackupTo(ctx, targetDBPath)
}

// ValidateDatabase opens a history directory through the normal schema
// authority and closes it again. It is used before a directory becomes the
// canonical production authority.
func ValidateDatabase(dir string) error {
	db, err := OpenDB(dir)
	if err != nil {
		return err
	}
	return db.Close()
}

// BackupTo writes a new history database using modernc SQLite's online backup
// connection. The destination must not already exist; callers own the final
// atomic directory switch.
func (d *DB) BackupTo(ctx context.Context, targetDBPath string) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("history database is not initialized")
	}
	if targetDBPath == "" {
		return fmt.Errorf("backup target is empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(targetDBPath); err == nil {
		return fmt.Errorf("backup target already exists: %s", targetDBPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect backup target: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetDBPath), 0o755); err != nil {
		return fmt.Errorf("create backup target directory: %w", err)
	}

	// Build privately in the destination filesystem. Link publishes atomically
	// without replacing any target created while the snapshot was in progress.
	temporary, err := os.CreateTemp(filepath.Dir(targetDBPath), ".history-backup-*.tmp")
	if err != nil {
		return fmt.Errorf("create private backup: %w", err)
	}
	temporaryPath, err := filepath.Abs(temporary.Name())
	if err != nil {
		temporary.Close()
		os.Remove(temporary.Name())
		return err
	}
	defer removeSQLiteArtifacts(temporaryPath)
	if err := temporary.Close(); err != nil {
		return err
	}

	conn, err := d.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open backup connection: %w", err)
	}
	defer conn.Close()

	err = conn.Raw(func(driverConn any) error {
		backuper, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("sqlite driver does not support online backup")
		}
		backup, err := backuper.NewBackup((&url.URL{Scheme: "file", Path: temporaryPath}).String())
		if err != nil {
			return fmt.Errorf("create online backup: %w", err)
		}
		var more bool
		var stepErr error
		for {
			if stepErr = ctx.Err(); stepErr != nil {
				break
			}
			more, stepErr = backup.Step(128)
			if stepErr != nil || !more {
				break
			}
		}
		finishErr := backup.Finish()
		if stepErr != nil {
			return fmt.Errorf("copy online backup pages: %w", stepErr)
		}
		if finishErr != nil {
			return fmt.Errorf("finish online backup: %w", finishErr)
		}
		if more {
			return fmt.Errorf("online backup did not finish")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, targetDBPath); err != nil {
		return fmt.Errorf("publish backup without replacement: %w", err)
	}
	return nil
}

func removeSQLiteArtifacts(dbPath string) {
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
}

package history

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// ErrUpgradeBackup means the existing v11 database was not upgraded because
// its verified pre-upgrade snapshot could not be completed.
var ErrUpgradeBackup = errors.New("v11 pre-upgrade backup failed")

// Narrow per-call hooks let failure tests exercise the real migration transaction
// without process-wide mutable state or a public bypass of the production gate.
type upgradeBackupHooks struct {
	backup func(context.Context, string, string) error
	sync   func(string) error
}

// createVerifiedV11Rollback never uses OpenDB/ValidateDatabase on the snapshot:
// both are migration authorities. A completed backup survives later migration
// failure; incomplete attempts are private and are never reused on retries.
// Cleanup can fail under real permission/I/O errors: report it and quarantine
// only this attempt if the parent directory still permits the move. A manifest
// alone is never proof that this function's durability gate returned success.
func createVerifiedV11Rollback(ctx context.Context, dir string, hooks upgradeBackupHooks) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if hooks.backup == nil {
		hooks.backup = BackupDatabase
	}
	if hooks.sync == nil {
		hooks.sync = syncUpgradeBackupPath
	}
	root := filepath.Join(dir, ".schema-upgrade-backups")
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create backup directory: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 {
		return fmt.Errorf("backup directory must be a private directory (0700): %s", root)
	}
	attempt, err := os.MkdirTemp(root, "v11-to-v12-")
	if err != nil {
		return fmt.Errorf("create backup attempt: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			if err := os.RemoveAll(attempt); err != nil {
				retained, quarantineErr := quarantineFailedUpgradeAttempt(root, attempt)
				retErr = errors.Join(retErr,
					fmt.Errorf("cleanup incomplete backup; residual artifacts at %s: %w", retained, err),
					quarantineErr)
			}
		}
	}()
	snapshot := filepath.Join(attempt, "history.db")
	if err := hooks.backup(ctx, dir, snapshot); err != nil {
		return fmt.Errorf("snapshot v11: %w", err)
	}
	if err := verifyV11Rollback(ctx, snapshot); err != nil {
		return fmt.Errorf("verify v11 snapshot: %w", err)
	}
	file, err := os.Open(snapshot)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, hashErr := io.Copy(hash, file)
	closeErr := file.Close()
	if hashErr != nil {
		return hashErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := hooks.sync(snapshot); err != nil {
		return fmt.Errorf("sync snapshot: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	manifest := struct {
		Format    int    `json:"format_version"`
		From      int    `json:"from_schema"`
		To        int    `json:"to_schema"`
		Snapshot  string `json:"snapshot"`
		SHA256    string `json:"sha256"`
		CreatedAt string `json:"created_at"`
	}{1, 11, 12, "history.db", hex.EncodeToString(hash.Sum(nil)), time.Now().UTC().Format(time.RFC3339Nano)}
	metadata, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	private := filepath.Join(attempt, ".complete.tmp")
	if err := os.WriteFile(private, metadata, 0600); err != nil {
		return fmt.Errorf("write backup manifest: %w", err)
	}
	if err := hooks.sync(private); err != nil {
		return fmt.Errorf("sync backup manifest: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Link(private, filepath.Join(attempt, "complete.json")); err != nil {
		return fmt.Errorf("publish backup manifest: %w", err)
	}
	if err := os.Remove(private); err != nil {
		return err
	}
	// Persist both the files and each newly-created directory entry before DDL.
	for _, path := range []string{attempt, root, dir} {
		if err := hooks.sync(path); err != nil {
			return fmt.Errorf("sync backup directory: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	complete = true
	return nil
}

// Reserve an unused destination name without touching any prior backup.
// Rename within the same parent avoids needing to update an unwritable attempt's
// parent entry (moving it into a wrapper can itself fail on macOS).
// If even that fails, leave the original path observable in the returned error;
// do not chmod directories or silently treat a residual complete.json as success.
func quarantineFailedUpgradeAttempt(root, attempt string) (string, error) {
	retained, err := os.MkdirTemp(root, ".failed-")
	if err != nil {
		return attempt, fmt.Errorf("quarantine incomplete backup: %w", err)
	}
	// macOS requires an absent destination for this directory rename. Remove
	// only the empty reservation just created by this call, never old artifacts.
	if err := os.Remove(retained); err != nil {
		return attempt, fmt.Errorf("prepare quarantine destination: %w", err)
	}
	if err := os.Rename(attempt, retained); err != nil {
		return attempt, fmt.Errorf("quarantine incomplete backup: %w", err)
	}
	// Persist the changed directory entries where the filesystem permits it.
	if err := syncUpgradeBackupPath(root); err != nil {
		return retained, fmt.Errorf("sync failed-backup quarantine: %w", err)
	}
	return retained, nil
}

func syncUpgradeBackupPath(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(syncErr, closeErr)
}

func verifyV11Rollback(ctx context.Context, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return fmt.Errorf("snapshot must be a private regular file (0600)")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	uri := (&url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro"}).String()
	raw, err := sql.Open("sqlite", uri)
	if err != nil {
		return err
	}
	defer raw.Close()
	tx, err := raw.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	hasMeta, version, err := readSchemaVersion(tx)
	if err != nil {
		return err
	}
	if !hasMeta || version != 11 {
		return fmt.Errorf("rollback schema must be v11, got %d", version)
	}
	if err := validateCurrentSchema(tx, 11); err != nil {
		return err
	}
	if err := validateMonitorBudgetSchema(tx); err != nil {
		return err
	}
	columns, err := tableColumns(tx, "workbench_latency_tests")
	if err != nil {
		return err
	}
	if columns["network_path_json"] {
		return fmt.Errorf("rollback contains v12 path column")
	}
	rows, err := tx.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			rows.Close()
			return err
		}
		if result != "ok" {
			rows.Close()
			return fmt.Errorf("snapshot integrity check failed: %s", result)
		}
		count++
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return rowErr
	}
	if count != 1 {
		return fmt.Errorf("snapshot integrity check returned %d rows", count)
	}
	rows, err = tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	hasViolation := rows.Next()
	rowErr = rows.Err()
	rows.Close()
	if rowErr != nil {
		return rowErr
	}
	if hasViolation {
		return fmt.Errorf("snapshot foreign key check failed")
	}
	return ctx.Err()
}

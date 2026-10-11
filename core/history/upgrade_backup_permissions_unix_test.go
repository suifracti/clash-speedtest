//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package history

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fail visibly if this Unix runner cannot reproduce real directory write denial.
// Never treat an unsupported permission model as a passing cleanup regression.
func requireUpgradePermissionModel(t *testing.T) {
	t.Helper()
	if os.Geteuid() <= 0 {
		t.Fatal("permission regression requires a non-root Unix test runner")
	}
	probe := filepath.Join(t.TempDir(), "permission-probe")
	if err := os.Mkdir(probe, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(probe, 0700); err != nil {
			t.Error(err)
		}
	})
	if err := os.Chmod(probe, 0500); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(probe, "denied"), []byte("synthetic"), 0600); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("runner does not enforce Unix directory write denial: %v", err)
	}
}

// Real OS unlink denial reproduces Sol D's post-publication cleanup failure.
// The sync error is synthetic; the permission denial is not mocked.
func TestV11UpgradeBackupCleanupPermissionFailure(t *testing.T) {
	requireUpgradePermissionModel(t)
	dir, live, hashes := upgradeWALFixture(t)
	// A prior valid snapshot must survive both failure and the fresh retry.
	if err := createVerifiedV11Rollback(context.Background(), dir, upgradeBackupHooks{}); err != nil {
		t.Fatal(err)
	}
	prior := completedUpgradeSnapshots(t, dir)
	priorBytes, err := os.ReadFile(prior[0])
	if err != nil {
		t.Fatal(err)
	}
	priorHash := sha256.Sum256(priorBytes)
	var attempt string
	root := filepath.Join(dir, ".schema-upgrade-backups")
	t.Cleanup(func() {
		// Permissions are restored only on this test's synthetic directories.
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return nil
		})
	})
	hooks := upgradeBackupHooks{sync: func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasPrefix(filepath.Base(path), "v11-to-v12-") {
			attempt = path
			if err := os.Chmod(path, 0500); err != nil {
				return err
			}
			return errors.New("synthetic final attempt sync failure")
		}
		return syncUpgradeBackupPath(path)
	}}
	err = migrateSchema(live, func() error { return createVerifiedV11Rollback(context.Background(), dir, hooks) })
	if !errors.Is(err, ErrUpgradeBackup) {
		t.Fatalf("gate error=%v", err)
	}
	assertUpgradeVersion(t, live, 11)
	assertUpgradeSourceHashes(t, hashes)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("cleanup permission failure was swallowed: %v", err)
	}
	if _, statErr := os.Stat(attempt); !os.IsNotExist(statErr) {
		t.Fatalf("failed attempt remains in completion namespace: stat=%v gate=%v", statErr, err)
	}
	quarantines, err := filepath.Glob(filepath.Join(root, ".failed-*"))
	if err != nil || len(quarantines) != 1 {
		t.Fatalf("failed quarantine=%v err=%v", quarantines, err)
	}
	failed := quarantines[0]
	if _, err := os.Stat(filepath.Join(failed, "complete.json")); err != nil {
		t.Fatalf("failed artifacts not observable: %v", err)
	}
	if got := len(completedUpgradeSnapshots(t, dir)); got != 1 {
		t.Fatalf("failed attempt counted as complete: %d", got)
	}
	// Restore permission only on the isolated fixture. Retry must not reuse it.
	if err := os.Chmod(failed, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	assertUpgradeVersion(t, live, 12)
	if got := len(completedUpgradeSnapshots(t, dir)); got != 2 {
		t.Fatalf("fresh retry backups=%d want2", got)
	}
	b, err := os.ReadFile(prior[0])
	if err != nil || sha256.Sum256(b) != priorHash {
		t.Fatal("prior backup changed")
	}
	if _, err := os.Stat(filepath.Join(failed, "complete.json")); err != nil {
		t.Fatal("retry deleted failed quarantine")
	}
	t.Log("post-publication failure: source v11 and DB/WAL hashes unchanged; real cleanup denial surfaced; failed attempt quarantined; fresh retry preserves prior and failed artifacts")
}

func TestV11UpgradeBackupCleanupAndQuarantineDenied(t *testing.T) {
	requireUpgradePermissionModel(t)
	dir, live, hashes := upgradeWALFixture(t)
	root := filepath.Join(dir, ".schema-upgrade-backups")
	var attempt string
	t.Cleanup(func() {
		os.Chmod(root, 0700)
		if attempt != "" {
			os.Chmod(attempt, 0700)
		}
	})
	hooks := upgradeBackupHooks{sync: func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasPrefix(filepath.Base(path), "v11-to-v12-") {
			attempt = path
			if err := os.Chmod(path, 0500); err != nil {
				return err
			}
			if err := os.Chmod(root, 0500); err != nil {
				return err
			}
			return errors.New("synthetic directory sync failure with parent permission denial")
		}
		return syncUpgradeBackupPath(path)
	}}
	err := migrateSchema(live, func() error { return createVerifiedV11Rollback(context.Background(), dir, hooks) })
	if !errors.Is(err, ErrUpgradeBackup) || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("failure did not retain gate/permission causes: %v", err)
	}
	if !strings.Contains(err.Error(), attempt) || !strings.Contains(err.Error(), "quarantine incomplete backup") {
		t.Fatalf("residual path/quarantine failure not observable: %v", err)
	}
	assertUpgradeVersion(t, live, 11)
	assertUpgradeSourceHashes(t, hashes)
	for _, path := range []string{root, attempt} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0500 {
			t.Fatalf("production changed fixture permissions at %s: %v %v", path, info, err)
		}
	}
	// A readable manifest can remain when both removal and rename are denied.
	// It is not proof of gate success; OpenDB must create a different attempt.
	manifest := filepath.Join(attempt, "complete.json")
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(attempt, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	assertUpgradeVersion(t, live, 12)
	paths, err := filepath.Glob(filepath.Join(root, "v11-to-v12-*", "complete.json"))
	if err != nil || len(paths) != 2 {
		t.Fatalf("retry did not create a distinct attempt: %v %v", paths, err)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("retry modified failed manifest")
	}
	t.Log("cleanup and quarantine both denied: joined errors expose residual path; source DB/WAL unchanged; permissions untouched; retry creates new backup without trusting residual complete.json")
}

//go:build windows

package history

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This is a Windows runtime regression, not a successful-upgrade support claim.
// Run on Windows with: go test ./core/history -run '^TestV11UpgradeBackupWindowsUnsupportedPrivateRootBlocksDDL$' -count=1 -v
func TestV11UpgradeBackupWindowsUnsupportedPrivateRootBlocksDDL(t *testing.T) {
	dir, live, hashes := upgradeWALFixture(t)
	root := filepath.Join(dir, ".schema-upgrade-backups")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	// Go reports Windows writable directory attributes as 0777; chmod's Unix
	// owner/group bits do not establish or inspect a private Windows DACL.
	if !info.IsDir() || info.Mode().Perm() != 0777 {
		t.Fatalf("unexpected Windows directory mode: %v", info.Mode())
	}
	for attempt := 0; attempt < 2; attempt++ {
		db, err := OpenDB(dir)
		if db != nil {
			db.Close()
			t.Fatal("unsupported private root returned an open database")
		}
		if !errors.Is(err, ErrUpgradeBackup) || !strings.Contains(err.Error(), "backup directory must be a private directory (0700)") {
			t.Fatalf("expected explanatory fail-closed permission gate: %v", err)
		}
		assertUpgradeVersion(t, live, 11)
		assertUpgradeSourceHashes(t, hashes)
		var columns int
		if err := live.QueryRow("SELECT count(*) FROM pragma_table_info('workbench_latency_tests') WHERE name='network_path_json'").Scan(&columns); err != nil || columns != 0 {
			t.Fatalf("v12 DDL ran: columns=%d err=%v", columns, err)
		}
		var value, count int
		if err := live.QueryRow("SELECT latency_ms FROM workbench_latency_samples WHERE seq=3").Scan(&value); err != nil || value != 47 {
			t.Fatalf("committed synthetic WAL page changed: value=%d err=%v", value, err)
		}
		if err := live.QueryRow("SELECT count(*) FROM workbench_latency_samples").Scan(&count); err != nil || count != 3 {
			t.Fatalf("synthetic samples changed: count=%d err=%v", count, err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("gate created backup attempts: %v err=%v", entries, err)
		}
	}
	t.Log("Windows unsupported 0700 root: explanatory error; two OpenDB attempts fail closed; schema11, committed WAL sample47 and DB/WAL hashes unchanged; no v12 DDL or backup publication")
}

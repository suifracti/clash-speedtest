package desktop

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
)

func TestNativeBackupCompletionBoundary(t *testing.T) {
	for _, outcome := range []string{"destination-fails", "record-fails", "saved"} {
		t.Run(outcome, func(t *testing.T) {
			paths, err := appdata.Resolve(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.DataRoot, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(paths.DataRoot, "last-backup-at")
			const previous = "2026-10-01T00:00:00Z"
			if outcome == "record-fails" {
				if err := os.Mkdir(marker, 0700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(marker, []byte(previous), 0600); err != nil {
				t.Fatal(err)
			}
			app := NewAppWithOptions(nil, paths, "synthetic-test", application.Options{NoAutoCredentials: true})
			app.ctx = context.Background()
			defer app.app.Close()
			targetDir := t.TempDir()
			target := filepath.Join(targetDir, "backup.zip")
			if outcome == "destination-fails" {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "keep.txt"), []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err = app.exportDataBackupTo(target)
			if outcome == "destination-fails" {
				if err == nil {
					t.Fatal("destination rename must fail")
				}
				stamp, _ := os.ReadFile(marker)
				if string(stamp) != previous {
					t.Fatalf("failed final save advanced backup time: %s", stamp)
				}
				original, _ := os.ReadFile(filepath.Join(target, "keep.txt"))
				if string(original) != "original" {
					t.Fatal("failed save changed the destination")
				}
			} else {
				if outcome == "record-fails" {
					if err == nil || !strings.Contains(err.Error(), "备份文件已保存") {
						t.Fatalf("saved file and record error must be distinguished: %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				archive, openErr := zip.OpenReader(target)
				if openErr != nil {
					t.Fatalf("completed file must survive: %v", openErr)
				}
				archive.Close()
				if outcome == "saved" {
					stamp, _ := os.ReadFile(marker)
					if string(stamp) == previous || len(stamp) == 0 {
						t.Fatal("completed save did not record its time")
					}
				}
			}
			leftovers, _ := filepath.Glob(filepath.Join(targetDir, ".speedtest-export-*.tmp"))
			if len(leftovers) != 0 {
				t.Fatalf("temporary exports remain: %v", leftovers)
			}
		})
	}
}

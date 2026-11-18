package web

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
)

type backupResponseWriter struct {
	*httptest.ResponseRecorder
	write func([]byte) (int, error)
}

func TestWebBackupKeepsArchiveWhenRecordFails(t *testing.T) {
	paths, err := appdata.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.DataRoot, "last-backup-at"), 0700); err != nil {
		t.Fatal(err)
	}
	service := application.NewAppServiceWithOptions(nil, paths, nil, application.Options{NoAutoCredentials: true})
	defer service.Close()
	rec := httptest.NewRecorder()
	server := &Server{app: service}
	server.handleExportDataRoot(rec, httptest.NewRequest(http.MethodGet, "/api/data/export", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("completed archive was discarded: %s", rec.Body.String())
	}
	if _, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len())); err != nil {
		t.Fatal(err)
	}
	setup, err := service.GetProfileSetup()
	if err != nil || setup.LastBackupAt != nil || !strings.Contains(setup.LastBackupError, "备份已导出") {
		t.Fatalf("record warning missing: %+v, %v", setup, err)
	}
}

func (w backupResponseWriter) Write(p []byte) (int, error) { return w.write(p) }

func TestWebBackupCompletionBoundary(t *testing.T) {
	for _, outcome := range []string{"disconnected", "short-write", "cancelled", "transferred"} {
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
			if err := os.WriteFile(marker, []byte(previous), 0600); err != nil {
				t.Fatal(err)
			}
			service := application.NewAppServiceWithOptions(nil, paths, nil, application.Options{NoAutoCredentials: true})
			defer service.Close()
			server := &Server{app: service}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := httptest.NewRequest(http.MethodGet, "/api/data/export", nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			writes := 0
			writer := backupResponseWriter{ResponseRecorder: rec, write: func(p []byte) (int, error) {
				writes++
				stamp, _ := os.ReadFile(marker)
				if string(stamp) != previous {
					t.Error("backup time advanced before response transfer completed")
				}
				switch outcome {
				case "disconnected":
					return 0, errors.New("client disconnected")
				case "short-write":
					return len(p) / 2, nil
				case "cancelled":
					cancel()
				}
				return rec.Write(p)
			}}
			server.handleExportDataRoot(writer, req)
			if writes == 0 || rec.Header().Get("Content-Type") != "application/zip" {
				t.Fatalf("did not reach final ZIP transfer: %s", rec.Body.String())
			}
			stamp, _ := os.ReadFile(marker)
			if outcome != "transferred" {
				if string(stamp) != previous {
					t.Fatal("failed/cancelled transfer changed backup record")
				}
			} else {
				if string(stamp) == previous || len(stamp) == 0 {
					t.Fatal("completed response did not record export time")
				}
				if _, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len())); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/history"
)

func TestDataBackupRestoresCommittedHistoryAndUserFiles(t *testing.T) {
	paths, err := appdata.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"profiles/airports.json":   `{"airports":[{"name":"Synthetic airport"}]}`,
		"settings.json":            `{"preferred_browser":"edge"}`,
		"session.lock":             "lock",
		"working.tmp":              "unfinished",
		"webview2/Default/Cookies": "synthetic browser cache",
	} {
		path := filepath.Join(paths.DataRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := history.NewStore(paths.HistoryDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	stamp := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if err := store.SaveLatencyTest(ctx, &history.LatencyTest{
		AttemptID: "backup-latency", ProfileID: "synthetic-profile", NodeKey: "synthetic-node",
		TestProject: "latency_stability", Target: "cloudflare", RequestedAt: stamp, StartedAt: stamp, FinishedAt: stamp,
		Status: "completed", TotalSamples: 1, SuccessSamples: 1,
		Samples: []history.LatencyTestSample{{Seq: 1, Timestamp: stamp, LatencyMs: 74, Success: true}},
	}); err != nil {
		t.Fatal(err)
	}
	wal, err := os.Stat(filepath.Join(paths.HistoryDir, "history.db-wal"))
	if err != nil || wal.Size() == 0 {
		t.Fatalf("fixture must retain committed WAL pages: %v", err)
	}
	service := &AppService{appPaths: paths, historyStore: store}
	var output bytes.Buffer
	if err := service.ExportDataBackup(ctx, &output); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil {
		t.Fatal(err)
	}
	restoredRoot := t.TempDir()
	files := make(map[string][]byte)
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		contents, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[file.Name] = contents
		if file.Name == "history/history.db" {
			if err := os.MkdirAll(filepath.Join(restoredRoot, "history"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(restoredRoot, "history", "history.db"), contents, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := string(files["profiles/airports.json"]); got != `{"airports":[{"name":"Synthetic airport"}]}` {
		t.Fatalf("subscription file changed: %s", got)
	}
	if got := string(files["settings.json"]); got != `{"preferred_browser":"edge"}` {
		t.Fatalf("settings file changed: %s", got)
	}
	for _, name := range []string{"session.lock", "working.tmp", "history/history.db-wal", "history/history.db-shm", "webview2/Default/Cookies"} {
		if _, exists := files[name]; exists {
			t.Fatalf("transient file included in backup: %s", name)
		}
	}
	var metadata map[string]any
	if err := json.Unmarshal(files["export_meta.json"], &metadata); err != nil || metadata["format_version"] != float64(1) {
		t.Fatalf("backup metadata missing or changed: %v", err)
	}
	restored, err := history.NewStore(filepath.Join(restoredRoot, "history"))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	record, err := restored.GetLatencyTest(ctx, "backup-latency")
	if err != nil || record == nil || len(record.Samples) != 1 || record.Samples[0].LatencyMs != 74 || !record.Samples[0].Success {
		t.Fatalf("committed sample lost from restored backup: record=%+v err=%v", record, err)
	}
	if err := service.ExportDataBackup(ctx, failingBackupWriter{}); err == nil {
		t.Fatal("backup must report a destination write failure")
	}
}

type failingBackupWriter struct{}

func (failingBackupWriter) Write([]byte) (int, error) {
	return 0, errors.New("destination unavailable")
}

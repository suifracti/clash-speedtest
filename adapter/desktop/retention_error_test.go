package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestDesktopRetentionPartialFailurePreservesCommittedCounts(t *testing.T) {
	paths, err := appdata.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := history.NewStore(paths.HistoryDir)
	if err != nil {
		t.Fatal(err)
	}
	app := NewAppWithPaths(store, paths, "synthetic-test")
	defer app.app.Close()
	ctx := context.Background()
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	samples := make([]*monitor.MonitorSample, 1200)
	for i := range samples {
		samples[i] = &monitor.MonitorSample{SampleID: fmt.Sprintf("native-retention-%04d", i), RunID: "synthetic-run", NodeKey: "synthetic-node", Timestamp: cutoff.Add(-time.Duration(i+1) * time.Minute), Success: true}
	}
	if err := store.SaveMonitorSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	store.SetTestBatchFailAt(3)
	_, err = app.ApplyRetention(monitor.RetentionRequest{Policy: monitor.Retention30d, CutoffTime: &cutoff})
	if err == nil || !strings.HasPrefix(err.Error(), "CST_RETENTION_ERROR:") {
		t.Fatalf("partial progress must survive Wails promise rejection: %v", err)
	}
	var detail struct {
		Partial        bool   `json:"partial"`
		SamplesDeleted int64  `json:"samples_deleted"`
		RunsDeleted    int64  `json:"runs_deleted"`
		Error          string `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(err.Error(), "CST_RETENTION_ERROR:")), &detail); err != nil {
		t.Fatal(err)
	}
	if !detail.Partial || detail.SamplesDeleted != 1000 || detail.RunsDeleted != 0 || detail.Error == "" {
		t.Fatalf("incorrect native partial result: %+v", detail)
	}
	preview, err := store.PreviewRetention(ctx, monitor.RetentionRequest{Policy: monitor.Retention30d, CutoffTime: &cutoff})
	if err != nil || preview.SamplesToDelete != 200 {
		t.Fatalf("reported deletion must match committed rows: %+v, %v", preview, err)
	}
}

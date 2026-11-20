package autopick

import (
	"testing"
	"time"
)

func TestNextRunAfterUsesLocalGrid(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 10, 21, 7, 30, 0, 0, loc)
	for _, tc := range []struct {
		schedule string
		hour     int
		want     time.Time
	}{
		{Schedule6h, 0, time.Date(2026, 10, 21, 12, 0, 0, 0, loc)},
		{Schedule12h, 0, time.Date(2026, 10, 21, 12, 0, 0, 0, loc)},
		{ScheduleDaily, 6, time.Date(2026, 10, 22, 6, 0, 0, 0, loc)},
		{ScheduleDaily, 9, time.Date(2026, 10, 21, 9, 0, 0, 0, loc)},
	} {
		cfg := Config{Schedule: tc.schedule, DailyHour: tc.hour}
		if got := NextRunAfter(now, cfg); !got.Equal(tc.want) {
			t.Fatalf("%s@%d: got %s want %s", tc.schedule, tc.hour, got, tc.want)
		}
	}
}

func TestConfigDefaultsAndEstimate(t *testing.T) {
	cfg := Config{AirportIDs: []string{" a ", "a", ""}}
	cfg.Normalize()
	if cfg.Schedule != Schedule6h || cfg.MinResponseRate != 90 || cfg.MaxLatencyMS != 800 || cfg.SampleCount != 6 {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if len(cfg.AirportIDs) != 1 || cfg.AirportIDs[0] != "a" {
		t.Fatalf("airport ids not normalized: %+v", cfg.AirportIDs)
	}
	if got := cfg.EstimateRequests(10); got != 10 {
		t.Fatalf("latency-only estimate: %d", got)
	}
	cfg.IncludeAI = true
	if got := cfg.EstimateRequests(10); got != 20 {
		t.Fatalf("ai estimate: %d", got)
	}
	if got := cfg.RequiredServices(PurposeAI); len(got) != 1 || got[0] != "gemini_unlock" {
		t.Fatalf("ai defaults: %+v", got)
	}
	cfg.IncludeStreaming = true
	if got := cfg.EstimateRequests(10); got != 30 {
		t.Fatalf("ai+streaming estimate: %d", got)
	}
	if got := cfg.RequiredServices(PurposeStreaming); len(got) != 1 || got[0] != "netflix_unlock" {
		t.Fatalf("streaming defaults: %+v", got)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	cfg.AirportIDs = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("empty scope must be rejected")
	}
}

func TestRequiredServicesAreTheOnlyOnesProbed(t *testing.T) {
	cfg := Config{AirportIDs: []string{"a"}, Schedule: Schedule6h, IncludeAI: true, IncludeStreaming: true}
	cfg.Normalize()
	if got := cfg.Projects(); got != 3 {
		t.Fatalf("projects: %d, want 1 latency + 1 ai + 1 streaming", got)
	}
	cfg.AIRequired = []string{"chatgpt_unlock", "gemini_unlock", "claude_unlock", "antigravity", "nope"}
	cfg.StreamingRequired = []string{"tiktok_unlock", "dlsite_unlock", "youtube_premium", "nope"}
	cfg.Normalize()
	if got := cfg.AIRequired; len(got) != 3 || got[2] != "claude_unlock" {
		t.Fatalf("unknown AI service not dropped or legacy AI services lost: %+v", got)
	}
	if got := cfg.StreamingRequired; len(got) != 3 || got[0] != "tiktok_unlock" || got[1] != "dlsite_unlock" || got[2] != "youtube_premium" {
		t.Fatalf("streaming services normalized unexpectedly: %+v", got)
	}
	if got := cfg.Projects(); got != 7 {
		t.Fatalf("projects after edit: %d", got)
	}
	// 显式清空必需服务不能默默退回默认值。
	cfg.AIRequired = []string{}
	cfg.Normalize()
	if len(cfg.AIRequired) != 0 {
		t.Fatalf("explicit empty list must stay empty: %+v", cfg.AIRequired)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("empty required services must be rejected")
	}
}

func TestSummaryLine(t *testing.T) {
	report := Report{CreatedAt: time.Date(2026, 10, 22, 6, 0, 0, 0, time.Local), SelectedCount: 18, GroupCounts: map[string]int{PurposeDaily: 12, PurposeStreaming: 6, PurposeAI: 4}}
	if got := SummaryLine(report); got != "选中 18 个 · 日常 12／流媒体 6／AI 4 · 10/22 06:00" {
		t.Fatalf("summary: %q", got)
	}
	report.KeptPrevious = true
	if got := SummaryLine(report); got != "沿用上次结果 · 10/22 06:00" {
		t.Fatalf("kept previous summary: %q", got)
	}
}

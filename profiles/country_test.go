package profiles

import (
	"reflect"
	"testing"
	"time"
)

func TestDetectCountry(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"🇭🇰 香港 HK-01", "HK"},
		{"Premium|广港|IEPL|01", "HK"},
		{"台湾 TW-02", "TW"},
		{"🇨🇳台湾专线01|BGP|流媒体", "TW"},
		{"🇯🇵 日本东京01", "JP"},
		{"US-LosAngeles-1", "US"},
		{"新加坡 SG-IEPL", "SG"},
		{"韩国 首尔 01", "KR"},
		{"印度尼西亚-01", "ID"},
		{"印度 Mumbai", "IN"},
		{"Relay-IEPL-01", "OTHER"},
		{"🇬🇧 London-01", "UK"},
	}
	for _, tt := range tests {
		if got := DetectCountry(tt.name); got != tt.want {
			t.Errorf("DetectCountry(%q) = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestGroupByCountry(t *testing.T) {
	names := []string{
		"香港 01",
		"香港 02",
		"日本 01",
		"🇨🇳台湾专线01|BGP|流媒体",
		"套餐到期: 长期有效",
		"未知节点",
	}
	groups := GroupByCountry(names)
	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3 (HK, JP, TW; OTHER should be excluded), got: %+v", len(groups), groups)
	}
	foundTW := false
	for _, g := range groups {
		if g.Code == "TW" {
			foundTW = true
			if g.Label != "台湾" {
				t.Fatalf("TW label = %q, want 台湾", g.Label)
			}
		}
		if g.Code == "OTHER" {
			t.Fatalf("OTHER should be excluded from country groups")
		}
	}
	if !foundTW {
		t.Fatalf("TW not found in groups: %+v", groups)
	}
}

func TestParseCountrySelection(t *testing.T) {
	groups := []CountryGroup{
		{Code: "HK", Label: "香港"},
		{Code: "JP", Label: "日本"},
		{Code: "US", Label: "美国"},
	}

	all, codes, err := parseCountrySelection("a", groups)
	if err != nil || !all {
		t.Fatalf("all: all=%v codes=%v err=%v", all, codes, err)
	}

	all, codes, err = parseCountrySelection("1,3", groups)
	if err != nil || all || !reflect.DeepEqual(codes, []string{"HK", "US"}) {
		t.Fatalf("numbers: all=%v codes=%v err=%v", all, codes, err)
	}

	all, codes, err = parseCountrySelection("香港,JP", groups)
	if err != nil || all || !reflect.DeepEqual(codes, []string{"HK", "JP"}) {
		t.Fatalf("names: all=%v codes=%v err=%v", all, codes, err)
	}

	_, _, err = parseCountrySelection("9", groups)
	if err == nil {
		t.Fatal("expected out of range error")
	}
}

func TestParseMetricSelection(t *testing.T) {
	all, err := parseMetricSelection("a")
	if err != nil || !all.Latency || !all.Download || !all.Upload || !all.Antigravity {
		t.Fatalf("all: %+v err=%v", all, err)
	}

	combo, err := parseMetricSelection("1,3")
	if err != nil || !combo.Latency || combo.Download || !combo.Upload {
		t.Fatalf("1,3: %+v err=%v", combo, err)
	}

	ag, err := parseMetricSelection("4")
	if err != nil || !ag.Antigravity || ag.Latency || ag.Download || ag.Upload {
		t.Fatalf("4: %+v err=%v", ag, err)
	}

	named, err := parseMetricSelection("下载,延迟")
	if err != nil || !named.Latency || !named.Download || named.Upload {
		t.Fatalf("named: %+v err=%v", named, err)
	}

	if _, err := parseMetricSelection("9"); err == nil {
		t.Fatal("expected out of range")
	}
}

func TestParseDurationSelection(t *testing.T) {
	zero, err := parseDurationSelection("1")
	if err != nil || zero != 0 {
		t.Fatalf("one round: %v %v", zero, err)
	}
	ten, err := parseDurationSelection("4")
	if err != nil || ten != 10*time.Minute {
		t.Fatalf("option 4: %v %v", ten, err)
	}
	custom, err := parseDurationSelection("12分钟")
	if err != nil || custom != 12*time.Minute {
		t.Fatalf("12分钟: %v %v", custom, err)
	}
	parsed, err := parseDurationSelection("8m")
	if err != nil || parsed != 8*time.Minute {
		t.Fatalf("8m: %v %v", parsed, err)
	}
}

func TestParsePlanSelection(t *testing.T) {
	tests := []struct {
		input         string
		isLatencyOnly bool
		wantDuration  time.Duration
		wantRounds    int
	}{
		{"", false, 0, 1},
		{"", true, 0, 1},
		{"1", false, 0, 1},
		{"1l", false, 0, 1},
		{"2l", false, 0, 2},
		{"2L", false, 0, 2},
		{"3l", false, 0, 3},
		{"5l", false, 0, 5},
		{"2r", false, 0, 2},
		{"2轮", false, 0, 2},
		{"2", false, 0, 2}, // in bandwidth mode, '2' means 2 rounds
		{"3", false, 0, 3}, // in bandwidth mode, '3' means 3 rounds
		{"2", true, 3 * time.Minute, 0}, // in latency mode, '2' means 3 minutes
		{"3", true, 5 * time.Minute, 0}, // in latency mode, '3' means 5 minutes
		{"2l", true, 0, 2},              // in latency mode, '2l' explicitly means 2 rounds
		{"5m", false, 5 * time.Minute, 0},
		{"5m", true, 5 * time.Minute, 0},
	}

	for _, tc := range tests {
		d, r, err := parsePlanSelection(tc.input, tc.isLatencyOnly)
		if err != nil {
			t.Fatalf("parsePlanSelection(%q, %v) returned error: %v", tc.input, tc.isLatencyOnly, err)
		}
		if d != tc.wantDuration || r != tc.wantRounds {
			t.Errorf("parsePlanSelection(%q, %v) = (%v, %v), want (%v, %v)",
				tc.input, tc.isLatencyOnly, d, r, tc.wantDuration, tc.wantRounds)
		}
	}
}

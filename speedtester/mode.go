package speedtester

import (
	"fmt"
	"strings"
)

type SpeedMode string

const (
	SpeedModeFast     SpeedMode = "fast"
	SpeedModeDownload SpeedMode = "download"
	SpeedModeFull     SpeedMode = "full"
)

func ParseSpeedMode(value string) (SpeedMode, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch SpeedMode(normalized) {
	case SpeedModeFast:
		return SpeedModeFast, nil
	case SpeedModeDownload:
		return SpeedModeDownload, nil
	case SpeedModeFull:
		return SpeedModeFull, nil
	default:
		return "", fmt.Errorf("unsupported speed mode %q", value)
	}
}

func (m SpeedMode) IsFast() bool {
	return m == SpeedModeFast
}

func (m SpeedMode) DownloadEnabled() bool {
	return m == SpeedModeDownload || m == SpeedModeFull
}

func (m SpeedMode) UploadEnabled() bool {
	return m == SpeedModeFull
}

// MetricSet selects which measurements to run. Empty means follow SpeedMode.
type MetricSet struct {
	Latency     bool
	Download    bool
	Upload      bool
	Antigravity bool
}

func (m MetricSet) IsZero() bool {
	return !m.Latency && !m.Download && !m.Upload && !m.Antigravity
}

// IsLatencyOnly returns true if only latency testing is selected.
func (m MetricSet) IsLatencyOnly() bool {
	return m.Latency && !m.Download && !m.Upload && !m.Antigravity
}

func (m MetricSet) ToSpeedMode() SpeedMode {
	if m.Upload {
		return SpeedModeFull
	}
	if m.Download {
		return SpeedModeDownload
	}
	return SpeedModeFast
}

func MetricsFromMode(mode SpeedMode) MetricSet {
	switch mode {
	case SpeedModeFast:
		return MetricSet{Latency: true}
	case SpeedModeFull:
		return MetricSet{Latency: true, Download: true, Upload: true}
	default:
		return MetricSet{Latency: true, Download: true}
	}
}

func ParseMetrics(value string) (MetricSet, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return MetricSet{}, nil
	}
	if normalized == "all" || normalized == "全部" {
		return MetricSet{Latency: true, Download: true, Upload: true, Antigravity: true}, nil
	}
	var metrics MetricSet
	for _, part := range strings.FieldsFunc(normalized, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '、' || r == '|' || r == ' '
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch part {
		case "latency", "ping", "delay", "rtt", "延迟":
			metrics.Latency = true
		case "download", "down", "dl", "下载":
			metrics.Download = true
		case "upload", "up", "ul", "上传":
			metrics.Upload = true
		case "antigravity", "ag", "gravity", "地区":
			metrics.Antigravity = true
		default:
			return MetricSet{}, fmt.Errorf("unsupported metric %q", part)
		}
	}
	if metrics.IsZero() {
		return MetricSet{}, fmt.Errorf("no metrics selected")
	}
	return metrics, nil
}
